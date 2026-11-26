// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// slackAcciones es un Slack de mentira para crear, abrir directos, invitar, explorar y
// los ficheros (el camino de subida de 2024).
type slackAcciones struct {
	mu       sync.Mutex
	srv      *httptest.Server
	llamadas map[string][]map[string]string
	subido   []byte
}

func nuevoSlackAcciones(t *testing.T) (*slackAcciones, *Slack) {
	f := &slackAcciones{llamadas: map[string][]map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.servir))
	t.Cleanup(f.srv.Close)
	old := APISlack
	APISlack = f.srv.URL + "/api"
	t.Cleanup(func() { APISlack = old })
	return f, NuevaSlack("xoxp-bueno", "", f.srv.Client())
}

func (f *slackAcciones) servir(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer xoxp-bueno" && r.URL.Path != "/subir" {
		escribir(w, map[string]any{"ok": false, "error": "invalid_auth"})
		return
	}
	switch r.URL.Path {
	case "/subir":
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.subido = raw
		f.mu.Unlock()
		_, _ = io.WriteString(w, "OK")
		return
	case "/bajar/acta.pdf":
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "contenido del acta")
		return
	case "/bajar/sin-permiso":
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>entra</html>")
		return
	}
	_ = r.ParseForm()
	metodo := strings.TrimPrefix(r.URL.Path, "/api/")
	form := map[string]string{}
	for k := range r.Form {
		form[k] = r.Form.Get(k)
	}
	f.mu.Lock()
	f.llamadas[metodo] = append(f.llamadas[metodo], form)
	f.mu.Unlock()
	switch metodo {
	case "auth.test":
		escribir(w, map[string]any{"ok": true, "user": "ana", "user_id": "U1", "team": "Acme"})
	case "users.info":
		escribir(w, map[string]any{"ok": true, "user": map[string]any{"name": "bea", "profile": map[string]string{"display_name": "Bea"}}})
	case "conversations.create":
		if form["name"] != "plan-2027" {
			escribir(w, map[string]any{"ok": false, "error": "invalid_name"})
			return
		}
		escribir(w, map[string]any{"ok": true, "channel": map[string]any{"id": "C9", "name": form["name"], "is_private": form["is_private"] == "true"}})
	case "conversations.setTopic":
		escribir(w, map[string]any{"ok": true})
	case "conversations.open":
		escribir(w, map[string]any{"ok": true, "channel": map[string]any{"id": "D2", "is_im": true, "user": form["users"]}})
	case "users.list":
		escribir(w, map[string]any{"ok": true, "members": []map[string]any{
			{"id": "U1", "name": "ana"},
			{"id": "U2", "name": "bea", "real_name": "Bea Gil"},
			{"id": "U3", "name": "robot", "is_bot": true},
			{"id": "U4", "name": "ex", "deleted": true},
		}})
	case "users.lookupByEmail":
		if form["email"] == "bea@acme.test" {
			escribir(w, map[string]any{"ok": true, "user": map[string]any{"id": "U2", "name": "bea", "real_name": "Bea Gil"}})
			return
		}
		escribir(w, map[string]any{"ok": false, "error": "users_not_found"})
	case "conversations.invite":
		escribir(w, map[string]any{"ok": true})
	case "conversations.list":
		escribir(w, map[string]any{"ok": true, "channels": []map[string]any{
			{"id": "C1", "name": "general", "is_member": true},
			{"id": "C5", "name": "ventas", "purpose": map[string]string{"value": "clientes"}},
			{"id": "C6", "name": "random"},
		}})
	case "conversations.join":
		escribir(w, map[string]any{"ok": true, "channel": map[string]any{"id": form["channel"], "name": "ventas"}})
	case "files.getUploadURLExternal":
		escribir(w, map[string]any{"ok": true, "upload_url": f.srv.URL + "/subir", "file_id": "F1"})
	case "files.completeUploadExternal":
		escribir(w, map[string]any{"ok": true})
	case "files.info":
		escribir(w, map[string]any{"ok": true, "file": map[string]any{
			"id": form["file"], "name": "acta.pdf",
			"url_private_download": f.srv.URL + "/bajar/acta.pdf",
			"shares":               map[string]any{"public": map[string]any{"C1": []map[string]string{{"ts": "1700000500.000100"}}}},
		}})
	case "conversations.history":
		escribir(w, map[string]any{"ok": true, "messages": []map[string]any{{
			"type": "message", "subtype": "file_share", "user": "U1", "text": "el acta", "ts": "1700000500.000100",
			"files": []map[string]any{{
				"id": "F1", "name": "acta.pdf", "mimetype": "application/pdf", "size": 18,
				"url_private_download": f.srv.URL + "/bajar/acta.pdf",
			}},
		}}})
	default:
		escribir(w, map[string]any{"ok": false, "error": "unknown_method"})
	}
}

