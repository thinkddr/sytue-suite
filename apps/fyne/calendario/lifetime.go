// SPDX-License-Identifier: Apache-2.0
package calendario

import (
	"context"
	"weak"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// registrarDialogo does not keep already dismissed dialogs (and their private
// form data) alive. The canvas keeps an open dialog alive until it is closed.
func (a *App) registrarDialogo(d *ui.Dialogo) {
	if d == nil {
		return
	}
	vivos := a.dialogos[:0]
	for _, previo := range a.dialogos {
		if previo.Value() != nil {
			vivos = append(vivos, previo)
		}
	}
	vivos = append(vivos, weak.Make(d))
	a.dialogos = vivos
}

func (a *App) mostrarDialogo(d *ui.Dialogo, c fyne.Canvas) {
	if c == nil || a.vida.Err() != nil {
		return
	}
	a.registrarDialogo(d)
	d.Mostrar(c)
}

// iniciarTrabajo admits each worker before it can start. The repository owns
// its own session lease; this gate tracks only the presenter's lifetime.
func (a *App) iniciarTrabajo(fn func(context.Context)) bool {
	if fn == nil {
		return false
	}
	vida, done, err := a.trabajos.Start(a.vida)
	if err != nil {
		return false
	}
	go func() {
		defer done()
		fn(vida)
	}()
	return true
}

// encolarUI discards a callback if its presenter was retired before dispatch
// or while the callback waited in the UI queue.
func (a *App) encolarUI(fn func()) {
	if fn == nil || a.vida.Err() != nil {
		return
	}
	epoca := a.epocaUI.Load()
	a.despachar(func() {
		if a.vida.Err() == nil && a.epocaUI.Load() == epoca {
			fn()
		}
	})
}

// Cerrar is called on the UI thread. It cancels the original identity's work
// without waiting for callbacks that may themselves require that thread. It
// must run before replacing this presenter's window content or mounting the
// next identity: all overlays on that canvas belong to the retired window.
func (a *App) Cerrar() {
	if a.cerrada {
		return
	}
	a.cerrada = true
	lienzo := a.lienzo()
	a.cancelarVida()
	a.epocaUI.Add(1)
	a.PararImportador()
	for _, e := range a.P.peticiones {
		e.Soltar()
	}
	a.P.peticionesDueno.Soltar()
	for _, aviso := range a.avisos.Avisos() {
		a.avisos.Descartar(aviso)
	}
	a.cerrarVistaRapida()
	if a.recPanel != nil {
		a.recPanel.Cerrar()
		a.recPanel = nil
	}
	// Dialogo.Cerrar also stops mobile keyboard tracking; removing its layer
	// directly would leave that goroutine running while the keyboard stays up.
	for i := len(a.dialogos) - 1; i >= 0; i-- {
		if d := a.dialogos[i].Value(); d != nil {
			d.Cerrar()
		}
	}
	a.dialogos = nil
	// This includes nested selector and date popovers, which are separate canvas
	// layers, and non-dismissible dialogs. Retirement is not a user dismissal.
	ui.OlvidarSuperposiciones(lienzo)
}

// Join is terminal: it invalidates the instance and its queued callbacks, then
// drains work away from the UI thread. A caller whose wait expires can retry;
// it cannot reopen this identity. Cerrar must still retire its UI on that thread.
func (a *App) Join(ctx context.Context) error {
	a.cancelarVida()
	a.epocaUI.Add(1)
	return a.trabajos.Drain(ctx)
}
