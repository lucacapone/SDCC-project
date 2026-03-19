package max

// Algorithm implementa l'algoritmo di massimo globale.
type Algorithm struct{}

// Type restituisce il nome canonico dell'aggregazione.
func (Algorithm) Type() string { return "max" }

// Merge mantiene il massimo tra valore locale e remoto.
//
// Supporta input finiti e ±Inf. NaN non è un input semantico supportato: il
// comportamento resta quello dei confronti float64 di Go, quindi un NaN remoto
// non sostituisce un valore locale ordinato, mentre un NaN locale viene
// preservato.
func (Algorithm) Merge(local, remote float64) float64 {
	if remote > local {
		return remote
	}
	return local
}
