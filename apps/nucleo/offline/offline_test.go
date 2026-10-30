// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package offline

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func abrir(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "offline")
	s, err := OpenWithSecrets(dir, new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func TestStore(t *testing.T) {
	s, _ := abrir(t)
	type borrador struct{ Asunto string }
	var b borrador

	if ok, err := s.Read("ada", "borradores", "1", &b); ok || err != nil {
		t.Fatalf("lectura en vacío: %v, %v", ok, err)
	}
	for _, id := range []string{"1", "2", "3"} {
		if err := s.Write("ada", "borradores", id, borrador{"asunto " + id}); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := s.Read("ada", "borradores", "2", &b); !ok || err != nil || b.Asunto != "asunto 2" {
		t.Fatalf("Read = %+v, %v, %v", b, ok, err)
	}
	// Lo de Ada no lo ve Grace, ni otro espacio de Ada.
	if ok, _ := s.Read("grace", "borradores", "2", &b); ok {
		t.Fatal("una cuenta lee lo de otra")
	}
	if l, _ := s.List("ada", "notas"); len(l) != 0 {
		t.Fatalf("otro espacio ve %d valores", len(l))
	}

	// Mutate: transforma, y si la función falla no toca nada.
	err := s.Mutate("ada", "borradores", "1", func(prev json.RawMessage) (any, error) {
		var p borrador
		_ = json.Unmarshal(prev, &p)
		return borrador{p.Asunto + " (editado)"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	quieto := errors.New("me arrepiento")
	if err := s.Mutate("ada", "borradores", "1", func(json.RawMessage) (any, error) { return borrador{"ROTO"}, quieto }); !errors.Is(err, quieto) {
		t.Fatal(err)
	}
	if _, _ = s.Read("ada", "borradores", "1", &b); b.Asunto != "asunto 1 (editado)" {
		t.Fatalf("tras un Mutate fallido queda %q", b.Asunto)
	}

	// List: lo último tocado, primero.
	l, err := s.List("ada", "borradores")
	if err != nil || len(l) != 3 || l[0].ID != "1" {
		t.Fatalf("List = %+v, %v", l, err)
	}

	if err := s.Delete("ada", "borradores", "3"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("ada", "borradores", "no-existe"); err != nil {
		t.Fatalf("borrar lo que no está: %v", err)
	}
	if l, _ := s.List("ada", "borradores"); len(l) != 2 {
		t.Fatalf("tras borrar quedan %d", len(l))
	}

	// Cerrar sesión se lo lleva todo, y el almacén sigue sirviendo.
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.List("ada", "borradores"); len(l) != 0 {
		t.Fatalf("tras Clear quedan %d", len(l))
	}
	if err := s.Write("ada", "borradores", "4", borrador{"sigo vivo"}); err != nil {
		t.Fatal(err)
	}
}

func TestStoreNoSeSaleDeSuDirectorio(t *testing.T) {
	s, dir := abrir(t)
	malo := "../../../../fuera"
	if err := s.Write(malo, malo, malo, "x"); err != nil {
		t.Fatal(err)
	}
	var fuera []string
	_ = filepath.WalkDir(filepath.Dir(dir), func(p string, d os.DirEntry, _ error) error {
		if !d.IsDir() && !strings.HasPrefix(p, dir+string(filepath.Separator)) {
			fuera = append(fuera, p)
		}
		return nil
	})
	if len(fuera) != 0 {
		t.Fatalf("se ha escrito fuera del almacén: %v", fuera)
	}
	var v string
	if ok, _ := s.Read(malo, malo, malo, &v); !ok || v != "x" {
		t.Fatal("no se recupera lo guardado con un id hostil")
	}
}

// red es un servidor que se puede «desenchufar».
type red struct {
	http.RoundTripper
	caida bool
}

func (r *red) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.caida {
		return nil, errors.New("dial tcp: no route to host")
	}
	return r.RoundTripper.RoundTrip(req)
}

func TestTransportRedPrimero(t *testing.T) {
	peticiones := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peticiones++
		if r.URL.Path == "/mail/api/roto" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, "respuesta de "+r.URL.RequestURI())
	}))
	defer srv.Close()

	s, _ := abrir(t)
	owner := "ada"
	r := &red{RoundTripper: http.DefaultTransport}
	var cambios []State
	m := &Monitor{}
	m.Subscribe(func(e State) { cambios = append(cambios, e) })
	c := &http.Client{Transport: &Transport{
		Base: r, Store: s, Owner: func() string { return owner }, Product: "mail", Monitor: m,
		Private: []string{"/api/v1/secretos"},
	}}
	pedir := func(metodo, ruta string, cabeceras ...string) (string, string, error) {
		t.Helper()
		req, _ := http.NewRequestWithContext(context.Background(), metodo, srv.URL+ruta, http.NoBody)
		for i := 0; i+1 < len(cabeceras); i += 2 {
			req.Header.Set(cabeceras[i], cabeceras[i+1])
		}
		resp, err := c.Do(req)
		if err != nil {
			return "", "", err
		}
		defer func() { _ = resp.Body.Close() }()
		cuerpo, _ := io.ReadAll(resp.Body)
		return string(cuerpo), resp.Header.Get("X-Sytue-Offline"), nil
	}

	const lista = "/mail/api/messages?folder=inbox"
	rutas := map[string]bool{ // ruta → ¿se tiene que poder leer sin red?
		lista:                            true,
		"/mail/api/content/42?offline=1": true,  // fichero pedido para offline, con tamaño
		"/mail/api/content/42":           false, // fichero sin pedirlo
		"/mail/api/attachments?index=0":  false,
		"/mail/auth/session":             false, // privado, con prefijo de app
		"/api/token":                     false, // privado, sin prefijo
		"/mail/api/ai/resumen":           false,
		"/mail/api/v1/secretos/1":        false, // privado de este servicio
		"/mail/api/roto":                 false, // un 500 no se guarda
	}
	for ruta := range rutas {
		if _, marca, err := pedir(http.MethodGet, ruta); (err != nil && ruta != "/mail/api/roto") || marca != "" {
			t.Fatalf("con red, %s: %v (marca %q)", ruta, err, marca)
		}
	}
	_, _, _ = pedir(http.MethodGet, "/mail/api/parcial", "Range", "bytes=0-9")
	_, _, _ = pedir(http.MethodGet, "/mail/api/descarga", "Cache-Control", "no-store")
	_, _, _ = pedir(http.MethodPost, "/mail/api/messages")

	r.caida = true
	for ruta, quiero := range rutas {
		cuerpo, marca, err := pedir(http.MethodGet, ruta)
		if got := err == nil; got != quiero {
			t.Errorf("sin red, %s: servida=%v, quiero %v", ruta, got, quiero)
		}
		if err == nil && (marca != "cache" || cuerpo != "respuesta de "+ruta) {
			t.Errorf("sin red, %s: cuerpo %q, marca %q", ruta, cuerpo, marca)
		}
	}
	for _, c := range [][]string{{http.MethodGet, "/mail/api/parcial"}, {http.MethodGet, "/mail/api/descarga"}, {http.MethodPost, "/mail/api/messages"}} {
		if _, _, err := pedir(c[0], c[1]); err == nil {
			t.Errorf("sin red, %s %s se ha servido de la copia", c[0], c[1])
		}
	}
	// Otra cuenta en el mismo equipo no hereda la copia.
	owner = "grace"
	if _, _, err := pedir(http.MethodGet, lista); err == nil {
		t.Error("Grace lee sin red la bandeja de Ada")
	}
	// Y sin sesión, ni se guarda ni se sirve.
	owner = ""
	if _, _, err := pedir(http.MethodGet, lista); err == nil {
		t.Error("sin sesión se ha servido una copia")
	}
	owner = "ada"

	r.caida = false
	if _, marca, err := pedir(http.MethodGet, lista); err != nil || marca != "" {
		t.Fatalf("al volver la red: %v, marca %q", err, marca)
	}
	if len(cambios) != 2 || cambios[0] != Offline || cambios[1] != Online {
		t.Fatalf("cambios de conectividad = %v, quiero [Offline Online]", cambios)
	}
}

func TestCancelarNoEsQuedarseSinRed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer srv.Close()
	s, _ := abrir(t)
	m := &Monitor{}
	c := &http.Client{Transport: &Transport{Store: s, Owner: func() string { return "ada" }, Monitor: m}}
	resp, err := c.Get(srv.URL + "/api/x")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/x", http.NoBody)
	if resp, err := c.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Fatal("una petición cancelada ha devuelto la copia")
	}
	if m.State() != Online {
		t.Fatal("cancelar una petición ha marcado la app como sin conexión")
	}
}

