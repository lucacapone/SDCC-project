package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"sdcc-project/internal/convergence"
)

// TestRunReportsMissingSixthConfiguredNode riproduce una run scale nella quale
// tutti i cinque nodi osservati sono già sul riferimento, ma node-6 è assente.
func TestRunReportsMissingSixthConfiguredNode(t *testing.T) {
	temporaryDirectory := t.TempDir()
	csvPath := filepath.Join(temporaryDirectory, "convergence.csv")
	svgPath := filepath.Join(temporaryDirectory, "convergence.svg")
	summaryPath := filepath.Join(temporaryDirectory, "summary.txt")
	csvFile, err := os.Create(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]convergence.Sample, 0, 5)
	for node := 1; node <= 5; node++ {
		samples = append(samples, convergence.Sample{
			Timestamp:   time.Unix(int64(node), 0).UTC(),
			NodeID:      "node-" + strconv.Itoa(node),
			Round:       uint64(node),
			Aggregation: "average",
			Estimate:    60,
			EventType:   "local_round",
		})
	}
	if err := convergence.WriteCSV(csvFile, samples); err != nil {
		csvFile.Close()
		t.Fatal(err)
	}
	if err := csvFile.Close(); err != nil {
		t.Fatal(err)
	}

	configPaths := make([]string, 0, 6)
	for node := 1; node <= 6; node++ {
		configPaths = append(configPaths, filepath.Join("..", "..", "configs", "node"+strconv.Itoa(node)+".yaml"))
	}
	if err := run("", csvPath, svgPath, summaryPath, strings.Join(configPaths, ","), .05); err != nil {
		t.Fatal(err)
	}

	assertFileContains(t, summaryPath, []string{
		"nodes_expected=6",
		"nodes_observed=5",
		"missing_nodes=node-6",
		"unexpected_nodes=nessuno",
		"convergence=non osservata: nodi mancanti: node-6",
	})
	assertFileContains(t, svgPath, []string{"Convergenza non osservata: nodi mancanti: node-6"})
}

// assertFileContains verifica che un artefatto contenga tutta la diagnostica richiesta.
func assertFileContains(t *testing.T, path string, expectedFragments []string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range expectedFragments {
		if !strings.Contains(string(contents), fragment) {
			t.Errorf("%s non contiene %q: %s", path, fragment, contents)
		}
	}
}
