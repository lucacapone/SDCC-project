package integration_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	composeBootstrapStrategy  = "cluster locale Docker Compose"
	composeProjectNameDefault = "sdcc-bootstrap"
	composeFileNameDefault    = "docker-compose.yml"
	composeNetworkName        = "sdcc-net"
	composeReadyTimeout       = 90 * time.Second
	composeReadyPollInterval  = 2 * time.Second
	composePollInterval       = 1 * time.Second
	composeConvergenceTimeout = 18 * time.Second
	composeShutdownTimeout    = 40 * time.Second
	composeMetricsPort        = 8080
)

const composeServicesFileDefault = "deploy/compose_services.env"

var composeServices = loadComposeServices()
var composeServiceNodeIDs = buildComposeServiceNodeIDs(composeServices)

type composeHarnessOptions struct {
	composeFile string
	projectName string
	services    []string
}

var shutdownEstimatePattern = regexp.MustCompile(`node_id=([^ ]+) .*estimate=([-+0-9.eE]+)`)
var composeServicePattern = regexp.MustCompile(`^node(\d+)$`)

// loadComposeServices risolve i servizi Compose con precedenza env -> file -> default canonico.
func loadComposeServices() []string {
	if envValue := strings.TrimSpace(os.Getenv("SDCC_SERVICES")); envValue != "" {
		services := splitServicesList(envValue)
		if len(services) > 0 {
			return services
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return []string{"node1", "node2", "node3"}
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	filePath := os.Getenv("SDCC_SERVICES_FILE")
	if strings.TrimSpace(filePath) == "" {
		filePath = filepath.Join(repoRoot, composeServicesFileDefault)
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return []string{"node1", "node2", "node3"}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(trimmed, "SDCC_SERVICES=") {
			continue
		}
		value := strings.TrimPrefix(trimmed, "SDCC_SERVICES=")
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		services := splitServicesList(value)
		if len(services) > 0 {
			return services
		}
	}
	return []string{"node1", "node2", "node3"}
}

// splitServicesList normalizza una lista servizi supportando separatori spazio o virgola.
func splitServicesList(raw string) []string {
	normalized := strings.ReplaceAll(raw, ",", " ")
	fields := strings.Fields(normalized)
	services := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, exists := seen[field]; exists {
			continue
		}
		seen[field] = struct{}{}
		services = append(services, field)
	}
	sort.Strings(services)
	return services
}

// buildComposeServiceNodeIDs produce il mapping service -> node_id inferendolo da nomi node<N>.
func buildComposeServiceNodeIDs(services []string) map[string]string {
	ids := make(map[string]string, len(services))
	for _, service := range services {
		matches := composeServicePattern.FindStringSubmatch(service)
		if len(matches) == 2 {
			ids[service] = fmt.Sprintf("node-%s", matches[1])
			continue
		}
		ids[service] = service
	}
	return ids
}

// composeHarness orchestra gli script Compose canonici della repository per i test end-to-end reali.
type composeHarness struct {
	t          *testing.T
	repoRoot   string
	artifacts  string
	scriptsDir string

	composeFileRel     string
	composeProjectName string
	composeServices    []string
	serviceNodeIDs     map[string]string
}

// newComposeHarness costruisce l'harness reale basato sul deployment Compose di root.
func newComposeHarness(t *testing.T) *composeHarness {
	t.Helper()
	return newComposeHarnessWithOptions(t, composeHarnessOptions{})
}

// newComposeHarnessWithOptions costruisce un harness Compose con override di file/progetto/servizi.
func newComposeHarnessWithOptions(t *testing.T, options composeHarnessOptions) *composeHarness {
	t.Helper()

	repoRoot := integrationRepoRoot(t)
	composeFileRel := strings.TrimSpace(options.composeFile)
	if composeFileRel == "" {
		composeFileRel = composeFileNameDefault
	}
	composeProjectName := strings.TrimSpace(options.projectName)
	if composeProjectName == "" {
		composeProjectName = composeProjectNameDefault
	}
	services := options.services
	if len(services) == 0 {
		services = append([]string(nil), composeServices...)
	}
	services = append([]string(nil), services...)
	sort.Strings(services)
	return &composeHarness{
		t:          t,
		repoRoot:   repoRoot,
		artifacts:  filepath.Join(repoRoot, "artifacts", "cluster"),
		scriptsDir: filepath.Join(repoRoot, "scripts"),

		composeFileRel:     composeFileRel,
		composeProjectName: composeProjectName,
		composeServices:    services,
		serviceNodeIDs:     buildComposeServiceNodeIDs(services),
	}
}

// scriptEnv restituisce le variabili condivise per rendere gli script coerenti con opzioni compose custom.
func (h *composeHarness) scriptEnv() map[string]string {
	return map[string]string{
		"SDCC_COMPOSE_FILE": h.composeFileRel,
		"SDCC_PROJECT_NAME": h.composeProjectName,
		"SDCC_SERVICES":     strings.Join(h.composeServices, ","),
	}
}

// requireDocker verifica le prerequisizioni runtime Docker/Compose; in ambienti senza supporto reale la suite viene skippata.
func (h *composeHarness) requireDocker() {
	h.t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		h.t.Skip("docker non disponibile nel PATH; suite Compose reale saltata")
	}

	// Verifica prima la raggiungibilità del daemon Docker, così gli skip restano diagnostici ma non trasformano
	// una prerequisizione ambientale mancante in un fallimento della suite.
	infoCmd := exec.Command("docker", "info")
	if output, err := infoCmd.CombinedOutput(); err != nil {
		h.t.Skipf("docker daemon non raggiungibile; suite Compose reale saltata: %v\noutput:\n%s", err, string(output))
	}

	// La suite reale usa esplicitamente il subcommand/plugin `docker compose`; se non è disponibile skippiamo il test
	// con un messaggio che distingua plugin mancante, problemi di permessi o errori del client.
	composeVersionCmd := exec.Command("docker", "compose", "version")
	if output, err := composeVersionCmd.CombinedOutput(); err != nil {
		h.t.Skipf("docker compose non disponibile o non utilizzabile; la suite Compose reale richiede il plugin/subcommand `docker compose`: %v\noutput:\n%s", err, string(output))
	}
}

