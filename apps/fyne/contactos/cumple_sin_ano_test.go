// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El cumpleaños SIN año (`birthday_no_year`, migración 0007 del servicio) cuenta en
// «Próximos cumpleaños», sin edad, igual que `proximosCumpleanos` de la web; un
// «--MM-DD» que no es un día de verdad se ignora sin ruido, como allí.
func TestLosCumpleanosSinAnoTambienCuentan(t *testing.T) {
	hoy := time.Date(2026, 2, 10, 9, 0, 0, 0, time.Local)
	con := func(id, sinAno string) api.Contact {
		c := persona(id, id)
		c.BirthdayNoYear = sinAno
		return c
	}
	cs := ProximosCumpleanos([]api.Contact{
		con("febrero", "--02-15"), con("hoy", "--02-10"), con("bisiesto", "--02-29"),
		con("imposible", "--02-31"), con("roto", "15 feb"),
	}, hoy, 365)
	if len(cs) != 3 || cs[0].Contacto.ID != "hoy" || cs[1].Contacto.ID != "febrero" || cs[2].Contacto.ID != "bisiesto" {
		t.Fatalf("cumpleaños: %+v", cs)
	}
	if cs[1].Dias != 5 || cs[1].Edad != 0 {
		t.Errorf("el del 15 de febrero: %d días, edad %d; quiero 5 y sin edad", cs[1].Dias, cs[1].Edad)
	}
	// 2026 no es bisiesto: cae el 1 de marzo, como `Date` en la web.
	if cs[2].Fecha.Month() != time.March || cs[2].Fecha.Day() != 1 {
		t.Errorf("el 29 de febrero cae el %s", cs[2].Fecha.Format(time.DateOnly))
	}
}

// La ficha lo enseña como «15 de febrero», sin año: `formatearCumpleanos` de la web.
func TestLaFichaEnsenaElCumpleanosSinAno(t *testing.T) {
	p := pantallaDeMuestra(t)
	c := fichaDeMuestra()
	c.Birthday, c.BirthdayNoYear = nil, "--02-15"
	p.Seleccionado = &c
	textos := textosDe(p.ficha())
	if !contieneTexto(textos, "15 de febrero") || !contieneTexto(textos, "Cumpleaños") {
		t.Errorf("la ficha no dice «Cumpleaños: 15 de febrero»: %q", textos)
	}
	c.BirthdayNoYear = "--02-29"
	if got := p.cumpleDeLaFicha(c); got != "29 de febrero" {
		t.Errorf("el 29 de febrero sale %q", got)
	}
	c.BirthdayNoYear = ""
	if got := p.cumpleDeLaFicha(c); got != "" {
		t.Errorf("sin cumpleaños la ficha dice %q", got)
	}
}

// Al editar, el formulario avisa de que hay un cumpleaños sin año (el campo de fecha
// no puede enseñarlo) y lo reenvía tal cual; elegir una fecha lo sustituye.
func TestEditarConservaElCumpleanosSinAno(t *testing.T) {
	c := fichaDeMuestra()
	c.Birthday, c.BirthdayNoYear, c.ETag = nil, "--02-15", "e1"
	s := &servicioFalso{contactos: []api.Contact{c}}
	_, f := formularioDePrueba(t, s, true)
	if f.cumple.Texto() != "" {
		t.Errorf("el campo de fecha dice %q", f.cumple.Texto())
	}
	if !contieneTexto(textosDe(f.cuerpo), "Hay un cumpleaños sin año guardado; elige una fecha para reemplazarlo.") {
		t.Error("el formulario no avisa del cumpleaños sin año")
	}
	f.guardar()
	if in := cuerpoDe(t, s, "PUT /api/v1/contacts/ada"); in.BirthdayNoYear != "--02-15" || in.Birthday != "" {
		t.Errorf("guardar sin tocarlo: %q / %q", in.Birthday, in.BirthdayNoYear)
	}

	s = &servicioFalso{contactos: []api.Contact{c}}
	_, f = formularioDePrueba(t, s, true)
	f.cumple.PonerTexto("15/02/1990")
	f.guardar()
	if in := cuerpoDe(t, s, "PUT /api/v1/contacts/ada"); in.Birthday != "1990-02-15" || in.BirthdayNoYear != "" {
		t.Errorf("con fecha: %q / %q", in.Birthday, in.BirthdayNoYear)
	}
}

// Sin cumpleaños sin año, el campo no lleva ayuda: la web solo la enseña entonces.
func TestElCampoDeFechaSinAyudaDeMas(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{fichaDeMuestra()}}
	_, f := formularioDePrueba(t, s, true)
	if f.ayudaCumple() != "" {
		t.Errorf("con fecha completa la ayuda dice %q", f.ayudaCumple())
	}
}
