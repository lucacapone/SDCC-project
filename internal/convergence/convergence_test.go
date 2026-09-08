package convergence

import (
	"encoding/xml"
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
