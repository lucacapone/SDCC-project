// Command convergence-chart costruisce CSV, SVG e riepilogo senza partecipare al protocollo gossip.
package main

import (
	"flag"
	"fmt"
	"os"

	"sdcc-project/internal/convergencechart"
)

// main interpreta la CLI e delega la pipeline al package interno importabile.
func main() {
	logsPath := flag.String("logs", "", "log Compose sorgente opzionale da convertire")
	csvPath := flag.String("csv", "convergence.csv", "CSV da scrivere/leggere")
	svgPath := flag.String("svg", "convergence.svg", "SVG di destinazione")
	summaryPath := flag.String("summary", "summary.txt", "riepilogo di destinazione")
	configPaths := flag.String("configs", "", "configurazioni della run separate da virgola")
	tolerance := flag.Float64("tolerance", 0.05, "tolleranza assoluta")
	flag.Parse()
	if err := convergencechart.Run(*logsPath, *csvPath, *svgPath, *summaryPath, *configPaths, *tolerance); err != nil {
		fmt.Fprintln(os.Stderr, "ERRORE:", err)
		os.Exit(1)
	}
}
