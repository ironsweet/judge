package judge

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type SessionState struct {
	SessionID   string     `json:"session_id"`
	Intent      string     `json:"intent"`
	ToolCalls   []ToolCall `json:"tool_calls,omitempty"`
	LastUpdated time.Time  `json:"last_updated"`
}

type Session struct {
	state     *SessionState
	judge     Interface
	persister Persister
	logger    *slog.Logger
}

type Interface interface {
	MergeIntent(ctx context.Context, prevIntent, newMessage string) (MergedIntent, error)
	Evaluate(ctx context.Context, input Input) (Result, error)
}

type Persister interface {
	Load(sessionID string) (*SessionState, error)
	Save(state *SessionState) error
}

func NewSession(state *SessionState, judge Interface, persister Persister, opts ...SessionOption) *Session {
	if state == nil || judge == nil || persister == nil {
		panic("judge: state, judge, and persister must not be nil")
	}
	s := &Session{
		state:     state,
		judge:     judge,
		persister: persister,
		logger:    slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type SessionOption func(*Session)

func WithSessionLogger(logger *slog.Logger) SessionOption {
	return func(s *Session) {
		s.logger = logger
	}
}

func LoadSession(sessionID string, judge Interface, persister Persister, opts ...SessionOption) (*Session, error) {
	state, err := persister.Load(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session %s: %w", sessionID, err)
	}
	if state == nil {
		state = &SessionState{
			SessionID:   sessionID,
			LastUpdated: time.Now(),
		}
	}
	return NewSession(state, judge, persister, opts...), nil
}

func (s *Session) OnUserInput(ctx context.Context, prompt string) error {
	prompt = normalizeUserPrompt(prompt)
	if strings.TrimSpace(prompt) == "" {
		return nil
	}

	mergedIntent, err := s.judge.MergeIntent(ctx, s.state.Intent, prompt)
	if err != nil {
		s.logger.Warn("Failed to merge intent", "error", err)
		// still use mergedContent (Judge returns fallback when fail-open)
	}

	s.state.Intent = mergedIntent.Content
	s.state.LastUpdated = time.Now()
	err = s.persister.Save(s.state)
	if err != nil {
		s.logger.Warn("Failed to save session", "error", err)
	}

	return nil
}

const systemReminderOpen = "<system-reminder>"
const systemReminderClose = "</system-reminder>"

// normalizeUserPrompt removes system appended parts
func normalizeUserPrompt(prompt string) string {
	var parts []string
	s := prompt
	for {
		next := strings.Index(s, systemReminderOpen)
		if next < 0 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:next])

		s = s[next+len(systemReminderOpen):]
		next = strings.Index(s, systemReminderClose)
		if next < 0 {
			parts = append(parts, s)
			break
		}

		s = s[next+len(systemReminderClose):]
	}
	return strings.Join(parts, "")
}

func (s *Session) OnToolCall(_ context.Context, tc ToolCall) error {
	s.state.ToolCalls = append(s.state.ToolCalls, tc)
	s.state.LastUpdated = time.Now()
	err := s.persister.Save(s.state)
	if err != nil {
		s.logger.Warn("Failed to save session", "error", err)
	}
	return nil
}

func (s *Session) OnTurnEnd(ctx context.Context, agentResponse string) (Result, error) {
	s.logger.Info("turn end",
		"toolCallCount", len(s.state.ToolCalls),
		"skipped", len(s.state.ToolCalls) == 0)

	if len(s.state.ToolCalls) == 0 {
		s.state.LastUpdated = time.Now()
		err := s.persister.Save(s.state)
		if err != nil {
			s.logger.Warn("Failed to save session", "error", err)
		}
		return Result{Satisfied: true}, nil
	}

	res, evalErr := s.judge.Evaluate(ctx, Input{
		UserIntent:    s.state.Intent,
		AgentResponse: agentResponse,
		ToolCalls:     s.state.ToolCalls,
	})

	s.state.ToolCalls = nil
	s.state.LastUpdated = time.Now()
	saveErr := s.persister.Save(s.state)
	if saveErr != nil {
		s.logger.Warn("Failed to save session", "error", saveErr)
	}
	return res, evalErr
}
