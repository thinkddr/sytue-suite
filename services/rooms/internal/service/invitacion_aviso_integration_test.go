// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// COM-15 (decidido por el dueño el 2026-10-17): a quien invitan desde OTRA organización,
// a un canal o a una conversación, se le avisa en su campana. El aviso es de la
// anfitriona (que el Hub marca al verlo desde fuera), va a su `sub` y lleva a SUS
// invitaciones sin la organización anfitriona: no abre nada más de ella. Volver a abrir
// la misma conversación no repite el aviso. Antes no salía ninguno (#829).
func TestLaInvitacionDeFueraSaleEnSuCampana(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	s := New(config.Config{MaxMessageBytes: 4096, MessagePageSize: 50, PublicURL: "https://rooms.sytue.eu"}, st, nil, nil)
	s.SetDirectorio(directorioFijo{
		"ana": {UserID: "ana", Name: "Ana Acme", Username: "ana"},
		"bea": {UserID: "bea", Name: "Bea Otra", Username: "bea"},
	})
	avisos := make(chan hubnotify.Notification, 8)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		avisos <- n
		w.WriteHeader(http.StatusCreated)
	}))
	defer hub.Close()
	s.SetHubNotifier(hubnotify.New(hub.URL, "secreto"))
	siguiente := func(que string) hubnotify.Notification {
		t.Helper()
		select {
		case n := <-avisos:
			return n
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: no salió ningún aviso", que)
			return hubnotify.Notification{}
		}
	}
	comprobar := func(n hubnotify.Notification, tipo string) {
		t.Helper()
		if n.Tenant != "acme" || n.Subject != "bea" || n.Type != tipo || (n.Params["actor"] != "Ana" && n.Params["actor"] != "Ana Acme") {
			t.Errorf("aviso %+v; quiero de acme, a bea, %s, de Ana", n, tipo)
		}
		if n.URL != "https://rooms.sytue.eu/rooms/?invitations=1" || strings.Contains(n.URL, "acme") {
			t.Errorf("el aviso lleva a %q; quiero las invitaciones de quien lo recibe, sin la anfitriona", n.URL)
		}
	}

	acme := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	equipos, err := s.EnsureDefaultTeams(acme, "ana", "Ana")
	if err != nil {
		t.Fatal(err)
	}
	canal, err := s.CreateChannel(acme, CreateChannelInput{TeamID: equipos[0].ID, Name: "Proyecto", Kind: domain.ChannelPrivate, Creator: "ana"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddChannelMember(acme, canal.ID, "ana", "bea", ""); err != nil {
		t.Fatal(err)
	}
	n := siguiente("invitación al canal")
	comprobar(n, "rooms.invitation")
	if n.Params["channel"] != "Proyecto" {
		t.Errorf("el aviso del canal no dice cuál: %+v", n.Params)
	}
	// En su bandeja, la organización que invita sale por su nombre visible (decisión del
	// dueño, 17/10), y solo se pregunta por las que la invitan.
	nombres := &nombresFijos{nombres: map[string]string{"acme": "Acme Ingeniería S.L."}}
	s.SetNombresOrgs(nombres)
	inv, err := s.Invitaciones(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "otra"}), "bea")
	if err != nil || len(inv) != 1 || inv[0].Organizacion != "acme" || inv[0].NombreOrganizacion != "Acme Ingeniería S.L." {
		t.Fatalf("la bandeja de Bea: %+v %v", inv, err)
	}
	if len(nombres.pedidos) != 1 || len(nombres.pedidos[0]) != 1 || nombres.pedidos[0][0] != "acme" {
		t.Errorf("se pregunta solo por la que invita: %v", nombres.pedidos)
	}

	if _, err := s.OpenGroupDirect(acme, equipos[0].ID, "ana", []string{"bea"}); err != nil {
		t.Fatal(err)
	}
	comprobar(siguiente("invitación a la conversación"), "rooms.invitation.direct")
	if _, err := s.OpenGroupDirect(acme, equipos[0].ID, "ana", []string{"bea"}); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-avisos:
		t.Errorf("volver a abrir la conversación repitió el aviso: %+v", n)
	case <-time.After(500 * time.Millisecond):
	}
}

// nombresFijos da nombres fijos y apunta por qué slugs le preguntaron.
type nombresFijos struct {
	nombres map[string]string
	pedidos [][]string
}

func (n *nombresFijos) Nombres(_ context.Context, slugs []string) map[string]string {
	n.pedidos = append(n.pedidos, slugs)
	out := map[string]string{}
	for _, s := range slugs {
		if v, ok := n.nombres[s]; ok {
			out[s] = v
		}
	}
	return out
}
