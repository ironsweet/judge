package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
)

type ToolCall struct {
	Name   string
	Input  json.RawMessage
	Output json.RawMessage
}

type Input struct {
	UserIntent    string
	AgentResponse string
	ToolCalls     []ToolCall
}

type Result struct {
	Satisfied bool
	Reason    string
}

type Judge struct {
	fn LLMFunc

	evaluationPrompt  string
	mergeIntentPrompt string
	maxPromptChars    int

	logger *slog.Logger
}

type LLMFunc func(ctx context.Context, prompt string) (string, error)

func New(fn LLMFunc, opts ...Option) *Judge {
	j := &Judge{
		fn:                fn,
		evaluationPrompt:  defaultEvaluatePrompt,
		mergeIntentPrompt: defaultMergeIntentPrompt,
		maxPromptChars:    defaultMaxPromptChars,
		logger:            slog.Default(),
	}
	for _, opt := range opts {
		opt(j)
	}
	return j
}

type Option func(*Judge)

func WithEvaluationPrompt(tmpl string) Option {
	return func(j *Judge) {
		j.evaluationPrompt = tmpl
	}
}
func WithMergeIntentPrompt(tmpl string) Option {
	return func(j *Judge) {
		j.mergeIntentPrompt = tmpl
	}
}
func WithLogger(logger *slog.Logger) Option {
	return func(j *Judge) {
		j.logger = logger
	}
}
func WithMaxPromptChars(n int) Option {
	return func(j *Judge) {
		j.maxPromptChars = n
	}
}

const defaultEvaluatePrompt = `Analyze if the AI agent properly fulfilled what the user asked for.

Captured User Intent:
%s

Agent Response:
%s

Agent Actions:
%s

Instructions:
1. What did the user explicitly ask AI agent to do or verify?
2. Did AI agent actually perform those actions, or just claim completion?
3. Are there verification steps the user requested but AI agent skipped?

Output Rules:
If the AI agent fully fulfilled the intent, respond with ONLY "OK".
If there are gaps or issues, output correction message in this format:
"ISSUE: <Your observation>. <Question or reminder>"
Do not use any other format.

Examples:
1. Missing verification
   User: "Build the feature and ensure it compiles successfully"
   Assistant: "Feature implemented. Task complete."
   Actions: [filesystem: write main.go]
   Output: "ISSUE: You claimed the task is done, but I don't see evidence you verified compilation. Did you run 'go build' or equivalent?"
2. Missing git check
	 User: "Make the changes and check git status"
   Assistant: "Changes made successfully."
   Actions: [filesystem: write config.json]
   Output: "ISSUE: The user asked you to check git status, but I don't see that in your actions. Can you run 'git status' as requested?"
3. Everything done
   User: "Build and test the feature"
   Assistant: "Built successfully, all tests pass."
   Actions: [shell: go build, shell: go test]
	 Output: "OK"
`

const defaultMergeIntentPrompt = `Summarize the user's intent in 1-2 short sentences. Focus on:
1. What the user wants done
2. Any specific verification or artifact expected

Be concise. Do not explain your reasoning. Output ONLY the summary.

For example:
- "Build feature X and verify it compiles"
- "Fix the null pointer bug in handler.go"
- "Review the code for security issues"
- "Explain how the auth module works"

Previous intent:
%s

Latest user message:
%s
`

const defaultMaxPromptChars = 8000

func (j *Judge) Evaluate(ctx context.Context, input Input) (Result, error) {
	var fallbackResult = Result{Satisfied: true}
	if j.fn == nil {
		return fallbackResult, nil
	}

	fixed := len(fmt.Sprintf(j.evaluationPrompt, "", "", ""))
	nonOptional := fixed + len(input.UserIntent) + len(input.AgentResponse)
	budget := j.maxPromptChars - nonOptional
	if budget <= 0 {
		j.logger.Warn("LLM evaluation prompt exceeds maxPromptChars, skipping evaluation", "maxPromptChars", j.maxPromptChars, "fixed", fixed, "nonOptional", nonOptional)
		return fallbackResult, nil
	}

	toolCallsStr := formatToolCalls(input.ToolCalls, budget)
	p := fmt.Sprintf(j.evaluationPrompt, input.UserIntent, input.AgentResponse, toolCallsStr)
	output, err := j.fn(ctx, p)
	if err != nil {
		j.logger.Warn("LLM evaluation failed, continuing with fail-open", "error", err)
		return fallbackResult, nil // fail-open
	}
	return j.parseEvaluation(output), nil
}

