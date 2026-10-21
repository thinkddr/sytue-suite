// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"encoding/json"
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func conCorreo(c api.Contact, correos ...string) api.Contact {
	for _, e := range correos {
		c.Emails = append(c.Emails, api.ContactEmail{Email: e})
	}
	return c
}

// TestLosDuplicadosSeAgrupanDeFormaTransitiva es `findDuplicateGroups`: Ana y «ana»
// por el nombre sin mayúsculas ni puntos, Ana y Bea por el correo, Bea y Carla por el
// teléfono (con otro formato): los cuatro son UN grupo. Daniel, solo, no sale; y un
// teléfono de cinco dígitos no une a nadie.
func TestLosDuplicadosSeAgrupanDeFormaTransitiva(t *testing.T) {
	ana := conCorreo(persona("ana", "Ana Ruiz"), "ana@acme.test")
	ana2 := persona("ana2", "ana.ruiz")
	bea := conCorreo(persona("bea", "Bea"), "ANA@acme.test")
	bea.Phones = []api.ContactPhone{{Phone: "+34 600 11 22 33"}, {Phone: "12345"}}
	carla := persona("carla", "Carla")
	carla.Phones = []api.ContactPhone{{Phone: "34600112233"}}
	daniel := persona("daniel", "Daniel")
	daniel.Phones = []api.ContactPhone{{Phone: "12345"}}
	grupos := GruposDuplicados([]api.Contact{ana, ana2, bea, carla, daniel})
	if len(grupos) != 1 || len(grupos[0]) != 4 {
		t.Fatalf("grupos %v", grupos)
	}
	if got := nombres(grupos[0]); !iguales(got, []string{"Ana Ruiz", "ana.ruiz", "Bea", "Carla"}) {
		t.Errorf("el grupo es %v, en el orden de la libreta", got)
	}
}

// TestLaFusionUneSinRepetirYNoPierdeLaOrganizacion es `buildMergeInput`: los correos de
// los dos sin repetir (sin mirar mayúsculas), el nombre elegido y la organización del
// perdedor si el superviviente no tiene.
func TestLaFusionUneSinRepetirYNoPierdeLaOrganizacion(t *testing.T) {
	s := conCorreo(persona("s", "Ana"), "ana@acme.test")
	s.Title = "Directora"
	l := conCorreo(persona("l", "Ana Ruiz"), "ANA@acme.test", "ana@casa.test")
	l.Organization = "Acme"
	in := Fusion(s, l, true, false)
	if in.FormattedName != "Ana Ruiz" || in.Organization != "Acme" || in.Title != "Directora" {
		t.Errorf("nombre %q, organización %q, cargo %q", in.FormattedName, in.Organization, in.Title)
	}
	if len(in.Emails) != 2 || in.Emails[0].Email != "ana@acme.test" || in.Emails[1].Email != "ana@casa.test" {
		t.Errorf("correos %+v", in.Emails)
	}
}

// TestCombinarGuardaRepuntaYBorraEnEseOrden es `performMerge`: primero el superviviente
// (PUT), luego la etiqueta del perdedor pasa al superviviente, y solo al final se borra
// el perdedor.
func TestCombinarGuardaRepuntaYBorraEnEseOrden(t *testing.T) {
	bruno := conCorreo(persona("bruno", "Bruno"), "bruno@acme.test")
	bruno2 := conCorreo(persona("bruno2", "Bruno Díaz"), "bruno@casa.test")
	bruno.UID, bruno2.UID = "u-b1", "u-bruno" // «Amigos» tiene a u-bruno
	s := &servicioFalso{contactos: []api.Contact{bruno, bruno2}}
	a := appDePrueba(t, s)
	a.Cargar()
	f := a.abrirFusion(bruno, a.candidatosDe(bruno))
	if len(f.opciones()) != 1 {
		t.Fatalf("candidatos %v", f.opciones())
	}
	f.perdedor = &bruno2
	f.pintar()
	if !contieneTexto(textosDe(f.cuerpo), "Nombre a conservar") {
		t.Errorf("con nombres distintos no se pregunta cuál se queda: %v", textosDe(f.cuerpo))
	}
	botones := botonesDe(f.pie)
	botones[len(botones)-1].AlPulsar()
	rutas := make([]string, 0, len(s.escrituras))
	for _, e := range s.escrituras {
		rutas = append(rutas, e.ruta)
	}
	quiero := []string{"PUT /api/v1/contacts/bruno", "PUT /api/v1/groups/g1", "DELETE /api/v1/contacts/bruno2"}
	if !iguales(rutas, quiero) {
		t.Fatalf("peticiones %v y el orden es %v", rutas, quiero)
	}
	var in api.ContactInput
	_ = json.Unmarshal([]byte(s.escrituras[0].cuerpo), &in)
	if len(in.Emails) != 2 {
		t.Errorf("el superviviente queda con %+v", in.Emails)
	}
	if s.escrituras[1].cuerpo != `{"members":["u-b1"],"name":"Amigos"}` {
		t.Errorf("la etiqueta queda con %s", s.escrituras[1].cuerpo)
	}
	if av := a.avisos.Avisos(); len(av) != 1 || av[0].Titulo != "Contactos combinados" {
		t.Errorf("avisos %v", av)
	}
}

// TestElEnlaceDelBuscadorAbreLaFicha: `sytue-contacts:/contacts/{id}` (y con el prefijo
// de la app, que es como llega de la dirección de la organización) abre esa ficha en su
// libreta; un enlace a otra cosa no es nuestro.
func TestElEnlaceDelBuscadorAbreLaFicha(t *testing.T) {
	const id = "2b0e7c1a-5d3f-4a8e-9c61-0f4b2d7e8a19"
	for _, crudo := range []string{
		"sytue-contacts:/contacts/" + id,
		"https://corporate.sytue.eu/contacts/contacts/" + id,
	} {
		u, _ := url.Parse(crudo)
		if got := IDDeEnlace(u); got != id {
			t.Errorf("%s da %q", crudo, got)
		}
	}
	for _, crudo := range []string{"sytue-contacts:/", "sytue-contacts:/contacts/no-es-un-id"} {
		u, _ := url.Parse(crudo)
		if got := IDDeEnlace(u); got != "" {
			t.Errorf("%s da %q y no es un enlace de contacto", crudo, got)
		}
	}
	ana, bruno := persona("ana", "Ana"), persona(id, "Bruno")
	a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{ana, bruno}})
	a.Cargar()
	a.AbrirContacto(id)
	if a.Pantalla.Seleccionado == nil || a.Pantalla.Seleccionado.ID != id || !a.Pantalla.DetalleAbierto {
		t.Errorf("tras el enlace la ficha es %v", a.Pantalla.Seleccionado)
	}
}
