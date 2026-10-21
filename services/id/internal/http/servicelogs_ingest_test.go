// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// El store se deja a nil A PROPÓSITO: todos estos casos tienen que resolverse
// ANTES de tocar la base. Si alguno dejara de filtrar, el test no falla con un
// contador distinto sino con un pánico, que es imposible de leer como aprobado.
func postLogs(t *testing.T, s *Server, token, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/service-logs", strings.NewReader(cuerpo))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	s.handleIngestServiceLogs(rr, r)
	return rr
}

func TestIngestServiceLogsSinConfigurar(t *testing.T) {
	s := &Server{cfg: Config{LogIngestToken: ""}}
	if rr := postLogs(t, s, "loquesea", `[]`); rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d; quería 503 (recolector no configurado)", rr.Code)
	}
}

func TestIngestServiceLogsTokenInvalido(t *testing.T) {
	s := &Server{cfg: Config{LogIngestToken: "secreto"}}
	for _, tok := range []string{"", "otro", "secret"} {
		if rr := postLogs(t, s, tok, `[]`); rr.Code != http.StatusUnauthorized {
			t.Fatalf("token %q: status=%d; quería 401", tok, rr.Code)
		}
	}
}

// Un lote de INFO se acepta y no guarda nada: es el 99 % del volumen que el
// capturador existe para NO guardar, y bastaría un servicio mal configurado
// para llenar con él la tabla de diagnóstico de la plataforma.
func TestIngestServiceLogsFiltraNivelYServicio(t *testing.T) {
	s := &Server{cfg: Config{LogIngestToken: "secreto"}}
	cuerpo := `[
		{"at":"2026-08-15T10:00:00Z","level":"INFO","service":"drive","message":"ruido"},
		{"at":"2026-08-15T10:00:00Z","level":"DEBUG","service":"drive","message":"ruido"},
		{"at":"2026-08-15T10:00:00Z","level":"ERROR","service":"","message":"sin servicio"}
	]`
	if rr := postLogs(t, s, "secreto", cuerpo); rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d; quería 204 sin tocar el store", rr.Code)
	}
}

func TestIngestServiceLogsLoteEnorme(t *testing.T) {
	s := &Server{cfg: Config{LogIngestToken: "secreto"}}
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < ingestaMaxEntradas+1; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"level":"INFO","service":"drive","message":"x"}`)
	}
	b.WriteString("]")
	if rr := postLogs(t, s, "secreto", b.String()); rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d; quería 400 (lote por encima del tope)", rr.Code)
	}
}

func TestIngestServiceLogsCuerpoRoto(t *testing.T) {
	s := &Server{cfg: Config{LogIngestToken: "secreto"}}
	if rr := postLogs(t, s, "secreto", `{"no":"es un array"}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d; quería 400", rr.Code)
	}
}
