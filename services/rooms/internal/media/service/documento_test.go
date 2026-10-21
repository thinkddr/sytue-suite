// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de las salas que nacen en un documento: quién entra lo decide la ACL de Sytue
// Docs (DocumentGate), por encima del rol en el tenant, de haberla creado o de haber
// participado, y siempre fail-closed.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

const docUUID = "6b0e1c0e-1f6c-4d7e-9a53-2f0f6a1f0a11"

type fakeGate struct {
	err    error
	vistos []string
}

func (g *fakeGate) CanAccessDocument(_ context.Context, id string) error {
	g.vistos = append(g.vistos, id)
	return g.err
}

func docRoom() domain.Room {
	return domain.Room{
		ID: uuid.New(), Name: "r-doc", Status: domain.RoomActive, CreatedBy: "alice",
		Metadata: map[string]any{"source": map[string]any{"kind": "document", "ref_id": docUUID}},
	}
}

func TestSalaDeDocumentoManda_LaACLDelDocumento(t *testing.T) {
	room := docRoom()
	s := &Service{store: &fakeStore{room: room}}
	g := &fakeGate{err: domain.ErrPermission}
	s.SetDocumentGate(g)
	// Ni el creador ni un administrador ni quien ya participó saltan la verja.
	for _, quien := range []struct {
		sub   string
		admin bool
	}{{"alice", false}, {"root", true}, {"eve", false}} {
		if err := s.requireRoomAccess(accessCtx(t), room, quien.sub, quien.admin); !errors.Is(err, domain.ErrPermission) {
			t.Fatalf("%s sin acceso al documento no debe entrar; err=%v", quien.sub, err)
		}
	}
	g.err = nil
	if err := s.requireRoomAccess(accessCtx(t), room, "bob", false); err != nil {
		t.Fatalf("con acceso al documento debe entrar: %v", err)
	}
	if g.vistos[0] != docUUID {
		t.Fatalf("la verja pregunta por el documento de la sala, no por otra cosa: %v", g.vistos)
	}
}

func TestSalaDeDocumentoSinVerjaQuedaCerrada(t *testing.T) {
	room := docRoom()
	s := &Service{store: &fakeStore{room: room}} // docGate nil
	if err := s.requireRoomAccess(accessCtx(t), room, "alice", true); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("sin verja configurada debe denegar; err=%v", err)
	}
	if _, err := s.CreateRoomFromSource(accessCtx(t), domain.SourceDocument, docUUID, "Plan", "alice", 0); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("crear la sala sin verja debe denegar; err=%v", err)
	}
}

func TestCrearSalaDeDocumentoExigeAccesoYGuardaSoloTituloEId(t *testing.T) {
	room := docRoom()
	fs := &fakeStore{room: room}
	s := newSvc(fs, &fakeLK{}, nil)
	g := &fakeGate{err: domain.ErrPermission}
	s.SetDocumentGate(g)
	if _, err := s.CreateRoomFromSource(accessCtx(t), domain.SourceDocument, docUUID, "Plan", "eve", 0); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("sin acceso al documento no se crea la sala; err=%v", err)
	}
	if fs.newRoom != nil {
		t.Fatal("no debe haberse creado nada")
	}
	g.err = nil
	if _, err := s.CreateRoomFromSource(accessCtx(t), domain.SourceDocument, docUUID, "Plan", "bob", 0); err != nil {
		t.Fatalf("con acceso debe crearse: %v", err)
	}
	src, _ := fs.newRoom.Metadata["source"].(map[string]any)
	if src["kind"] != "document" || src["ref_id"] != docUUID || src["title"] != "Plan" {
		t.Fatalf("la metadata guarda el origen: %v", fs.newRoom.Metadata)
	}
	// Nada del contenido ni de las personas con acceso viaja a Rooms.
	if len(src) != 3 {
		t.Fatalf("solo kind, ref_id y title: %v", src)
	}
}

func TestSoloUnDocumentoConIdValidoEsOrigen(t *testing.T) {
	if (domain.RoomSource{Kind: domain.SourceDocument, RefID: "no-es-uuid"}).Valid() {
		t.Fatal("el id de un documento es un UUID")
	}
	if !(domain.RoomSource{Kind: domain.SourceDocument, RefID: docUUID}).Valid() {
		t.Fatal("un UUID es un origen válido")
	}
}

