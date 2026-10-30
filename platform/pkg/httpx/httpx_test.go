// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewRouterRequestIDAndNotFound(t *testing.T) {
	r := NewRouter(RouterOptions{})
	req := httptest.NewRequest(http.MethodGet, "/missing", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get(HeaderRequestID) == "" {
		t.Fatal("falta cabecera request id")
	}
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("json: %v", err)
	}
	if env.Error.Code != "not_found" || env.RequestID == "" {
		t.Fatalf("envelope inesperado: %+v", env.Error)
	}
}

func TestRequestIDRespectsIncoming(t *testing.T) {
	r := NewRouter(RouterOptions{})
	r.Get("/x", func(w http.ResponseWriter, req *http.Request) {
		JSON(w, req, http.StatusOK, map[string]string{"id": RequestIDFromContext(req.Context())})
	})
	req := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	req.Header.Set(HeaderRequestID, "fixed-id")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Header().Get(HeaderRequestID) != "fixed-id" {
		t.Fatalf("request id no respetado: %q", rec.Header().Get(HeaderRequestID))
	}
}

func TestRecoverer(t *testing.T) {
	r := NewRouter(RouterOptions{})
	r.Get("/boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", http.NoBody))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestFailHidesCause(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	Fail(rec, req, ErrInternal.Wrap(errors.New("secreto de DB")))
	if strings.Contains(rec.Body.String(), "secreto") {
		t.Fatal("la causa subyacente no debe filtrarse al cliente")
	}
}

func TestDecodeJSON(t *testing.T) {
	type body struct {
		Name string `json:"name"`
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ok"}`))
	var b body
	if err := DecodeJSON(rec, req, &b); err != nil || b.Name != "ok" {
		t.Fatalf("decode válido falló: %v", err)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x","extra":1}`))
	if err := DecodeJSON(rec, req, &body{}); err == nil {
		t.Fatal("esperaba error por campo desconocido")
	}
}

func TestHealthAndReady(t *testing.T) {
	r := NewRouter(RouterOptions{})
	healthy := CheckerFunc{Label: "db", Fn: func(context.Context) error { return nil }}
	broken := CheckerFunc{Label: "s3", Fn: func(context.Context) error { return errors.New("down") }}

	MountHealth(r, "v1.2.3", time.Second, healthy)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz sano = %d", rec.Code)
	}

	r2 := NewRouter(RouterOptions{})
	MountHealth(r2, "v1", time.Second, healthy, broken)
	rec = httptest.NewRecorder()
	r2.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz con dependencia caída = %d", rec.Code)
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	r := NewRouter(RouterOptions{})
	r.Get("/healthz", HealthHandler("test"))
	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0", Handler: r, ShutdownTimeout: time.Second})
	// Direccionamiento dinámico no expuesto; probamos que Run respeta el ctx.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run devolvió error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run no terminó tras cancelar el contexto")
	}
}

// El commit exacto viaja en /healthz. Es lo que la consola de plataforma enseña
// para responder «qué está corriendo», y un tag de versión no basta: se puede
// mover a otro commit, y entonces la respuesta sería la de ayer.
func TestHealthzLlevaElCommit(t *testing.T) {
	original := buildCommit
	t.Cleanup(func() { buildCommit = original })

	// Sin commit horneado (imagen hecha a mano) la clave NO aparece: un campo
	// vacío se lee como «el commit es desconocido», que no es lo mismo que
	// «esta imagen no lo trae».
	buildCommit = ""
	rec := httptest.NewRecorder()
	HealthHandler("v1.2.3")(rec, httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody))
	if strings.Contains(rec.Body.String(), "commit") {
		t.Errorf("sin commit horneado la clave sobra: %s", rec.Body)
	}

	buildCommit = "0a1b2c3d4e5f60718293a4b5c6d7e8f901234567"
	rec = httptest.NewRecorder()
	HealthHandler("v1.2.3")(rec, httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody))
	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta ilegible: %v (%s)", err, rec.Body)
	}
	if body.Status != "ok" || body.Version != "v1.2.3" {
		t.Errorf("healthz = %+v", body)
	}
	if body.Commit != buildCommit {
		t.Errorf("commit = %q, quiero el completo: acortarlo aquí impide buscarlo en la forja", body.Commit)
	}
}

// La versión y el commit de /healthz no pueden salir a internet: le dicen a
// cualquiera qué fallos conocidos probar. Traefik añade X-Forwarded-For, así que
// esa cabecera es la que separa «viene de fuera» de «lo pregunta el kubelet o
// Sytue ID por la URL interna». Se comprobó contra producción que se estaba
// filtrando de verdad (v2.2.1 y su commit completo, sin autenticar).
func TestHealthNoFiltraLaVersionHaciaFuera(t *testing.T) {
	original := buildCommit
	t.Cleanup(func() { buildCommit = original })
	buildCommit = "0a1b2c3d4e5f60718293a4b5c6d7e8f901234567"

	deFuera := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
	deFuera.Header.Set("X-Forwarded-For", "203.0.113.7")
	rec := httptest.NewRecorder()
	HealthHandler("v1.2.3")(rec, deFuera)
	if rec.Code != http.StatusOK {
		t.Fatalf("la sonda tiene que seguir dando 200, dio %d", rec.Code)
	}
	if cuerpo := rec.Body.String(); strings.Contains(cuerpo, "v1.2.3") || strings.Contains(cuerpo, "commit") {
		t.Errorf("desde internet se filtró la versión o el commit: %s", cuerpo)
	}

	// Y de dentro sigue diciéndolo: es lo que consulta el panel de plataforma.
	rec = httptest.NewRecorder()
	HealthHandler("v1.2.3")(rec, httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody))
	cuerpo := rec.Body.String()
	if !strings.Contains(cuerpo, "v1.2.3") || !strings.Contains(cuerpo, "0a1b2c3d") {
		t.Errorf("desde dentro debe seguir informando de la versión: %s", cuerpo)
	}
}

// /readyz tampoco puede regalar hacia fuera el mapa de dependencias internas ni
// los errores del driver de PostgreSQL o S3 (INF-12): a través de Traefik
// (X-Forwarded-For) solo devuelve el estado general; sin esa cabecera, el
// kubelet y el diagnóstico interno siguen viendo el desglose por dependencia.
func TestReadyNoFiltraLasDependenciasHaciaFuera(t *testing.T) {
	healthy := CheckerFunc{Label: "postgres", Fn: func(context.Context) error { return nil }}
	broken := CheckerFunc{Label: "s3", Fn: func(context.Context) error { return errors.New("dial tcp 10.43.0.9:8333: i/o timeout") }}
	h := ReadyHandler(time.Second, healthy, broken)

	deFuera := httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody)
	deFuera.Header.Set("X-Forwarded-For", "203.0.113.7")
	rec := httptest.NewRecorder()
	h(rec, deFuera)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz con dependencia caída desde fuera = %d, quería 503", rec.Code)
	}
	if cuerpo := rec.Body.String(); strings.Contains(cuerpo, "dependencies") || strings.Contains(cuerpo, "postgres") || strings.Contains(cuerpo, "10.43.0.9") {
		t.Errorf("desde internet se filtraron las dependencias internas: %s", cuerpo)
	}

	deDentro := httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody)
	rec = httptest.NewRecorder()
	h(rec, deDentro)
	if cuerpo := rec.Body.String(); !strings.Contains(cuerpo, "dependencies") || !strings.Contains(cuerpo, "postgres") {
		t.Errorf("desde dentro debe seguir informando de las dependencias: %s", cuerpo)
	}
}
