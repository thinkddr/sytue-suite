// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// pendientesDeParidad son los comandos del registro común con los que la presentación de
// la web cuenta (o el registro da por suyos) y la cinta nativa de Diapos aún no, cada uno con
// su porqué. «web:» abre el motivo de los que la web tampoco pinta en su cinta (la medida
// está en `test/e2e/paridad-cinta-web.json`): no son una diferencia entre las dos. La prueba
// compara por IGUALDAD: cerrar una brecha obliga a quitarla de aquí y abrir una nueva obliga
// a anotarla.
var pendientesDeParidad = map[string]string{
	"aiPanel":       "web: el registro lo da para la presentación, pero el panel Asistente se abre desde el carril y no desde la cinta",
	"iafill":        "web: id heredado de «escribir con IA»; el panel Asistente del carril lo cubre",
	"scriptsManage": "los scripts de documento (administrar y ejecutar scripts) no tienen todavía control en la cinta nativa; llegaron a la web con el registro de comandos de scripts",
	"grafico":       "web: insertar un gráfico en la presentación no tiene botón en la cinta de la web (los gráficos de la hoja se llevan a una diapositiva por otra vía)",
}

// TestLaCintaDeDiaposTieneTodosLosComandosDeLaPresentacion: cada comando del registro
// común que existe en una presentación tiene su control en la cinta nativa, salvo los
// anotados arriba.
func TestLaCintaDeDiaposTieneTodosLosComandosDeLaPresentacion(t *testing.T) {
	e, ctx, _ := montar(t, `{"slides":[{"id":"a","boxes":[]}]}`)
	ctx.IA = true
	ctx.Bloques = &editor.BloquesDoc{} // sin servicio: el desplegable sale, inerte
	faltan := editor.ComandosSinControl(Registro(e), ctx)
	web := webref.SinControlEnLaWeb(t, office.Presentacion)
	if p := editor.CotejarParidad(faltan, pendientesDeParidad, web); len(p) > 0 {
		t.Errorf("la paridad de la cinta de Diapos con la web no cuadra:\n  %s", strings.Join(p, "\n  "))
	}
}

// TestLasPestanasDeDiaposSonLasDeLaWeb: las pestañas de la cinta de Diapos son, por IGUALDAD, las que la web pinta para ese tipo
// de documento (su copia de build de `pestanas.json`): mismas, mismo orden, mismo rótulo.
func TestLasPestanasDeDiaposSonLasDeLaWeb(t *testing.T) {
	web := []byte(webref.Read(t, editor.PestanasWeb))
	if p := editor.CotejarPestanas(Registro(nil), editor.Presentacion, web); len(p) > 0 {
		t.Error(strings.Join(p, "\n"))
	}
}
