// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// directorioFijo es un Sytue ID de mentira con las personas que conoce.
type directorioFijo map[string]Persona

func (d directorioFijo) Resolver(_ context.Context, q string) (Persona, error) {
	for _, p := range d {
		if "@"+p.Username == q {
			return p, nil
		}
	}
	return Persona{}, domain.ErrNotFound
}

func (d directorioFijo) PorID(_ context.Context, id string) (Persona, error) {
	if p, ok := d[id]; ok {
		return p, nil
	}
	return Persona{}, domain.ErrNotFound
}

// De punta a punta por el servicio: Ana abre un DM con Bea, de otra organización;
// cuando Bea le escribe desde la suya, cae en la MISMA conversación; y un canal
// suma a Bea con el nombre que da Sytue ID, no con el que mande el navegador.
func TestConversacionConOtraOrganizacion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	s := New(config.Config{MaxMessageBytes: 4096, MessagePageSize: 50}, st, nil, nil)
	s.SetDirectorio(directorioFijo{
		"ana": {UserID: "ana", Name: "Ana Acme", Username: "ana"},
		"bea": {UserID: "bea", Name: "Bea Otra", Username: "bea"},
	})
	acme := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	otra := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "otra"})

	equiposAcme, err := s.EnsureDefaultTeams(acme, "ana", "Ana")
	if err != nil {
		t.Fatalf("equipo de acme: %v", err)
	}
	equiposOtra, err := s.EnsureDefaultTeams(otra, "bea", "Bea")
	if err != nil {
		t.Fatalf("equipo de otra: %v", err)
	}

	// Alguien que Sytue ID no conoce no entra por la puerta de fuera.
	if _, err := s.OpenGroupDirect(acme, equiposAcme[0].ID, "ana", []string{"fantasma"}); !errors.Is(err, domain.ErrNotMember) {
		t.Fatalf("DM con un sub que no existe: %v", err)
	}

	dm, err := s.OpenGroupDirect(acme, equiposAcme[0].ID, "ana", []string{"bea"})
	if err != nil {
		t.Fatalf("Ana abre el DM con Bea: %v", err)
	}
	// Bea, desde SU organización, abre «su» DM con Ana: es la misma conversación.
	mismo, err := s.OpenGroupDirect(otra, equiposOtra[0].ID, "bea", []string{"ana"})
	if err != nil || mismo.ID != dm.ID || !mismo.External {
		t.Fatalf("Bea debía caer en la conversación que ya existe en acme: %+v %v", mismo, err)
	}
	canales, _, err := s.ListExternalChannels(otra, "bea")
	if err != nil || len(canales) != 1 || canales[0].ID != dm.ID {
		t.Fatalf("Bea ve la conversación en su barra: %+v %v", canales, err)
	}
	if tenant, ok := s.CanAccessChannel(context.Background(), "otra", dm.ID, "bea"); !ok || tenant != "acme" {
		t.Fatalf("el tiempo real de Bea va por la sala de acme: %q %v", tenant, ok)
	}
	if anfitrion, err := s.Anfitrion(otra, "bea", RefAnfitrion{Canal: dm.ID}); err != nil || anfitrion != "acme" {
		t.Fatalf("Anfitrion: %q %v", anfitrion, err)
	}

	// A un canal: el nombre lo pone Sytue ID.
	canal, err := s.CreateChannel(acme, CreateChannelInput{TeamID: equiposAcme[0].ID, Name: "Proyecto", Kind: domain.ChannelPrivate, Creator: "ana"})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	m, err := s.AddChannelMember(acme, canal.ID, "ana", "bea", "Director General")
	if err != nil || !m.External || m.Name != "Bea Otra" {
		t.Fatalf("Bea en el canal: %+v %v", m, err)
	}
	if _, err := s.AddChannelMember(acme, canal.ID, "ana", "fantasma", "X"); !errors.Is(err, domain.ErrNotMember) {
		t.Fatalf("a quien no existe no se le añade: %v", err)
	}
}
