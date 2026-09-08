// Package convergencechart orchestra la generazione offline degli artefatti di convergenza.
package convergencechart

import (
	"fmt"
	"os"
	"strings"

	"sdcc-project/internal/config"
	"sdcc-project/internal/convergence"
)

// Run legge log o CSV, valida le configurazioni della run e genera SVG e riepilogo.
func Run(logsPath, csvPath, svgPath, summaryPath, configPaths string, tolerance float64) error {
	if logsPath != "" {
		in, err := os.Open(logsPath)
		if err != nil {
			return err
		}
		samples, err := convergence.ParseLogs(in)
		in.Close()
		if err != nil {
			return err
		}
		if len(samples) == 0 {
			return fmt.Errorf("nessun convergence_sample valido")
		}
		out, err := os.Create(csvPath)
		if err != nil {
			return err
		}
		err = convergence.WriteCSV(out, samples)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	in, err := os.Open(csvPath)
	if err != nil {
		return err
	}
	samples, err := convergence.ReadCSV(in)
	in.Close()
	if err != nil {
		return err
	}
	var values []float64
	var expectedNodes []string
	configuredNodes := map[string]string{}
	aggregation := ""
	for _, path := range strings.Split(configPaths, ",") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		cfg, err := config.Load(path)
		if err != nil {
			return fmt.Errorf("config %s: %w", path, err)
		}
		if aggregation == "" {
			aggregation = cfg.Aggregation
		} else if aggregation != cfg.Aggregation {
			return fmt.Errorf("aggregazioni config non omogenee")
		}
		if previousPath, exists := configuredNodes[cfg.NodeID]; exists {
			return fmt.Errorf("node_id %q duplicato nelle config %s e %s", cfg.NodeID, previousPath, path)
		}
		configuredNodes[cfg.NodeID] = path
		expectedNodes = append(expectedNodes, cfg.NodeID)
		values = append(values, cfg.InitialValue)
	}
	if len(samples) == 0 {
		return fmt.Errorf("CSV senza campioni")
	}
	if aggregation == "" {
		return fmt.Errorf("specificare -configs")
	}
	if samples[0].Aggregation != aggregation {
		return fmt.Errorf("aggregazione CSV %s diversa dalle config %s", samples[0].Aggregation, aggregation)
	}
	expected, err := convergence.Expected(aggregation, values)
	if err != nil {
		return err
	}
	comparison := convergence.CompareNodes(samples, expectedNodes)
	svg, err := convergence.SVG(samples, expectedNodes, expected, tolerance)
	if err != nil {
		return err
	}
	if err = os.WriteFile(svgPath, []byte(svg), 0644); err != nil {
		return err
	}
	when, ok := convergence.ConvergenceTime(samples, expectedNodes, expected, tolerance)
	status := "non osservata"
	if !comparison.Complete() {
		status += ": " + nodeDifferenceSummary(comparison)
	} else if ok {
		status = fmt.Sprintf("osservata da %.6f s", when)
	}
	summary := fmt.Sprintf("aggregation=%s\nexpected=%.12g\ntolerance=%.12g\nnodes_expected=%d\nnodes_observed=%d\nmissing_nodes=%s\nunexpected_nodes=%s\nsamples=%d\nconvergence=%s\n", aggregation, expected, tolerance, len(expectedNodes), len(convergence.Series(samples)), nodeList(comparison.Missing), nodeList(comparison.Unexpected), len(samples), status)
	return os.WriteFile(summaryPath, []byte(summary), 0644)
}

// nodeList usa un valore esplicito anche quando una classe di differenze è vuota.
func nodeList(nodes []string) string {
	if len(nodes) == 0 {
		return "nessuno"
	}
	return strings.Join(nodes, ",")
}

// nodeDifferenceSummary descrive perché un insieme di campioni non è valido.
func nodeDifferenceSummary(comparison convergence.NodeSetComparison) string {
	parts := make([]string, 0, 2)
	if len(comparison.Missing) > 0 {
		parts = append(parts, "nodi mancanti: "+strings.Join(comparison.Missing, ", "))
	}
	if len(comparison.Unexpected) > 0 {
		parts = append(parts, "nodi non configurati: "+strings.Join(comparison.Unexpected, ", "))
	}
	return strings.Join(parts, "; ")
}
