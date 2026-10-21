// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate_test

// Tests OFFLINE del DESTINO real (*migrate.Sytue) contra un Sytue Drive simulado:
// comprueban EN QUÉ CUENTA aterriza el contenido mirando la cabecera Authorization
// que recibe el destino, y que un token renovado a mitad de trabajo se usa desde
// ese momento (el token de impersonación dura 5 minutos; el trabajo, más).

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
)

// errNoPermiso simula el fallo del canje (persona dada de baja, organización sin
// delegación aprovisionada).
var errNoPermiso = errors.New("sin permiso para escribir en la cuenta de grace@acme.test")

// sytueDrive es un Sytue Drive mínimo que EXIGE un token del usuario esperado:
// cualquier otro (el del admin, uno caducado) se rechaza con 401, igual que haría
// el servicio real al verificar el `sub` del JWT. Registra los tokens vistos.
type sytueDrive struct {
	wantUser string

	mu     sync.Mutex
	tokens []string
	files  int
}

func (s *sytueDrive) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		s.mu.Lock()
		s.tokens = append(s.tokens, tok)
		s.mu.Unlock()
		// El token debe ser de la persona destino. Es lo que sustituye a mirar el
		// `sub` del JWT: el simulado no verifica firmas, pero sí de quién es.
		if !strings.HasPrefix(tok, "imp:"+s.wantUser+":") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/v1/folders/root":
			writeJSON(w, map[string]any{"id": "root"})
		case r.URL.Path == "/api/v1/folders":
			writeJSON(w, map[string]any{"id": "fld-1"})
		case strings.HasSuffix(r.URL.Path, "/files"):
			s.mu.Lock()
			s.files++
			id := s.files
			s.mu.Unlock()
			writeJSON(w, map[string]any{"id": "fil-" + strconv.Itoa(id)})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func (s *sytueDrive) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.tokens...)
}

// TestEscribeConElTokenDelUsuarioObjetivo es el contrato de esta ola: la migración
// escribe con el permiso de la persona destino, no con el del admin. Y renueva a
// mitad de trabajo sin perder el hilo.
func TestEscribeConElTokenDelUsuarioObjetivo(t *testing.T) {
	allowLoopback(t)
	src, _ := graphServer(t)

	dest := &sytueDrive{wantUser: "grace@acme.test"}
	sytue := httptest.NewServer(dest.handler())
	t.Cleanup(sytue.Close)

	// Fuente de token que CADUCA: a partir de la cuarta petición entrega el token
	// renovado. Modela lo que hace directory.Delegation con el reloj real.
	calls := 0
	token := func(context.Context) (string, error) {
		calls++
		if calls > 3 {
			return "imp:grace@acme.test:2", nil
		}
		return "imp:grace@acme.test:1", nil
	}

	j := job("job-1", domain.SourceM365, domain.DomainDrive)
	st, _, deps := newRun(t, m365Instance(src.URL), j)
	deps.Dst = migrate.NewSytue(sytue.URL, sytue.URL, token, nil)

	if err := migrate.Run(context.Background(), deps, j); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := st.get("job-1")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != domain.MigrationDone {
		t.Fatalf("estado final = %q (%s)", got.Status, got.LastError)
	}

	seen := dest.seen()
	if len(seen) < 4 {
		t.Fatalf("el destino solo recibió %d peticiones: %v", len(seen), seen)
	}
	for i, tok := range seen {
		if !strings.HasPrefix(tok, "imp:grace@acme.test:") {
			t.Fatalf("la petición %d escribió con un token que no es del usuario objetivo: %q", i, tok)
		}
	}
	// El token se pide EN CADA petición, así que la renovación se usa de verdad.
	if seen[0] != "imp:grace@acme.test:1" {
		t.Fatalf("la primera escritura no usó el token inicial: %q", seen[0])
	}
	if seen[len(seen)-1] != "imp:grace@acme.test:2" {
		t.Fatalf("el token renovado no llegó a usarse (última petición: %q)", seen[len(seen)-1])
	}
}

// TestSinPermisoNoSeEscribeNada: si no se puede minar el permiso para esa persona,
// la migración no escribe NADA en ninguna cuenta (ni cae de vuelta a la del admin).
func TestSinPermisoNoSeEscribeNada(t *testing.T) {
	allowLoopback(t)
	src, _ := graphServer(t)

	dest := &sytueDrive{wantUser: "grace@acme.test"}
	sytue := httptest.NewServer(dest.handler())
	t.Cleanup(sytue.Close)

	failing := func(context.Context) (string, error) {
		return "", errNoPermiso
	}
	j := job("job-1", domain.SourceM365, domain.DomainDrive)
	st, _, deps := newRun(t, m365Instance(src.URL), j)
	deps.Dst = migrate.NewSytue(sytue.URL, sytue.URL, failing, nil)

	if err := migrate.Run(context.Background(), deps, j); err == nil {
		t.Fatal("el trabajo terminó bien sin permiso para escribir")
	}
	if len(dest.seen()) != 0 {
		t.Fatalf("se escribió en el destino sin permiso: %v", dest.seen())
	}
	got, _ := st.get("job-1")
	if got.Status != domain.MigrationError || !strings.Contains(got.LastError, "sin permiso") {
		t.Fatalf("el trabajo no quedó en error con motivo legible: %q / %q", got.Status, got.LastError)
	}
}

// TestStaticTokenSigueSirviendo: el atajo sin impersonación (un único Bearer) se
// mantiene para los tests y para cualquier destino que no la necesite.
func TestStaticTokenSigueSirviendo(t *testing.T) {
	var seen string
	sytue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		writeJSON(w, map[string]any{"id": "root"})
	}))
	t.Cleanup(sytue.Close)

	dst := migrate.NewSytue(sytue.URL, sytue.URL, migrate.StaticToken("tok-fijo"), nil)
	if _, err := dst.Root(context.Background()); err != nil {
		t.Fatalf("Root: %v", err)
	}
	if seen != "Bearer tok-fijo" {
		t.Fatalf("Authorization = %q", seen)
	}
}
