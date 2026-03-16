package aggregation

import "testing"

func TestFactory(t *testing.T) {
	tests := []struct {
		kind       string
		expectType string
		expectErr  bool
	}{
		{kind: "sum", expectType: "sum", expectErr: false},
		{kind: "average", expectType: "average", expectErr: false},
		{kind: "min", expectType: "min", expectErr: false},
		{kind: "max", expectType: "max", expectErr: false},
		{kind: "median", expectErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			algo, err := Factory(tt.kind)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("atteso errore per kind=%s", tt.kind)
				}
				return
			}

			if err != nil {
				t.Fatalf("errore inatteso: %v", err)
			}
			if algo.Type() != tt.expectType {
				t.Fatalf("tipo inatteso: got=%s want=%s", algo.Type(), tt.expectType)
			}
		})
	}
}

func TestMergeRules(t *testing.T) {
	sumAlgo, _ := Factory("sum")
	if got := sumAlgo.Merge(10, 3); got != 13 {
		t.Fatalf("merge sum inatteso: got=%v want=13", got)
	}

	avgAlgo, _ := Factory("average")
	if got := avgAlgo.Merge(10, 4); got != 7 {
		t.Fatalf("merge average inatteso: got=%v want=7", got)
	}

	minAlgo, _ := Factory("min")
	if got := minAlgo.Merge(10, 4); got != 4 {
		t.Fatalf("merge min inatteso: got=%v want=4", got)
	}

	maxAlgo, _ := Factory("max")
	if got := maxAlgo.Merge(10, 4); got != 10 {
		t.Fatalf("merge max inatteso: got=%v want=10", got)
	}
}
