// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// La edición local (`edicionlocal`) no tiene sesión de la suite: este test abre una de
// verdad (suiteConSesion) y solo corre en la conectada.

//go:build !edicionlocal

package ui

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestElSelectorDeCuentaGuardaLaForma: el selector de forma va junto al de color, con
// las claves de la web (`account.forma*`). Guarda la elección en los ajustes compartidos
// y la pinta; sin elección enseña la que esté en vigor —la de la organización o Marco—.
//
// VISTO EN ROJO sin `Ajustes.Forma` en lo que el selector escribe.
func TestElSelectorDeCuentaGuardaLaForma(t *testing.T) {
	suite := suiteConSesion(t)
	t.Cleanup(func() { tema.FormaDeLosGuardianes() })
	c := nuevaCuentaBarra(traductorPrueba(t), suite)
	// «Organización», Marco y Carril; sin elección, la primera.
	if s := c.forma(suite.Estado()); len(s.Opciones) != 3 || s.Elegida != 0 || s.Rotulo != c.t.T("account.forma", nil) {
		t.Fatalf("sin elección el selector ofrece «Organización», Marco y Carril y marca la primera: %+v", s)
	}
	c.forma(suite.Estado()).Elegir(2)
	if e := ajustes.Leer(); e.Forma != "carril" || tema.EsMarco() {
		t.Fatalf("elegir Carril ha guardado %q y marco=%v", e.Forma, tema.EsMarco())
	}
	if elegido := c.forma(suite.Estado()).Elegida; elegido != 2 {
		t.Errorf("reabrir el selector marca %d, quiero Carril", elegido)
	}
	c.forma(suite.Estado()).Elegir(1)
	if e := ajustes.Leer(); e.Forma != "marco" || !tema.EsMarco() {
		t.Errorf("elegir Marco ha guardado %q y marco=%v", e.Forma, tema.EsMarco())
	}
	// «Organización» borra la elección: se vuelve a seguir a la suya.
	c.forma(suite.Estado()).Elegir(0)
	if e := ajustes.Leer(); e.Forma != "" || c.forma(suite.Estado()).Elegida != 0 {
		t.Errorf("elegir «Organización» ha dejado la forma en %q; quiero que borre la elección", e.Forma)
	}
	// Una cuenta personal no tiene a quién seguir: no la ve, y sin elección propia el
	// selector marca lo que se está pintando.
	if s := nuevaCuentaBarra(traductorPrueba(t), nil).forma(estadoSesion{Forma: "carril"}); len(s.Opciones) != 2 || s.Elegida != 1 {
		t.Errorf("una cuenta personal ve %d opciones y marca %d; quiero Marco y Carril, y Carril marcado", len(s.Opciones), s.Elegida)
	}
	// Con organización y sin elección propia marca «Organización», la tenga como la tenga.
	if s := nuevaCuentaBarra(traductorPrueba(t), nil).forma(estadoSesion{Forma: "carril", ConOrganizacion: true}); s.Elegida != 0 {
		t.Error("quien sigue a su organización tiene que ver marcada «Organización»")
	}
	for _, locale := range i18n.Locales {
		tr, err := i18n.New(locale, i18n.Common())
		if err != nil {
			t.Fatal(err)
		}
		if s := nuevaCuentaBarra(tr, nil).forma(estadoSesion{}); s.Opciones[0] == "" || s.Opciones[0] == "account.forma.marco" {
			t.Errorf("%s: falta «Marco» traducido: %q", locale, s.Opciones[0])
		}
	}
}
