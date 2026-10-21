// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"context"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// pendientesDeParidad son los comandos del registro común con los que la hoja de la web
// cuenta (o el registro da por suyos) y la cinta nativa de Calcus aún no, cada uno con su
// porqué. «web:» abre el motivo de los que la web tampoco pinta en su cinta (la medida está en
// `test/e2e/paridad-cinta-web.json`): no son una diferencia entre las dos. Los demás SON la
// brecha de Calcus contra la web. La prueba compara por IGUALDAD: cerrar una brecha obliga a
// quitarla de aquí y abrir una nueva obliga a anotarla.
var pendientesDeParidad = map[string]string{
	"aiPanel": "web: el registro lo da para la hoja, pero el panel Asistente de la hoja se abre desde el carril y no desde la cinta",
	"aifill":  "web: id heredado del relleno con IA; el control es `fillcells` (Fórmulas en nativo, Datos en la web)",
	"copy":    "web: copiar y pegar se hacen con Ctrl+C y Ctrl+V sobre la rejilla; la cinta de la hoja no tiene grupo Portapapeles",
	"paste":   "web: ídem copy",

	"scenadd":    "web: los cuatro verbos del administrador de escenarios viven dentro de su diálogo (`data.scenarios`), no en la cinta",
	"scendel":    "web: ídem scenadd",
	"scenreport": "web: ídem scenadd",
	"scenupdate": "web: ídem scenadd",

	"sparkdel":      "web: quitar el minigráfico es una opción de su menú contextual, no un control de la cinta",
	"tabledel":      "web: quitar la tabla, su estilo y su fila de totales viven en el panel de la tabla (selección de tabla)",
	"tablestyle":    "web: ídem tabledel",
	"tabletotalfn":  "web: ídem tabledel",
	"tabletotals":   "web: ídem tabledel",
	"unprotect":     "web: desproteger se hace con el mismo botón que proteger, que alterna",
	"unprotectbook": "web: ídem unprotect",
	"validdel":      "web: quitar la validación es una opción del diálogo de validación de datos",

	"scriptsManage": "los scripts de documento (administrar y ejecutar scripts) no tienen todavía control en la cinta nativa; llegaron a la web con el registro de comandos de scripts",
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

// TestLaCintaDeCalcusTieneTodosLosComandosDeLaHoja: cada comando del registro común que
// existe en una hoja tiene su control en la cinta nativa, salvo los anotados arriba.
func TestLaCintaDeCalcusTieneTodosLosComandosDeLaHoja(t *testing.T) {
	lz := lienzoDePrueba()
	// Con servicio de IA detrás: sin él, la cinta no ofrece el relleno (como la web).
	lz.IA = func(context.Context, string, string) (string, error) { return "", nil }
	ctx := &editor.Contexto{Tipo: editor.Hoja, Lienzo: lz, IA: true, T: traductorDeParidad(t)}
	ctx.Bloques = &editor.BloquesDoc{} // sin servicio: el desplegable sale, inerte
	faltan := editor.ComandosSinControl(Registro(lz), ctx)
	web := webref.SinControlEnLaWeb(t, office.Hoja)
	if p := editor.CotejarParidad(faltan, pendientesDeParidad, web); len(p) > 0 {
		t.Errorf("la paridad de la cinta de Calcus con la web no cuadra:\n  %s", strings.Join(p, "\n  "))
	}
}

// TestLasPestanasDeCalcusSonLasDeLaWeb: las pestañas de la cinta de Calcus son, por IGUALDAD, las que la web pinta para ese tipo
// de documento (su copia de build de `pestanas.json`): mismas, mismo orden, mismo rótulo.
func TestLasPestanasDeCalcusSonLasDeLaWeb(t *testing.T) {
	web := []byte(webref.Read(t, editor.PestanasWeb))
	if p := editor.CotejarPestanas(Registro(nil), editor.Hoja, web); len(p) > 0 {
		t.Error(strings.Join(p, "\n"))
	}
}
