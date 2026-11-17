// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// pendientesDeParidad son los comandos del registro común con los que el documento de la
// web cuenta (o el registro da por suyos) y la cinta nativa de Docus aún no, cada uno con su
// porqué. «web:» abre el motivo de los que la web tampoco pinta en su cinta (la medida está en
// `test/e2e/paridad-cinta-web.json`), que se mide como aquí: con un documento recién abierto,
// sin selección. No son una diferencia entre las dos. Los demás SON la brecha de Docus contra
// la web. La prueba compara por IGUALDAD: cerrar una brecha obliga a quitarla de aquí y abrir
// una nueva obliga a anotarla.
//
// La medida busca el rótulo CONTENIDO en el nombre de algún control, así que un comando cuyo
// rótulo es prefijo de otro pasa por tenerlo: «Insertar campo» (`formField`) lo da por
// bueno «Insertar campo de combinación» de Correspondencia, aunque Docus no inserta campos de
// formulario (`insertar.go`). La web mide igual y tiene el mismo punto ciego.
var pendientesDeParidad = map[string]string{
	// Lo que la web solo pinta con algo seleccionado o en marcha.
	"heading": "web: el estilo de párrafo se elige con las fichas de la galería de Estilos; ningún control lleva el rótulo «Estilo de párrafo»",

	"imageAlign":      "web: las herramientas de imagen salen solo con una imagen seleccionada",
	"imageAlt":        "web: ídem imageAlign",
	"imageDeselect":   "web: ídem imageAlign",
	"imageRemove":     "web: ídem imageAlign",
	"imageRemoveCrop": "web: ídem imageAlign",
	"imageWrap":       "web: ídem imageAlign",

	"mergeFinish":     "web: la navegación y el cierre de la combinación salen solo con una combinación en marcha (datos cargados)",
	"mergeNextRecord": "web: ídem mergeFinish",
	"mergePrevRecord": "web: ídem mergeFinish",
	"mergeReplace":    "web: ídem mergeFinish",

	"tableCellBg":     "web: las herramientas de tabla salen solo con el cursor en una tabla",
	"tableCellValign": "web: ídem tableCellBg",
	"tableColLeft":    "web: ídem tableCellBg",
	"tableColRight":   "web: ídem tableCellBg",
	"tableDelCol":     "web: ídem tableCellBg",
	"tableDelRow":     "web: ídem tableCellBg",
	"tableDelete":     "web: ídem tableCellBg",
	"tableMergeCells": "web: ídem tableCellBg",
	"tableRowAbove":   "web: ídem tableCellBg",
	"tableRowBelow":   "web: ídem tableCellBg",
	"tableSplitCell":  "web: ídem tableCellBg",

	// La brecha de verdad de la cinta nativa.
	"bibSources": "el botón existe («Fuentes»), pero la web lo encuentra por su `title` («Gestionar fuentes bibliográficas») y los botones de la cinta nativa aún no tienen pista con un texto distinto de su nombre accesible",
	// D-DOCU-04: el folio nativo ya entiende y pinta las listas de casillas, pero la cinta no los tiene aún.
	"listCheck":          "la cinta nativa no crea ni quita listas de casillas (las lee y pinta, y Intro deja la nueva sin hacer)",
	"markdownAutoformat": "el autoformato Markdown al escribir es solo de la web: el folio nativo no reconoce las marcas al teclear",
	"powerBi":            "el folio nativo ya conserva y pinta el informe como bloque atómico (`rico/atomicos.go`), pero falta el diálogo que lo inserta",
	"scriptsManage":      "los scripts de documento (administrar y ejecutar scripts) no tienen todavía control en la cinta nativa; llegaron a la web con el registro de comandos de scripts",
}

// traductorDeParidad es el catálogo real en español: la brecha se mide por el TEXTO del
// rótulo, como la web, y no por la clave.
func traductorDeParidad(t *testing.T) func(string, i18n.Vars) string {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatalf("no se pudo cargar el catálogo: %v", err)
	}
	return tr.T
}

// TestLaParidadDeLaCintaDeDocus: cada comando del registro común que
// existe en un documento tiene su control en la cinta nativa, salvo los anotados arriba.
func TestLaParidadDeLaCintaDeDocus(t *testing.T) {
	f := folioCon(t, `<p>hola</p>`)
	ctx := f.ctx
	ctx.T, ctx.Loc = traductorDeParidad(t), "es"
	ctx.IA = true                      // con Sytue AI detrás: sin ella, Inicio no ofrece el Asistente (como la web)
	ctx.Bloques = &editor.BloquesDoc{} // sin servicio: el desplegable sale, inerte
	faltan := editor.ComandosSinControl(registroDocus("es"), ctx)
	web := webref.SinControlEnLaWeb(t, office.Documento)
	if p := editor.CotejarParidad(faltan, pendientesDeParidad, web); len(p) > 0 {
		t.Errorf("la paridad de la cinta de Docus con la web no cuadra:\n  %s", strings.Join(p, "\n  "))
	}
}
