package convergence

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

func sample(at int, node string, value float64) Sample {
	return Sample{Timestamp: time.Unix(int64(at), 0).UTC(), NodeID: node, Round: uint64(at), Aggregation: "average", Estimate: value, EventType: "local_round"}
}

func TestParseSortNormalizeAndCSV(t *testing.T) {
	logs := `node2 | time=2026-01-01T00:00:02Z level=INFO msg="campione" event=convergence_sample timestamp=2026-01-01T00:00:02Z node_id=node-2 aggregation=average round=2 estimate=30 sample_type=local_round
node1 | time=2026-01-01T00:00:01Z level=INFO msg="campione" event=convergence_sample timestamp=2026-01-01T00:00:01Z node_id=node-1 aggregation=average round=1 estimate=10 sample_type=initial
noise`
	s, err := ParseLogs(strings.NewReader(logs))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 2 || s[0].NodeID != "node-1" || s[0].ElapsedSeconds != 0 || s[1].ElapsedSeconds != 1 {
		t.Fatalf("normalizzazione inattesa: %+v", s)
	}
	var b strings.Builder
	if err = WriteCSV(&b, s); err != nil {
		t.Fatal(err)
	}
	roundtrip, err := ReadCSV(strings.NewReader(b.String()))
	if err != nil || len(roundtrip) != 2 || roundtrip[1].EventType != "local_round" {
		t.Fatalf("roundtrip: %+v %v", roundtrip, err)
	}
}

func TestExpectedAllAggregations(t *testing.T) {
	for kind, want := range map[string]float64{"sum": 12, "average": 4, "min": 1, "max": 8} {
		got, err := Expected(kind, []float64{1, 3, 8})
		if err != nil || got != want {
			t.Errorf("%s got=%v want=%v err=%v", kind, got, want, err)
		}
	}
}

func TestDynamicSeriesToleranceAndConvergence(t *testing.T) {
	s := []Sample{sample(1, "a", 10), sample(1, "b", 50), sample(2, "a", 29.98), sample(3, "b", 30.02), sample(4, "a", 30.01)}
	series := Series(s)
	if len(series) != 2 || len(series["a"]) != 3 {
		t.Fatalf("serie dinamiche: %+v", series)
	}
	when, ok := ConvergenceTime(s, 30, .05)
	if !ok || when != 2 {
		t.Fatalf("convergenza got=%v,%v", when, ok)
	}
}
func TestNoConvergence(t *testing.T) {
	if _, ok := ConvergenceTime([]Sample{sample(1, "a", 10), sample(2, "b", 50)}, 30, .05); ok {
		t.Fatal("convergenza inattesa")
	}
}
func TestConvergenceMustRemainInBand(t *testing.T) {
	s := []Sample{sample(1, "a", 30), sample(1, "b", 30), sample(2, "a", 31), sample(3, "a", 30)}
	when, ok := ConvergenceTime(s, 30, .05)
	if !ok || when != 2 {
		t.Fatalf("deve scegliere il rientro stabile: %v %v", when, ok)
	}
}
func TestSVGEssentialValidity(t *testing.T) {
	svg, err := SVG([]Sample{sample(1, "node-a", 10), sample(2, "node-a", 30), sample(1, "node-b", 50), sample(2, "node-b", 30)}, 30, .05)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err = xml.Unmarshal([]byte(svg), &v); err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"<svg", "Convergenza aggregazione average", "node-a", "atteso 30", "tempo trascorso (s)", "stima"} {
		if !strings.Contains(svg, needle) {
			t.Errorf("SVG privo di %q", needle)
		}
	}
}

// TestSVGGeneratesSampleMarkers verifica che ogni campione di una serie non
// densa produca un marker con lo stesso colore del relativo path a gradini.
func TestSVGGeneratesSampleMarkers(t *testing.T) {
	samples := []Sample{sample(1, "node-a", 10), sample(2, "node-a", 20), sample(3, "node-a", 30)}
	svg, err := SVG(samples, 30, .05)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(svg, "class=\"sample-marker\"") != len(samples) {
		t.Fatalf("numero di marker inatteso: SVG=%s", svg)
	}
	for _, needle := range []string{
		`d="M 70.00 430.00 H 450.00 V 260.42 H 830.00 V 90.85"`,
		`stroke="#2563eb" stroke-width="2" stroke-opacity="0.65"`,
		`fill="#2563eb" stroke="white"`,
		`I punti rappresentano campioni osservati; il tratto mantiene l'ultima stima nota.`,
	} {
		if !strings.Contains(svg, needle) {
			t.Errorf("SVG privo di %q", needle)
		}
	}
}

// TestSVGMarkerLimitForDenseSeries congela il limite visuale e il raggio
// ridotto usati per non saturare il grafico quando i campioni sono numerosi.
func TestSVGMarkerLimitForDenseSeries(t *testing.T) {
	samples := make([]Sample, 0, 150)
	for i := 0; i < cap(samples); i++ {
		samples = append(samples, sample(i+1, "node-a", float64(i)))
	}
	svg, err := SVG(samples, 75, .05)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(svg, "class=\"sample-marker\""); got != 100 {
		t.Fatalf("marker della serie densa: got=%d want=100", got)
	}
	if !strings.Contains(svg, `r="2.00"`) {
		t.Fatal("marker della serie densa privo del raggio ridotto")
	}
}

func TestSVGGridIncludesExtremeAndIntermediateLabels(t *testing.T) {
	svg, err := SVG([]Sample{sample(1, "node-a", 10), sample(11, "node-a", 50)}, 30, .05)
	if err != nil {
		t.Fatal(err)
	}

	// Le verifiche coprono entrambi gli estremi e una tacca interna per asse.
	for _, label := range []string{">0 s</text>", ">2 s</text>", ">10 s</text>", ">10</text>", ">42</text>", ">50</text>"} {
		if !strings.Contains(svg, label) {
			t.Errorf("SVG privo dell'etichetta di griglia %q", label)
		}
	}
}

