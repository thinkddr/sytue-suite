// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	s := &servicioDocs{rol: "editor", html: "<h1>Propuesta</h1><p>Hola <strong>mundo</strong></p>"}
	if os.Getenv("SYTUE_CAPTURAS_PLAY") != "" {
		s.html = "<h1>Propuesta de reforma de la oficina</h1><p>Esta propuesta resume el alcance, " +
			"el calendario y el presupuesto de la reforma de la <strong>segunda planta</strong>.</p>" +
			"<h2>Alcance</h2><ul><li>Sala de reuniones con videollamada</li><li>Doce puestos nuevos</li>" +
			"<li>Iluminación y climatización</li></ul><h2>Calendario</h2><p>Las obras empiezan el 5 de " +
			"octubre y terminan antes de Navidad. Cada fase se revisa con el equipo antes de seguir.</p>"
	}
	for _, doc := range []string{"", "d1"} {
		a := appDePrueba(t, s)
		w := test.NewTempWindow(t, a.Objeto())
		w.Resize(fyne.NewSize(1280, 800))
		a.Arrancar()
		if doc != "" {
			a.AbrirID(doc, "")
		}
		recorrido.Comprobar(t, w, 400)
	}
}

// traductorPlay es nil —las claves a la vista, que es lo que miran los demás tests— salvo
// con SYTUE_CAPTURAS_PLAY: las capturas de la ficha de Google Play van en castellano.
func traductorPlay(t *testing.T) func(string, i18n.Vars) string {
	if os.Getenv("SYTUE_CAPTURAS_PLAY") == "" {
		return nil
	}
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	return tr.T
}
