package membership

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestConcurrentSetOperations esercita in parallelo Upsert/Touch/LeaveAt/Snapshot
// sullo stesso membership.Set per validare l'assenza di regressioni logiche sotto concorrenza.
func TestConcurrentSetOperations(t *testing.T) {
	set := NewSetWithConfig(Config{
		SuspectTimeout: 500 * time.Millisecond,
		DeadTimeout:    time.Second,
		PruneRetention: 5 * time.Second,
	})
	base := time.Date(2026, time.April, 20, 9, 0, 0, 0, time.UTC)

	for i := 0; i < 6; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		addr := fmt.Sprintf("node-%d:700%d", i, i)
		set.Upsert(Peer{NodeID: nodeID, Addr: addr, Status: Alive, Incarnation: 1, LastSeen: base})
	}

	const workers = 24
	const iterations = 200

	start := make(chan struct{})
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			<-start

			for i := 0; i < iterations; i++ {
				nodeIdx := (workerID + i) % 6
				nodeID := fmt.Sprintf("node-%d", nodeIdx)
				addr := fmt.Sprintf("node-%d:700%d", nodeIdx, nodeIdx)
				now := base.Add(time.Duration(workerID*iterations+i) * time.Millisecond)

				switch i % 4 {
				case 0:
					set.Upsert(Peer{
						NodeID:      nodeID,
						Addr:        addr,
						Status:      Alive,
						Incarnation: uint64(2 + i + workerID),
						LastSeen:    now,
					})
				case 1:
					set.Touch(nodeID, now)
				case 2:
					set.LeaveAt(nodeID, now)
				default:
					_ = set.Snapshot()
				}
			}
		}(w)
	}

	close(start)
	wg.Wait()

	snapshot := set.Snapshot()
	if len(snapshot) == 0 {
		t.Fatal("snapshot finale vuoto inatteso dopo workload concorrente")
	}

	seenByAddr := make(map[string]string)
	for _, peer := range snapshot {
		if peer.NodeID == "" || peer.Addr == "" {
			t.Fatalf("peer invalido nello snapshot finale: %+v", peer)
		}
		if peer.Incarnation == 0 {
			t.Fatalf("incarnation deve restare valorizzata: %+v", peer)
		}

		if previousNodeID, exists := seenByAddr[peer.Addr]; exists && previousNodeID != peer.NodeID {
			// Invariante principale: nessun duplicato canonico/alias sullo stesso endpoint.
			t.Fatalf("duplica endpoint tra alias/canonico: addr=%s first=%s second=%s", peer.Addr, previousNodeID, peer.NodeID)
		}
		seenByAddr[peer.Addr] = peer.NodeID
	}
}
