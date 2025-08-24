// File: internal/infra/logging/logging.go
package logging

import (
	"context"
	"os"
	"strings"
	"time"

	"telegram-ai-subscription/internal/config"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// New creates a zerolog logger configured from config.
// Supports "trace" | "debug" | "info" | "warn" | "error" levels
// and "json" | "console" formats. Sampling can be enabled to reduce noise in prod.
func New(cfg config.LogConfig, dev bool) *zerolog.Logger {
	level, _ := zerolog.ParseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	var base zerolog.Logger
	if strings.ToLower(cfg.Format) == "console" || dev {
		out := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
		base = zerolog.New(out).With().Timestamp().Logger()
	} else {
		base = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	if cfg.Sampling && !dev {
		// Simple sampling: keep first 100, then 1 every 100 thereafter.
		sampled := base.Sample(&zerolog.BasicSampler{N: 100})
		return &sampled
	}
	return &base
}

// With attaches common context fields such as trace_id, user_id, tg_id etc.
type ctxKey string

const (
	ctxTraceID ctxKey = "trace_id"
	ctxUserID  ctxKey = "user_id"
	ctxTgID    ctxKey = "tg_id"
	ctxSessID  ctxKey = "session_id"
)

func With(ctx context.Context, base *zerolog.Logger, extra ...zerolog.Context) *zerolog.Logger {
	l := base.With()
	if v := ctx.Value(ctxTraceID); v != nil {
		l = l.Str("trace_id", v.(string))
	}
	if v := ctx.Value(ctxUserID); v != nil {
		l = l.Str("user_id", v.(string))
	}
	if v := ctx.Value(ctxTgID); v != nil {
		l = l.Int64("tg_id", v.(int64))
	}
	if v := ctx.Value(ctxSessID); v != nil {
		l = l.Str("session_id", v.(string))
	}
	logger := l.Logger()
	return &logger
}

// TraceDuration logs start and end with elapsed duration at TRACE level.
// Usage: defer logging.TraceDuration(logger, "ChatUC.SendMessage")()
func TraceDuration(logger *zerolog.Logger, name string) func() {
	start := time.Now()
	logger.Trace().Str("method", name).Msg("start")
	return func() {
		elapsed := time.Since(start)
		logger.Trace().Str("method", name).Dur("duration", elapsed).Msg("finish")
	}
}

// Redact hides PII when not in dev; keep short/preview.
func Redact(s string, dev bool) string {
	if dev {
		return s
	}
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "..." + s[len(s)-2:]
}

// Helpers to put IDs into context.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxTraceID, id)
}
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxUserID, id)
}
func WithTgID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, ctxTgID, id)
}
func WithSessID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxSessID, id)
}

// Expose global (optional). Prefer injection where possible.
var Global = log.Logger
