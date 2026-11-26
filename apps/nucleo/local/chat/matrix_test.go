// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servidorFalso es lo justo de la API cliente-servidor de Matrix: login, whoami, sync,
// send, messages, redact y read_markers, con una sala en claro y otra cifrada.
type servidorFalso struct {
	mu        sync.Mutex
	t         *testing.T
	enviados  []map[string]any
	sincros   int
	nuevo     chan struct{}
	pendiente []map[string]any
	leidos    []string
}

const (
	salaClara   = "!clara:ejemplo.org"
	salaCifrada = "!cifrada:ejemplo.org"
	yoMx        = "@ana:ejemplo.org"
	otroMx      = "@luis:ejemplo.org"
)

func ev(tipo, id, remite string, ts int64, contenido map[string]any) map[string]any {
	return map[string]any{"type": tipo, "event_id": id, "sender": remite, "origin_server_ts": ts, "content": contenido}
}

func estado(tipo, clave string, contenido map[string]any) map[string]any {
	e := ev(tipo, "$e-"+tipo+clave, otroMx, 1, contenido)
	e["state_key"] = clave
	return e
}

func (f *servidorFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	responder := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	if r.URL.Path != "/_matrix/client/v3/login" && r.Header.Get("Authorization") != "Bearer tok" {
		w.WriteHeader(http.StatusUnauthorized)
		responder(map[string]string{"errcode": "M_UNKNOWN_TOKEN", "error": "token"})
		return
	}
	p := r.URL.Path
	switch {
	case p == "/_matrix/client/v3/login":
		var c struct {
			Password   string `json:"password"`
			Identifier struct {
				User string `json:"user"`
			} `json:"identifier"`
		}
		_ = json.NewDecoder(r.Body).Decode(&c)
		if c.Password != "secreta" || c.Identifier.User != "ana" {
			w.WriteHeader(http.StatusForbidden)
			responder(map[string]string{"errcode": "M_FORBIDDEN", "error": "mala"})
			return
		}
		responder(map[string]string{"user_id": yoMx, "access_token": "tok", "device_id": "DEV"})
	case p == "/_matrix/client/v3/account/whoami":
		responder(map[string]string{"user_id": yoMx})
	case p == "/_matrix/client/v3/sync":
		f.sync(w, r)
	case strings.HasSuffix(p, "/messages"):
		if r.URL.Query().Get("from") != "t-atras" || r.URL.Query().Get("dir") != "b" {
			f.t.Errorf("paginación inesperada: %s", r.URL.RawQuery)
		}
		responder(map[string]any{"chunk": []any{
			ev("m.room.message", "$viejo2", otroMx, 2000, map[string]any{"msgtype": "m.text", "body": "viejo 2"}),
			ev("m.room.message", "$viejo1", otroMx, 1000, map[string]any{"msgtype": "m.text", "body": "viejo 1"}),
		}})
	case strings.Contains(p, "/send/"):
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		partes := strings.Split(p, "/")
		f.mu.Lock()
		id := fmt.Sprintf("$enviado%d", len(f.enviados))
		c["_tipo"] = partes[len(partes)-2]
		f.enviados = append(f.enviados, c)
		f.pendiente = append(f.pendiente, ev(partes[len(partes)-2], id, yoMx, time.Now().UnixMilli(), c))
		f.mu.Unlock()
		select {
		case f.nuevo <- struct{}{}:
		default:
		}
		responder(map[string]string{"event_id": id})
	case strings.Contains(p, "/redact/"):
		responder(map[string]string{"event_id": "$redaccion"})
	case strings.HasSuffix(p, "/read_markers"):
		f.mu.Lock()
		f.leidos = append(f.leidos, p)
		f.mu.Unlock()
		responder(map[string]any{})
	case strings.HasSuffix(p, "/joined_members"):
		responder(map[string]any{"joined": map[string]any{yoMx: map[string]string{"display_name": "Ana"}, otroMx: map[string]string{"display_name": "Luis"}}})
	default:
		w.WriteHeader(http.StatusNotFound)
		responder(map[string]string{"errcode": "M_UNRECOGNIZED"})
	}
}

