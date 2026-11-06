package judge

type ToolCall struct {
	Name   string
	Input  json.RawMessage
	Output json.RawMessage
}

type Input struct {
	UserMessage string
	ToolCalls   []ToolCall
}

type Result struct {
	Satisfied bool
	Reason    string
}

type Judge struct {
	fn LLMFunc
}

type LLMFunc func(ctx context.Context, prompt string) (string, error)

func New(fn LLMFunc) *Judge {
	return &Judge{fn: fn}
}

func (j *Judge) Evaluate(ctx context.Context, input Input) (Result, error) {
	panic("niy")
}