// up avvia il cluster reale usando lo script canonico della repository.
func (h *composeHarness) up() {
	h.t.Helper()
	h.runScript(composeShutdownTimeout, "cluster_up.sh")
}

// waitReady attende che bootstrap e transport risultino osservabili nei log dei tre container.
func (h *composeHarness) waitReady() {
	h.t.Helper()
	h.runScriptWithEnv(
		composeReadyTimeout+10*time.Second,
		map[string]string{
			"TIMEOUT_SECONDS":       strconv.Itoa(int(composeReadyTimeout / time.Second)),
			"POLL_INTERVAL_SECONDS": strconv.Itoa(int(composeReadyPollInterval / time.Second)),
		},
		"cluster_wait_ready.sh",
	)
}

// down effettua stop pulito + raccolta artefatti finali; gli errori di cleanup vengono solo loggati se il test è già fallito.
func (h *composeHarness) down() {
	h.t.Helper()
	if h.t.Failed() {
		h.tryRunScript(composeShutdownTimeout, "cluster_down.sh")
		return
	}
	h.runScript(composeShutdownTimeout, "cluster_down.sh")
}

// waitForConvergence mantiene il cluster reale in esecuzione per una finestra esplicita,
// verificando che i nodi restino raggiungibili prima di raccogliere i valori finali di shutdown.
func (h *composeHarness) waitForConvergence(expectedValue float64, threshold float64, timeout time.Duration, pollEvery time.Duration) (clusterObservation, bool) {
	h.t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !h.allNodesHealthy() {
			h.t.Fatalf("cluster Compose non sano durante la finestra di convergenza di %s", timeout)
		}
		time.Sleep(pollEvery)
	}

	h.down()
	observation, err := h.readFinalObservation(expectedValue)
	if err != nil {
		h.t.Fatalf("lettura valori finali cluster Compose: %v", err)
	}
	return observation, isClusterConverged(observation, threshold)
}

// allNodesHealthy richiede che i tre nodi espongano /health con HTTP 200 sul bridge Docker locale.
func (h *composeHarness) allNodesHealthy() bool {
	for _, service := range h.composeServices {
		baseURL, err := h.serviceBaseURL(service)
		if err != nil {
			return false
		}
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
	}
	return true
}

// readFinalObservation carica gli artifact strutturati prodotti da cluster_down.sh e ricostruisce lo snapshot finale.
func (h *composeHarness) readFinalObservation(expectedValue float64) (clusterObservation, error) {
	valuesPath := filepath.Join(h.artifacts, "latest-final-values.txt")
	raw, err := os.ReadFile(valuesPath)
	if err != nil {
		return clusterObservation{}, fmt.Errorf("read %s: %w", valuesPath, err)
	}
	values, err := h.parseShutdownEstimates(raw)
	if err != nil {
		return clusterObservation{}, err
	}
	return clusterObservation{
		values:             values,
		referenceValue:     expectedValue,
		maxDelta:           observationMaxDelta(values),
		referenceMaxOffset: observationMaxDistance(values, expectedValue),
	}, nil
}

