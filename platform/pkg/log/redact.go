// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package log

import (
	"context"
	"log/slog"
	"strings"
)

// redactedPlaceholder sustituye a cualquier valor considerado sensible.
const redactedPlaceholder = "[REDACTED]"

// defaultSensitiveKeys son claves de atributo cuyo VALOR se redacta siempre
// (secretos y PII). La comparación es case-insensitive y por subcadena, de modo
// que "user_password" o "auth_token" también se capturan.
var defaultSensitiveKeys = []string{
	"password", "passwd", "secret", "token", "authorization",
	"cookie", "set-cookie", "api_key", "apikey", "private_key",
	"access_token", "refresh_token", "id_token", "client_secret",
	"email", "phone", "ssn", "dni", "nif", "credit_card", "card_number",
	"session", "csrf",
}

// RedactOptions configura el handler de redacción.
type RedactOptions struct {
	// ExtraKeys añade claves sensibles a las de por defecto.
	ExtraKeys []string
	// DisableDefaults elimina la lista por defecto (solo se usan ExtraKeys).
	DisableDefaults bool
}

// redactHandler envuelve otro slog.Handler y redacta los valores de atributos
// cuya clave case-insensitive contenga una subcadena sensible. Preserva la
// correlación (no toca request_id/trace_id/sub/tenant) pero garantiza que
// ningún secreto/PII llegue al sink (RGPD art. 25 / secretos fuera de logs).
type redactHandler struct {
	inner slog.Handler
	keys  []string
}

// NewRedactingHandler construye un handler que redacta valores sensibles.
func NewRedactingHandler(inner slog.Handler, opts RedactOptions) slog.Handler {
	keys := make([]string, 0, len(defaultSensitiveKeys)+len(opts.ExtraKeys))
	if !opts.DisableDefaults {
		keys = append(keys, defaultSensitiveKeys...)
	}
	for _, k := range opts.ExtraKeys {
		keys = append(keys, strings.ToLower(k))
	}
	return &redactHandler{inner: inner, keys: keys}
}

func (h *redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *redactHandler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(h.redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, nr)
}

func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	red := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		red[i] = h.redactAttr(a)
	}
	return &redactHandler{inner: h.inner.WithAttrs(red), keys: h.keys}
}

func (h *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{inner: h.inner.WithGroup(name), keys: h.keys}
}

// redactAttr redacta el atributo si su clave es sensible; desciende en grupos.
func (h *redactHandler) redactAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindGroup {
		grp := a.Value.Group()
		out := make([]slog.Attr, len(grp))
		for i, ga := range grp {
			out[i] = h.redactAttr(ga)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	}
	if h.isSensitive(a.Key) {
		return slog.String(a.Key, redactedPlaceholder)
	}
	return a
}

func (h *redactHandler) isSensitive(key string) bool {
	lk := strings.ToLower(key)
	for _, s := range h.keys {
		if strings.Contains(lk, s) {
			return true
		}
	}
	return false
}
