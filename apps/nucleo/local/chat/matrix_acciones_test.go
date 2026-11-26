// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// accionesFalso añade al servidor falso lo de crear, invitar, explorar y medios.
type accionesFalso struct {
	*servidorFalso
	mu       sync.Mutex
	creadas  []map[string]any
	invites  []string
	directo  map[string][]string
	medios   map[string][]byte
	unidas   []string
	viejaAPI bool // el servidor no tiene la API de medios autenticada
}

func (f *accionesFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	responder := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	p := r.URL.Path
	if r.Header.Get("Authorization") != "Bearer tok" {
		f.servidorFalso.ServeHTTP(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case p == "/_matrix/client/v3/createRoom":
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		f.creadas = append(f.creadas, c)
		responder(map[string]string{"room_id": "!nueva" + string(rune('0'+len(f.creadas))) + ":ejemplo.org"})
	case strings.HasSuffix(p, "/invite"):
		var c map[string]string
		_ = json.NewDecoder(r.Body).Decode(&c)
		f.invites = append(f.invites, c["user_id"])
		responder(map[string]any{})
	case strings.HasSuffix(p, "/account_data/m.direct"):
		_ = json.NewDecoder(r.Body).Decode(&f.directo)
		responder(map[string]any{})
	case p == "/_matrix/client/v3/publicRooms":
		responder(map[string]any{"chunk": []any{
			map[string]any{"room_id": salaClara, "name": "General", "join_rule": "public"},
			map[string]any{"room_id": "!plaza:ejemplo.org", "name": "Plaza", "topic": "De todo", "join_rule": "public"},
		}})
	case strings.HasPrefix(p, "/_matrix/client/v3/join/"):
		f.unidas = append(f.unidas, strings.TrimPrefix(p, "/_matrix/client/v3/join/"))
		responder(map[string]string{"room_id": "!plaza:ejemplo.org"})
	case strings.HasPrefix(p, "/_matrix/client/v3/profile/"):
		if strings.HasSuffix(p, otroMx) {
			responder(map[string]string{"displayname": "Luis"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		responder(map[string]string{"errcode": "M_NOT_FOUND"})
	case p == "/_matrix/client/v3/user_directory/search":
		responder(map[string]any{"results": []any{map[string]string{"user_id": otroMx, "display_name": "Luis"}}})
	case p == "/_matrix/media/v3/upload":
		raw, _ := io.ReadAll(r.Body)
		if f.medios == nil {
			f.medios = map[string][]byte{}
		}
		id := "m" + string(rune('0'+len(f.medios)))
		f.medios[id] = raw
		if r.URL.Query().Get("filename") == "" || r.Header.Get("Content-Type") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		responder(map[string]string{"content_uri": "mxc://ejemplo.org/" + id})
	case strings.HasPrefix(p, "/_matrix/client/v1/media/") && f.viejaAPI:
		w.WriteHeader(http.StatusNotFound)
		responder(map[string]string{"errcode": "M_UNRECOGNIZED"})
	case strings.HasPrefix(p, "/_matrix/client/v1/media/download/ejemplo.org/"), strings.HasPrefix(p, "/_matrix/media/v3/download/ejemplo.org/"):
		i := strings.LastIndex(p, "/")
		_, _ = w.Write(f.medios[p[i+1:]])
	default:
		f.mu.Unlock()
		f.servidorFalso.ServeHTTP(w, r)
		f.mu.Lock()
	}
}

func nuevaMatrixAcciones(t *testing.T) (*accionesFalso, *Matrix) {
	base, _ := nuevoFalso(t)
	f := &accionesFalso{servidorFalso: base}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	m := NuevaMatrix(srv.URL, yoMx, "tok", srv.Client())
	if _, _, err := m.Canales(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f, m
}

func TestMatrixCrearSalaYDirecto(t *testing.T) {
	ctx := context.Background()
	f, m := nuevaMatrixAcciones(t)
	c, err := m.CrearCanal(ctx, "Proyecto", "El tema", true)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Proyecto" || c.Kind != "private" || c.Topic != "El tema" {
		t.Fatalf("canal creado: %+v", c)
	}
	if f.creadas[0]["preset"] != "private_chat" || f.creadas[0]["topic"] != "El tema" {
		t.Fatalf("createRoom: %v", f.creadas[0])
	}
	if _, ok := f.creadas[0]["initial_state"]; ok {
		t.Fatal("no se pide cifrado")
	}
	// La sala nueva se puede abrir antes del siguiente /sync.
	if _, err := m.Mensajes(ctx, c.ID, time.Time{}, 50); err != nil {
		t.Fatalf("la sala recién creada no se abre: %v", err)
	}

	d, err := m.AbrirDirecto(ctx, []string{otroMx})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != "direct" || len(d.PeerSubs) != 1 || d.PeerSubs[0] != otroMx {
		t.Fatalf("directo: %+v", d)
	}
	if f.creadas[1]["is_direct"] != true || f.creadas[1]["preset"] != "trusted_private_chat" {
		t.Fatalf("createRoom del directo: %v", f.creadas[1])
	}
	if got := f.directo[otroMx]; len(got) != 1 || got[0] != d.ID {
		t.Fatalf("m.direct: %v", f.directo)
	}
	// El segundo se reutiliza.
	otra, err := m.AbrirDirecto(ctx, []string{otroMx})
	if err != nil || otra.ID != d.ID || len(f.creadas) != 2 {
		t.Fatalf("el directo no se reutiliza: %+v %v (%d creadas)", otra, err, len(f.creadas))
	}
	if _, err := m.AbrirDirecto(ctx, []string{"luis"}); err == nil {
		t.Fatal("un nombre sin servidor no es un usuario de Matrix")
	}
}

func TestMatrixPersonasInvitarYExplorar(t *testing.T) {
	ctx := context.Background()
	f, m := nuevaMatrixAcciones(t)
	g, err := m.Gente(ctx)
	if err != nil || len(g) != 1 || g[0].Subject != otroMx || g[0].Name != "Luis" {
		t.Fatalf("gente: %+v %v", g, err)
	}
	p, err := m.Resolver(ctx, otroMx)
	if err != nil || p.UserID != otroMx || p.Name != "Luis" {
		t.Fatalf("resolver por mxid: %+v %v", p, err)
	}
	if _, err := m.Resolver(ctx, "@nadie:ejemplo.org"); !errors.Is(err, ErrNoEncontrada) {
		t.Fatalf("quien no existe: %v", err)
	}
	if p, err := m.Resolver(ctx, "lu"); err != nil || p.UserID != otroMx {
		t.Fatalf("resolver en el directorio: %+v %v", p, err)
	}
	mb, err := m.Anadir(ctx, salaClara, "@eva:ejemplo.org")
	if err != nil || !mb.Pending || f.invites[0] != "@eva:ejemplo.org" {
		t.Fatalf("invitar: %+v %v %v", mb, err, f.invites)
	}
	cs, err := m.Explorar(ctx, "")
	if err != nil || len(cs) != 1 || cs[0].ID != "!plaza:ejemplo.org" {
		t.Fatalf("explorar (sin las salas en las que ya se está): %+v %v", cs, err)
	}
	c, err := m.Unirse(ctx, cs[0].ID)
	if err != nil || c.ID != "!plaza:ejemplo.org" || len(f.unidas) != 1 {
		t.Fatalf("unirse: %+v %v", c, err)
	}
}

func TestMatrixAdjuntosIdaYVuelta(t *testing.T) {
	for _, vieja := range []bool{false, true} {
		ctx := context.Background()
		f, m := nuevaMatrixAcciones(t)
		f.viejaAPI = vieja
		datos := []byte("\x89PNG falso")
		fs := []Fichero{
			{Nombre: "foto.png", Tipo: "image/png", Tamano: int64(len(datos)), Abrir: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(datos)), nil }},
			{Nombre: "acta.pdf", Tipo: "application/pdf", Tamano: 3, Abrir: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("pdf")), nil }},
		}
		msg, err := m.EnviarFicheros(ctx, salaClara, "", "mirad", fs)
		if err != nil {
			t.Fatal(err)
		}
		if len(msg.Attachments) != 1 || msg.Attachments[0].Name != "foto.png" || msg.Body != "mirad" || msg.Attachments[0].ContentType != "image/png" {
			t.Fatalf("mensaje con adjunto: %+v", msg)
		}
		f.servidorFalso.mu.Lock()
		env := f.enviados
		f.servidorFalso.mu.Unlock()
		if len(env) != 2 || env[0]["msgtype"] != "m.image" || env[1]["msgtype"] != "m.file" || env[0]["filename"] != "foto.png" || env[1]["body"] != "acta.pdf" {
			t.Fatalf("eventos mandados: %v", env)
		}
		rc, err := m.Bajar(ctx, salaClara, msg.Attachments[0].FileID, false)
		if err != nil {
			t.Fatalf("bajar (API vieja %v): %v", vieja, err)
		}
		got, _ := io.ReadAll(rc)
		rc.Close()
		if !bytes.Equal(got, datos) {
			t.Fatalf("bajado %q", got)
		}
		if _, err := m.Bajar(ctx, salaClara, "https://otro/sitio", false); err == nil {
			t.Fatal("solo mxc://")
		}
	}
	// En una sala cifrada no se manda nada.
	_, m := nuevaMatrixAcciones(t)
	if _, err := m.EnviarFicheros(context.Background(), salaCifrada, "", "", []Fichero{{Nombre: "x"}}); !errors.Is(err, ErrSalaCifrada) {
		t.Fatalf("sala cifrada: %v", err)
	}
}

