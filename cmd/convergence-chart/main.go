// Command convergence-chart costruisce CSV, SVG e riepilogo senza partecipare al protocollo gossip.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"sdcc-project/internal/config"
	"sdcc-project/internal/convergence"
)

func main() {
	logsPath := flag.String("logs", "", "log Compose sorgente opzionale da convertire")
	csvPath := flag.String("csv", "convergence.csv", "CSV da scrivere/leggere")
	svgPath := flag.String("svg", "convergence.svg", "SVG di destinazione")
	summaryPath := flag.String("summary", "summary.txt", "riepilogo di destinazione")
	configPaths := flag.String("configs", "", "configurazioni della run separate da virgola")
	tolerance := flag.Float64("tolerance", 0.05, "tolleranza assoluta")
	flag.Parse()
	if err := run(*logsPath, *csvPath, *svgPath, *summaryPath, *configPaths, *tolerance); err != nil {
		fmt.Fprintln(os.Stderr, "ERRORE:", err)
		os.Exit(1)
	}
}

func run(logsPath, csvPath, svgPath, summaryPath, configPaths string, tolerance float64) error {
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
	svg, err := convergence.SVG(samples, expected, tolerance)
	if err != nil {
		return err
	}
	if err = os.WriteFile(svgPath, []byte(svg), 0644); err != nil {
		return err
	}
	when, ok := convergence.ConvergenceTime(samples, expected, tolerance)
	status := "non osservata"
	if ok {
		status = fmt.Sprintf("osservata da %.6f s", when)
	}
	summary := fmt.Sprintf("aggregation=%s\nexpected=%.12g\ntolerance=%.12g\nnodes_observed=%d\nsamples=%d\nconvergence=%s\n", aggregation, expected, tolerance, len(convergence.Series(samples)), len(samples), status)
	return os.WriteFile(summaryPath, []byte(summary), 0644)
}
