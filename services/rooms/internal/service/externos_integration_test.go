// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service

import (
	"context"
	"errors"
	"testing"
	"time"

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

// Lo que Bea guarda, sigue, deja a medias o programa en una conversación de OTRA
// organización se escribe en la anfitriona (junto al mensaje y al canal) y sale en sus
// listas, que se leen desde la suya, junto con lo de su propia organización. Y deja de
// salir cuando ya no está en la conversación.
func TestListasDeLaPersonaDeFuera(t *testing.T) {
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
		t.Fatal(err)
	}
	equiposOtra, err := s.EnsureDefaultTeams(otra, "bea", "Bea")
	if err != nil {
		t.Fatal(err)
	}
	canal, err := s.CreateChannel(acme, CreateChannelInput{TeamID: equiposAcme[0].ID, Name: "Proyecto", Kind: domain.ChannelPrivate, Creator: "ana"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddChannelMember(acme, canal.ID, "ana", "bea", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.AceptarInvitacion(otra, "bea", "Bea", canal.ID); err != nil {
		t.Fatalf("Bea acepta: %v", err)
	}
	// Lo que hace Bea en la conversación de fuera entra por las rutas del canal o del
	// mensaje, que la llevan a la anfitriona (EnAnfitrion): aquí, el contexto de acme.
	raiz, err := s.PostMessage(acme, PostMessageInput{ChannelID: canal.ID, Body: "¿revisas el plano?", AuthorSub: "ana", AuthorName: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMessage(acme, raiz.ID, "bea"); err != nil {
		t.Fatalf("Bea guarda: %v", err)
	}
	if err := s.FollowThread(acme, raiz.ID, "bea"); err != nil {
		t.Fatalf("Bea sigue el hilo: %v", err)
	}
	prog, err := s.PostScheduled(acme, ScheduleMessageInput{ChannelID: canal.ID, Body: "mañana lo miro", SendAt: time.Now().Add(time.Hour), AuthorSub: "bea", AuthorName: "Bea"})
	if err != nil {
		t.Fatalf("Bea programa: %v", err)
	}
	// El borrador va por /me/drafts con el canal en el cuerpo: desde la suya.
	if _, err := s.PutDraft(otra, "bea", canal.ID, nil, "a medias"); err != nil {
		t.Fatalf("Bea deja un borrador: %v", err)
	}
	// Y lo suyo de su organización, para ver que se junta.
	propio, err := s.CreateChannel(otra, CreateChannelInput{TeamID: equiposOtra[0].ID, Name: "Suyo", Kind: domain.ChannelPublic, Creator: "bea"})
	if err != nil {
		t.Fatal(err)
	}
	m2, err := s.PostMessage(otra, PostMessageInput{ChannelID: propio.ID, Body: "nota", AuthorSub: "bea", AuthorName: "Bea"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMessage(otra, m2.ID, "bea"); err != nil {
		t.Fatal(err)
	}

	guardados, _, err := s.ListSaved(otra, "bea", nil, 0)
	if err != nil || len(guardados) != 2 || guardados[0].ID != m2.ID || guardados[1].ID != raiz.ID {
		t.Fatalf("guardados desde la suya = %+v (%v); quiero el suyo y el de acme, del más reciente al más antiguo", guardados, err)
	}
	if hilos, _, err := s.ListFollowedThreads(otra, "bea", nil, 0); err != nil || len(hilos) != 1 || hilos[0].Root.ID != raiz.ID {
		t.Fatalf("hilos desde la suya = %+v (%v)", hilos, err)
	}
	if borradores, err := s.ListDrafts(otra, "bea"); err != nil || len(borradores) != 1 || borradores[0].ChannelID != canal.ID || borradores[0].Body != "a medias" {
		t.Fatalf("borradores desde la suya = %+v (%v)", borradores, err)
	}
	// El borrador quedó en la anfitriona, junto al canal, y no en «otra».
	if enAcme, err := st.ListDrafts(acme, "bea"); err != nil || len(enAcme) != 1 {
		t.Fatalf("el borrador no está en acme: %+v (%v)", enAcme, err)
	}
	if prog2, err := s.ListScheduled(otra, "bea", nil); err != nil || len(prog2) != 1 || prog2[0].ID != prog.ID {
		t.Fatalf("programados desde la suya = %+v (%v)", prog2, err)
	}
	// Editar o cancelar el programado va por su ruta, que lo encuentra en acme.
	if a, err := s.Anfitrion(otra, "bea", RefAnfitrion{Programado: prog.ID}); err != nil || a != "acme" {
		t.Fatalf("Anfitrion del programado = %q (%v), quiero acme", a, err)
	}
	if a, err := s.Anfitrion(otra, "ana", RefAnfitrion{Programado: prog.ID}); err != nil || a != "" {
		t.Fatalf("el programado de Bea no es de Ana: %q (%v)", a, err)
	}
	// Vaciar el borrador lo borra en la anfitriona.
	if _, err := s.PutDraft(otra, "bea", canal.ID, nil, "  "); err != nil {
		t.Fatal(err)
	}
	if borradores, _ := s.ListDrafts(otra, "bea"); len(borradores) != 0 {
		t.Fatalf("el borrador vaciado sigue: %+v", borradores)
	}

	// Fuera de la conversación, lo de allí deja de listarse.
	if err := s.RemoveChannelMember(acme, canal.ID, "ana", "bea"); err != nil {
		t.Fatalf("sacar a Bea: %v", err)
	}
	if guardados, _, _ := s.ListSaved(otra, "bea", nil, 0); len(guardados) != 1 || guardados[0].ID != m2.ID {
		t.Fatalf("tras salir, guardados = %+v; quiero solo el suyo", guardados)
	}
	if hilos, _, _ := s.ListFollowedThreads(otra, "bea", nil, 0); len(hilos) != 0 {
		t.Fatalf("tras salir, hilos = %+v", hilos)
	}
}

// La política de contacto con gente de fuera (mig 0083) en sus tres sitios: quien abre
// la conversación no añade a quien su organización no admite (y se le dice); y una
// conversación que ya existe deja de existir para la persona de fuera —ni se lista ni
// se entra en ella, ni se acepta su invitación— en cuanto cualquiera de las dos
// organizaciones deja de admitir a la otra.
func TestPoliticaDeContactoExterno(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	s := New(config.Config{MaxMessageBytes: 4096, MessagePageSize: 50}, st, nil, nil)
	s.SetDirectorio(directorioFijo{
		"ana":   {UserID: "ana", Name: "Ana Acme", Username: "ana"},
		"bea":   {UserID: "bea", Name: "Bea Otra", Username: "bea"},
		"carla": {UserID: "carla", Name: "Carla Tercera", Username: "carla"},
	})
	s.SetOrganizacionesDePersona(func(_ context.Context, sub string) ([]string, bool, error) {
		return map[string][]string{"bea": {"otra"}, "carla": {"tercera"}}[sub], false, nil
	})
	acme := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	otra := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "otra"})
	equipos, err := s.EnsureDefaultTeams(acme, "ana", "Ana")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureDefaultTeams(otra, "bea", "Bea"); err != nil {
		t.Fatal(err)
	}
	guardar := func(ctx context.Context, p domain.PoliticaContactoExterno) {
		t.Helper()
		if _, err := s.GuardarPoliticaContactoExterno(ctx, p, "admin"); err != nil {
			t.Fatalf("guardar la política: %v", err)
		}
	}

	// Cerrada en acme: Ana no abre nada con nadie de fuera, y se le dice.
	guardar(acme, domain.PoliticaContactoExterno{Modo: domain.ContactoCerrado})
	if _, err := s.OpenGroupDirect(acme, equipos[0].ID, "ana", []string{"bea"}); !errors.Is(err, domain.ErrContactoExternoCerrado) {
		t.Fatalf("DM con la organización cerrada: %v", err)
	}
	canal, err := s.CreateChannel(acme, CreateChannelInput{TeamID: equipos[0].ID, Name: "Obra", Kind: domain.ChannelPrivate, Creator: "ana"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddChannelMember(acme, canal.ID, "ana", "bea", ""); !errors.Is(err, domain.ErrContactoExternoCerrado) {
		t.Fatalf("añadir a alguien de fuera con la organización cerrada: %v", err)
	}

	// De confianza con «otra»: Bea sí; Carla, de «tercera», no.
	guardar(acme, domain.PoliticaContactoExterno{Modo: domain.ContactoConfianza, Confianza: []string{"otra"}})
	if _, err := s.AddChannelMember(acme, canal.ID, "ana", "carla", ""); !errors.Is(err, domain.ErrContactoExternoCerrado) {
		t.Fatalf("alguien de una organización que no es de confianza: %v", err)
	}
	dm, err := s.OpenGroupDirect(acme, equipos[0].ID, "ana", []string{"bea"})
	if err != nil {
		t.Fatalf("DM con alguien de confianza: %v", err)
	}
	if inv, err := s.Invitaciones(otra, "bea"); err != nil || len(inv) != 1 {
		t.Fatalf("la invitación de una organización que la admite se ve: %+v %v", inv, err)
	}

	// Ahora cierra la de Bea: la invitación no se ve ni se acepta.
	guardar(otra, domain.PoliticaContactoExterno{Modo: domain.ContactoCerrado})
	if inv, err := s.Invitaciones(otra, "bea"); err != nil || len(inv) != 0 {
		t.Fatalf("con su organización cerrada la invitación se ve: %+v %v", inv, err)
	}
	if err := s.AceptarInvitacion(otra, "bea", "Bea", dm.ID); !errors.Is(err, domain.ErrContactoExternoCerrado) {
		t.Fatalf("aceptar con su organización cerrada: %v", err)
	}

	// Abierta otra vez: acepta y la conversación es suya…
	guardar(otra, domain.PoliticaContactoExterno{Modo: domain.ContactoAbierto})
	if err := s.AceptarInvitacion(otra, "bea", "Bea", dm.ID); err != nil {
		t.Fatalf("aceptar: %v", err)
	}
	if a, err := s.Anfitrion(otra, "bea", RefAnfitrion{Canal: dm.ID}); err != nil || a != "acme" {
		t.Fatalf("Anfitrion con las dos abiertas: %q %v", a, err)
	}
	// …hasta que acme deja de confiar en «otra»: ni se lista ni se entra.
	guardar(acme, domain.PoliticaContactoExterno{Modo: domain.ContactoConfianza, Confianza: []string{"tercera"}})
	if a, err := s.Anfitrion(otra, "bea", RefAnfitrion{Canal: dm.ID}); err != nil || a != "" {
		t.Fatalf("Anfitrion sin la confianza de la anfitriona: %q %v", a, err)
	}
	if canales, _, err := s.ListExternalChannels(otra, "bea"); err != nil || len(canales) != 0 {
		t.Fatalf("la lista de fuera la enseña: %+v %v", canales, err)
	}
	// Y lo mismo si la que cierra es la suya.
	guardar(acme, domain.PoliticaContactoExterno{Modo: domain.ContactoAbierto})
	guardar(otra, domain.PoliticaContactoExterno{Modo: domain.ContactoConfianza, Confianza: []string{"tercera"}})
	if a, _ := s.Anfitrion(otra, "bea", RefAnfitrion{Canal: dm.ID}); a != "" {
		t.Fatalf("Anfitrion sin la confianza de la suya: %q", a)
	}
	guardar(otra, domain.PoliticaContactoExterno{Modo: domain.ContactoAbierto})
	if canales, _, err := s.ListExternalChannels(otra, "bea"); err != nil || len(canales) != 1 {
		t.Fatalf("abiertas las dos, vuelve: %+v %v", canales, err)
	}

	// La política se guarda limpia.
	p, err := s.GuardarPoliticaContactoExterno(acme, domain.PoliticaContactoExterno{
		Modo: domain.ContactoConfianza, Confianza: []string{"Otra", "acme", "otra"},
	}, "admin")
	if err != nil || len(p.Confianza) != 1 || p.Confianza[0] != "otra" || p.Actualizada == nil {
		t.Fatalf("guardar limpia: %+v %v", p, err)
	}
	var mala *OrganizacionNoValida
	if _, err := s.GuardarPoliticaContactoExterno(acme, domain.PoliticaContactoExterno{
		Modo: domain.ContactoConfianza, Confianza: []string{"no vale"},
	}, "admin"); !errors.As(err, &mala) || mala.Org != "no vale" {
		t.Fatalf("un nombre que no es de organización: %v", err)
	}
}
