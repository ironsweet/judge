package facet

import (
	"context"
	"log/slog"
	"time"

	"github.com/ironsweet/hookfacet"
	"github.com/ironsweet/judge"
)

type Handler struct {
	judge     judge.Interface
	persister judge.Persister
	logger    *slog.Logger
}

func New(j judge.Interface, p judge.Persister, opts ...Option) *Handler {
	h := &Handler{
		judge:     j,
		persister: p,
		logger:    slog.Default(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

type Option func(*Handler)

func WithLogger(logger *slog.Logger) Option {
	return func(h *Handler) {
		h.logger = logger
	}
}

func (h *Handler) Mode(kind hookfacet.EventKind) hookfacet.HookMode {
	switch kind {
	case hookfacet.KindUserInput, hookfacet.KindToolResult:
		return hookfacet.ModeInspect
	case hookfacet.KindTurnEnd:
		return hookfacet.ModeDecide
	default:
		return hookfacet.ModeInspect
	}
}

func (h *Handler) FailureMode() hookfacet.FailureMode {
	return hookfacet.FailOpen
}

func (h *Handler) Timeout() time.Duration {
	return 10 * time.Second
}

func (h *Handler) OnEvent(ctx context.Context, ev hookfacet.Event, st *hookfacet.State) (hookfacet.Decision, error) {
	s, exists := hookfacet.Get[*judge.Session](st)
	if !exists {
		var err error
		s, err = judge.LoadSession(ev.SessionID, h.judge, h.persister, judge.WithSessionLogger(h.logger))
		if err != nil {
			h.logger.Warn("failed to load session, using empty state", "sessionID", ev.SessionID, "error", err)
			s = judge.NewSession(&judge.SessionState{SessionID: ev.SessionID}, h.judge, h.persister, judge.WithSessionLogger(h.logger))
		}
		hookfacet.Set[*judge.Session](st, s)
	}

	switch ev.Kind {
	case hookfacet.KindUserInput:
		err := s.OnUserInput(ctx, ev.Payload.Prompt)
		return hookfacet.Allow(), err
	case hookfacet.KindToolResult:
		err := s.OnToolCall(ctx, judge.ToolCall{
			Name:   ev.Payload.ToolName,
			Input:  ev.Payload.ToolInput,
			Output: ev.Payload.ToolResponse,
		})
		return hookfacet.Allow(), err
	case hookfacet.KindTurnEnd:
		res, err := s.OnTurnEnd(ctx, ev.Payload.LastAssistantMessage)
		if err != nil || res.Satisfied {
			return hookfacet.Allow(), err
		}
		return hookfacet.Block(res.Reason), nil
	default:
		return hookfacet.Allow(), nil
	}
}

var _ hookfacet.Handler = (*Handler)(nil)