// serviceBaseURL risolve l'IP del container nel bridge Compose per raggiungere l'endpoint HTTP interno.
func (h *composeHarness) serviceBaseURL(service string) (string, error) {
	containerID, err := h.composeContainerID(service)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("docker", "inspect", "-f", fmt.Sprintf("{{with index .NetworkSettings.Networks %q}}{{.IPAddress}}{{end}}", composeNetworkName), containerID)
	cmd.Dir = h.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker inspect rete %s/%s: %w: %s", composeNetworkName, containerID, err, strings.TrimSpace(string(output)))
	}
	ipAddress := strings.TrimSpace(string(output))
	if ipAddress == "" {
		return "", fmt.Errorf("indirizzo IP non disponibile per %s", containerID)
	}
	return fmt.Sprintf("http://%s:%d", ipAddress, composeMetricsPort), nil
}

// composeContainerID risolve il container ID del servizio reale orchestrato dal file Compose di root.
func (h *composeHarness) composeContainerID(service string) (string, error) {
	cmd := exec.Command("docker", "compose", "-p", h.composeProjectName, "-f", h.composeFileRel, "ps", "-q", service)
	cmd.Dir = h.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker compose ps -q %s: %w: %s", service, err, strings.TrimSpace(string(output)))
	}
	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		return "", fmt.Errorf("container Compose non trovato per il servizio %s", service)
	}
	return containerID, nil
}

// runScript esegue uno degli script canonici della repo propagando stdout/stderr in caso di errore.
func (h *composeHarness) runScript(timeout time.Duration, scriptName string) {
	h.t.Helper()
	h.runScriptWithEnv(timeout, nil, scriptName)
}

// tryRunScript esegue il cleanup best-effort senza far fallire ulteriormente il test.
func (h *composeHarness) tryRunScript(timeout time.Duration, scriptName string) {
	h.t.Helper()
	if err := h.execScript(timeout, nil, scriptName); err != nil {
		h.t.Logf("cleanup best-effort fallito (%s): %v", scriptName, err)
	}
}

// runScriptWithEnv esegue lo script richiesto con timeout esplicito e variabili extra.
func (h *composeHarness) runScriptWithEnv(timeout time.Duration, extraEnv map[string]string, scriptName string) {
	h.t.Helper()
	if err := h.execScript(timeout, extraEnv, scriptName); err != nil {
		h.t.Fatal(err)
	}
}

// execScript centralizza l'esecuzione shell degli script canonici in modo leggibile per il test.
func (h *composeHarness) execScript(timeout time.Duration, extraEnv map[string]string, scriptName string) error {
	_, err := h.execScriptCapture(timeout, extraEnv, scriptName)
	return err
}

// execScriptCapture esegue uno script e restituisce anche l'output aggregato utile per artefatti e debug.
func (h *composeHarness) execScriptCapture(timeout time.Duration, extraEnv map[string]string, scriptName string) (string, error) {
	scriptPath := filepath.Join(h.scriptsDir, scriptName)
	// Nota di compatibilità: gli script invocati da questo harness devono rimanere
	// compatibili con Bash 3.2 (shell predefinita su macOS e setup GoLand tipici).
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = h.repoRoot
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	for key, value := range h.scriptEnv() {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	for key, value := range extraEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("avvio %s: %w", scriptName, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			return output.String(), fmt.Errorf("script %s fallito: %w\noutput:\n%s", scriptName, err, output.String())
		}
		return output.String(), nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return output.String(), fmt.Errorf("script %s in timeout dopo %s\noutput:\n%s", scriptName, timeout, output.String())
	}
}

// composeNodeMetrics raccoglie il sottoinsieme di metriche runtime usato dai test Compose reali.
type composeNodeMetrics struct {
	Estimate     float64
	Rounds       uint64
	RemoteMerges uint64
	KnownPeers   float64
	Ready        bool
}

