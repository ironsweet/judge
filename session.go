package judge

type SessionState struct {
	SessionID   string     `json:"session_id"`
	IntentParts []string   `json:"intent_parts"`
	ToolCalls   []ToolCall `json:"tool_calls"`
	LastUpdated time.Time  `json:"last_updated"`
}

func (s *SessionState) MergedIntent() string {
	panic("niy")
}

type Session struct {
	state     *SessionState
	judge     *Judge
	persister Persister
}

func NewSession(state *SessionState, judge *Judge, persister Persister) *Session {
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

func (s *Session) OnTurnEnd(ctx context.Context) (satisfied bool, reason string, err error) {
	panic("niy")
}