func (f *servidorFalso) sync(w http.ResponseWriter, r *http.Request) {
	since := r.URL.Query().Get("since")
	if since == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"next_batch": "s1",
			"rooms": map[string]any{"join": map[string]any{
				salaClara: map[string]any{
					"state": map[string]any{"events": []any{
						estado("m.room.name", "", map[string]any{"name": "General"}),
						estado("m.room.member", otroMx, map[string]any{"membership": "join", "displayname": "Luis"}),
						estado("m.room.member", yoMx, map[string]any{"membership": "join", "displayname": "Ana"}),
					}},
					"timeline": map[string]any{"prev_batch": "t-atras", "limited": true, "events": []any{
						ev("m.room.message", "$hola", otroMx, 3000, map[string]any{"msgtype": "m.text", "body": "hola Ana"}),
						ev("m.reaction", "$reac", otroMx, 3100, map[string]any{"m.relates_to": map[string]any{"rel_type": "m.annotation", "event_id": "$hola", "key": "👋"}}),
						ev("m.room.message", "$edic", otroMx, 3200, map[string]any{"msgtype": "m.text", "body": "* hola, Ana", "m.new_content": map[string]any{"msgtype": "m.text", "body": "hola, Ana"}, "m.relates_to": map[string]any{"rel_type": "m.replace", "event_id": "$hola"}}),
					}},
					"unread_notifications": map[string]any{"notification_count": 2, "highlight_count": 1},
				},
				salaCifrada: map[string]any{
					"state": map[string]any{"events": []any{
						estado("m.room.name", "", map[string]any{"name": "Secreta"}),
						estado("m.room.encryption", "", map[string]any{"algorithm": "m.megolm.v1.aes-sha2"}),
					}},
					"timeline": map[string]any{"events": []any{
						ev("m.room.encrypted", "$cif", otroMx, 3000, map[string]any{"algorithm": "m.megolm.v1.aes-sha2", "ciphertext": "xxx"}),
					}},
				},
			}},
		})
		return
	}
	// Sondeo largo: devuelve lo pendiente en cuanto lo hay, o vacío al rato.
	f.mu.Lock()
	vacio := len(f.pendiente) == 0
	f.mu.Unlock()
	if vacio {
		select {
		case <-f.nuevo:
		case <-time.After(200 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
	}
	f.mu.Lock()
	evs := f.pendiente
	f.pendiente = nil
	f.sincros++
	n := f.sincros
	f.mu.Unlock()
	linea := []any{}
	for _, e := range evs {
		linea = append(linea, e)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"next_batch": fmt.Sprintf("s%d", n+1),
		"rooms":      map[string]any{"join": map[string]any{salaClara: map[string]any{"timeline": map[string]any{"events": linea}}}},
	})
}

