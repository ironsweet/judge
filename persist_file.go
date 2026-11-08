package judge

type FilePersister struct {
}

func NewFilePersister(baseDir string) *FilePersister {
	panic("niy")
}

func (f *FilePersister) Load(sessionID string) (*SessionState, error) {
	panic("niy")
}

func (f *FilePersister) Save(state *SessionState) error {
	panic("niy")
}

var _ Persister = (*FilePersister)(nil)
