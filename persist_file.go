package judge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FilePersister struct {
	baseDir string
}

func NewFilePersister(baseDir string) *FilePersister {
	return &FilePersister{baseDir: baseDir}
}

func (p *FilePersister) Load(sessionID string) (*SessionState, error) {
	if !isValidSessionID(sessionID) {
		return nil, fmt.Errorf("invalid session id: %s", sessionID)
	}

	f, err := os.Open(formatStatePath(p.baseDir, sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open session state file: %w", err)
	}
	defer f.Close()

	var s SessionState
	err = json.NewDecoder(f).Decode(&s)
	if err != nil {
		return nil, fmt.Errorf("failed to decode session state file: %w", err)
	}
	return &s, nil
}

func isValidSessionID(sessionID string) bool {
	return sessionID != "" &&
		sessionID != "." &&
		sessionID != ".." &&
		!strings.ContainsAny(sessionID, "/\\") &&
		!filepath.IsAbs(sessionID)
}

func formatStatePath(baseDir, sessionID string) string {
	return filepath.Join(baseDir, sessionID, "judge.json")
}

func (p *FilePersister) Save(state *SessionState) error {
	if !isValidSessionID(state.SessionID) {
		return fmt.Errorf("invalid session id: %s", state.SessionID)
	}

	sp := formatStatePath(p.baseDir, state.SessionID)
	err := os.MkdirAll(filepath.Dir(sp), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// use create->rename algorithm
	f, err := os.CreateTemp(filepath.Dir(sp), "judge-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	tmpPath := f.Name()
	defer os.Remove(tmpPath)

	err = json.NewEncoder(f).Encode(state)
	if err != nil {
		return fmt.Errorf("failed to encode session state: %w", err)
	}
	err = f.Close()
	if err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	err = os.Rename(tmpPath, sp)
	if err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

var _ Persister = (*FilePersister)(nil)