func nuevoFalso(t *testing.T) (*servidorFalso, *httptest.Server) {
	f := &servidorFalso{t: t, nuevo: make(chan struct{}, 1)}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

type llaveroMemoria map[string]string

func (l llaveroMemoria) Leer(n string) (string, error) { return l[n], nil }
func (l llaveroMemoria) Guardar(n, v string) error     { l[n] = v; return nil }
func (l llaveroMemoria) Borrar(n string) error         { delete(l, n); return nil }

func TestMatrixAltaConContrasenaYReabrir(t *testing.T) {
	ctx := context.Background()
	_, srv := nuevoFalso(t)
	ruta := t.TempDir() + "/chat-cuentas.json"
	llavero := llaveroMemoria{}
	r, err := AbrirRegistro(ruta, llavero)
	if err != nil {
		t.Fatal(err)
	}
	// Sus servidores escuchan en este equipo: sin cliente (el del portero, en la
	// edición) no se sale a la red (R-RED).
	r.Cliente = &http.Client{}
	if _, _, err := r.AltaMatrix(ctx, srv.URL, "ana", "mala", ""); err == nil {
		t.Fatal("una contraseña mala no da de alta")
	}
	c, _, err := r.AltaMatrix(ctx, srv.URL, "ana", "secreta", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Usuario != yoMx || llavero[secretoToken(c.ID)] != "tok" {
		t.Fatalf("cuenta: %+v llavero %v", c, llavero)
	}
	r2, _ := AbrirRegistro(ruta, llavero)
	r2.Cliente = &http.Client{}
	if len(r2.Cuentas()) != 1 {
		t.Fatalf("cuentas tras reabrir: %+v", r2.Cuentas())
	}
	p, err := r2.Abrir(r2.Cuentas()[0])
	if err != nil {
		t.Fatal(err)
	}
	if cs, _, err := p.Canales(ctx); err != nil || len(cs) != 2 {
		t.Fatalf("canales: %v %+v", err, cs)
	}
	if err := r2.Baja(c.ID); err != nil || len(llavero) != 0 {
		t.Fatalf("baja: %v %v", err, llavero)
	}
}

func TestMatrixLeeEscribeYEscucha(t *testing.T) {
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	f, srv := nuevoFalso(t)
	m := NuevaMatrix(srv.URL, yoMx, "tok", srv.Client())

	cs, us, err := m.Canales(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].Name != "General" || cs[0].Kind != "private" || cs[1].Name != "Secreta" {
		t.Fatalf("canales: %+v", cs)
	}
	if !strings.HasPrefix(cs[1].Topic, "🔒") {
		t.Fatalf("la cifrada se marca: %+v", cs[1])
	}
	if len(us) != 1 || us[0].ChannelID != salaClara || us[0].Unread != 2 || us[0].Mentions != 1 {
		t.Fatalf("pendientes: %+v", us)
	}

	// La página más reciente pide lo anterior a /messages hasta llenar.
	ms, err := m.Mensajes(ctx, salaClara, time.Time{}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 3 || ms[0].Body != "viejo 1" || ms[2].Body != "hola, Ana" || ms[2].EditedAt == nil {
		t.Fatalf("mensajes: %+v", ms)
	}
	if len(ms[2].Reactions) != 1 || ms[2].Reactions[0].Emoji != "👋" || ms[2].AuthorName != "Luis" {
		t.Fatalf("reacción y autor: %+v", ms[2])
	}

	// La cifrada se ve pero no se lee ni se escribe.
	cif, _ := m.Mensajes(ctx, salaCifrada, time.Time{}, 10)
	if len(cif) != 1 || cif[0].Body != TextoCifrado {
		t.Fatalf("cifrada: %+v", cif)
	}
	if _, err := m.Enviar(ctx, salaCifrada, "", "hola"); !errors.Is(err, ErrSalaCifrada) {
		t.Fatalf("enviar a la cifrada: %v", err)
	}

	llegan := make(chan api.RoomsEvent, 16)
	go func() { _ = m.Escuchar(ctx, func(e api.RoomsEvent) { llegan <- e }) }()

	env, err := m.Enviar(ctx, salaClara, "", "qué tal")
	if err != nil || env.ID != "$enviado0" || env.AuthorSub != Yo {
		t.Fatalf("enviar: %v %+v", err, env)
	}
	if _, err := m.Enviar(ctx, salaClara, "$hola", "en el hilo"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	rel, _ := f.enviados[1]["m.relates_to"].(map[string]any)
	f.mu.Unlock()
	if rel["rel_type"] != "m.thread" || rel["event_id"] != "$hola" {
		t.Fatalf("relación de hilo: %+v", rel)
	}
	h, err := m.Hilo(ctx, salaClara, "$hola")
	if err != nil || len(h.Replies) != 1 || h.Root.ReplyCount != 1 {
		t.Fatalf("hilo: %v %+v", err, h)
	}

	// Un mensaje de otro llega por /sync como trama.
	f.mu.Lock()
	f.pendiente = append(f.pendiente, ev("m.room.message", "$nuevo", otroMx, time.Now().UnixMilli(), map[string]any{"msgtype": "m.text", "body": "¡ey!"}))
	f.mu.Unlock()
	f.nuevo <- struct{}{}
	limite := time.After(5 * time.Second)
	for {
		select {
		case e := <-llegan:
			if e.Type == "message" && strings.Contains(string(e.Payload), "¡ey!") {
				if e.ChannelID != salaClara || e.Subject != otroMx {
					t.Fatalf("trama: %+v", e)
				}
				goto recibido
			}
		case <-limite:
			t.Fatal("no llegó la trama del mensaje nuevo")
		}
	}
recibido:
	if _, err := m.Editar(ctx, salaClara, "$enviado0", "qué tal?"); err != nil {
		t.Fatal(err)
	}
	if err := m.Reaccionar(ctx, salaClara, "$hola", "❤️", true); err != nil {
		t.Fatal(err)
	}
	if err := m.Borrar(ctx, salaClara, "$enviado0"); err != nil {
		t.Fatal(err)
	}
	if err := m.MarcarLeido(ctx, salaClara); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	leidos := len(f.leidos)
	f.mu.Unlock()
	if leidos != 1 {
		t.Fatal("no se mandó el recibo de lectura")
	}
	if ms, _ := m.Miembros(ctx, salaClara); len(ms) != 2 || ms[0].Subject != Yo {
		t.Fatalf("miembros: %+v", ms)
	}
}
