// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"fmt"
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func conCorreos(c api.Contact, es ...api.ContactEmail) api.Contact {
	c.Emails = es
	return c
}

// TestCorreosDe: los mismos casos que seleccion.test.ts. El principal de cada uno, sin
// repetir (sin mirar mayúsculas), sin quien no tiene correo y hasta el tope.
func TestCorreosDe(t *testing.T) {
	ana := conCorreos(persona("ana", "Ana"), api.ContactEmail{Email: "ana@casa.eu", Type: "home"},
		api.ContactEmail{Email: "ana@obra.eu", Type: "work", IsPrimary: true})
	ana2 := conCorreos(persona("ana2", "Ana"), api.ContactEmail{Email: "ANA@obra.eu"})
	luis := conCorreos(persona("luis", "Luis"), api.ContactEmail{Email: "luis@x.eu"})
	got, recortado := CorreosDe([]api.Contact{ana, ana2, luis, persona("sin", "Sin")})
	if !iguales(got, []string{"ana@obra.eu", "luis@x.eu"}) || recortado {
		t.Errorf("correos %v (recortado %v)", got, recortado)
	}
	muchos := make([]api.Contact, TopeDestinatarios+5)
	for i := range muchos {
		muchos[i] = conCorreos(persona(fmt.Sprint(i), "x"), api.ContactEmail{Email: fmt.Sprintf("c%d@x.eu", i)})
	}
	if got, recortado := CorreosDe(muchos); len(got) != TopeDestinatarios || !recortado {
		t.Errorf("con %d salen %d (recortado %v)", len(muchos), len(got), recortado)
	}
}

// TestCorreoATodosDesdeLaBarra: «Enviar correo a todos» de la barra abre el redactor de
// Mail con los marcados en «Para»; sin ningún correo no abre nada y lo dice.
func TestCorreoATodosDesdeLaBarra(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{
		conCorreos(persona("ana", "Ana"), api.ContactEmail{Email: "ana@x.eu"}),
		conCorreos(persona("bruno", "Bruno"), api.ContactEmail{Email: "bruno@x.eu"}),
		persona("carla", "Carla"),
	}}
	a := appDePrueba(t, s)
	ab := &abridorFalso{}
	a.Abridor = ab
	a.Cargar()
	a.Pantalla.AlMarcar("ana")
	a.Pantalla.AlMarcar("bruno")
	b := a.Pantalla.barraLote()
	if b == nil || b.Acciones[0].Texto != "Enviar correo a todos" {
		t.Fatalf("la barra es %v", b)
	}
	b.Acciones[0].AlPulsar()
	if len(ab.urls) != 1 {
		t.Fatalf("se abrió %v", ab.urls)
	}
	u, err := url.Parse(ab.urls[0])
	if err != nil || u.Scheme != "sytue-mail" || u.Query().Get("action") != "compose" ||
		u.Query().Get("to") != "ana@x.eu, bruno@x.eu" {
		t.Errorf("se abrió %q", ab.urls[0])
	}

	clear(a.marcados)
	a.Pantalla.AlMarcar("carla")
	a.Pantalla.barraLote().Acciones[0].AlPulsar()
	if len(ab.urls) != 1 {
		t.Errorf("sin correos se abrió %v", ab.urls[1:])
	}
	var dicho bool
	for _, av := range a.avisos.Avisos() {
		dicho = dicho || (av.Tono == ui.AvisoInfo && av.Titulo == a.Pantalla.t("batch.noEmail", nil))
	}
	if !dicho {
		t.Error("sin correos no se dice nada")
	}
}

// TestExportarSeleccionYEtiqueta: «Exportar (.vcf)» de la barra deja solo las tarjetas
// de los marcados; el del menú de la etiqueta, las de quien la lleva, con su nombre.
func TestExportarSeleccionYEtiqueta(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana"), persona("bruno", "Bruno"), persona("carla", "Carla")}}
	a := appDePrueba(t, s)
	guardados := map[string]string{}
	a.Descargas = func(nombre string, contenido []byte) error {
		guardados[nombre] = string(contenido)
		return nil
	}
	a.Cargar()
	a.Pantalla.AlMarcar("ana")
	a.Pantalla.AlMarcar("carla")
	a.Pantalla.barraLote().Acciones[1].AlPulsar()
	tarjeta := func(id string) string { return "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:" + id + "\r\nEND:VCARD\r\n" }
	if got := guardados["contactos.vcf"]; got != tarjeta("ana")+tarjeta("carla") {
		t.Errorf("el .vcf de la selección es %q", got)
	}

	a.Pantalla.EtiquetaActiva = "g1"
	a.Pantalla.AlExportarEtiqueta()
	if got := guardados["Amigos.vcf"]; got != tarjeta("bruno") {
		t.Errorf("el .vcf de la etiqueta es %q (guardados %v)", got, guardados)
	}
}

// TestElMenuDeLaEtiquetaSeApagaSinMiembrosCargados: como la web, el correo y el .vcf
// del menú se apagan si nadie de la libreta lleva la etiqueta, aunque su MEMBER nombre
// a alguien que ya no está.
func TestElMenuDeLaEtiquetaSeApagaSinMiembrosCargados(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}})
	a.Cargar()
	a.Pantalla.EtiquetaActiva = "g1" // MEMBER: u-bruno, que no está en la libreta
	if !a.Pantalla.etiquetaSinMiembros() {
		t.Error("la etiqueta nombra a alguien que no está y el menú sale encendido")
	}
	b := appDePrueba(t, &servicioFalso{contactos: []api.Contact{persona("ana", "Ana"), persona("bruno", "Bruno")}})
	b.Cargar()
	b.Pantalla.EtiquetaActiva = "g1"
	if b.Pantalla.etiquetaSinMiembros() {
		t.Error("bruno lleva la etiqueta y el menú sale apagado")
	}
}
