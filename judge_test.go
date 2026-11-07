package judge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvaluate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fn     LLMFunc
		result Result
	}{{
		name:   "nil",
		result: Result{Satisfied: true},
	}, {
		name: "satisfied",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return `{"satisfied":true,"reason":"done"}`, nil
		},
		result: Result{Satisfied: true, Reason: "done"},
	}, {
		name: "unsatisfied",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return `{"satisfied":false,"reason":"missing X"}`, nil
		},
		result: Result{Satisfied: false, Reason: "missing X"},
	}, {
		name: "error but fail-open",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "", errors.New("llm error")
		},
		result: Result{Satisfied: true},
	}, {
		name: "invalid JSON but fail-open",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "I think it's probably done", nil
		},
		result: Result{Satisfied: true},
	}, {
		name: "wrap in markdown JSON tags",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "```\n" + `{"satisfied":false,"reason":"missing Y"}` + "\n```", nil
		},
		result: Result{Satisfied: false, Reason: "missing Y"},
	}, {
		name: "missing field",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return `{"reason":"missing Z"}`, nil
		},
		result: Result{Satisfied: true},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			j := New(tc.fn)
			ctx := context.Background()
			input := Input{
				UserMessage: "any message",
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
