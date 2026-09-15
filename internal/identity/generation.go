// Package identity gestisce l'identita' durevole delle istanze runtime.
package identity

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const generationFileMode = 0o600

// AllocateGeneration alloca e persiste atomicamente la generazione successiva.
// La persistenza precede il ritorno: un crash puo' lasciare un salto, mai un riuso.
func AllocateGeneration(path string) (uint64, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if strings.TrimSpace(path) == "" || cleanPath == "." {
		return 0, errors.New("generation path vuoto")
	}
	directory := filepath.Dir(cleanPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return 0, fmt.Errorf("creazione directory generation: %w", err)
	}
	lock, err := os.OpenFile(cleanPath+".lock", os.O_CREATE|os.O_RDWR, generationFileMode)
	if err != nil {
		return 0, fmt.Errorf("apertura lock generation: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return 0, fmt.Errorf("acquisizione lock generation: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()

	current, err := readGeneration(cleanPath)
	if err != nil {
		return 0, err
	}
	if current == math.MaxUint64 {
		return 0, errors.New("generation overflow")
	}
	next := current + 1
	if err := persistGeneration(cleanPath, next); err != nil {
		return 0, err
	}
	return next, nil
}

// readGeneration restituisce zero soltanto quando lo storage non e' mai esistito.
func readGeneration(path string) (uint64, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("lettura generation: %w", err)
	}
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return 0, errors.New("storage generation vuoto")
	}
	generation, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("storage generation corrotto: %w", err)
	}
	return generation, nil
}

// persistGeneration usa file temporaneo, fsync e rename nella stessa directory.
func persistGeneration(path string, generation uint64) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".generation-*")
	if err != nil {
		return fmt.Errorf("creazione file temporaneo generation: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(generationFileMode); err != nil {
		temporary.Close()
		return fmt.Errorf("permessi file generation: %w", err)
	}
	if _, err := fmt.Fprintf(temporary, "%d\n", generation); err != nil {
		temporary.Close()
		return fmt.Errorf("scrittura generation: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync generation: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("chiusura generation: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("rename atomico generation: %w", err)
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("apertura directory generation: %w", err)
	}
	defer directoryHandle.Close()
	if err := directoryHandle.Sync(); err != nil {
		return fmt.Errorf("sync directory generation: %w", err)
	}
	return nil
}
