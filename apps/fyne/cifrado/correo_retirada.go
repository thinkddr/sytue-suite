// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// recuerdoRetirada recuerda, por cuenta, que Rooms dijo que la autoridad de Matrix
// del correo está retirada (ADR 0087, entrega 5), para que la duda no abra Matrix.
// Es lo que hace la web con `localStorage` (`aprenderRetirada`): se aprende de la
// respuesta de Rooms, nunca de un conmutador propio, y una respuesta de Matrix lo
// olvida. Con preferencias de la app sobrevive a cerrarla; sin ellas, vale para la
// ejecución. Un recuerdo nil no sabe nada y no guarda nada.
type recuerdoRetirada struct {
	prefs fyne.Preferences

	mu  sync.Mutex
	mem map[auth.IdentitySnapshot]bool
}

// retiradaEnPreferencias guarda el recuerdo en las preferencias de la app, si las hay.
func retiradaEnPreferencias(app fyne.App) *recuerdoRetirada {
	if app == nil {
		return &recuerdoRetirada{}
	}
	return &recuerdoRetirada{prefs: app.Preferences()}
}

func claveRetirada(owner auth.IdentitySnapshot) string {
	return "sytue.mail.matrixRetirada:" + owner.Issuer + "|" + owner.Organization + "|" + owner.Subject
}

func (r *recuerdoRetirada) sabe(owner auth.IdentitySnapshot) bool {
	if r == nil {
		return false
	}
	if r.prefs != nil {
		return r.prefs.Bool(claveRetirada(owner))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mem[owner]
}

func (r *recuerdoRetirada) recordar(owner auth.IdentitySnapshot, retirada bool) {
	if r == nil {
		return
	}
	if r.prefs != nil {
		if retirada {
			r.prefs.SetBool(claveRetirada(owner), true)
		} else {
			r.prefs.RemoveValue(claveRetirada(owner))
		}
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.mem == nil {
		r.mem = map[auth.IdentitySnapshot]bool{}
	}
	if retirada {
		r.mem[owner] = true
	} else {
		delete(r.mem, owner)
	}
}
