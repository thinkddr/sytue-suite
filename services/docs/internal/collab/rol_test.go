// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package collab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

type sinOps struct {
	aplicadas int
	sinAcceso map[string]bool
	applyErr  error
}

func (s *sinOps) ApplyOps(_ context.Context, _ domain.Subject, _ string, ops []domain.Op) ([]domain.SequencedOp, int64, error) {
	if s.applyErr != nil {
		return nil, 0, s.applyErr
	}
	s.aplicadas += len(ops)
	return nil, 1, nil
}

func (s *sinOps) OpsSince(context.Context, string, int64, int) ([]domain.SequencedOp, error) {
	return nil, nil
}

func (s *sinOps) Rol(_ context.Context, subj domain.Subject, _ string) (domain.Role, error) {
	if s.sinAcceso[subj.Sub] {
		return "", domain.ErrForbidden
	}
	return domain.RoleEditor, nil
}

// La conexión WebSocket también deniega los roles desconocidos o de solo lectura.
func TestOpsRoleMatrix(t *testing.T) {
	for _, role := range []domain.Role{"", domain.RoleViewer, domain.RoleCommenter, domain.RoleEditor, domain.RoleOwner, "unknown"} {
		t.Run(string(role), func(t *testing.T) {
			c := NewClient(NewHub(nil), nil, "d1", domain.Subject{Sub: "ana"}, role, "Ana")
			h := &sinOps{}
			c.handle(context.Background(), h, ClientMessage{Type: MsgCData, Ops: []domain.Op{{Kind: domain.OpSet, Key: "k"}}})
			m := <-c.send
			want, count := MsgError, 0
			if role.CanWrite() {
				want, count = MsgAck, 1
			}
			if m.Type != want || h.aplicadas != count {
				t.Fatalf("rol %q: respuesta=%+v, operaciones=%d", role, m, h.aplicadas)
			}
		})
	}
}

// Un rol de editor aún almacenado en la conexión no vence la denegación del servicio.
func TestOpsRejectRevokedPermissionWithoutRoleNotification(t *testing.T) {
	c := NewClient(NewHub(nil), nil, "d1", domain.Subject{Sub: "ana"}, domain.RoleEditor, "Ana")
	h := &sinOps{applyErr: domain.ErrForbidden}
	c.handle(context.Background(), h, ClientMessage{Type: MsgCData, Ops: []domain.Op{{Kind: domain.OpSet, Key: "k"}}})
	if m := <-c.send; m.Type != MsgError || h.aplicadas != 0 {
		t.Fatalf("revocación sin notificación permitió escritura: %+v, %d", m, h.aplicadas)
	}
}