func TestMatrixLeeAdjuntosYHilos(t *testing.T) {
	s := &salaMx{id: salaClara, miembros: map[string]string{}, reacciones: map[string]reaccionMx{}}
	m := NuevaMatrix("https://x", yoMx, "tok", nil)
	raw, _ := json.Marshal(map[string]any{"msgtype": "m.file", "body": "informe.pdf", "url": "mxc://x/abc", "info": map[string]any{"mimetype": "application/pdf", "size": 42}})
	_, msg, ok := m.aplicar(s, eventoMx{Tipo: "m.room.message", ID: "$f", Remite: otroMx, Hora: 1, Contenid: raw}, false)
	if !ok || msg.Body != "" || len(msg.Attachments) != 1 || !igualAdjunto(msg.Attachments[0], api.RoomsAttachment{FileID: "mxc://x/abc", Name: "informe.pdf", ContentType: "application/pdf", SizeBytes: 42}) {
		t.Fatalf("adjunto leído: %+v", msg)
	}
	raw, _ = json.Marshal(map[string]any{"msgtype": "m.text", "body": "respuesta", "m.relates_to": map[string]any{"rel_type": "m.thread", "event_id": "$f"}})
	m.aplicar(s, eventoMx{Tipo: "m.room.message", ID: "$r", Remite: yoMx, Hora: 2, Contenid: raw}, false)
	m.salas[salaClara] = s
	hs, _ := m.MisHilos(context.Background())
	if len(hs) != 1 || hs[0].Root.ID != "$f" || hs[0].Root.ThreadParticipants[0].Sub != Yo {
		t.Fatalf("hilos: %+v", hs)
	}
}

func igualAdjunto(a, b api.RoomsAttachment) bool {
	return a.FileID == b.FileID && a.Name == b.Name && a.ContentType == b.ContentType && a.SizeBytes == b.SizeBytes
}