// readLiveObservation legge i valori correnti esposti via /metrics per un sottoinsieme di servizi Compose.
func (h *composeHarness) readLiveObservation(expectedValue float64, services []string) (clusterObservation, map[string]composeNodeMetrics, error) {
	values := make(map[string]float64, len(services))
	metricsByNode := make(map[string]composeNodeMetrics, len(services))
	for _, service := range services {
		nodeID, ok := h.serviceNodeIDs[service]
		if !ok {
			return clusterObservation{}, nil, fmt.Errorf("mapping node_id assente per il servizio %s", service)
		}

		metrics, err := h.readServiceMetrics(service)
		if err != nil {
			return clusterObservation{}, nil, err
		}
		values[nodeID] = metrics.Estimate
		metricsByNode[nodeID] = metrics
	}

	return clusterObservation{
		values:             values,
		referenceValue:     expectedValue,
		maxDelta:           observationMaxDelta(values),
		referenceMaxOffset: observationMaxDistance(values, expectedValue),
	}, metricsByNode, nil
}

// readServiceMetrics estrae le metriche osservabili di un servizio Compose reale.
func (h *composeHarness) readServiceMetrics(service string) (composeNodeMetrics, error) {
	baseURL, err := h.serviceBaseURL(service)
	if err != nil {
		return composeNodeMetrics{}, err
	}

	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		return composeNodeMetrics{}, fmt.Errorf("GET %s/metrics: %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return composeNodeMetrics{}, fmt.Errorf("GET %s/metrics: status=%d", baseURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return composeNodeMetrics{}, fmt.Errorf("lettura body %s/metrics: %w", baseURL, err)
	}

	return parseComposeMetrics(body)
}

// parseComposeMetrics traduce il formato line-based delle metriche in una struttura minima per i test.
func parseComposeMetrics(raw []byte) (composeNodeMetrics, error) {
	metrics := composeNodeMetrics{}
	var (
		foundEstimate   bool
		foundRounds     bool
		foundMerges     bool
		foundKnownPeers bool
		foundReady      bool
	)

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}

		switch fields[0] {
		case "sdcc_node_estimate":
			value, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return composeNodeMetrics{}, fmt.Errorf("estimate non parseabile: %w", err)
			}
			metrics.Estimate = value
			foundEstimate = true
		case "sdcc_node_rounds_total":
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return composeNodeMetrics{}, fmt.Errorf("rounds_total non parseabile: %w", err)
			}
			metrics.Rounds = value
			foundRounds = true
		case "sdcc_node_remote_merges_total":
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return composeNodeMetrics{}, fmt.Errorf("remote_merges_total non parseabile: %w", err)
			}
			metrics.RemoteMerges = value
			foundMerges = true
		case "sdcc_node_known_peers":
			value, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return composeNodeMetrics{}, fmt.Errorf("known_peers gauge non parseabile: %w", err)
			}
			metrics.KnownPeers = value
			foundKnownPeers = true
		case "sdcc_node_ready":
			value, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return composeNodeMetrics{}, fmt.Errorf("ready gauge non parseabile: %w", err)
			}
			metrics.Ready = value >= 1
			foundReady = true
		}
	}

	if err := scanner.Err(); err != nil {
		return composeNodeMetrics{}, fmt.Errorf("scansione metrics: %w", err)
	}
	if !foundEstimate || !foundRounds || !foundMerges || !foundKnownPeers || !foundReady {
		return composeNodeMetrics{}, fmt.Errorf("metriche incomplete: estimate=%t rounds=%t remote_merges=%t known_peers=%t ready=%t", foundEstimate, foundRounds, foundMerges, foundKnownPeers, foundReady)
	}
	return metrics, nil
}

// waitForServiceDown attende che un servizio non risponda più via endpoint HTTP.
func (h *composeHarness) waitForServiceDown(service string, timeout time.Duration, pollEvery time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		baseURL, err := h.serviceBaseURL(service)
		if err != nil {
			return nil
		}
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			return nil
		}
		_ = resp.Body.Close()
		time.Sleep(pollEvery)
	}
	return fmt.Errorf("il servizio %s risulta ancora raggiungibile dopo %s", service, timeout)
}

// waitForServiceReady attende che il servizio riparta e torni esplicitamente ready.
func (h *composeHarness) waitForServiceReady(service string, timeout time.Duration, pollEvery time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		baseURL, err := h.serviceBaseURL(service)
		if err == nil {
			resp, reqErr := http.Get(baseURL + "/ready")
			if reqErr == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		time.Sleep(pollEvery)
	}
	return fmt.Errorf("il servizio %s non è tornato ready entro %s", service, timeout)
}

