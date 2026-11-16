// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los motivos técnicos de los dos lienzos se dicen en llano: un dibujo trae su <figure> y
// su imagen de respaldo, y eso es «dibujos», no «parte del formato»; un <iframe> es contenido
// incrustado; la protección de Diapos, protección. Lo que no tiene nombre llano, «parte de
// su contenido o formato», que dice algo en vez de callar.
func TestLosMotivosDeSoloLecturaSeDicenEnLlano(t *testing.T) {
	casos := []struct {
		motivos []string
		quiero  []string
	}{
		{[]string{"<figure>", "img[data-dibujo-respaldo]", "img[hidden]"}, []string{"editor.readOnly.what.drawing"}},
		{[]string{"<iframe>", "div[data-embed-live]"}, []string{"editor.readOnly.what.embed"}},
		// Los bloques atómicos fuera de su sitio (DOCU-02): su motivo lleva la marca.
		{[]string{"figure[data-dibujo] dentro de otro"}, []string{"editor.readOnly.what.drawing"}},
		{[]string{"div[data-sytue-embed] dentro de otro"}, []string{"editor.readOnly.what.embed"}},
		{[]string{"span[data-formfield]", "span[data-name]"}, []string{"editor.readOnly.what.formFields"}},
		{[]string{"protect"}, []string{"editor.readOnly.what.protection"}},
		// En el orden de la frase, no en el de los motivos.
		{
			[]string{"span[data-formfield]", "<iframe>", "img[data-dibujo-respaldo]"},
			[]string{"editor.readOnly.what.drawing", "editor.readOnly.what.embed", "editor.readOnly.what.formFields"},
		},
		{[]string{"<table> en <td>"}, []string{"editor.readOnly.what.other"}},
		{nil, []string{"editor.readOnly.what.other"}},
	}
	for _, c := range casos {
		if got := ClavesNoEditable(c.motivos); !slices.Equal(got, c.quiero) {
			t.Errorf("%v → %v; quiero %v", c.motivos, got, c.quiero)
		}
	}
}

// Un documento que el lienzo no entiende tiene su franja: dice qué no sabe editar —no
// «Solo lectura», que suena a permisos— y su botón lleva a la web. Pedir acceso no
// sale: el permiso no lo arreglaría. Sin dirección de la web (un fichero suelto), sin
// botón. Y SinMotivo devuelve la franja de siempre, la del permiso.
func TestLaFranjaDeLoQueNoSabeEditarLlevaALaWeb(t *testing.T) {
	textos := map[string]string{
		MotivoNoSabeEditar:                "No sé editar ({what}).",
		"editor.readOnly.unsupportedHere": "No sé editar ({what}), y aquí no hay web.",
		"editor.readOnly.what.drawing":    "dibujos",
		"editor.readOnly.what.protection": "protección",
		"editor.readOnly.openInWeb":       "Editar en la web",
		"editor.readOnly":                 "Solo lectura.",
		"readOnly.requestEdit":            "Pedir acceso",
	}
	tr := func(k string, v i18n.Vars) string {
		s := textos[k]
		for n, x := range v {
			s = strings.ReplaceAll(s, "{"+n+"}", fmt.Sprint(x))
		}
		return s
	}
	marco := func(c *Contexto) *Marco {
		m := NuevoMarco(c, Genericas(), nil)
		m.Resize(fyne.NewSize(1200, 800))
		return m
	}
	abierta := 0
	c := &Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, SoloLectura: true, T: tr, EnLaWeb: func() { abierta++ }}
	c.Peticion.Pedir = func() { t.Error("ha pedido acceso a un documento que no se entiende") }
	c.NoSabeEditar("prueba", []string{"protect", "img[data-dibujo-respaldo]"})
	m := marco(c)
	if m.franja == nil || m.franja.Texto != "No sé editar (dibujos, protección)." {
		t.Fatalf("la franja no dice qué no sabe editar: %+v", m.franja)
	}
	if m.franja.Accion == nil || m.franja.Accion.Texto != "Editar en la web" || m.franja.Secundaria != nil {
		t.Fatal("la franja no ofrece «Editar en la web» (y solo eso)")
	}
	test.Tap(m.franja.Accion)
	if abierta != 1 {
		t.Error("«Editar en la web» no abre la web")
	}

	c.EnLaWeb = nil
	if m := marco(c); m.franja == nil || m.franja.Accion != nil || m.franja.Texto != "No sé editar (dibujos, protección), y aquí no hay web." {
		t.Errorf("sin dirección de la web, la franja no puede ofrecer ni prometer ir a ella: %+v", m.franja)
	}

	c.SinMotivo()
	c.Peticion.Pedir = func() {}
	if m := marco(c); m.franja == nil || m.franja.Texto != "Solo lectura." || m.franja.Accion == nil || m.franja.Accion.Texto != "Pedir acceso" {
		t.Errorf("sin motivo, la franja es la del permiso, con su petición: %+v", m.franja)
	}
}
