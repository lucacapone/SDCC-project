// Package convergence trasforma esclusivamente dati osservativi in report offline.
package convergence

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"html"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Sample rappresenta una riga stabile del dataset di convergenza.
type Sample struct {
	Timestamp      time.Time
	ElapsedSeconds float64
	NodeID         string
	Round          uint64
	Aggregation    string
	Estimate       float64
	EventType      string
}

// ParseLogs estrae i soli eventi convergence_sample dai log Compose testuali.
func ParseLogs(r io.Reader) ([]Sample, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var samples []Sample
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "event=convergence_sample") {
			continue
		}
		fields := parseKeyValues(line)
		ts, err := time.Parse(time.RFC3339Nano, fields["timestamp"])
		if err != nil {
			continue
		}
		round, err := strconv.ParseUint(fields["round"], 10, 64)
		if err != nil {
			continue
		}
		estimate, err := strconv.ParseFloat(fields["estimate"], 64)
		if err != nil {
			continue
		}
		if fields["node_id"] == "" || fields["aggregation"] == "" || fields["sample_type"] == "" {
			continue
		}
		samples = append(samples, Sample{Timestamp: ts.UTC(), NodeID: fields["node_id"], Round: round, Aggregation: fields["aggregation"], Estimate: estimate, EventType: fields["sample_type"]})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("lettura log: %w", err)
	}
	return Normalize(samples), nil
}

// parseKeyValues interpreta coppie slog key=value, incluse stringhe quotate.
func parseKeyValues(line string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(line); {
		for i < len(line) && line[i] == ' ' {
			i++
		}
		start := i
		for i < len(line) && line[i] != '=' && line[i] != ' ' {
			i++
		}
		if i >= len(line) || line[i] != '=' {
			i++
			continue
		}
		key := line[start:i]
		i++
		var value string
		if i < len(line) && line[i] == '"' {
			start = i
			i++
			for i < len(line) {
				if line[i] == '\\' {
					i += 2
					continue
				}
				i++
				if line[i-1] == '"' {
					break
				}
			}
			quoted := line[start:i]
			value, _ = strconv.Unquote(quoted)
		} else {
			start = i
			for i < len(line) && line[i] != ' ' {
				i++
			}
			value = line[start:i]
		}
		out[key] = value
	}
	return out
}

// Normalize ordina cronologicamente e pone l'origine sul primo campione globale.
func Normalize(samples []Sample) []Sample {
	out := append([]Sample(nil), samples...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Timestamp.Equal(out[j].Timestamp) {
			if out[i].NodeID == out[j].NodeID {
				return out[i].Round < out[j].Round
			}
			return out[i].NodeID < out[j].NodeID
		}
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	if len(out) == 0 {
		return out
	}
	origin := out[0].Timestamp
	for i := range out {
		out[i].ElapsedSeconds = out[i].Timestamp.Sub(origin).Seconds()
	}
	return out
}

// WriteCSV serializza il contratto CSV pubblico in ordine stabile.
func WriteCSV(w io.Writer, samples []Sample) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{"timestamp", "elapsed_seconds", "node_id", "round", "aggregation", "estimate", "event_type"}); err != nil {
		return err
	}
	for _, s := range Normalize(samples) {
		if err := cw.Write([]string{s.Timestamp.Format(time.RFC3339Nano), strconv.FormatFloat(s.ElapsedSeconds, 'f', 6, 64), s.NodeID, strconv.FormatUint(s.Round, 10), s.Aggregation, strconv.FormatFloat(s.Estimate, 'g', -1, 64), s.EventType}); err != nil {
			return err
		}
	}
	return cw.Error()
}

// ReadCSV valida e carica il contratto CSV pubblico.
func ReadCSV(r io.Reader) ([]Sample, error) {
	cr := csv.NewReader(r)
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || strings.Join(rows[0], ",") != "timestamp,elapsed_seconds,node_id,round,aggregation,estimate,event_type" {
		return nil, fmt.Errorf("header CSV non valido")
	}
	out := make([]Sample, 0, len(rows)-1)
	for i, row := range rows[1:] {
		if len(row) != 7 {
			return nil, fmt.Errorf("riga %d non valida", i+2)
		}
		ts, e := time.Parse(time.RFC3339Nano, row[0])
		if e != nil {
			return nil, e
		}
		elapsed, e := strconv.ParseFloat(row[1], 64)
		if e != nil {
			return nil, e
		}
		round, e := strconv.ParseUint(row[3], 10, 64)
		if e != nil {
			return nil, e
		}
		estimate, e := strconv.ParseFloat(row[5], 64)
		if e != nil {
			return nil, e
		}
		out = append(out, Sample{ts, elapsed, row[2], round, row[4], estimate, row[6]})
	}
	return Normalize(out), nil
}

