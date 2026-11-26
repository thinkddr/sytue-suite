// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaBusquedaMiraApodoCargoCiudadYNota: «la abogada», «Pili» o «el de Zaragoza»
// encuentran a quien se busca, como en la web (`textoBuscable`).
func TestLaBusquedaMiraApodoCargoCiudadYNota(t *testing.T) {
	ana := persona("ana", "Ana Ruiz")
	ana.Nickname, ana.Title, ana.Note = "Pili", "Abogada", "Conocida en el congreso"
	ana.Addresses = []api.ContactAddress{{Locality: "Zaragoza"}}
	otros := []api.Contact{persona("luis", "Luis"), ana}
	for _, q := range []string{"pili", "ABOGADA", "zaragoza", "congreso"} {
		if got := filtrar(otros, q, nil, false, false, nil); len(got) != 1 || got[0].ID != "ana" {
			t.Errorf("«%s» encuentra %v", q, got)
		}
	}
}

// TestLosAniversariosSalenConLosCumpleanos: el ANNIVERSARY entra en «Próximos
// cumpleaños» con su marca y sin edad.
func TestLosAniversariosSalenConLosCumpleanos(t *testing.T) {
	hoy := time.Date(2026, 11, 25, 10, 0, 0, 0, time.Local)
	boda := time.Date(2015, 11, 27, 0, 0, 0, 0, time.UTC)
	nac := time.Date(1990, 11, 26, 0, 0, 0, 0, time.UTC)
	c := persona("ana", "Ana")
	c.Birthday, c.Anniversary = &nac, &boda
	l := ProximosCumpleanos([]api.Contact{c}, hoy, 30)
	if len(l) != 2 || l[0].Aniversario || l[0].Edad != 36 || !l[1].Aniversario || l[1].Edad != 0 || l[1].Dias != 2 {
		t.Fatalf("próximos = %+v", l)
	}
}

// TestOrdenarPorApellido: el orden y las cabeceras por la inicial del apellido; sin
// apellido, por el nombre; y se guarda por persona.
func TestOrdenarPorApellido(t *testing.T) {
	zoe, ana, acme := persona("zoe", "Zoe Abad"), persona("ana", "Ana Zubiri"), persona("acme", "Acme")
	zoe.FamilyName, ana.FamilyName = "Abad", "Zubiri"
	lista := []api.Contact{ana, acme, zoe}
	ordenar(lista, "es", true)
	if ids := []string{lista[0].ID, lista[1].ID, lista[2].ID}; !slices.Equal(ids, []string{"zoe", "acme", "ana"}) {
		t.Errorf("por apellido: %v", ids)
	}
	ordenar(lista, "es", false)
	if ids := []string{lista[0].ID, lista[1].ID, lista[2].ID}; !slices.Equal(ids, []string{"acme", "ana", "zoe"}) {
		t.Errorf("por nombre: %v", ids)
	}

	s := &servicioFalso{contactos: []api.Contact{ana, acme, zoe}}
	a := appDePrueba(t, s)
	alm := &almacenDePrueba{}
	a.Almacen = alm
	a.Cargar()
	a.cambiarOrden()
	if !a.Pantalla.PorApellido || alm.v[ClaveOrden] != "apellido" || a.todos[0].ID != "zoe" {
		t.Fatalf("cambiar el orden: %v, %v, %s", a.Pantalla.PorApellido, alm.v, a.todos[0].ID)
	}
	a.restaurarOrden()
	if !a.Pantalla.PorApellido {
		t.Error("el orden guardado no se restaura")
	}
}

// TestCopiarLosCorreosDeLaSeleccion: el principal de cada uno, sin repetir ni tope, al
// portapapeles.
func TestCopiarLosCorreosDeLaSeleccion(t *testing.T) {
	cs := make([]api.Contact, 0, TopeDestinatarios+6)
	for i := range TopeDestinatarios + 5 {
		c := persona(string(rune('a'+i%26))+strings.Repeat("x", i), "P")
		c.Emails = []api.ContactEmail{{Email: c.ID + "@sytue.eu"}}
		cs = append(cs, c)
	}
	cs = append(cs, cs[0])
	if got := CorreosParaCopiar(cs); len(got) != TopeDestinatarios+5 {
		t.Fatalf("copiaría %d correos", len(got))
	}
	a := appDePrueba(t, &servicioFalso{contactos: cs})
	a.copiarCorreos(cs[:2])
	if got := fyne.CurrentApp().Clipboard().Content(); got != cs[0].Emails[0].Email+", "+cs[1].Emails[0].Email {
		t.Errorf("portapapeles = %q", got)
	}
}

// TestAlmohadillaMandaALaPapelera: «#» sobre la ficha abierta va por el lote de la
// papelera (con su «Deshacer»).
func TestAlmohadillaMandaALaPapelera(t *testing.T) {
	ana := persona("ana", "Ana")
	s := &servicioFalso{contactos: []api.Contact{ana, persona("luis", "Luis")}}
	a := appDePrueba(t, s)
	a.Cargar()
	a.elegir(ana)
	a.Atajo('#')
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != "POST /api/v1/contacts/batch/trash" {
		t.Fatalf("«#» hizo %+v", s.escrituras)
	}
}

type almacenDePrueba struct{ v map[string]string }

func (a *almacenDePrueba) Leer(k string) string { return a.v[k] }
func (a *almacenDePrueba) Poner(k, v string) {
	if a.v == nil {
		a.v = map[string]string{}
	}
	a.v[k] = v
}

// TestLaFichaEnTextoParaCompartir: lo mismo que `textoDeFicha` de la web, línea a línea.
func TestLaFichaEnTextoParaCompartir(t *testing.T) {
	c := persona("ana", "Ana Ruiz")
	c.Title, c.Organization = "Abogada", "Ruiz y Asociados"
	c.Emails = []api.ContactEmail{{Email: "ana@ruiz.eu"}}
	c.Phones = []api.ContactPhone{{Phone: "+34 600 000 000"}}
	c.Addresses = []api.ContactAddress{{Street: "Calle Mayor 1", PostalCode: "50001", Locality: "Zaragoza", Country: "España"}}
	want := "Ana Ruiz\nAbogada · Ruiz y Asociados\nana@ruiz.eu\n+34 600 000 000\nCalle Mayor 1, 50001 Zaragoza, España"
	if got := TextoDeFicha(c); got != want {
		t.Errorf("ficha =\n%s\nwant\n%s", got, want)
	}
	if got := TextoDeFicha(persona("x", "Solo nombre")); got != "Solo nombre" {
		t.Errorf("sin datos = %q", got)
	}
}