func TestSlackCrearDirectoPersonasYExplorar(t *testing.T) {
	ctx := context.Background()
	f, s := nuevoSlackAcciones(t)
	c, err := s.CrearCanal(ctx, "Plan 2027", "Lo que viene", true)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "C9" || c.Kind != "private" || c.Topic != "Lo que viene" {
		t.Fatalf("canal: %+v", c)
	}
	d, err := s.AbrirDirecto(ctx, []string{"U2"})
	if err != nil || d.ID != "D2" || d.Kind != "direct" || d.Name != "Bea" {
		t.Fatalf("directo: %+v %v", d, err)
	}
	g, err := s.Gente(ctx)
	if err != nil || len(g) != 1 || g[0].Subject != "U2" || g[0].Name != "Bea Gil" {
		t.Fatalf("gente (sin bots, sin borrados, sin mí): %+v %v", g, err)
	}
	if p, err := s.Resolver(ctx, "bea@acme.test"); err != nil || p.UserID != "U2" {
		t.Fatalf("por correo: %+v %v", p, err)
	}
	if _, err := s.Resolver(ctx, "nadie@acme.test"); !errors.Is(err, ErrNoEncontrada) {
		t.Fatalf("correo desconocido: %v", err)
	}
	if p, err := s.Resolver(ctx, "@bea gil"); err != nil || p.UserID != "U2" {
		t.Fatalf("por nombre: %+v %v", p, err)
	}
	if _, err := s.Anadir(ctx, "C9", "U2"); err != nil || f.llamadas["conversations.invite"][0]["users"] != "U2" {
		t.Fatalf("invitar: %v %v", err, f.llamadas["conversations.invite"])
	}
	cs, err := s.Explorar(ctx, "client")
	if err != nil || len(cs) != 1 || cs[0].ID != "C5" {
		t.Fatalf("explorar: %+v %v", cs, err)
	}
	if u, err := s.Unirse(ctx, "C5"); err != nil || u.ID != "C5" {
		t.Fatalf("unirse: %+v %v", u, err)
	}
}

func TestSlackFicherosIdaYVuelta(t *testing.T) {
	ctx := context.Background()
	f, s := nuevoSlackAcciones(t)
	fs := []Fichero{{Nombre: "acta.pdf", Tipo: "application/pdf", Tamano: 18, Abrir: func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("contenido del acta")), nil
	}}}
	m, err := s.EnviarFicheros(ctx, "C1", "", "el acta", fs)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "1700000500.000100" || m.Body != "el acta" || len(m.Attachments) != 1 || m.Attachments[0].FileID != "F1" {
		t.Fatalf("mensaje del fichero: %+v", m)
	}
	if string(f.subido) != "contenido del acta" {
		t.Fatalf("subido: %q", f.subido)
	}
	comp := f.llamadas["files.completeUploadExternal"][0]
	var lista []map[string]string
	_ = json.Unmarshal([]byte(comp["files"]), &lista)
	if comp["channel_id"] != "C1" || comp["initial_comment"] != "el acta" || len(lista) != 1 || lista[0]["id"] != "F1" {
		t.Fatalf("completeUploadExternal: %v", comp)
	}
	rc, err := s.Bajar(ctx, "C1", "F1", false)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(rc)
	rc.Close()
	if string(raw) != "contenido del acta" {
		t.Fatalf("bajado: %q", raw)
	}
	// Sin `files:read`, Slack contesta con su página de entrada: eso no es el fichero.
	s.mu.Lock()
	s.ficheros["F2"] = ficheroSlack{ID: "F2", Mimetype: "application/pdf", URLDescarga: f.srv.URL + "/bajar/sin-permiso"}
	s.mu.Unlock()
	if _, err := s.Bajar(ctx, "C1", "F2", false); err == nil {
		t.Fatal("una página HTML no es el PDF")
	}
	if NombreCanalSlack("  Mi Canal.Nuevo  ") != "mi-canal-nuevo" {
		t.Fatal(NombreCanalSlack("  Mi Canal.Nuevo  "))
	}
}
