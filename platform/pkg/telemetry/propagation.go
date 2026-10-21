// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package telemetry

import (
	"context"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// HeaderTraceParent es la cabecera W3C Trace Context (RFC estándar del W3C).
const HeaderTraceParent = "traceparent"

// InjectTraceParent escribe el traceparent del span activo en las cabeceras
// salientes (para propagar la traza a otros servicios de la suite).
func InjectTraceParent(ctx context.Context, h http.Header) {
	sc := SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return
	}
	flags := "00"
	if sc.Sampled {
		flags = "01"
	}
	h.Set(HeaderTraceParent, "00-"+sc.TraceID.String()+"-"+sc.SpanID.String()+"-"+flags)
}

// ExtractTraceParent lee el traceparent entrante y devuelve el SpanContext
// remoto (o el cero si falta/es inválido). Formato: version-traceid-spanid-flags.
func ExtractTraceParent(h http.Header) SpanContext {
	raw := h.Get(HeaderTraceParent)
	if raw == "" {
		return SpanContext{}
	}
	parts := strings.Split(raw, "-")
	if len(parts) != 4 || parts[0] != "00" {
		return SpanContext{}
	}
	tid, err := hex.DecodeString(parts[1])
	if err != nil || len(tid) != 16 {
		return SpanContext{}
	}
	sid, err := hex.DecodeString(parts[2])
	if err != nil || len(sid) != 8 {
		return SpanContext{}
	}
	var sc SpanContext
	copy(sc.TraceID[:], tid)
	copy(sc.SpanID[:], sid)
	sc.Sampled = len(parts[3]) == 2 && parts[3][1]&1 == 1
	sc.remote = true
	if sc.TraceID.IsZero() {
		return SpanContext{}
	}
	return sc
}

// Middleware extrae/crea la traza de cada petición, abre un span de servidor y
// correlaciona los logs (inyecta trace_id/span_id en el logger del contexto).
// Debe montarse tras el RequestLogger de httpx para que el logger exista.
func (p *Provider) Middleware(next http.Handler) http.Handler {
	tracer := p.Tracer("http.server")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if remote := ExtractTraceParent(r.Header); remote.IsValid() {
			ctx = ContextWithSpanContext(ctx, remote)
		}
		// El nombre y `http.route` son el PATRÓN de la ruta, no la URL: una URL lleva
		// secretos en la ruta (el token de un enlace público, `/public/{token}`) y
		// acabarían en el backend de trazas. El patrón solo se sabe tras enrutar.
		ctx, span := tracer.Start(ctx, r.Method, String("http.method", r.Method))
		defer span.End() //nolint:contextcheck // falso positivo: End() no recibe context por diseño (API estilo OTel).

		// Correlación de logs: enriquecer el logger del contexto.
		sc := span.SpanContext()
		ctx = withCorrelatedLogger(ctx, sc)
		next.ServeHTTP(w, r.WithContext(ctx))
		ruta := "desconocida"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			ruta = rc.RoutePattern()
		}
		span.name = r.Method + " " + ruta
		span.SetAttributes(String("http.route", ruta))
	})
}

// logIntoFunc permite a httpx/log inyectar su propio helper sin ciclo de import.
// Por defecto usa slog.Default vía context si está disponible.
var logIntoFunc func(ctx context.Context, attrs ...any) context.Context

// SetLogInjector conecta el helper de log del servicio (p. ej. log.With) para
// que Middleware correlacione trace_id/span_id en el logger del contexto.
func SetLogInjector(fn func(ctx context.Context, attrs ...any) context.Context) {
	logIntoFunc = fn
}

func withCorrelatedLogger(ctx context.Context, sc SpanContext) context.Context {
	if logIntoFunc == nil || !sc.IsValid() {
		return ctx
	}
	return logIntoFunc(ctx, "trace_id", sc.TraceID.String(), "span_id", sc.SpanID.String())
}

// LogAttrs devuelve los atributos de correlación para logging manual.
func LogAttrs(ctx context.Context) []slog.Attr {
	sc := SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return nil
	}
	return []slog.Attr{
		slog.String("trace_id", sc.TraceID.String()),
		slog.String("span_id", sc.SpanID.String()),
	}
}