func TestRoomBySourceDeDocumentoNoRevelaLaSala(t *testing.T) {
	fs := &fakeStore{findRoom: docRoom(), findFound: true}
	s := &Service{store: fs}
	s.SetDocumentGate(&fakeGate{err: domain.ErrPermission})
	if _, err := s.RoomBySource(accessCtx(t), domain.SourceDocument, docUUID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin acceso no se distingue de que no exista; err=%v", err)
	}
}

func TestListRoomsNoListaLasSalasDeDocumento(t *testing.T) {
	directa := domain.Room{ID: uuid.New(), Name: "r-directa", Status: domain.RoomActive}
	s := &Service{store: &fakeStore{rooms: []domain.Room{docRoom(), directa}}}
	got, err := s.ListRooms(accessCtx(t), "", 0)
	if err != nil || len(got) != 1 || got[0].ID != directa.ID {
		t.Fatalf("la sala de un documento no se lista: %v %v", got, err)
	}
}

func TestDocsClientPresentaElTokenYFallaCerrado(t *testing.T) {
	var cabecera, ruta string
	estado := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cabecera, ruta = r.Header.Get("Authorization"), r.URL.Path
		w.WriteHeader(estado)
	}))
	defer srv.Close()
	c := NewDocsClient(srv.URL)
	ctx := WithBearer(context.Background(), "tok")
	if err := c.CanAccessDocument(ctx, docUUID); err != nil {
		t.Fatalf("200 = puede ver: %v", err)
	}
	if cabecera != "Bearer tok" || ruta != "/api/v1/documents/"+docUUID {
		t.Fatalf("pide el documento con el token de quien llama: %q %q", cabecera, ruta)
	}
	for _, code := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusBadGateway} {
		estado = code
		if err := c.CanAccessDocument(ctx, docUUID); !errors.Is(err, domain.ErrPermission) {
			t.Fatalf("%d debe denegar; err=%v", code, err)
		}
	}
	if err := c.CanAccessDocument(context.Background(), docUUID); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("sin token debe denegar; err=%v", err)
	}
	srv.Close()
	if err := c.CanAccessDocument(ctx, docUUID); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("con Docs caído debe denegar; err=%v", err)
	}
	if NewDocsClient("") != nil {
		t.Fatal("sin URL no hay cliente")
	}
}

func TestDocumentCallProofRejectsUnboundAndExpiredAssertions(t *testing.T) {
	valid := domain.DocumentCallAccess{DocumentID: docUUID, Actor: "alice", Tenant: "acme", Version: 7, ExpiresAtMS: time.Now().Add(time.Minute).UnixMilli()}
	for _, tc := range []struct {
		name  string
		alter func(*domain.DocumentCallAccess)
	}{
		{"other document", func(p *domain.DocumentCallAccess) { p.DocumentID = uuid.NewString() }},
		{"missing actor", func(p *domain.DocumentCallAccess) { p.Actor = "" }},
		{"missing tenant", func(p *domain.DocumentCallAccess) { p.Tenant = "" }},
		{"negative version", func(p *domain.DocumentCallAccess) { p.Version = -1 }},
		{"negative expiry", func(p *domain.DocumentCallAccess) { p.ExpiresAtMS = -1 }},
		{"expired", func(p *domain.DocumentCallAccess) { p.ExpiresAtMS = time.Now().Add(-time.Second).UnixMilli() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proof := valid
			tc.alter(&proof)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer actor-session" || r.URL.Path != "/api/v1/documents/"+docUUID+"/call-access" {
					t.Error("document proof request lost its actor or path")
				}
				_ = json.NewEncoder(w).Encode(proof)
			}))
			defer srv.Close()
			if _, err := NewDocsClient(srv.URL).CallAccess(WithBearer(context.Background(), "actor-session"), docUUID); !errors.Is(err, domain.ErrPermission) {
				t.Fatalf("unsafe assertion accepted: %v", err)
			}
		})
	}
}
