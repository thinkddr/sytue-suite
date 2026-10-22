// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestOrganizacionCreaAQuienFaltaYNoTocaLoQueHay es `POST /api/directorio` de la web: al
// entrar se crea en «Organización» a quien falte, con TODAS sus direcciones; a quien ya
// está por cualquiera de ellas no se le toca, a uno mismo no se le añade, y la libreta
// que se abre sigue siendo la predeterminada.
func TestOrganizacionCreaAQuienFaltaYNoTocaLoQueHay(t *testing.T) {
	s := &servicioFalso{
		contactos: []api.Contact{persona("ana", "Ana")},
		directorio: []api.Miembro{
			{ID: "yo", Email: "yo@acme.es", Nombre: "Yo"},
			{ID: "p1", Email: "ana@acme.es", Emails: []string{"ana@acme.es", "ana.ruiz@acme.es"}, Nombre: "Ana"},
			{
				ID: "p2", Email: "bruno@acme.es", Emails: []string{"bruno@acme.es", "BRUNO@acme.es", "b@acme.es"},
				Nombre: "Bruno Gil", Departamento: "Ventas", Puesto: "Comercial",
			},
			{ID: "p3", Emails: []string{"sin-principal@acme.es"}},
			{ID: "p4", Email: "carla@acme.es"},
		},
		organizacion: []api.Contact{{ID: "o1", FormattedName: "Ana R.", Emails: []api.ContactEmail{{Email: "ANA.RUIZ@acme.es"}}}},
	}
	a := appDePrueba(t, s)
	a.Cargar()
	var altas []api.ContactInput
	for _, e := range s.escrituras {
		if e.ruta != "POST /api/v1/address-books/l2/contacts" {
			t.Errorf("escritura inesperada: %v", e)
			continue
		}
		var in api.ContactInput
		if err := json.Unmarshal([]byte(e.cuerpo), &in); err != nil {
			t.Fatal(err)
		}
		altas = append(altas, in)
	}
	if len(altas) != 2 {
		t.Fatalf("altas %+v", altas)
	}
	bruno, carla := altas[0], altas[1]
	if bruno.FormattedName != "Bruno Gil" || bruno.Organization != "Ventas" || bruno.Title != "Comercial" ||
		len(bruno.Emails) != 2 || bruno.Emails[0] != (api.ContactEmail{Email: "bruno@acme.es", Type: "work", IsPrimary: true}) ||
		bruno.Emails[1] != (api.ContactEmail{Email: "b@acme.es", Type: "work"}) {
		t.Errorf("Bruno: %+v", bruno)
	}
	if carla.FormattedName != "carla@acme.es" {
		t.Errorf("sin nombre, la ficha se llama por su correo: %+v", carla)
	}
	if a.Pantalla.Activa == nil || a.Pantalla.Activa.ID != "l1" {
		t.Errorf("se abre %+v", a.Pantalla.Activa)
	}
}

// TestOrganizacionSeEstrenaUnaVez: sin la libreta, se crea en su ruta fija, con el
// rótulo en la lengua de quien la estrena y el color de la web.
func TestOrganizacionSeEstrenaUnaVez(t *testing.T) {
	s := &servicioFalso{sinOrganizacion: true, directorio: []api.Miembro{}}
	a := appDePrueba(t, s)
	a.Cargar()
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != "POST /api/v1/address-books" ||
		s.escrituras[0].cuerpo != `{"color":"#8A6A2F","display_name":"Organización","name":"organizacion"}` {
		t.Fatalf("escrituras %v", s.escrituras)
	}
}
