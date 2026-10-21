// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	cl, err := newClient(&Config{API: srv.URL, Token: "scl_test", Output: "table"})
	if err != nil {
		t.Fatal(err)
	}
	return cl
}

func TestServicesRestoreSendsBackup(t *testing.T) {
	var gotPath, gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"restoring":true}`))
	}))
	defer srv.Close()

	err := servicesRestore([]string{"svc-123", "--backup", "20260720T030000Z.sql.gz", "--api", srv.URL, "--token", "scl_test"})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/services/svc-123/restore" {
		t.Errorf("got %s %s, want POST /api/v1/services/svc-123/restore", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"backup":"20260720T030000Z.sql.gz"`) {
		t.Errorf("backup no viaja en el cuerpo: %q", gotBody)
	}
}

func TestServicesRestoreRequiresBackup(t *testing.T) {
	err := servicesRestore([]string{"svc-123", "--api", "http://127.0.0.1:0", "--token", "scl_test"})
	if err == nil || !strings.Contains(err.Error(), "--backup") {
		t.Errorf("sin --backup debería fallar pidiéndolo, got %v", err)
	}
}

func TestServicesBackupsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/services/svc-9/backups" {
			t.Errorf("path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"backups":[{"key":"a.sql.gz","size":1024,"last_modified":"2026-07-20T03:00:00Z"}]}`))
	}))
	defer srv.Close()

	if err := servicesBackups([]string{"svc-9", "--api", srv.URL, "--token", "scl_test", "-o", "json"}); err != nil {
		t.Fatalf("backups: %v", err)
	}
}

func TestRequestVerbs(t *testing.T) {
	var gotMethod, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	cl := newTestClient(t, srv)

	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete} {
		var body any
		if m == http.MethodPost || m == http.MethodPut || m == http.MethodPatch {
			body = map[string]any{"k": "v"}
		}
		if _, err := cl.request(context.Background(), m, "/x", body); err != nil {
			t.Fatalf("%s: %v", m, err)
		}
		if gotMethod != m {
			t.Errorf("method: got %s want %s", gotMethod, m)
		}
		if gotAuth != "Bearer scl_test" {
			t.Errorf("auth header: got %q", gotAuth)
		}
		if body != nil && !strings.Contains(gotBody, `"k":"v"`) {
			t.Errorf("body not sent: %q", gotBody)
		}
	}
}

func TestRequest404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"not_found","message":"no existe"}}`))
	}))
	defer srv.Close()
	_, err := newTestClient(t, srv).request(context.Background(), http.MethodGet, "/x", nil)
	if err == nil || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "no existe") {
		t.Fatalf("esperado error tipado 404, got %v", err)
	}
}

func TestTypedErrorNoLeak(t *testing.T) {
	// Cuerpo no-contrato con un supuesto secreto: no debe aparecer en el error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"secret":"scl_supersecret"}`))
	}))
	defer srv.Close()
	_, err := newTestClient(t, srv).request(context.Background(), http.MethodGet, "/x", nil)
	if err == nil || strings.Contains(err.Error(), "scl_supersecret") {
		t.Fatalf("no debe volcar el cuerpo: %v", err)
	}
}

func TestNoToken(t *testing.T) {
	if _, err := newClient(&Config{API: "http://x", Token: "", Output: "table"}); err == nil {
		t.Fatal("esperado error por token ausente")
	}
}

func TestBadOutput(t *testing.T) {
	if _, err := newClient(&Config{API: "http://x", Token: "t", Output: "yaml"}); err == nil {
		t.Fatal("esperado error por --output invalido")
	}
}

func TestParseFlagsInterspersed(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	o := fs.String("o", "", "")
	// flag antes y después de posicionales
	pos, err := parseFlags(fs, []string{"id1", "-o", "json", "id2"})
	if err != nil {
		t.Fatal(err)
	}
	if *o != "json" {
		t.Errorf("flag tras posicional no parseada: %q", *o)
	}
	if len(pos) != 2 || pos[0] != "id1" || pos[1] != "id2" {
		t.Errorf("posicionales: %v", pos)
	}
}

func TestDispatchUnknown(t *testing.T) {
	if err := run([]string{"nope"}); err == nil {
		t.Fatal("esperado error por subcomando desconocido")
	}
	if err := run(nil); err == nil {
		t.Fatal("esperado error sin subcomando")
	}
	if err := run([]string{"version"}); err != nil {
		t.Fatalf("version debe salir 0: %v", err)
	}
}