// Las listas de prefijos están copiadas de worker.ts. Si allí cambian, esto lo dice.
func TestLasReglasSonLasDelServiceWorker(t *testing.T) {
	worker := webref.Read(t, "apps/frontend/packages/offline/src/worker.ts")
	for _, p := range append(append([]string{}, prefijosPrivados...), prefijosDeFichero...) {
		if !strings.Contains(worker, "'"+p+"'") {
			t.Errorf("worker.ts ya no tiene el prefijo %q", p)
		}
	}
	for _, literal := range []string{
		"const MAX_CACHE_BYTES = 50 * 1024 * 1024;",
		`/^\/[a-z-]+(?=\/(?:auth|api|_astro)\/)/`,
		"PRIVATE_PREFIXES = [\n  '/auth/',\n  '/api/token',\n  '/api/session',\n  '/api/intent',\n  '/api/presign',\n  '/api/ws-ticket',\n  '/api/collab',\n  '/api/call',\n  '/api/conference',\n  '/api/ai',\n];",
	} {
		if !strings.Contains(worker, literal) {
			t.Errorf("worker.ts ya no dice %q: revisar transport.go", literal)
		}
	}
	for ruta, quiero := range map[string]string{
		"/mail/api/x": "/api/x", "/mail/auth/token": "/auth/token", "/api/x": "/api/x",
		"/docs/edit": "/docs/edit", "/cloud-dashboard/_astro/a.js": "/_astro/a.js",
	} {
		if got := sinBase(ruta); got != quiero {
			t.Errorf("sinBase(%q) = %q, quiero %q", ruta, got, quiero)
		}
	}
}

// TestContarVeTodosLosDuenos: lo que se perdería al vaciar el almacén, de cualquier cuenta.
func TestContarVeTodosLosDuenos(t *testing.T) {
	s, err := OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.Count("notes-capturas"); err != nil || n != 0 {
		t.Fatalf("vacío: %d %v", n, err)
	}
	for _, e := range [][3]string{
		{"ada/acme", "notes-capturas", "c1"},
		{"ada/otra", "notes-capturas", "c2"},
		{"luis/acme", "notes-borradores", "b1"},
		{"ada/acme", espacioHTTP("notes"), "https://notes/1"},
	} {
		if err := s.Write(e[0], e[1], e[2], "x"); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.Count("notes-capturas", "notes-borradores"); err != nil || n != 3 {
		t.Fatalf("Count = %d, %v; quiero 3 (la caché no cuenta)", n, err)
	}
}
