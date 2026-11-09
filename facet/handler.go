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
	panic("niy")
}

type Option func(*Handler)

func WithLogger(logger *slog.Logger) Option {
	return func(h *Handler) {
		h.logger = logger
	}
}

func (h *Handler) Mode(kind hookfacet.EventKind) hookfacet.HookMode {
	panic("niy")
}

func (h *Handler) FailureMode() hookfacet.FailureMode {
	panic("niy")
}

func (h *Handler) Timeout() time.Duration {
	panic("niy")
}

func (h *Handler) OnEvent(ctx context.Context, ev hookfacet.Event, st *hookfacet.State) (hookfacet.Decision, error) {
	panic("niy")
}

var _ hookfacet.Handler = (*Handler)(nil)
