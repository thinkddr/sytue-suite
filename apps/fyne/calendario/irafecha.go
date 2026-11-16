// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # «Ir a una fecha» (`IrAFecha.tsx`)
//
// La tecla g, como en Google Calendar: saltar a un día lejano sin pulsar «siguiente»
// cuarenta veces. No es una vista nueva: mueve el cursor y la vista que hubiera se
// queda. El campo es el CampoFecha de la suite con su calendario desplegable, que es lo
// que el navegador cuelga del `<input type="date">` de la web.

// irAFecha abre el diálogo en el día que se ve. Sin lienzo no hay dónde abrirlo.
func (a *App) irAFecha() bool {
	lienzo := a.lienzo()
	if lienzo == nil {
		return false
	}
	a.dialogoIrAFecha().Mostrar(lienzo)
	return true
}

// dialogoIrAFecha arma el diálogo: el campo arranca en el cursor y «Ir» se apaga
// mientras lo escrito no sea una fecha.
func (a *App) dialogoIrAFecha() *ui.Dialogo {
	p := a.P
	zona := p.zona()
	campo := ui.ConCalendario(ui.CampoFecha(ui.CampoMD), p.locale(), zona)
	campo.PonerTexto(ui.EscribirFecha(p.Cursor.In(zona)))
	campo.Rotulo = p.t("shortcuts.goToDateField", nil)
	cuerpo := ui.GrupoCampo(p.t("shortcuts.goToDateField", nil), false, campo, "", "")

	var d *ui.Dialogo
	leer := func() (time.Time, bool) {
		dia, vacia, ok := ui.LeerFecha(campo.Texto(), zona)
		return dia, ok && !vacia
	}
	ir := func() {
		dia, ok := leer()
		if !ok {
			return
		}
		d.Cerrar()
		a.cambiar(func() { p.Cursor = dia })
	}
	d = ui.Confirmacion(p.t("shortcuts.goToDate", nil), p.t("common.cancel", nil),
		p.t("shortcuts.goToDateSubmit", nil), false, cuerpo, nil, ir)
	campo.AlCambiar(func(string) {
		_, ok := leer()
		d.SinConfirmar(!ok)
	})
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		ir()
		return true
	}
	d.Foco = campo
	return d
}
