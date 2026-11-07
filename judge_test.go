package judge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJudge_Evaluate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fn     LLMFunc
		result Result
	}{{
		name:   "nil LLM fn",
		result: Result{Satisfied: true},
	}, {
		name: "satisfied",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return `OK`, nil
		},
		result: Result{Satisfied: true},
	}, {
		name: "unsatisfied",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return `ISSUE: missing X. Do X.`, nil
		},
		result: Result{Satisfied: false, Reason: "missing X. Do X."},
	}, {
		name: "error but fail-open",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "", errors.New("llm error")
		},
		result: Result{Satisfied: true},
	}, {
		name: "OK with newline",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "OK\n", nil
		},
		result: Result{Satisfied: true},
	}, {
		name: "lowercase ok",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "ok", nil
		},
		result: Result{Satisfied: true},
	}, {
		name: "non-english",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "好的，没问题", nil
		},
		result: Result{Satisfied: true},
	}, {
		name: "no issue prefix",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "missing X. Do X.", nil
		},
		result: Result{Satisfied: true},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			j := New(tc.fn)
			ctx := context.Background()
			input := Input{
				UserIntent:    "any message",
				AgentResponse: "any response",
				ToolCalls: []ToolCall{
					{Name: "toolA", Input: json.RawMessage(`{"param": 1}`), Output: json.RawMessage(`{"result": "ok"}`)},
					{Name: "toolB", Input: json.RawMessage(`{"param": 2}`), Output: json.RawMessage(`{"result": "ok"}`)},
				},
			}
			result, err := j.Evaluate(ctx, input)
			assert.NoError(t, err)
			assert.Equal(t, tc.result, result)
		})
	}
}

func TestJudge_EvaluateNoRetry(t *testing.T) {
	calls := 0
	j := New(func(ctx context.Context, prompt string) (string, error) {
		calls++
		return "", errors.New("llm error")
	})
	ctx := context.Background()
	input := Input{
		UserIntent:    "any message",
		AgentResponse: "any response",
		ToolCalls: []ToolCall{
			{Name: "toolA", Input: json.RawMessage(`{"param": 1}`), Output: json.RawMessage(`{"result": "ok"}`)},
			{Name: "toolB", Input: json.RawMessage(`{"param": 2}`), Output: json.RawMessage(`{"result": "ok"}`)},
		},
	}
	result, err := j.Evaluate(ctx, input)
	assert.NoError(t, err)
	assert.Equal(t, Result{Satisfied: true}, result)
	assert.Equal(t, 1, calls)
}

func TestJudge_MergeIntent(t *testing.T) {
	for _, tc := range []struct {
		name           string
		fn             LLMFunc
		prevIntent     string
		newMessage     string
		expectedIntent MergedIntent
	}{{
		name:           "nil LLM fn",
		prevIntent:     "do X",
		newMessage:     "also Y",
		expectedIntent: MergedIntent{Content: "do X\nalso Y"},
	}, {
		name: "successful merge",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return `do X and Y`, nil
		},
		prevIntent:     "do X",
		newMessage:     "also Y",
		expectedIntent: MergedIntent{Content: "do X and Y"},
	}, {
		name: "error",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "", errors.New("llm error")
		},
		prevIntent:     "do X",
		newMessage:     "also Y",
		expectedIntent: MergedIntent{Content: "do X\nalso Y"},
	}, {
		name: "empty output",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "", nil
		},
		prevIntent:     "do X",
		newMessage:     "also Y",
		expectedIntent: MergedIntent{Content: "do X\nalso Y"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			j := New(tc.fn)
			ctx := context.Background()
			intent, err := j.MergeIntent(ctx, tc.prevIntent, tc.newMessage)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedIntent, intent)
		})
	}
}

func TestJudge_SimpleE2E(t *testing.T) {
	var calls int
	fn := func(ctx context.Context, prompt string) (string, error) {
		calls++
		if calls == 1 {
			return `merged`, nil
		}
		return `OK`, nil
	}
	j := New(fn)
	ctx := context.Background()
	intent, err := j.MergeIntent(ctx, "do X", "also Y")
	assert.NoError(t, err)
	assert.Equal(t, MergedIntent{Content: "merged"}, intent)

	result, err := j.Evaluate(ctx, Input{UserIntent: "msg", AgentResponse: "any response", ToolCalls: []ToolCall{
		{Name: "toolA", Input: json.RawMessage(`{"param": 1}`), Output: json.RawMessage(`{"result": "ok"}`)},
		{Name: "toolB", Input: json.RawMessage(`{"param": 2}`), Output: json.RawMessage(`{"result": "ok"}`)},
	}})
	assert.NoError(t, err)
	assert.Equal(t, Result{Satisfied: true}, result)

	assert.Equal(t, 2, calls)
}

func TestFormatToolCalls_Budget(t *testing.T) {
	calls := []ToolCall{
		{Name: "write_file", Input: json.RawMessage(`{"content":"` + strings.Repeat("x", 5000) + `"}`)},
		{Name: "write_file", Input: json.RawMessage(`{"content":"` + strings.Repeat("y", 5000) + `"}`)},
		{Name: "read_file", Input: json.RawMessage(`{"path":"/tmp/file.txt"}`)},
	}
	out := formatToolCalls(calls, 20000)
	assert.NotContains(t, out, "omitted")

	out = formatToolCalls(calls, 6000)
	assert.Contains(t, out, "omitted")
	assert.Contains(t, out, "/tmp/file.txt")

	out = formatToolCalls(calls, 200)
	assert.Equal(t, 2, strings.Count(out, "omitted"))

	out = formatToolCalls(calls, 50)
	assert.Equal(t, 3, strings.Count(out, "omitted"))
	assert.NotContains(t, out, "/tmp/file.txt")
}
