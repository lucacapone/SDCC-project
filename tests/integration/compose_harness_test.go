package integration_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	composeBootstrapStrategy  = "cluster locale Docker Compose"
	composeProjectName        = "sdcc-bootstrap"
	composeFileName           = "docker-compose.yml"
	composeNetworkName        = "sdcc-net"
	composeReadyTimeout       = 90 * time.Second
	composeReadyPollInterval  = 2 * time.Second
	composePollInterval       = 1 * time.Second
	composeConvergenceTimeout = 18 * time.Second
	composeShutdownTimeout    = 40 * time.Second
	composeMetricsPort        = 8080
)

var composeServices = []string{"node1", "node2", "node3"}

var shutdownEstimatePattern = regexp.MustCompile(`node_id=([^ ]+) .*estimate=([-+0-9.eE]+)`)

// composeHarness orchestra gli script Compose canonici della repository per i test end-to-end reali.
type composeHarness struct {
	t          *testing.T
	repoRoot   string
	artifacts  string
	scriptsDir string
}

// newComposeHarness costruisce l'harness reale basato sul deployment Compose di root.
func newComposeHarness(t *testing.T) *composeHarness {
	t.Helper()

	repoRoot := integrationRepoRoot(t)
	return &composeHarness{
		t:          t,
		repoRoot:   repoRoot,
		artifacts:  filepath.Join(repoRoot, "artifacts", "cluster"),
		scriptsDir: filepath.Join(repoRoot, "scripts"),
	}
}

// requireDocker verifica la prerequisizione runtime; in ambienti senza Docker il test viene skippato.
func (h *composeHarness) requireDocker() {
	h.t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		h.t.Skip("docker non disponibile nel PATH; suite Compose reale saltata")
	}
	cmd := exec.Command("docker", "info")
	if output, err := cmd.CombinedOutput(); err != nil {
		h.t.Skipf("docker daemon non raggiungibile; suite Compose reale saltata: %v\noutput:\n%s", err, string(output))
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
	for _, service := range composeServices {
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
	values, err := parseShutdownEstimates(raw)
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
	cmd := exec.Command("docker", "compose", "-p", composeProjectName, "-f", composeFileName, "ps", "-q", service)
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
	scriptPath := filepath.Join(h.scriptsDir, scriptName)
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = h.repoRoot
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	for key, value := range extraEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("avvio %s: %w", scriptName, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("script %s fallito: %w\noutput:\n%s", scriptName, err, output.String())
		}
		return nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("script %s in timeout dopo %s\noutput:\n%s", scriptName, timeout, output.String())
	}
}

// parseShutdownEstimates ricostruisce il mapping node_id -> estimate a partire dai log finali strutturati.
func parseShutdownEstimates(raw []byte) (map[string]float64, error) {
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
	if len(values) != len(composeServices) {
		return nil, fmt.Errorf("attesi %d valori finali, ottenuti %d", len(composeServices), len(values))
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
