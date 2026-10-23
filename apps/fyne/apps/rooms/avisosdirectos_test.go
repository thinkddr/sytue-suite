// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// «Desactivar avisos de mensajes directos» pone en el Hub la regla que calla
// `rooms.message` de Rooms, se lee al arrancar y vuelve si el Hub dice que no. Y ese es
// el tipo que Rooms manda por un directo: si cambia allí, esto se entera aquí.
//
// VISTO EN ROJO (2026-09-24, en el frontal): con el Kind "rooms.mention" en
// PrefAvisoDirectos, el ajuste callaría las menciones y el guardián de la fuente falla.
func TestLosAvisosDeDirectosSonUnaReglaDelHub(t *testing.T) {
	fuente := webref.Read(t, "services/rooms/internal/service/push.go")
	if (!strings.Contains(fuente, `Type:    "`+PrefAvisoDirectos.Kind+`"`) && !strings.Contains(fuente, `notifType := "`+PrefAvisoDirectos.Kind+`"`)) || !strings.Contains(fuente, `Source:  "`+PrefAvisoDirectos.Source+`"`) {
		t.Fatalf("Rooms ya no avisa de un directo con %s/%s", PrefAvisoDirectos.Source, PrefAvisoDirectos.Kind)
	}
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	var puestas []api.HubNotificationPref
	var fallo error
	sa.PrefsAviso = func(context.Context) ([]api.HubNotificationPref, error) {
		return []api.HubNotificationPref{{Source: "rooms", Kind: "rooms.message", Muted: true}}, nil
	}
	sa.PonerPrefAviso = func(_ context.Context, p api.HubNotificationPref) (api.HubNotificationPref, error) {
		puestas = append(puestas, p)
		return p, fallo
	}
	sa.Arrancar(context.Background())
	if !sa.directosCallados {
		t.Fatal("la regla del Hub no se lee al arrancar")
	}
	sa.AlternarAvisosDirectos()
	if sa.directosCallados || len(puestas) != 1 || puestas[0].Muted || puestas[0].Kind != "rooms.message" {
		t.Errorf("activar pone %+v y deja callados=%v", puestas, sa.directosCallados)
	}
	fallo = errors.New("caído")
	sa.AlternarAvisosDirectos()
	if sa.directosCallados {
		t.Error("con el Hub caído el ajuste no vuelve")
	}
}

func TestDirectAlertRuleHasTheSameSpecificityAsHub(t *testing.T) {
	for _, test := range []struct {
		name  string
		prefs []api.HubNotificationPref
		muted bool
	}{
		{"default", nil, false},
		{"source mute", []api.HubNotificationPref{{Source: "rooms", Muted: true}}, true},
		{"exact enable overrides source", []api.HubNotificationPref{{Source: "rooms", Muted: true}, {Source: "rooms", Kind: "rooms.message"}}, false},
		{"exact mute first", []api.HubNotificationPref{{Source: "rooms", Kind: "rooms.message", Muted: true}, {Source: "rooms"}}, true},
		{"other kinds unchanged", []api.HubNotificationPref{{Source: "mail", Muted: true}, {Source: "rooms", Kind: "rooms.mention", Muted: true}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := directosCalladosEn(test.prefs); got != test.muted {
				t.Fatalf("direct-message rule=%v, Hub specificity requires %v", got, test.muted)
			}
		})
	}
}