// Expected calcola offline il riferimento dai valori iniziali della run.
func Expected(kind string, values []float64) (float64, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("nessun initial_value")
	}
	result := values[0]
	switch kind {
	case "sum":
		result = 0
		for _, v := range values {
			result += v
		}
	case "average":
		result = 0
		for _, v := range values {
			result += v
		}
		result /= float64(len(values))
	case "min":
		for _, v := range values[1:] {
			result = math.Min(result, v)
		}
	case "max":
		for _, v := range values[1:] {
			result = math.Max(result, v)
		}
	default:
		return 0, fmt.Errorf("aggregazione non supportata: %s", kind)
	}
	return result, nil
}

// Series raggruppa dinamicamente i campioni per node_id.
func Series(samples []Sample) map[string][]Sample {
	out := map[string][]Sample{}
	for _, s := range Normalize(samples) {
		out[s.NodeID] = append(out[s.NodeID], s)
	}
	return out
}

// ConvergenceTime trova il primo stato globale dopo il quale tutti i nodi osservati
// sono e restano nella tolleranza assoluta fino alla fine della run.
func ConvergenceTime(samples []Sample, expected, tolerance float64) (float64, bool) {
	ordered := Normalize(samples)
	nodes := Series(ordered)
	if len(nodes) == 0 {
		return 0, false
	}
	latest := map[string]float64{}
	valid := make([]bool, len(ordered))
	for i, s := range ordered {
		latest[s.NodeID] = s.Estimate
		ok := len(latest) == len(nodes)
		if ok {
			for _, v := range latest {
				if math.Abs(v-expected) > tolerance {
					ok = false
					break
				}
			}
		}
		valid[i] = ok
	}
	suffix := true
	answer := -1.0
	for i := len(valid) - 1; i >= 0; i-- {
		suffix = suffix && valid[i]
		if suffix {
			answer = ordered[i].ElapsedSeconds
		}
	}
	return answer, answer >= 0
}

