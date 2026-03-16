package max

// Algorithm implementa l'algoritmo di massimo globale.
type Algorithm struct{}

// Type restituisce il nome canonico dell'aggregazione.
func (Algorithm) Type() string { return "max" }

// Merge mantiene il massimo tra valore locale e remoto.
func (Algorithm) Merge(local, remote float64) float64 {
	if remote > local {
		return remote
	}
	return local
}
