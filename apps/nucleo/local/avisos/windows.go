// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package avisos

import "git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"

// Windows es el [Sistema] de Windows: `ScheduledToastNotification`, que el sistema
// entrega con la app cerrada y que al pulsarlo abre [Aviso.Enlace]. AppID es la AUMID de
// la app (su identificador, `eu.sytue.office.<app>`). Fuera de Windows devuelve
// escritorio.ErrNoDisponible.
type Windows struct{ AppID string }

// Programar deja el toast a su hora; el identificador es la etiqueta.
func (w Windows) Programar(etiqueta string, a Aviso) (string, error) {
	return etiqueta, escritorio.ProgramarToast(w.AppID, a.Enlace, a.Titulo, a.Cuerpo, etiqueta, a.Cuando)
}

// Cancelar lo quita de la cola de Windows.
func (w Windows) Cancelar(id string) error { return escritorio.CancelarToast(w.AppID, id) }
