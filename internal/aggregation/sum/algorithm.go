package sum

// Algorithm implementa la prima aggregazione globale concreta: somma.
type Algorithm struct{}

// Type restituisce il nome canonico dell'aggregazione.
func (Algorithm) Type() string { return "sum" }

// Merge combina i contributi locale e remoto tramite somma aritmetica.
//
// Supporta input finiti e ±Inf secondo IEEE-754 del float64. NaN non è un
// input semantico supportato: se presente viene propagato dall'aritmetica Go.
func (Algorithm) Merge(local, remote float64) float64 {
	return local + remote
}
