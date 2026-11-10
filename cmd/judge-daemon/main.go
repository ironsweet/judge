package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/ironsweet/hookfacet"
	"github.com/ironsweet/judge"
	"github.com/ironsweet/judge/facet"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	socket := getenv("JUDGE_SOCKET", filepath.Join(os.TempDir(), "judge.sock"))
	workspace := getenv("JUDGE_WORKSPACE", filepath.Join(os.Getenv("HOME"), ".judge"))
	logger.Info("starting judge daemon", "socket", socket, "workspace", workspace)

	p := judge.NewFilePersister(workspace)
	llmFn, err := buildLLMFn()
	if err != nil {
		logger.Error("failed to build LLM function", "error", err)
		os.Exit(1)
	}

	j := judge.New(llmFn, judge.WithLogger(logger))
	h := facet.New(j, p, facet.WithLogger(logger))

	r := hookfacet.NewRunner(h)
	err = hookfacet.Serve(ctx, socket, r, logger)
	if err != nil {
		logger.Error("failed to start judge server", "error", err)
		os.Exit(1)
	}
}

func getenv(key, defaultValue string) string {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	return v
}

func buildLLMFn() (judge.LLMFunc, error) {
	provider := os.Getenv("JUDGE_LLM_PROVIDER")
	if provider == "" {
		provider = "anthropic"
	}
	model := os.Getenv("JUDGE_LLM_MODEL")
	if model == "" {
		return nil, errors.New("JUDGE_LLM_MODEL is required")
	}
	apiKey := os.Getenv("JUDGE_LLM_API_KEY")
	if apiKey == "" {
		return nil, errors.New("JUDGE_LLM_API_KEY is required")
	}
	apiBase := os.Getenv("JUDGE_LLM_BASE_URL")

	switch provider {
	case "anthropic":
		var opts = []option.RequestOption{option.WithAPIKey(apiKey)}
		if apiBase != "" {
			opts = append(opts, option.WithBaseURL(apiBase))
		}
		ac := anthropic.NewClient(opts...)
		return func(ctx context.Context, prompt string) (string, error) {
			msg, err := ac.Messages.New(ctx, anthropic.MessageNewParams{
				Model:     model,
				MaxTokens: 1024,
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
				},
			})
			if err != nil {
				return "", fmt.Errorf("anthropic: %w", err)
			}

			for _, block := range msg.Content {
				if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
					return tb.Text, nil
				}
			}
			return "", errors.New("anthropic: no text block found")
		}, nil
	default:
		return nil, fmt.Errorf("unknown JUDGE_LLM_PROVIDER: %s", provider)
	}
}