// SVG produce un grafico autosufficiente con serie a gradini, legenda e riferimento.
func SVG(samples []Sample, expected, tolerance float64) (string, error) {
	series := Series(samples)
	if len(series) == 0 {
		return "", fmt.Errorf("nessun campione")
	}
	ordered := Normalize(samples)
	agg := ordered[0].Aggregation
	minY, maxY := expected-tolerance, expected+tolerance
	maxX := ordered[len(ordered)-1].ElapsedSeconds
	if maxX <= 0 {
		maxX = 1
	}
	for _, s := range ordered {
		minY = math.Min(minY, s.Estimate)
		maxY = math.Max(maxY, s.Estimate)
	}
	if maxY == minY {
		maxY++
	}
	x := func(v float64) float64 { return 70 + v/maxX*760 }
	y := func(v float64) float64 { return 430 - (v-minY)/(maxY-minY)*340 }
	colors := []string{"#2563eb", "#dc2626", "#16a34a", "#9333ea", "#ea580c", "#0891b2", "#4f46e5", "#be123c"}
	// La legenda cresce verticalmente sotto il titolo dell'asse X; il viewBox
	// segue il numero di serie e conserva spazio separato per l'annotazione.
	const (
		plotTop           = 90
		plotBottom        = 430
		plotRight         = 830
		xAxisTitleY       = 470
		legendFirstY      = 495
		legendRowHeight   = 18
		annotationSpacing = 30
		bottomPadding     = 20
		referenceLabelX   = plotRight - 5
		referenceLabelTop = plotTop + 12
		referenceLabelBot = plotBottom - 4
	)
	legendLastY := legendFirstY + (len(series)-1)*legendRowHeight
	annotationY := legendLastY + annotationSpacing
	svgHeight := annotationY + bottomPadding
	var b strings.Builder
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"900\" height=\"%d\" viewBox=\"0 0 900 %d\"><title>Convergenza %s</title><rect width=\"100%%\" height=\"100%%\" fill=\"white\"/><text x=\"450\" y=\"28\" text-anchor=\"middle\" font-size=\"20\">Convergenza aggregazione %s</text>", svgHeight, svgHeight, html.EscapeString(agg), html.EscapeString(agg))
	fmt.Fprintf(&b, "<rect x=\"70\" y=\"%.2f\" width=\"760\" height=\"%.2f\" fill=\"#dcfce7\"/><g stroke=\"#d1d5db\">", y(expected+tolerance), y(expected-tolerance)-y(expected+tolerance))
	for i := 0; i <= 5; i++ {
		xx := 70 + float64(i)*152
		yy := 90 + float64(i)*68
		timeValue := maxX * float64(i) / 5
		estimateValue := maxY - (maxY-minY)*float64(i)/5
		fmt.Fprintf(&b, "<line x1=\"%.1f\" y1=\"90\" x2=\"%.1f\" y2=\"430\"/><text x=\"%.1f\" y=\"447\" text-anchor=\"middle\" font-size=\"11\" stroke=\"none\">%s s</text><line x1=\"70\" y1=\"%.1f\" x2=\"830\" y2=\"%.1f\"/><text x=\"64\" y=\"%.1f\" text-anchor=\"end\" dominant-baseline=\"middle\" font-size=\"11\" stroke=\"none\">%s</text>", xx, xx, xx, formatAxisValue(timeValue, maxX/5), yy, yy, yy, formatAxisValue(estimateValue, (maxY-minY)/5))
	}
	b.WriteString("</g>")
	// L'etichetta termina prima del bordo destro e la sua baseline viene
	// confinata nel plot, così riferimenti lunghi e valori Y estremi restano visibili.
	referenceY := math.Max(referenceLabelTop, math.Min(referenceLabelBot, y(expected)-4))
	fmt.Fprintf(&b, "<line x1=\"70\" y1=\"%.2f\" x2=\"830\" y2=\"%.2f\" stroke=\"#111827\" stroke-dasharray=\"7 4\"/><text x=\"%d\" y=\"%.2f\" text-anchor=\"end\" font-size=\"11\">atteso %.4g ± %.4g</text>", y(expected), y(expected), referenceLabelX, referenceY, expected, tolerance)
	names := make([]string, 0, len(series))
	for n := range series {
		names = append(names, n)
	}
	sort.Strings(names)
	for i, n := range names {
		points := series[n]
		var path strings.Builder
		for j, p := range points {
			if j == 0 {
				fmt.Fprintf(&path, "M %.2f %.2f", x(p.ElapsedSeconds), y(p.Estimate))
			} else {
				fmt.Fprintf(&path, " H %.2f V %.2f", x(p.ElapsedSeconds), y(p.Estimate))
			}
		}
		c := colors[i%len(colors)]
		legendY := legendFirstY + i*legendRowHeight
		fmt.Fprintf(&b, "<path d=\"%s\" fill=\"none\" stroke=\"%s\" stroke-width=\"2\"/><line x1=\"75\" y1=\"%d\" x2=\"95\" y2=\"%d\" stroke=\"%s\" stroke-width=\"3\"/><text x=\"100\" y=\"%d\" font-size=\"11\">%s</text>", path.String(), c, legendY-4, legendY-4, c, legendY, html.EscapeString(n))
	}
	conv, ok := ConvergenceTime(ordered, expected, tolerance)
	label := "Convergenza non osservata"
	if ok {
		label = fmt.Sprintf("Convergenza stabile da %.3f s", conv)
	}
	fmt.Fprintf(&b, "<text x=\"450\" y=\"%d\" text-anchor=\"middle\">tempo trascorso (s)</text><text x=\"450\" y=\"%d\" text-anchor=\"middle\">%s</text><text transform=\"translate(18 260) rotate(-90)\" text-anchor=\"middle\">stima</text></svg>", xAxisTitleY, annotationY, xmlEscape(label))
	return b.String(), nil
}

// formatAxisValue sceglie un numero di decimali proporzionato alla distanza
// tra le tacche, limitandolo per mantenere leggibili anche intervalli piccoli.
func formatAxisValue(value, step float64) string {
	decimals := 0
	if step != 0 {
		decimals = 2 - int(math.Floor(math.Log10(math.Abs(step))))
	}
	if decimals < 0 {
		decimals = 0
	}
	if decimals > 6 {
		decimals = 6
	}
	formatted := strconv.FormatFloat(value, 'f', decimals, 64)
	if decimals == 0 {
		return formatted
	}
	return strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
}

func xmlEscape(s string) string { return html.EscapeString(s) }