func TestHoistGlobals(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"global antes de todo", []string{"-o", "json", "projects", "list"}, []string{"projects", "list", "-o", "json"}},
		{"global entre grupo y sub", []string{"projects", "-o", "json", "list"}, []string{"projects", "list", "-o", "json"}},
		{"token con valor", []string{"--token", "scl_x", "projects", "list"}, []string{"projects", "list", "--token", "scl_x"}},
		{"forma con =", []string{"--output=json", "projects", "list"}, []string{"projects", "list", "--output=json"}},
		{"no toca flags no globales", []string{"env", "set", "--project", "p", "K=V", "-o", "json"}, []string{"env", "set", "--project", "p", "K=V", "-o", "json"}},
		{"ya al final se mantiene", []string{"projects", "list", "-o", "json"}, []string{"projects", "list", "-o", "json"}},
	}
	for _, c := range cases {
		got := hoistGlobals(c.in)
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := renderJSON(&buf, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"a": 1`) {
		t.Errorf("json no reindentado: %q", buf.String())
	}
	// no-JSON (logs texto): se escribe tal cual
	buf.Reset()
	_ = renderJSON(&buf, []byte("plain log line"))
	if buf.String() != "plain log line" {
		t.Errorf("texto plano alterado: %q", buf.String())
	}
}

func TestRenderTable(t *testing.T) {
	var buf bytes.Buffer
	renderTable(&buf, []string{"ID", "NAME"}, [][]string{{"1", "web"}, {"22", "api"}})
	out := buf.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "web") || !strings.Contains(out, "api") {
		t.Errorf("tabla incompleta:\n%s", out)
	}
	if !strings.HasPrefix(out, "ID") {
		t.Errorf("cabecera ausente:\n%s", out)
	}
}

// El PATCH solo puede llevar lo que el usuario TECLEA. Si enviara todos los campos,
// los que no se han tocado viajarían en su valor cero y machacarían la configuración
// del proyecto: escalar instancias borraría el comando de build, por ejemplo.
func TestProjectsUpdateSoloEnviaLoTecleado(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"id":"p-1","slug":"web","instance_size":"pro","instance_count":3}`))
	}))
	defer srv.Close()

	err := projectsUpdate([]string{
		"p-1", "--instance-size", "pro", "--instances", "3",
		"--api", srv.URL, "--token", "scl_test",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/api/v1/projects/p-1" {
		t.Errorf("got %s %s, want PATCH /api/v1/projects/p-1", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"instance_size":"pro"`) || !strings.Contains(gotBody, `"instance_count":3`) {
		t.Errorf("el cambio no viaja en el cuerpo: %q", gotBody)
	}
	for _, campo := range []string{"build_command", "name", "repo_url", "app_port", "framework"} {
		if strings.Contains(gotBody, campo) {
			t.Errorf("viaja %q sin haberse tecleado: machacaría la configuración (%q)", campo, gotBody)
		}
	}
}

// Un flag tecleado con valor vacío SÍ debe viajar: es la forma de quitar un valor.
func TestProjectsUpdateVacioExplicitoViaja(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"id":"p-1"}`))
	}))
	defer srv.Close()

	if err := projectsUpdate([]string{
		"p-1", "--build-command", "", "--api", srv.URL, "--token", "scl_test",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(gotBody, `"build_command":""`) {
		t.Errorf("el vaciado explícito debe viajar: %q", gotBody)
	}
}

// Sin ningún cambio no se llama a la API: un PATCH vacío es una escritura inútil que
// además dejaría rastro en la auditoría del proyecto.
func TestProjectsUpdateSinCambiosNoLlama(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no debe llamarse a la API sin cambios")
	}))
	defer srv.Close()

	err := projectsUpdate([]string{"p-1", "--api", srv.URL, "--token", "scl_test"})
	if err == nil || !strings.Contains(err.Error(), "ningún cambio") {
		t.Errorf("debería quejarse de que no hay nada que cambiar, got %v", err)
	}
}

// Los verbos que llegaron con el panel tienen que llegar también aquí: la ruta y
// el método son el contrato, y equivocarlos da un 404 que se lee como «esto no
// existe en el servidor» cuando lo que no existe es la URL que compone el CLI.
func TestRutasDeLosVerbosNuevos(t *testing.T) {
	casos := []struct {
		nombre string
		fn     func(args []string) error
		args   []string
		method string
		path   string
	}{
		{
			"restart", projectsRestart,
			[]string{"p-1"},
			http.MethodPost, "/api/v1/projects/p-1/restart",
		},
		{
			"promote", deploymentsPromote,
			[]string{"d-1"},
			http.MethodPost, "/api/v1/deployments/d-1/promote",
		},
		{
			"run", servicesRun,
			[]string{"s-1"},
			http.MethodPost, "/api/v1/services/s-1/run",
		},
		{
			"download", servicesDownload,
			[]string{"s-1", "--backup", "x.sql.gz"},
			http.MethodGet, "/api/v1/services/s-1/backups/download",
		},
		{
			"env pull", envPull,
			[]string{"--project", "p-1"},
			http.MethodGet, "/api/v1/projects/p-1/env/export",
		},
		{
			"db-url", projectsDbURL,
			[]string{"p-1"},
			http.MethodGet, "/api/v1/projects/p-1/db/credentials",
		},
		{
			"clone", projectsClone,
			[]string{"p-1", "--slug", "web-staging"},
			http.MethodPost, "/api/v1/projects/p-1/clone",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var gotPath, gotMethod string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotMethod = r.URL.Path, r.Method
				_, _ = w.Write([]byte(`{"restarted":2,"url":"https://x","project":{"id":"p-2"},"env_vars_copied":3}`))
			}))
			defer srv.Close()

			args := append(append([]string{}, c.args...), "--api", srv.URL, "--token", "scl_test")
			if err := c.fn(args); err != nil {
				t.Fatalf("%s: %v", c.nombre, err)
			}
			if gotMethod != c.method || gotPath != c.path {
				t.Errorf("got %s %s, want %s %s", gotMethod, gotPath, c.method, c.path)
			}
		})
	}
}

// Un `clone` sin destino no puede inventarse un slug: crearía un proyecto con
// nombre aleatorio que nadie pidió y que además cuenta para la cuota.
func TestCloneExigeSlug(t *testing.T) {
	err := projectsClone([]string{"p-1", "--api", "http://127.0.0.1:0", "--token", "scl_test"})
	if err == nil || !strings.Contains(err.Error(), "--slug") {
		t.Errorf("sin --slug debería fallar pidiéndolo, got %v", err)
	}
}
