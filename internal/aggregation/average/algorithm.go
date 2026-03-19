package average

// Algorithm implementa l'algoritmo di media richiesto dalla factory aggregazioni.
//
// Nota: il merge pairwise locale/remoto è mantenuto per compatibilità API, ma la
// convergenza gossip reale per "average" è implementata nel layer gossip tramite
// metadati sum/count per nodo.
type Algorithm struct{}

// Type restituisce il nome canonico dell'aggregazione.
func (Algorithm) Type() string { return "average" }

// Merge combina due valori con media aritmetica semplice per compatibilità.
//
// Supporta input finiti e ±Inf secondo IEEE-754 del float64. NaN non è un
// input semantico supportato: se presente viene propagato dall'aritmetica Go.
func (Algorithm) Merge(local, remote float64) float64 {
	return (local + remote) / 2
}
