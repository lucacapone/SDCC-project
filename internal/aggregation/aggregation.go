package aggregation

import (
	"fmt"

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
		return averageStub{}, nil
	case "min":
		return minStub{}, nil
	case "max":
		return maxStub{}, nil
	default:
		return nil, fmt.Errorf("aggregazione non supportata: %s", kind)
	}
}

// averageStub mantiene la compatibilità runtime finché non viene introdotta una semantica distribuita completa.
type averageStub struct{}

func (averageStub) Type() string                        { return "average" }
func (averageStub) Merge(local, remote float64) float64 { return (local + remote) / 2 }

// minStub mantiene il minimo locale/remoto con semantica monotona semplice.
type minStub struct{}

func (minStub) Type() string { return "min" }
func (minStub) Merge(local, remote float64) float64 {
	if remote < local {
		return remote
	}
	return local
}

// maxStub mantiene il massimo locale/remoto con semantica monotona semplice.
type maxStub struct{}

func (maxStub) Type() string { return "max" }
func (maxStub) Merge(local, remote float64) float64 {
	if remote > local {
		return remote
	}
	return local
}
