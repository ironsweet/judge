package judge

type Persister interface {
	Load(sessionID string) (*SessionState, error)
	Save(state *SessionState) error
}
