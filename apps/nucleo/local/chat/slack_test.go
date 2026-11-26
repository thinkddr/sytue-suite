// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// slackFalso es un Slack de mentira: unos pocos métodos de la Web API y Socket Mode.
type slackFalso struct {
	t        *testing.T
	mu       sync.Mutex
	enviados []string
	marcados []string
	srv      *httptest.Server
	acuses   chan string
}

func nuevoSlackFalso(t *testing.T) *slackFalso {
	f := &slackFalso{t: t, acuses: make(chan string, 4)}
	f.srv = httptest.NewServer(http.HandlerFunc(f.servir))
	t.Cleanup(f.srv.Close)
	old := APISlack
	APISlack = f.srv.URL + "/api"
	t.Cleanup(func() { APISlack = old })
	return f
}

func escribir(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

func (f *slackFalso) servir(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/socket" {
		f.socket(w, r)
		return
	}
	_ = r.ParseForm()
	auth := r.Header.Get("Authorization")
	metodo := strings.TrimPrefix(r.URL.Path, "/api/")
	if metodo == "apps.connections.open" {
		if auth != "Bearer xapp-1" {
			escribir(w, map[string]any{"ok": false, "error": "invalid_auth"})
			return
		}
		escribir(w, map[string]any{"ok": true, "url": "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/socket"})
		return
	}
	if auth != "Bearer xoxp-bueno" {
		escribir(w, map[string]any{"ok": false, "error": "invalid_auth"})
		return
	}
	switch metodo {
	case "auth.test":
		escribir(w, map[string]any{"ok": true, "user": "ana", "user_id": "U1", "team": "Acme"})
	case "users.conversations":
		escribir(w, map[string]any{"ok": true, "channels": []map[string]any{
			{"id": "C1", "name": "general", "created": 1700000000, "topic": map[string]string{"value": "todo"}},
			{"id": "D1", "is_im": true, "user": "U2"},
			{"id": "G1", "name": "secreto", "is_private": true},
		}})
	case "users.info":
		nombres := map[string]string{"U1": "Ana", "U2": "Bea"}
		escribir(w, map[string]any{"ok": true, "user": map[string]any{"name": r.Form.Get("user"), "profile": map[string]string{"display_name": nombres[r.Form.Get("user")]}}})
	case "conversations.history":
		if r.Form.Get("oldest") != "" {
			escribir(w, map[string]any{"ok": true, "messages": []map[string]any{
				{"type": "message", "user": "U2", "text": "nuevo", "ts": "1700000100.000300"},
			}})
			return
		}
		escribir(w, map[string]any{"ok": true, "messages": []map[string]any{ // el más nuevo primero
			{"type": "message", "user": "U1", "text": "respuesta", "ts": "1700000050.000200", "thread_ts": "1700000000.000100"},
			{
				"type": "message", "user": "U2", "text": "hola", "ts": "1700000000.000100", "reply_count": 1,
				"reactions": []map[string]any{{"name": "tada", "count": 1, "users": []string{"U1"}}},
			},
		}})
	case "conversations.replies":
		escribir(w, map[string]any{"ok": true, "messages": []map[string]any{
			{"type": "message", "user": "U2", "text": "hola", "ts": "1700000000.000100"},
			{"type": "message", "user": "U1", "text": "respuesta", "ts": "1700000050.000200", "thread_ts": "1700000000.000100"},
		}})
	case "chat.postMessage":
		f.mu.Lock()
		f.enviados = append(f.enviados, r.Form.Get("channel")+"|"+r.Form.Get("thread_ts")+"|"+r.Form.Get("text"))
		f.mu.Unlock()
		escribir(w, map[string]any{"ok": true, "ts": "1700000200.000100", "message": map[string]any{"type": "message", "user": "U1", "text": r.Form.Get("text"), "ts": "1700000200.000100"}})
	case "conversations.mark":
		f.mu.Lock()
		f.marcados = append(f.marcados, r.Form.Get("channel")+"|"+r.Form.Get("ts"))
		f.mu.Unlock()
		escribir(w, map[string]any{"ok": true})
	case "reactions.add", "chat.delete":
		escribir(w, map[string]any{"ok": true})
	default:
		escribir(w, map[string]any{"ok": false, "error": "unknown_method"})
	}
}

// socket hace de Socket Mode: manda un sobre con un mensaje y espera su acuse.
func (f *slackFalso) socket(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	ctx := r.Context()
	sobre := `{"type":"events_api","envelope_id":"E1","payload":{"event":{"type":"message","channel":"C1","user":"U2","text":"en vivo","ts":"1700000300.000100"}}}`
	if err := c.Write(ctx, websocket.MessageText, []byte(sobre)); err != nil {
		return
	}
	_, raw, err := c.Read(ctx)
	if err == nil {
		var a struct {
			ID string `json:"envelope_id"`
		}
		_ = json.Unmarshal(raw, &a)
		f.acuses <- a.ID
	}
	<-ctx.Done()
}