// El desbloqueo en vivo afecta solo a quien recibe la concesión, sin reconectar.
func TestConcederCambiaElRolEnVivoSoloDeQuienLoPidio(t *testing.T) {
	hub := NewHub(nil)
	h := &sinOps{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		sub := r.URL.Query().Get("sub")
		NewClient(hub, conn, "d1", domain.Subject{Sub: sub}, domain.RoleViewer, sub).Serve(context.WithoutCancel(r.Context()), h, 0)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	abrir := func(sub string) *websocket.Conn {
		c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/?sub="+sub, nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	leer := func(c *websocket.Conn, tipo ServerMessageType) ServerMessage {
		for {
			var m ServerMessage
			if err := wsjson.Read(ctx, c, &m); err != nil {
				t.Fatalf("esperaba %q: %v", tipo, err)
			}
			if m.Type == tipo || m.Type == MsgError {
				return m
			}
		}
	}
	ana, otro := abrir("ana"), abrir("otro")
	defer ana.CloseNow()
	defer otro.CloseNow()
	leer(ana, MsgWelcome)
	leer(otro, MsgWelcome)

	op := ClientMessage{Type: MsgCData, Ops: []domain.Op{{Kind: domain.OpSet, Key: "k"}}}
	_ = wsjson.Write(ctx, ana, op)
	if m := leer(ana, MsgAck); m.Type != MsgError {
		t.Fatalf("una lectora escribió: %+v", m)
	}

	hub.PublishRole(ctx, "d1", "ana", domain.RoleEditor)
	if m := leer(ana, MsgRole); m.Type != MsgRole || m.Role != string(domain.RoleEditor) {
		t.Fatalf("ana no recibió su rol nuevo: %+v", m)
	}
	_ = wsjson.Write(ctx, ana, op)
	if m := leer(ana, MsgAck); m.Type != MsgAck || h.aplicadas != 1 {
		t.Fatalf("tras conceder, ana sigue sin poder escribir: %+v (aplicadas %d)", m, h.aplicadas)
	}
	_ = wsjson.Write(ctx, otro, op)
	if m := leer(otro, MsgAck); m.Type != MsgError {
		t.Fatalf("el rol de ana se le dio también a otro: %+v", m)
	}
}

// Al quitarle el acceso en vivo (permiso quitado, enlace revocado: ADR 0052) se le
// CIERRA el canal: no basta con que deje de escribir, porque seguiría recibiendo los
// cambios de los demás hasta reconectar. Y a otra persona del documento, no.
func TestQuitarElAccesoCierraSuCanal(t *testing.T) {
	hub := NewHub(nil)
	h := &sinOps{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		sub := r.URL.Query().Get("sub")
		NewClient(hub, conn, "d1", domain.Subject{Sub: sub}, domain.RoleEditor, sub).Serve(context.WithoutCancel(r.Context()), h, 0)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	abrir := func(sub string) *websocket.Conn {
		c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/?sub="+sub, nil)
		if err != nil {
			t.Fatal(err)
		}
		var m ServerMessage
		if err := wsjson.Read(ctx, c, &m); err != nil || m.Type != MsgWelcome {
			t.Fatalf("bienvenida de %s: %+v %v", sub, m, err)
		}
		return c
	}
	bea, ana := abrir("bea"), abrir("ana")
	defer bea.CloseNow()
	defer ana.CloseNow()

	hub.PublishRole(ctx, "d1", "bea", "")
	for {
		var m ServerMessage
		err := wsjson.Read(ctx, bea, &m)
		if err == nil {
			continue // lo que estuviera ya en cola
		}
		if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
			t.Fatalf("el canal de bea no se cerró por acceso quitado: %v", err)
		}
		break
	}
	// Ana sigue: escribe y le confirman.
	_ = wsjson.Write(ctx, ana, ClientMessage{Type: MsgCData, Ops: []domain.Op{{Kind: domain.OpSet, Key: "k"}}})
	for {
		var m ServerMessage
		if err := wsjson.Read(ctx, ana, &m); err != nil {
			t.Fatalf("a ana también se le cerró: %v", err)
		}
		if m.Type == MsgAck {
			break
		}
	}
}

// Lo que no avisa al quitarse (un enlace que caduca, un permiso de grupo retirado)
// también corta el canal: la conexión vuelve a preguntar por su rol cada poco.
func TestLaReautorizacionCierraAQuienYaNoPuede(t *testing.T) {
	hub := NewHub(nil)
	hub.reautorizarCada = 20 * time.Millisecond
	h := &sinOps{sinAcceso: map[string]bool{"bea": true}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		sub := r.URL.Query().Get("sub")
		NewClient(hub, conn, "d1", domain.Subject{Sub: sub}, domain.RoleEditor, sub).Serve(context.WithoutCancel(r.Context()), h, 0)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bea, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/?sub=bea", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bea.CloseNow()
	for {
		var m ServerMessage
		err := wsjson.Read(ctx, bea, &m)
		if err == nil {
			continue
		}
		if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
			t.Fatalf("sin acceso, el canal no se cerró: %v", err)
		}
		return
	}
}

// capturaProteccion deja ver a la prueba lo que el WebSocket entrega al servicio.
type capturaProteccion struct {
	sinOps
	visto domain.Proteccion
}

func (c *capturaProteccion) ApplyOps(ctx context.Context, subj domain.Subject, docID string, ops []domain.Op) ([]domain.SequencedOp, int64, error) {
	c.visto = domain.ProteccionDe(ctx)
	return c.sinOps.ApplyOps(ctx, subj, docID, ops)
}

// La contraseña de protección viaja FUERA de las operaciones y llega al servicio; y la
// denegación se distingue de un fallo cualquiera, para que el editor lo explique.
func TestOpsLlevaLaProteccionYDiceSiSeDeniega(t *testing.T) {
	op := []domain.Op{{Kind: domain.OpSet, Key: "doc:sheet"}}
	c := NewClient(NewHub(nil), nil, "d1", domain.Subject{Sub: "ana"}, domain.RoleEditor, "Ana")
	h := &capturaProteccion{}
	c.handle(context.Background(), h, ClientMessage{
		Type: MsgCData, Ops: op,
		Proteccion: &domain.Proteccion{Clave: "k", Nuevas: map[string]string{"hoja:H": "n"}},
	})
	<-c.send
	if h.visto.Clave != "k" || h.visto.Nuevas["hoja:H"] != "n" {
		t.Fatalf("el servicio no recibió la contraseña: %+v", h.visto)
	}
	for err, code := range map[error]string{
		domain.ErrProtegido:           "protected",
		domain.ErrClaveProteccion:     "protection_password",
		domain.ErrProteccionPendiente: "apply_failed",
	} {
		c := NewClient(NewHub(nil), nil, "d1", domain.Subject{Sub: "ana"}, domain.RoleEditor, "Ana")
		c.handle(context.Background(), &sinOps{applyErr: err}, ClientMessage{Type: MsgCData, Ops: op})
		if m := <-c.send; m.Type != MsgError || m.Code != code {
			t.Fatalf("%v: %+v, esperaba el código %q", err, m, code)
		}
	}
}
