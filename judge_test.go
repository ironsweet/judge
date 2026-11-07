package judge

import (
	"context"
	"encoding/json"
	"errors"
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

func TestJudge_EvaluateNoRetry(t *testing.T) {
	calls := 0
	j := New(func(ctx context.Context, prompt string) (string, error) {
		calls++
		return "", errors.New("llm error")
	})
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
		expectedIntent: MergedIntent{Content: "do X\nalso Y", TopicChanged: false},
	}, {
		name: "successful merge",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return `{"intent":"do X and Y","topic_changed":false}`, nil
		},
		prevIntent:     "do X",
		newMessage:     "also Y",
		expectedIntent: MergedIntent{Content: "do X and Y", TopicChanged: false},
	}, {
		name: "topic changed",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return `{"intent":"do Z","topic_changed":true}`, nil
		},
		prevIntent:     "do X",
		newMessage:     "forget that, do Z",
		expectedIntent: MergedIntent{Content: "do Z", TopicChanged: true},
	}, {
		name: "error",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return "", errors.New("llm error")
		},
		prevIntent:     "do X",
		newMessage:     "also Y",
		expectedIntent: MergedIntent{Content: "do X\nalso Y", TopicChanged: false},
	}, {
		name: "weak LLM",
		fn: func(ctx context.Context, prompt string) (string, error) {
			return `{"intent":"","topic_changed":false}`, nil
		},
		prevIntent:     "do X",
		newMessage:     "also Y",
		expectedIntent: MergedIntent{Content: "do X\nalso Y", TopicChanged: false},
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
			return `{"intent":"merged","topic_changed":false}`, nil
		}
		return `{"satisfied":true}`, nil
	}
	j := New(fn)
	ctx := context.Background()
	intent, err := j.MergeIntent(ctx, "do X", "also Y")
	assert.NoError(t, err)
	assert.Equal(t, MergedIntent{Content: "merged", TopicChanged: false}, intent)

	result, err := j.Evaluate(ctx, Input{UserMessage: "msg", ToolCalls: nil})
	assert.NoError(t, err)
	assert.Equal(t, Result{Satisfied: true}, result)

	assert.Equal(t, 2, calls)
}
