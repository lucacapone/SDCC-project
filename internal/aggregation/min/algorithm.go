package min

// Algorithm implementa l'algoritmo di minimo globale.
type Algorithm struct{}

// Type restituisce il nome canonico dell'aggregazione.
func (Algorithm) Type() string { return "min" }

// Merge mantiene il minimo tra valore locale e remoto.
func (Algorithm) Merge(local, remote float64) float64 {
	if remote < local {
		return remote
	}
	return local
}
