package identity_test

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"sdcc-project/internal/identity"
)

// TestAllocateGeneration verifica primo boot, restart e monotonicita' durevole.
func TestAllocateGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generation")
	for want := uint64(1); want <= 3; want++ {
		got, err := identity.AllocateGeneration(path)
		if err != nil {
			t.Fatalf("allocazione %d: %v", want, err)
		}
		if got != want {
			t.Fatalf("generation inattesa: got=%d want=%d", got, want)
		}
	}
}

// TestAllocateGenerationFailFast verifica storage corrotto, overflow e path non scrivibile.
func TestAllocateGenerationFailFast(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "corrotto", content: "non-un-numero\n"},
		{name: "vuoto", content: ""},
		{name: "overflow", content: fmt.Sprintf("%d\n", uint64(math.MaxUint64))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "generation")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := identity.AllocateGeneration(path); err == nil {
				t.Fatal("allocazione riuscita su storage invalido")
			}
		})
	}

	t.Run("directory non scrivibile", func(t *testing.T) {
		parentFile := filepath.Join(t.TempDir(), "non-directory")
		if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := identity.AllocateGeneration(filepath.Join(parentFile, "generation")); err == nil {
			t.Fatal("allocazione riuscita sotto un file non scrivibile come directory")
		}
	})
}

// TestAllocateGenerationConcurrent serializza allocazioni concorrenti sullo stesso storage.
func TestAllocateGenerationConcurrent(t *testing.T) {
	const workers = 12
	path := filepath.Join(t.TempDir(), "generation")
	results := make([]uint64, workers)
	errorsByWorker := make([]error, workers)
	var group sync.WaitGroup
	for index := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index], errorsByWorker[index] = identity.AllocateGeneration(path)
		}()
	}
	group.Wait()
	for index, err := range errorsByWorker {
		if err != nil {
			t.Fatalf("worker %d: %v", index, err)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i] < results[j] })
	for index, got := range results {
		if want := uint64(index + 1); got != want {
			t.Fatalf("sequenza concorrente inattesa: got=%v", results)
		}
	}
}
