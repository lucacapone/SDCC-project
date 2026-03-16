package aggregation

import (
	"fmt"

	"sdcc-project/internal/aggregation/average"
	"sdcc-project/internal/aggregation/max"
	"sdcc-project/internal/aggregation/min"
	"sdcc-project/internal/aggregation/sum"
)

// Algorithm definisce il contratto comune per le aggregazioni supportate dal dominio gossip.
type Algorithm interface {
	// Type restituisce il nome canonico dell'aggregazione (es. "sum").
	Type() string
	// Merge combina valore locale e remoto secondo la semantica dell'algoritmo.
	Merge(local, remote float64) float64
}

// Factory crea un'implementazione di Algorithm in base al tipo richiesto.
func Factory(kind string) (Algorithm, error) {
	switch kind {
	case "sum":
		return sum.Algorithm{}, nil
	case "average":
		return average.Algorithm{}, nil
	case "min":
		return min.Algorithm{}, nil
	case "max":
		return max.Algorithm{}, nil
	default:
		return nil, fmt.Errorf("aggregazione non supportata: %s", kind)
	}
}
