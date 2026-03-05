package aggregate

import "fmt"

// Algorithm definisce il contratto per gli algoritmi di aggregazione decentralizzata.
type Algorithm interface {
	Type() string
	Merge(local, remote float64) float64
}

// Factory crea algoritmi in base al tipo richiesto.
// TODO(tecnico): sostituire placeholder con implementazioni corrette (sum/average/min/max).
func Factory(kind string) (Algorithm, error) {
	switch kind {
	case "sum":
		return SumPlaceholder{}, nil
	case "average":
		return AveragePlaceholder{}, nil
	default:
		return nil, fmt.Errorf("aggregazione non supportata: %s", kind)
	}
}

// SumPlaceholder è uno stub tecnico: non implementa una semantica distribuita completa.
type SumPlaceholder struct{}

func (SumPlaceholder) Type() string                        { return "sum" }
func (SumPlaceholder) Merge(local, remote float64) float64 { return local + remote }

// AveragePlaceholder è uno stub tecnico: TODO usare count+sum per media consistente.
type AveragePlaceholder struct{}

func (AveragePlaceholder) Type() string                        { return "average" }
func (AveragePlaceholder) Merge(local, remote float64) float64 { return (local + remote) / 2 }