// TestSVGExpectedLabelStaysInsidePlot verifica sia l'ancoraggio orizzontale
// di riferimenti lunghi sia il confinamento verticale ai due estremi dell'asse.
func TestSVGExpectedLabelStaysInsidePlot(t *testing.T) {
	tests := []struct {
		name     string
		samples  []Sample
		expected float64
	}{
		{name: "limite inferiore", samples: []Sample{sample(1, "node-a", 987654321), sample(2, "node-a", 987654322)}, expected: 123456789},
		{name: "limite superiore", samples: []Sample{sample(1, "node-a", -987654322), sample(2, "node-a", -987654321)}, expected: -123456789},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svg, err := SVG(test.samples, test.expected, 0)
			if err != nil {
				t.Fatal(err)
			}
			attributes := svgTextAttributes(t, svg, "atteso ")
			if attributes["x"] != "825" || attributes["text-anchor"] != "end" {
				t.Fatalf("posizionamento orizzontale inatteso: x=%q text-anchor=%q", attributes["x"], attributes["text-anchor"])
			}
			y := parseCoordinate(t, attributes["y"])
			if y < 102 || y > 426 {
				t.Fatalf("etichetta del riferimento fuori dal plot: y=%v", y)
			}
		})
	}
}

// svgTextAttributes restituisce gli attributi del primo testo con il prefisso
// richiesto, mantenendo la verifica indipendente dall'ordine degli attributi XML.
func svgTextAttributes(t *testing.T, document, prefix string) map[string]string {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(document))
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "text" {
			continue
		}
		var label string
		if err := decoder.DecodeElement(&label, &start); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(label, prefix) {
			continue
		}
		attributes := make(map[string]string, len(start.Attr))
		for _, attribute := range start.Attr {
			attributes[attribute.Name.Local] = attribute.Value
		}
		return attributes
	}
	t.Fatalf("testo SVG con prefisso %q assente", prefix)
	return nil
}

// TestSVGLegendCoordinatesFitDynamicViewBox verifica la topologia scale
// canonica e impedisce che l'ultima voce della legenda venga tagliata.
func TestSVGLegendCoordinatesFitDynamicViewBox(t *testing.T) {
	samples := make([]Sample, 0, 6)
	for node := 1; node <= 6; node++ {
		samples = append(samples, sample(node, fmt.Sprintf("node-%d", node), float64(node*10)))
	}
	svg, err := SVG(samples, 35, .05)
	if err != nil {
		t.Fatal(err)
	}

	viewBoxHeight, labelY := svgViewBoxHeightAndTextCoordinates(t, svg)
	for node := 1; node <= 6; node++ {
		label := fmt.Sprintf("node-%d", node)
		y, found := labelY[label]
		if !found {
			t.Errorf("SVG privo della voce di legenda %q", label)
			continue
		}
		if y < 0 || y > viewBoxHeight {
			t.Errorf("coordinata legenda %q fuori dal viewBox: y=%v altezza=%v", label, y, viewBoxHeight)
		}
	}

	// I tre blocchi verticali devono mantenere ordine e spazio distinti.
	xAxisY := labelY["tempo trascorso (s)"]
	annotationY := labelY["Convergenza non osservata"]
	if !(xAxisY < labelY["node-1"] && labelY["node-6"] < annotationY && annotationY <= viewBoxHeight) {
		t.Fatalf("layout verticale sovrapposto: asse=%v legenda=[%v,%v] annotazione=%v viewBox=%v", xAxisY, labelY["node-1"], labelY["node-6"], annotationY, viewBoxHeight)
	}
}

// svgViewBoxHeightAndTextCoordinates estrae dal documento generato l'altezza
// dichiarata e le coordinate delle etichette senza dipendere da regex HTML.
func svgViewBoxHeightAndTextCoordinates(t *testing.T, document string) (float64, map[string]float64) {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(document))
	labels := make(map[string]float64)
	var viewBoxHeight float64
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "svg" {
			viewBoxHeight = parseViewBoxHeight(t, start.Attr)
		}
		if start.Name.Local != "text" {
			continue
		}
		var label string
		if err := decoder.DecodeElement(&label, &start); err != nil {
			t.Fatal(err)
		}
		for _, attribute := range start.Attr {
			if attribute.Name.Local == "y" {
				labels[label] = parseCoordinate(t, attribute.Value)
			}
		}
	}
	if viewBoxHeight <= 0 {
		t.Fatal("viewBox SVG assente o non valido")
	}
	return viewBoxHeight, labels
}

// parseViewBoxHeight valida il formato numerico del viewBox prodotto da SVG.
func parseViewBoxHeight(t *testing.T, attributes []xml.Attr) float64 {
	t.Helper()
	for _, attribute := range attributes {
		if attribute.Name.Local != "viewBox" {
			continue
		}
		parts := strings.Fields(attribute.Value)
		if len(parts) != 4 {
			t.Fatalf("viewBox non valido: %q", attribute.Value)
		}
		return parseCoordinate(t, parts[3])
	}
	t.Fatal("attributo viewBox assente")
	return 0
}

// parseCoordinate converte una coordinata SVG e rende gli errori espliciti.
func parseCoordinate(t *testing.T, value string) float64 {
	t.Helper()
	coordinate, err := strconv.ParseFloat(value, 64)
	if err != nil {
		t.Fatalf("coordinata SVG non valida %q: %v", value, err)
	}
	return coordinate
}
