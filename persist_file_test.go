package judge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPersist_SaveAndLoad(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "to_create")
	_, err := os.Stat(baseDir)
	assert.ErrorIs(t, err, os.ErrNotExist)

	savedTime := time.Now()
	p := NewFilePersister(baseDir)
	assert.NoError(t, p.Save(&SessionState{
		SessionID: "s1",
		Intent:    "do X",
		ToolCalls: []ToolCall{
			{Name: "toolA", Input: json.RawMessage(`{"param": 1}`), Output: json.RawMessage(`{"result": "ok"}`)},
			{Name: "toolB", Input: json.RawMessage(`{"param": 2}`), Output: json.RawMessage(`{"result": "ok"}`)},
		},
		LastUpdated: savedTime,
	}))
	_, err = os.Stat(baseDir)
	assert.NoError(t, err)

	s, err := p.Load("s1")
	assert.NoError(t, err)
	assert.Equal(t, "s1", s.SessionID)
	assert.Equal(t, "do X", s.Intent)
	assert.Equal(t, "toolA", s.ToolCalls[0].Name)
	assert.Equal(t, `{"param": 1}`, string(s.ToolCalls[0].Input))
	assert.Equal(t, `{"result": "ok"}`, string(s.ToolCalls[0].Output))
	assert.Equal(t, "toolB", s.ToolCalls[1].Name)
	assert.Equal(t, `{"param": 2}`, string(s.ToolCalls[1].Input))
	assert.Equal(t, `{"result": "ok"}`, string(s.ToolCalls[1].Output))
	assert.WithinDuration(t, savedTime, s.LastUpdated, time.Second)
}

func TestPersist_LoadNonexistent(t *testing.T) {
	baseDir := t.TempDir() // must exists
	p := NewFilePersister(baseDir)
	s, err := p.Load("nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, s)
}

func TestPersist_LoadNonexistentBaseDir(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "never_created")
	p := NewFilePersister(baseDir)
	s, err := p.Load("s1")
	assert.NoError(t, err)
	assert.Nil(t, s)
}

func TestPersist_SaveInvalidSessionID(t *testing.T) {
	for _, tc := range []struct {
		name      string
		sessionID string
	}{
		{sessionID: "", name: "empty"},
		{sessionID: ".", name: "single dot"},
		{sessionID: "..", name: "double dot"},
		{sessionID: "../foo", name: "parent traversal"},
		{sessionID: "/etc/passwd", name: "absolute path"},
		{sessionID: "a/b", name: "slash"},
		{sessionID: "a\\b", name: "back slash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseDir := filepath.Join(t.TempDir(), "persist")
			p := NewFilePersister(baseDir)
			err := p.Save(&SessionState{SessionID: tc.sessionID, Intent: "do X", LastUpdated: time.Now()})
			assert.Error(t, err)
			_, err = p.Load(tc.sessionID)
			assert.Error(t, err)
		})
	}
}

func TestPersist_SaveConsistent(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "persist")
	p := NewFilePersister(baseDir)
	firstTime := time.Now()
	assert.NoError(t, p.Save(&SessionState{SessionID: "s1", Intent: "do X", LastUpdated: firstTime}))
	time.Sleep(10 * time.Millisecond)
	secondTime := time.Now()
	assert.NoError(t, p.Save(&SessionState{SessionID: "s1", Intent: "do Y", LastUpdated: secondTime}))
	s, err := p.Load("s1")
	assert.NoError(t, err)
	assert.Equal(t, "s1", s.SessionID)
	assert.Equal(t, "do Y", s.Intent)
	assert.WithinDuration(t, secondTime, s.LastUpdated, time.Second)
}
