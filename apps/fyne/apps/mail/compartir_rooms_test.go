// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestCompartirEnRoomsAbreLaPantallaDeCompartirConElEnlace: «Más acciones» del lector
// ofrece «Compartir en Rooms», y pulsarlo abre Sytue Rooms por su esquema en `/share`
// con origen `mail`, el id del mensaje y, como mensaje propuesto, el asunto y el
// enlace canónico del correo con prefijo y organización —nunca el cuerpo—.
//
// VISTO EN ROJO (2026-11-05): sin el elemento en `menuMas` no se ofrece, y con la
// ruta del correo sin `/messages/` el enlace propuesto lleva a la bandeja.
func TestCompartirEnRoomsAbreLaPantallaDeCompartirConElEnlace(t *testing.T) {
	s := servicioConTodas()
	b := buzonConAvisos(t, s)
	a := &abridorFalso{}
	b.ConHub(nil, a)
	b.Org = func() string { return "acme" }
	b.Arrancar(context.Background())
	b.Abrir(b.P.Bandeja.Conversaciones[0])

	var compartir *ui.ElementoMenu
	for _, o := range b.P.Lector.menuMas().Elementos {
		if e, ok := o.(*ui.ElementoMenu); ok && e.Texto == "Compartir en Rooms" {
			compartir = e
		}
	}
	if compartir == nil {
		t.Fatal("«Más acciones» no ofrece «Compartir en Rooms»")
	}
	compartir.AlPulsar()
	if len(a.abiertas) != 1 {
		t.Fatalf("abiertas: %v", a.abiertas)
	}
	u, err := url.Parse(a.abiertas[0])
	if err != nil || u.Scheme != "sytue-rooms" || u.Path != "/share" {
		t.Fatalf("no abre la pantalla de compartir de Rooms: %v", a.abiertas[0])
	}
	q := u.Query()
	texto := q.Get("text")
	if q.Get("share_source") != "mail" || q.Get("share_id") != "m1" ||
		!strings.HasPrefix(texto, "Calendario\n") || !strings.HasSuffix(texto, "/mail/acme/messages/m1") {
		t.Errorf("lo compartido: %v", q)
	}
	if strings.Contains(texto, "Os paso") {
		t.Error("a la sala va el cuerpo del correo")
	}
}
