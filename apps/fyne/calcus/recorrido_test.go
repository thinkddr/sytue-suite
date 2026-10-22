// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

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
	s := &servicioHojas{rol: "editor", lamport: 4, hoja: `{"active":0,"sheets":[{"name":"Datos","cells":{"A1":"10","A2":"=A1*2"}}]}`}
	if os.Getenv("SYTUE_CAPTURAS_PLAY") != "" {
		s.hoja = `{"active":0,"sheets":[{"name":"Presupuesto","cells":{` +
			`"A1":"Partida","B1":"Enero","C1":"Febrero","D1":"Marzo","E1":"Total",` +
			`"A2":"Alquiler","B2":"1200","C2":"1200","D2":"1200","E2":"=SUM(B2:D2)",` +
			`"A3":"Nóminas","B3":"8400","C3":"8400","D3":"8650","E3":"=SUM(B3:D3)",` +
			`"A4":"Suministros","B4":"310","C4":"287","D4":"295","E4":"=SUM(B4:D4)",` +
			`"A5":"Viajes","B5":"450","C5":"120","D5":"780","E5":"=SUM(B5:D5)",` +
			`"A6":"Total","B6":"=SUM(B2:B5)","C6":"=SUM(C2:C5)","D6":"=SUM(D2:D5)","E6":"=SUM(E2:E5)"}}]}`
	}
	for _, doc := range []string{"", "h1"} {
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
