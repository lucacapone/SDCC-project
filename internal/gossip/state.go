package gossip

import "time"

// ApplyRemote applica merge minimale tra stato locale e messaggio remoto.
// TODO(tecnico): applicare regole di merge legate all'algoritmo selezionato.
func (s State) ApplyRemote(msg Message) State {
	s.Round++
	s.Value = (s.Value + msg.Value) / 2
	s.UpdatedAt = time.Now().UTC()
	return s
}
