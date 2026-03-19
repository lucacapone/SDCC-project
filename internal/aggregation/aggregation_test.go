package aggregation

import (
	"math"
	"testing"

	"sdcc-project/internal/aggregation/average"
	"sdcc-project/internal/aggregation/max"
	"sdcc-project/internal/aggregation/min"
	"sdcc-project/internal/aggregation/sum"
)

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

			switch tt.kind {
			case "sum":
				if _, ok := algo.(sum.Algorithm); !ok {
					t.Fatalf("factory non ha restituito sum.Algorithm: %T", algo)
				}
			case "average":
				if _, ok := algo.(average.Algorithm); !ok {
					t.Fatalf("factory non ha restituito average.Algorithm: %T", algo)
				}
			case "min":
				if _, ok := algo.(min.Algorithm); !ok {
					t.Fatalf("factory non ha restituito min.Algorithm: %T", algo)
				}
			case "max":
				if _, ok := algo.(max.Algorithm); !ok {
					t.Fatalf("factory non ha restituito max.Algorithm: %T", algo)
				}
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

// TestAlgorithmNumericContract rende esplicito il contratto numerico dei merge
// elementari esposti dalla factory aggregazioni.
//
// Contratto supportato:
//   - numeri finiti positivi/negativi;
//   - uguaglianza tra input;
//   - valori molto grandi e ±Inf secondo le regole IEEE-754 del float64.
//
// Contratto non supportato:
//   - NaN come input semantico applicativo.
//
// Per NaN non imponiamo una normalizzazione artificiale; congeliamo invece il
// comportamento corrente delle primitive aritmetiche/confronti Go per evitare
// assunzioni implicite o regressioni accidentali.
func TestAlgorithmNumericContract(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		local     float64
		remote    float64
		assertion func(t *testing.T, got float64)
	}{
		{
			name:   "sum_negativi_supportati",
			kind:   "sum",
			local:  -10.5,
			remote: -4.5,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, -15)
			},
		},
		{
			name:   "sum_input_uguali_supportati",
			kind:   "sum",
			local:  7.25,
			remote: 7.25,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, 14.5)
			},
		},
		{
			name:   "sum_valori_molto_grandi_supportati",
			kind:   "sum",
			local:  math.MaxFloat64,
			remote: math.MaxFloat64,
			assertion: func(t *testing.T, got float64) {
				if !math.IsInf(got, 1) {
					t.Fatalf("atteso +Inf da overflow IEEE-754, got=%v", got)
				}
			},
		},
		{
			name:   "sum_inf_supportato",
			kind:   "sum",
			local:  math.Inf(1),
			remote: 5,
			assertion: func(t *testing.T, got float64) {
				if !math.IsInf(got, 1) {
					t.Fatalf("atteso +Inf, got=%v", got)
				}
			},
		},
		{
			name:   "sum_nan_non_supportato_ma_propagato",
			kind:   "sum",
			local:  math.NaN(),
			remote: 5,
			assertion: func(t *testing.T, got float64) {
				if !math.IsNaN(got) {
					t.Fatalf("atteso NaN propagato, got=%v", got)
				}
			},
		},
		{
			name:   "average_negativi_supportati",
			kind:   "average",
			local:  -10,
			remote: -4,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, -7)
			},
		},
		{
			name:   "average_input_uguali_supportati",
			kind:   "average",
			local:  8,
			remote: 8,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, 8)
			},
		},
		{
			name:   "average_valori_molto_grandi_supportati",
			kind:   "average",
			local:  math.MaxFloat64,
			remote: math.MaxFloat64,
			assertion: func(t *testing.T, got float64) {
				if !math.IsInf(got, 1) {
					t.Fatalf("atteso +Inf per overflow intermedio IEEE-754, got=%v", got)
				}
			},
		},
		{
			name:   "average_inf_supportato",
			kind:   "average",
			local:  math.Inf(-1),
			remote: math.Inf(-1),
			assertion: func(t *testing.T, got float64) {
				if !math.IsInf(got, -1) {
					t.Fatalf("atteso -Inf, got=%v", got)
				}
			},
		},
		{
			name:   "average_nan_non_supportato_ma_propagato",
			kind:   "average",
			local:  10,
			remote: math.NaN(),
			assertion: func(t *testing.T, got float64) {
				if !math.IsNaN(got) {
					t.Fatalf("atteso NaN propagato, got=%v", got)
				}
			},
		},
		{
			name:   "min_negativi_supportati",
			kind:   "min",
			local:  -10,
			remote: -4,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, -10)
			},
		},
		{
			name:   "min_input_uguali_supportati",
			kind:   "min",
			local:  3.5,
			remote: 3.5,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, 3.5)
			},
		},
		{
			name:   "min_valori_molto_grandi_supportati",
			kind:   "min",
			local:  math.MaxFloat64,
			remote: math.MaxFloat64 / 2,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, math.MaxFloat64/2)
			},
		},
		{
			name:   "min_inf_supportato",
			kind:   "min",
			local:  math.Inf(1),
			remote: 42,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, 42)
			},
		},
		{
			name:   "min_nan_remoto_non_supportato_e_ignorato",
			kind:   "min",
			local:  10,
			remote: math.NaN(),
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, 10)
			},
		},
		{
			name:   "min_nan_locale_non_supportato_e_preservato",
			kind:   "min",
			local:  math.NaN(),
			remote: 10,
			assertion: func(t *testing.T, got float64) {
				if !math.IsNaN(got) {
					t.Fatalf("atteso NaN locale preservato, got=%v", got)
				}
			},
		},
		{
			name:   "max_negativi_supportati",
			kind:   "max",
			local:  -10,
			remote: -4,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, -4)
			},
		},
		{
			name:   "max_input_uguali_supportati",
			kind:   "max",
			local:  3.5,
			remote: 3.5,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, 3.5)
			},
		},
		{
			name:   "max_valori_molto_grandi_supportati",
			kind:   "max",
			local:  math.MaxFloat64,
			remote: math.MaxFloat64 / 2,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, math.MaxFloat64)
			},
		},
		{
			name:   "max_inf_supportato",
			kind:   "max",
			local:  math.Inf(-1),
			remote: 42,
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, 42)
			},
		},
		{
			name:   "max_nan_remoto_non_supportato_e_ignorato",
			kind:   "max",
			local:  10,
			remote: math.NaN(),
			assertion: func(t *testing.T, got float64) {
				assertFloatEqual(t, got, 10)
			},
		},
		{
			name:   "max_nan_locale_non_supportato_e_preservato",
			kind:   "max",
			local:  math.NaN(),
			remote: 10,
			assertion: func(t *testing.T, got float64) {
				if !math.IsNaN(got) {
					t.Fatalf("atteso NaN locale preservato, got=%v", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			algo, err := Factory(tt.kind)
			if err != nil {
				t.Fatalf("factory(%q) ha restituito errore inatteso: %v", tt.kind, err)
			}

			got := algo.Merge(tt.local, tt.remote)
			tt.assertion(t, got)
		})
	}
}

func assertFloatEqual(t *testing.T, got, want float64) {
	t.Helper()
	if got != want {
		t.Fatalf("valore inatteso: got=%v want=%v", got, want)
	}
}