// runFaultScript invoca gli helper di fault injection canonici sotto scripts/fault_injection/.
func (h *composeHarness) runFaultScript(timeout time.Duration, action string, service string, extraEnv map[string]string) {
	h.t.Helper()
	env := map[string]string{
		"ACTION":  action,
		"SERVICE": service,
	}
	for key, value := range extraEnv {
		env[key] = value
	}
	h.runScriptWithEnv(timeout, env, filepath.Join("fault_injection", "node_stop_start.sh"))
}

// runCombinedFaultScenario invoca lo scenario composito crash->partition->rejoin per test M10 estesi.
func (h *composeHarness) runCombinedFaultScenario(timeout time.Duration, extraEnv map[string]string) {
	h.t.Helper()
	h.runScriptWithEnv(timeout, extraEnv, filepath.Join("fault_injection", "scenario_sequential_crash_partition_rejoin.sh"))
}

// collectDebugSnapshot salva uno snapshot diagnostico e restituisce l'output dello script per i log del test.
func (h *composeHarness) collectDebugSnapshot(timeout time.Duration, service string, label string) string {
	h.t.Helper()
	output, err := h.execScriptCapture(timeout, map[string]string{
		"SERVICE":        service,
		"SNAPSHOT_LABEL": label,
	}, filepath.Join("fault_injection", "collect_debug_snapshot.sh"))
	if err != nil {
		h.t.Fatal(err)
	}
	return strings.TrimSpace(output)
}

// waitForLiveObservationConvergence richiede convergenza via endpoint runtime senza fermare il cluster.
func (h *composeHarness) waitForLiveObservationConvergence(expectedValue float64, threshold float64, timeout time.Duration, pollEvery time.Duration) (clusterObservation, map[string]composeNodeMetrics, bool) {
	var lastObservation clusterObservation
	var lastMetrics map[string]composeNodeMetrics
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		observation, metricsByNode, err := h.readLiveObservation(expectedValue, h.composeServices)
		if err == nil {
			lastObservation = observation
			lastMetrics = metricsByNode
			if isClusterConverged(observation, threshold) {
				return observation, metricsByNode, true
			}
		}
		time.Sleep(pollEvery)
	}
	return lastObservation, lastMetrics, false
}

// waitForResidualActivity prova che il cluster residuo continua a fare round gossip dopo lo stop del nodo target.
func (h *composeHarness) waitForResidualActivity(services []string, baseline map[string]composeNodeMetrics, timeout time.Duration, pollEvery time.Duration) (clusterObservation, map[string]composeNodeMetrics, bool) {
	var lastObservation clusterObservation
	var lastMetrics map[string]composeNodeMetrics
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		observation, metricsByNode, err := h.readLiveObservation(averageOf([]float64{30, 50}), services)
		if err == nil {
			lastObservation = observation
			lastMetrics = metricsByNode
			active := true
			for _, service := range services {
				nodeID := h.serviceNodeIDs[service]
				if !metricsByNode[nodeID].Ready || metricsByNode[nodeID].Rounds <= baseline[nodeID].Rounds {
					active = false
					break
				}
			}
			if active {
				return observation, metricsByNode, true
			}
		}
		time.Sleep(pollEvery)
	}
	return lastObservation, lastMetrics, false
}

// parseShutdownEstimates ricostruisce il mapping node_id -> estimate a partire dai log finali strutturati.
func (h *composeHarness) parseShutdownEstimates(raw []byte) (map[string]float64, error) {
	values := make(map[string]float64)
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		matches := shutdownEstimatePattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		estimate, err := strconv.ParseFloat(matches[2], 64)
		if err != nil {
			return nil, fmt.Errorf("estimate non parseabile nella riga %q: %w", line, err)
		}
		values[matches[1]] = estimate
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scansione artifact final-values: %w", err)
	}
	if len(values) != len(h.composeServices) {
		return nil, fmt.Errorf("attesi %d valori finali, ottenuti %d", len(h.composeServices), len(values))
	}
	return values, nil
}

// integrationRepoRoot risale alla root della repository partendo dalla working directory del package di test.
func integrationRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "docker-compose.yml")); err != nil {
		t.Fatalf("root repository non rilevata da %s: %v", wd, err)
	}
	return root
}

// ensureComposeObservation evita di interpretare un cluster vuoto come convergente.
func ensureComposeObservation(observation clusterObservation) error {
	if len(observation.values) == 0 {
		return errors.New("nessun valore finale raccolto dal cluster Compose")
	}
	return nil
}
