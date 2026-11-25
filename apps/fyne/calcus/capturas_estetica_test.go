// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// hojaDeCapturas es el presupuesto de las capturas: textos, números y fórmulas.
const hojaDeCapturas = `{"active":0,"sheets":[{"name":"Presupuesto","cells":{` +
	`"A1":"Partida","B1":"Enero","C1":"Febrero","D1":"Marzo","E1":"Total",` +
	`"A2":"Alquiler","B2":"1200","C2":"1200","D2":"1200","E2":"=SUM(B2:D2)",` +
	`"A3":"Nóminas","B3":"8400","C3":"8400","D3":"8650","E3":"=SUM(B3:D3)",` +
	`"A4":"Suministros","B4":"310","C4":"287","D4":"295","E4":"=SUM(B4:D4)",` +
	`"A5":"Viajes","B5":"450","C5":"120","D5":"780","E5":"=SUM(B5:D5)",` +
	`"A6":"Total","B6":"=SUM(B2:B5)","C6":"=SUM(C2:C5)","D6":"=SUM(D2:D5)","E6":"=SUM(E2:E5)"}}]}`

// TestCapturasEsteticas deja con SYTUE_CAPTURAS una hoja abierta en las ocho
// combinaciones de la auditoría del 2026-11-24 (ver `montaje.Capturar`), con los
// rótulos de verdad.
func TestCapturasEsteticas(t *testing.T) {
	s := &servicioHojas{rol: "editor", lamport: 4, hoja: hojaDeCapturas}
	previa := appDePrueba(t, s)
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	a := NuevaApp(tr.T, "es", previa.Svc)
	a.lanzar = previa.lanzar
	a.portada.Lanzar = a.lanzar
	w := test.NewTempWindow(t, a.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	a.Arrancar()
	a.AbrirID("h1", "")
	montaje.Capturar(t, w, "calcus-hoja", iconos.Sheets)
}