// formatToolCalls formats the tool calls into a string with length budget
// Omit field sorted by length, until the total length is within budget.
// Algorithm:
// 1. Scan all input and record the length of each field
// 2. Sort diffs descending and greedily omit the largest fields until totalLen fits within budget
// 3. Iterate the tool calls and format them, skipping the longest fields until the total length is within budget
func formatToolCalls(calls []ToolCall, budget int) string {
	var diffs []int
	var totalLen int
	for _, call := range calls {
		totalLen += len(fmt.Sprintf("- %s", call.Name))
		var input map[string]any
		err := json.Unmarshal(call.Input, &input)
		if err != nil {
			totalLen++ // space after name
			totalLen += len(call.Input)
			totalLen++ // newline
			continue
		}
		for k, v := range input {
			totalLen += len(fmt.Sprintf(" %s=\"%v\"", k, v))
			n := len(fmt.Sprintf("%v", v))
			diff := n - len(fmt.Sprintf("<%d bytes omitted>", n))
			if diff > 0 {
				diffs = append(diffs, diff)
			}
		}
		totalLen++ // for newline
	}

	var threshold = -1
	if totalLen > budget {
		slices.Sort(diffs)
		for i := len(diffs) - 1; i >= 0; i-- {
			if totalLen-diffs[i] <= budget {
				threshold = diffs[i]
				break
			}
			totalLen -= diffs[i]
		}
		if threshold == -1 {
			threshold = 0 // omit all
		}
	}

	var sb strings.Builder
	for _, call := range calls {
		sb.WriteString(fmt.Sprintf("- %s", call.Name))
		var input map[string]any
		err := json.Unmarshal(call.Input, &input)
		if err != nil {
			sb.WriteString(fmt.Sprintf(" %s", call.Input))
			sb.WriteString("\n")
			continue
		}
		for k, v := range input {
			n := len(fmt.Sprintf("%v", v))
			if threshold >= 0 && n > threshold {
				sb.WriteString(fmt.Sprintf(" %s=\"<%d bytes omitted>\"", k, n))
			} else {
				sb.WriteString(fmt.Sprintf(" %s=\"%v\"", k, v))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func (j *Judge) parseEvaluation(output string) Result {
	trimmed := strings.TrimSpace(output)
	upper := strings.ToUpper(trimmed)

	if strings.HasPrefix(upper, "ISSUE:") {
		reason := strings.TrimSpace(trimmed[len("ISSUE:"):])
		return Result{Satisfied: false, Reason: reason}
	}

	if upper == "OK" ||
		strings.HasPrefix(upper, "OK ") ||
		strings.HasPrefix(upper, "OK.") ||
		strings.HasPrefix(upper, "OK,") ||
		strings.HasPrefix(upper, "OK!") {
		return Result{Satisfied: true}
	}

	j.logger.Warn("LLM evaluation returned unexpected output, treating as satisfied", "output", trimmed)
	return Result{Satisfied: true}
}

type MergedIntent struct {
	Content string
}

func (j *Judge) MergeIntent(ctx context.Context, prevIntent, newMessage string) (MergedIntent, error) {
	fallbackStr := fmt.Sprintf("%s\n%s", prevIntent, newMessage)
	if j.fn == nil {
		return MergedIntent{Content: fallbackStr}, nil
	}

	p := fmt.Sprintf(j.mergeIntentPrompt, prevIntent, newMessage)
	output, err := j.fn(ctx, p)
	if err != nil {
		j.logger.Warn("LLM merge intent failed, continuing with fail-open", "error", err)
		return MergedIntent{Content: fallbackStr}, nil // fail-open
	}
	if strings.TrimSpace(output) == "" {
		j.logger.Warn("LLM merge intent returned empty, continuing with fail-open")
		return MergedIntent{Content: fallbackStr}, nil // fail-open
	}

	return MergedIntent{Content: output}, nil
}
