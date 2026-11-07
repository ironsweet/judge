package judge

import (
	"context"
	"time"
)

type SessionState struct {
	SessionID   string     `json:"session_id"`
	Intent      string     `json:"intent"`
	ToolCalls   []ToolCall `json:"tool_calls"`
	LastUpdated time.Time  `json:"last_updated"`
}

type Session struct {
	state     *SessionState
	judge     Interface
	persister Persister
}

type Interface interface {
	MergeIntent(ctx context.Context, prevIntent, newMessage string) (MergedIntent, error)
	Evaluate(ctx context.Context, input Input) (Result, error)
}

func NewSession(state *SessionState, judge Interface, persister Persister) *Session {
	return &Session{
		state:     state,
		judge:     judge,
		persister: persister,
	}
}

func (s *Session) OnUserInput(ctx context.Context, prompt string) error {
	panic("niy")
}

func (s *Session) OnToolCall(ctx context.Context, tc ToolCall) error {
	panic("niy")
}

func (s *Session) OnTurnEnd(ctx context.Context, agentResponse string) (Result, error) {
	panic("niy")
}
