package gossip

import (
	"time"

	shared "sdcc-project/internal/types"
)

// applyRemote applica merge minimale tra stato locale e messaggio remoto.
// TODO(tecnico): applicare regole di merge legate all'algoritmo selezionato.
func applyRemote(local shared.GossipState, msg shared.GossipMessage) shared.GossipState {
	local.Round++
	local.Value = (local.Value + msg.State.Value) / 2
	local.UpdatedAt = time.Now().UTC()
	return local
}
