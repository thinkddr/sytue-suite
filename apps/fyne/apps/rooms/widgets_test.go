// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

type servicioWidgets struct {
	Servicio
	errUnread, errExternal error
	lateAccount            bool
}

func (s *servicioWidgets) Teams(ctx context.Context) ([]api.RoomsTeam, error) {
	teams, err := s.Servicio.Teams(ctx)
	if s.lateAccount {
		ajustes.MarcarSesion()
	}
	return teams, err
}

func (s *servicioWidgets) Unread(ctx context.Context, team string) ([]api.RoomsUnread, error) {
	rows, err := s.Servicio.Unread(ctx, team)
	if s.errUnread != nil {
		return nil, s.errUnread
	}
	return rows, err
}

func (s *servicioWidgets) ExternalChannels(ctx context.Context) ([]api.RoomsChannel, []api.RoomsUnread, error) {
	channels, _, err := s.Servicio.ExternalChannels(ctx)
	if s.errExternal != nil {
		return nil, nil, s.errExternal
	}
	return channels, []api.RoomsUnread{{ChannelID: "outside", Unread: 2}}, err
}

func TestRoomsWidgetReusesCompleteInventoryAndReadReceipts(t *testing.T) {
	t.Cleanup(widgets.SimularLector(true))
	for _, name := range []string{"complete", "unread failed", "external failed", "late account"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("HOME", t.TempDir())
			ajustes.MarcarSesion()
			service := servicioConDos()
			service.fuera = []api.RoomsChannel{{ID: "outside", Name: "private channel name", External: true}}
			adapter := &servicioWidgets{Servicio: service, lateAccount: name == "late account"}
			if name == "unread failed" {
				adapter.errUnread = errors.New("unavailable")
			}
			if name == "external failed" {
				adapter.errExternal = errors.New("unavailable")
			}
			sa, _ := salaDePrueba(t, service)
			sa.S, sa.ctx = adapter, context.Background()
			sa.Widget = func() widgets.View {
				return widgets.NewView("eu.sytue.rooms", "rooms", widgets.Capture(ajustes.Leer().Sesion))
			}
			sa.cargarCanales()
			if service.veces("Channels(t1)") != 1 || service.veces("Unread(t1)") != 1 || service.veces("ExternalChannels()") != 1 {
				t.Fatalf("widget added inventory reads: %+v", service.llamadas)
			}
			snapshot, err := widgets.Read("eu.sytue.rooms")
			if name == "external failed" || name == "late account" {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("published partial or foreign inventory: %+v %v", snapshot, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if name == "unread failed" {
				if len(snapshot.Counters) != 1 || snapshot.Counters[0] != (widgets.Counter{Kind: "channels", Value: 3}) {
					t.Fatalf("failed unread became zero unread: %+v", snapshot)
				}
				return
			}
			// The first channel was opened and acknowledged as read. Its previous
			// unread value must disappear, while the direct and outside channel remain.
			want := []widgets.Counter{{Kind: "unread", Value: 6}, {Kind: "channels", Value: 3}}
			if len(snapshot.Counters) != 2 || snapshot.Counters[0] != want[0] || snapshot.Counters[1] != want[1] {
				t.Fatalf("channel/read count: %+v", snapshot)
			}
			directory, err := ajustes.Carpeta()
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(directory, "widgets", "eu.sytue.rooms.json"))
			if err != nil || strings.Contains(string(data), "private channel name") || strings.Contains(string(data), "Ana") || strings.Contains(string(data), "hola") {
				t.Fatalf("widget must contain counters only: %q %v", data, err)
			}
			service.pend[1].Unread = 9
			sa.refrescarPendientes()
			snapshot, err = widgets.Read("eu.sytue.rooms")
			if err != nil || snapshot.Counters[0].Value != 11 || service.veces("Unread(t1)") != 2 || service.veces("ExternalChannels()") != 2 {
				t.Fatalf("real-time refresh must reuse the existing reads: %+v %v %+v", snapshot, err, service.llamadas)
			}
		})
	}
}

func TestRoomsWidgetHonorsMutedMentionsAndDeduplicatesChannels(t *testing.T) {
	t.Cleanup(widgets.SimularLector(true))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	ajustes.MarcarSesion()
	view := widgets.NewView("eu.sytue.rooms", "rooms", widgets.Capture(ajustes.Leer().Sesion))
	sa := &Sala{
		ctx: context.Background(), Lanzar: func(f func()) { f() },
		Widget:                 func() widgets.View { return view },
		widgetCanalesCompletos: true, widgetPendientesCompletos: true,
		canales: []api.RoomsChannel{
			{ID: "normal"},
			{ID: "normal"},
			{ID: "muted", Muted: true},
			{ID: "mentions", NotifyLevel: "mentions"},
			{ID: "none", NotifyLevel: "none"},
			{ID: "archived", Archived: true},
		},
		pend: map[string]api.RoomsUnread{
			"normal": {Unread: 4}, "muted": {Unread: 90}, "mentions": {Unread: 50, Mentions: 2},
			"none": {Unread: 70}, "archived": {Unread: 80},
		},
	}
	sa.publicarWidget(view)
	snapshot, err := widgets.Read("eu.sytue.rooms")
	if err != nil || len(snapshot.Counters) != 2 || snapshot.Counters[0].Value != 6 || snapshot.Counters[1].Value != 4 {
		t.Fatalf("must match Rooms notification preferences: %+v %v", snapshot, err)
	}
}
