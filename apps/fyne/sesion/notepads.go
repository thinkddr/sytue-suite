// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"crypto/rand"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
)

// Lo que Notepads guardó en el equipo para no perderlo sin red —la cola de capturas y
// los borradores— no sobrevive a cerrar sesión: queda en claro en el disco (NAT-08).
// Decisión del dueño (2026-10-16): «avisar y luego borrar». Quien cierra sesión, en
// cualquiera de las trece apps, ve cuánto queda sin subir y elige subirlo o
// descartarlo; después Salir lo borra con todo lo demás, como la web.

// PendientesNotepads es cuánto tiene Notepads sin subir en este equipo, de todas las
// cuentas: lo que Salir va a borrar. Sin almacén (la web), nada.
func PendientesNotepads() int {
	alm, err := Almacen()
	if err != nil {
		return 0
	}
	n, _ := alm.Count(notas.EspacioCapturas, notas.EspacioBorradores)
	return n
}

// SubirNotepads sube lo de la persona y la organización de ahora —los borradores, como
// la captura que habría hecho Intro— y devuelve cuánto queda en el equipo sin subir:
// lo que no salió (sin red, o rechazado) y lo de otras cuentas u organizaciones, que
// con esta sesión no se puede enviar.
func (s *Suite) SubirNotepads(ctx context.Context) int {
	alm, err := Almacen()
	if err != nil {
		return 0
	}
	if dueño := s.dueñoNotas(); dueño != "" {
		n := api.NewNotes(s.URL("notes"), s.Sesion, nil)
		notas.Subir(ctx, alm, dueño, notas.EnvioDe(n, time.Now().Format(time.DateOnly)), rand.Text)
	}
	return PendientesNotepads()
}
