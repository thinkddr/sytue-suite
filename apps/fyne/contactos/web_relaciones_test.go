// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"reflect"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func conWebRelaciones() api.Contact {
	c := fichaDeMuestra()
	boda := time.Date(2012, 6, 9, 0, 0, 0, 0, time.UTC)
	c.URLs = []api.ContactURL{{URL: "https://acme.example", Type: "work"}}
	c.Related = []api.ContactRelated{{Name: "William", Type: "spouse"}, {Name: "Mary"}}
	c.Anniversary, c.ETag = &boda, "e1"
	return c
}

// La ficha enseña el sitio web, el aniversario y las personas relacionadas, como la web.
func TestLaFichaEnsenaWebRelacionesYAniversario(t *testing.T) {
	p := pantallaDeMuestra(t)
	c := conWebRelaciones()
	p.Seleccionado = &c
	textos := textosDe(p.ficha())
	for _, debe := range []string{
		"Sitio web", "https://acme.example", "Aniversario", "9 de junio de 2012",
		"Relaciones", "William", "Cónyuge", "Mary",
	} {
		if !contieneTexto(textos, debe) {
			t.Errorf("la ficha no dice %q: %q", debe, textos)
		}
	}
	sin := fichaDeMuestra()
	p.Seleccionado = &sin
	if contieneTexto(textosDe(p.ficha()), "Aniversario") {
		t.Error("sin aniversario la ficha pinta el campo vacío")
	}
}

// El formulario los carga, los deja cambiar y los reenvía en el PUT; sin esto, editar
// el teléfono desde la app los borraba de la tarjeta.
func TestElFormularioEditaWebRelacionesYAniversario(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{conWebRelaciones()}}
	_, f := formularioDePrueba(t, s, true)
	if len(f.webs) != 1 || f.webs[0].valor.Texto() != "https://acme.example" || len(f.relacionados) != 2 {
		t.Fatalf("filas: %d webs, %d personas", len(f.webs), len(f.relacionados))
	}
	if f.aniversario.Texto() == "" {
		t.Error("el aniversario no se carga")
	}
	f.guardar()
	in := cuerpoDe(t, s, "PUT /api/v1/contacts/ada")
	if !reflect.DeepEqual(in.URLs, []api.ContactURL{{URL: "https://acme.example", Type: "work"}}) ||
		!reflect.DeepEqual(in.Related, []api.ContactRelated{{Name: "William", Type: "spouse"}, {Name: "Mary"}}) ||
		in.Anniversary != "2012-06-09" {
		t.Errorf("PUT: %+v / %+v / %q", in.URLs, in.Related, in.Anniversary)
	}

	// Un sitio web con espacios no se manda: se marca la fila.
	s = &servicioFalso{contactos: []api.Contact{conWebRelaciones()}}
	_, f = formularioDePrueba(t, s, true)
	f.webs[0].valor.PonerTexto("no es web")
	if _, ok := f.entrada(); ok || f.errWeb != 0 {
		t.Errorf("acepta «no es web» (errWeb %d)", f.errWeb)
	}
	if !contieneTexto(textosDe(f.cuerpo), "Introduce una dirección web válida (ejemplo.com).") {
		t.Error("no dice por qué")
	}
}