func TestSlackCanalesMensajesHilosYEnvio(t *testing.T) {
	f := nuevoSlackFalso(t)
	s := NuevaSlack("xoxp-bueno", "", &http.Client{})
	ctx := t.Context()
	cs, _, err := s.Canales(ctx)
	if err != nil || len(cs) != 3 {
		t.Fatal(cs, err)
	}
	if cs[1].Kind != "direct" || cs[1].Name != "Bea" || cs[2].Kind != "private" || s.Rotulo() != "Acme" {
		t.Fatalf("tipos o nombres: %+v", cs)
	}
	ms, err := s.Mensajes(ctx, "C1", time.Time{}, 50)
	if err != nil || len(ms) != 1 || ms[0].Body != "hola" || ms[0].AuthorName != "Bea" || ms[0].ReplyCount != 1 {
		t.Fatalf("mensajes (sin las respuestas de hilo): %+v %v", ms, err)
	}
	if len(ms[0].Reactions) != 1 || ms[0].Reactions[0].Emoji != ":tada:" || ms[0].Reactions[0].Subs[0] != Yo {
		t.Fatalf("reacciones: %+v", ms[0].Reactions)
	}
	h, err := s.Hilo(ctx, "C1", ms[0].ID)
	if err != nil || h.Root.Body != "hola" || len(h.Replies) != 1 || h.Replies[0].AuthorSub != Yo {
		t.Fatalf("hilo: %+v %v", h, err)
	}
	m, err := s.Enviar(ctx, "C1", ms[0].ID, "vale")
	if err != nil || m.AuthorSub != Yo || m.ParentID == nil || *m.ParentID != ms[0].ID {
		t.Fatalf("enviar: %+v %v", m, err)
	}
	if err := s.MarcarLeido(ctx, "C1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reaccionar(ctx, "C1", ms[0].ID, ":tada:", true); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.enviados) != 1 || f.enviados[0] != "C1|1700000000.000100|vale" || len(f.marcados) != 1 || f.marcados[0] != "C1|1700000050.000200" {
		t.Fatalf("lo que llegó a Slack: %v %v", f.enviados, f.marcados)
	}
}

func TestSlackTokenMaloSeDice(t *testing.T) {
	nuevoSlackFalso(t)
	if _, err := NuevaSlack("xoxp-malo", "", &http.Client{}).QuienSoy(t.Context()); err == nil || !strings.Contains(err.Error(), "invalid_auth") {
		t.Fatal(err)
	}
}

func TestSlackSondeaLosCanalesAbiertos(t *testing.T) {
	nuevoSlackFalso(t)
	s := NuevaSlack("xoxp-bueno", "", &http.Client{})
	s.Sondeo = 20 * time.Millisecond
	ctx, parar := context.WithTimeout(t.Context(), 2*time.Second)
	defer parar()
	if _, err := s.Mensajes(ctx, "C1", time.Time{}, 50); err != nil {
		t.Fatal(err)
	}
	llega := make(chan api.RoomsEvent, 4)
	go func() { _ = s.Escuchar(ctx, func(ev api.RoomsEvent) { llega <- ev }) }()
	select {
	case ev := <-llega:
		var m api.RoomsMessage
		_ = json.Unmarshal(ev.Payload, &m)
		if ev.Type != "message" || ev.ChannelID != "C1" || m.Body != "nuevo" {
			t.Fatalf("%+v %+v", ev, m)
		}
	case <-ctx.Done():
		t.Fatal("el sondeo no entregó nada")
	}
	if _, us, _ := s.Canales(ctx); len(us) != 1 || us[0].Unread != 1 {
		t.Fatalf("sin leer: %+v", us)
	}
}

func TestSlackSocketModeEntregaYConfirma(t *testing.T) {
	f := nuevoSlackFalso(t)
	s := NuevaSlack("xoxp-bueno", "xapp-1", &http.Client{})
	ctx, parar := context.WithTimeout(t.Context(), 3*time.Second)
	defer parar()
	llega := make(chan api.RoomsEvent, 4)
	go func() { _ = s.Escuchar(ctx, func(ev api.RoomsEvent) { llega <- ev }) }()
	select {
	case ev := <-llega:
		var m api.RoomsMessage
		_ = json.Unmarshal(ev.Payload, &m)
		if ev.ChannelID != "C1" || m.Body != "en vivo" || m.AuthorName != "Bea" {
			t.Fatalf("%+v %+v", ev, m)
		}
	case <-ctx.Done():
		t.Fatal("Socket Mode no entregó el mensaje")
	}
	select {
	case id := <-f.acuses:
		if id != "E1" {
			t.Fatal("acuse", id)
		}
	case <-ctx.Done():
		t.Fatal("no se confirmó el sobre")
	}
}

func TestSlackAltaYBaja(t *testing.T) {
	nuevoSlackFalso(t)
	sec := &secretosMemoria{v: map[string]string{}}
	reg, err := AbrirRegistro(t.TempDir()+"/chat-cuentas.json", sec)
	if err != nil {
		t.Fatal(err)
	}
	// Sus servidores escuchan en este equipo: sin cliente (el del portero, en la
	// edición) no se sale a la red (R-RED).
	reg.Cliente = &http.Client{}
	if _, _, err := reg.AltaSlack(t.Context(), "nada", ""); err == nil {
		t.Fatal("aceptó un token que no es de Slack")
	}
	c, _, err := reg.AltaSlack(t.Context(), "xoxp-bueno", "xapp-1")
	if err != nil || c.Proveedor != ProveedorSlack || c.Rotulo != "Acme" {
		t.Fatal(c, err)
	}
	if p, err := reg.Abrir(c); err != nil || p.(*Slack).TokenApp != "xapp-1" {
		t.Fatal(p, err)
	}
	if err := reg.Baja(c.ID); err != nil || len(sec.v) != 0 {
		t.Fatal("quedaron secretos", sec.v, err)
	}
}

type secretosMemoria struct {
	mu sync.Mutex
	v  map[string]string
}

func (s *secretosMemoria) Leer(n string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.v[n], nil
}

func (s *secretosMemoria) Guardar(n, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.v[n] = v
	return nil
}

func (s *secretosMemoria) Borrar(n string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.v, n)
	return nil
}
