package sum

// Algorithm implementa la prima aggregazione globale concreta: somma.
type Algorithm struct{}

// Type restituisce il nome canonico dell'aggregazione.
func (Algorithm) Type() string { return "sum" }

// Merge combina i contributi locale e remoto tramite somma aritmetica.
func (Algorithm) Merge(local, remote float64) float64 {
	return local + remote
}
