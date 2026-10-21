// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	"git.sytue.eu/sytue/one/platform/pkg/log"
)

type ctxKeyRequestID struct{}

// HeaderRequestID es la cabecera estándar de correlación de Sytue One.
const HeaderRequestID = "X-Request-Id"

// RequestIDFromContext devuelve el request id del contexto, o cadena vacía.
func RequestIDFromContext(ctx context.Context) string {
	if ctx != nil {
		if id, ok := ctx.Value(ctxKeyRequestID{}).(string); ok {
			return id
		}
	}
	return ""
}

// RequestID genera (o respeta el entrante) un identificador de petición, lo
// expone en el contexto y en la cabecera de respuesta.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		ctx := context.WithValue(r.Context(), ctxKeyRequestID{}, id)
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusRecorder captura el código de estado y los bytes escritos.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status = http.StatusOK
		s.wrote = true
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Flush implementa http.Flusher para soportar SSE/streaming.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap deja que http.ResponseController llegue al writer de verdad. Sin él,
// SetWriteDeadline devolvía ErrNotSupported a través del recorder, así que una
// respuesta larga —la entrega de una exportación— moría en el WriteTimeout del
// servidor por mucho que el handler pidiera quitarlo.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Hijack implementa http.Hijacker para soportar upgrades WebSocket (Sytue Chat
// /ws): sin esto, la librería WS no puede tomar la conexión y responde 501.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := s.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("httpx: el ResponseWriter subyacente no soporta Hijack")
}

// El recorder DEBE seguir soportando streaming (SSE) y upgrades (WebSocket).
var (
	_ http.Flusher  = (*statusRecorder)(nil)
	_ http.Hijacker = (*statusRecorder)(nil)
)

// RequestLogger inyecta un logger con request_id/método en el contexto y registra el
// resultado de cada petición.
//
// LA RUTA SE REGISTRA POR SU PATRÓN, NUNCA POR LA URL: `/api/v1/share/{token}` y no
// `/api/v1/share/9f3b…`. Una URL puede llevar una CREDENCIAL —el token de un enlace
// compartido, el de un álbum publicado, el de un WOPI— y el log de peticiones lo lee
// cualquiera que tenga acceso a los logs (ADR 0052). Mismo criterio que la etiqueta
// de las métricas (`rutaDe`); el patrón solo existe al volver de `next`.
func RequestLogger(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := RequestIDFromContext(r.Context())

			l := base.With(
				"request_id", reqID,
				"method", r.Method,
			)
			ctx := log.Into(r.Context(), l)
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r.WithContext(ctx))

			l.Info("http_request",
				"route", rutaDe(r),
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote", clientIP(r),
			)
		})
	}
}

// Recoverer captura panics, los registra con stack y responde 500 JSON.
//
//nolint:contextcheck // falso positivo: el defer usa r.Context(), el mismo de la petición.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// http.ErrAbortHandler es el mecanismo estándar para abortar
				// una respuesta: se re-lanza tal cual (net/http lo silencia).
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}
				log.From(r.Context()).Error("panic recuperado en handler HTTP",
					"panic", rec,
					"stack", string(debug.Stack()),
					"request_id", RequestIDFromContext(r.Context()),
				)
				Fail(w, r, ErrInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// clientIP extrae la IP del cliente respetando X-Forwarded-For (primer salto).
// clientIP resuelve de quién es la petición para el registro.
//
// LEÍA LA PRIMERA ENTRADA de X-Forwarded-For sin mirar quién conectaba, y esa es
// justo la que escribe el cliente: cualquiera podía envenenar la columna de IP
// del registro de TODOS los servicios afirmando la dirección que quisiera
// (auditoría del 2026-09-21, docs/security.md §6.3). Tampoco valía leer la
// última: con navegador → Traefik → BFF → servicio, la última es la del salto
// anterior, no la de la persona.
//
// Ahora lo resuelve `platform/pkg/clientip`, que solo cree la cabecera si quien
// CONECTA es un proxy de la lista y la recorre de derecha a izquierda. La lista
// viaja en el contexto; sin ella se creen las redes privadas, que es el
// despliegue real y nunca una red pública.
//
// `X-Real-Ip` ya no se mira: la escribe quien quiera igual que la otra, y quien
// la pone (nginx, Traefik) pone también X-Forwarded-For.
func clientIP(r *http.Request) string {
	return clientip.TrustedProxiesFrom(r.Context()).ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
}
