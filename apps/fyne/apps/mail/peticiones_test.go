// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioConPeticiones es el falso de siempre con la ruta de «pedir acceso».
type servicioConPeticiones struct{ *servicioFalso }

func (s servicioConPeticiones) PeticionesBuzon(cuenta string) api.Peticiones {
	return api.NewMail("http://127.0.0.1:1", nil, nil).PeticionesBuzon(cuenta)
}

func contar(s *servicioFalso, f string) int {
	n := 0
	for _, l := range s.llamadas {
		if strings.HasPrefix(l, f+"(") {
			n++
		}
	}
	return n
}

// Un buzón compartido en "read" lleva la franja con «Pedir acceso»; en "send", no. Y
// cuando el Hub dice que se ha concedido, se vuelven a leer los compartidos.
func TestElBuzonCompartidoDeLecturaPideAccesoYSeDesbloqueaEnVivo(t *testing.T) {
	s := servicioDeEjemplo()
	s.compartidos = []api.MailShared{{AccountID: "c1", Address: "soporte@acme.es", Kind: "shared", Permission: "read"}}
	b, _ := buzonDePrueba(t, servicioConPeticiones{s})
	b.Arrancar(context.Background())
	b.ElegirCompartido(Compartido{ID: "c1"})
	f, ok := b.P.Franja.(*ui.FranjaSoloLectura)
	if !ok || f.Accion == nil {
		t.Fatalf("el buzón en lectura no tiene franja con «Pedir acceso»: %T", b.P.Franja)
	}
	antes := contar(s, "SharedMailboxes")
	s.compartidos[0].Permission = "send"
	ui.RepartirAviso(api.HubNotification{ID: "g", Type: "mail.access.granted", Params: map[string]string{"resource": "c1"}})
	fyne.DoAndWait(func() {})
	if contar(s, "SharedMailboxes") != antes+1 {
		t.Fatal("concedido, no se volvieron a leer los buzones compartidos")
	}
	if b.P.Franja != nil {
		t.Error("con permiso de envío sigue la franja de solo lectura")
	}
}
