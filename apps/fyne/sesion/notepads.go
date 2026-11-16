// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"crypto/rand"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Lo que Notepads guardó en el equipo para no perderlo sin red —la cola de capturas y
// los borradores— se cifra en el equipo y no sobrevive a cerrar sesión (NAT-08).
// Decisión del dueño (2026-10-16): «avisar y luego borrar». Quien cierra sesión, en
// cualquiera de las trece apps, ve cuánto queda sin subir y elige subirlo o
// descartarlo; después Salir lo borra con todo lo demás, como la web.

// PendientesNotepads es cuánto tiene Notepads sin subir en este equipo, de todas las
// cuentas: lo que Salir va a borrar. Sin almacén (la web), nada.
func PendientesNotepads() int {
	n, _ := contarAlmacen(notas.EspacioCapturas, notas.EspacioBorradores)
	return n
}

// SubirNotepads sube lo de la persona y la organización de ahora —los borradores, como
// la captura que habría hecho Intro— y devuelve cuánto queda en el equipo sin subir:
// lo que no salió (sin red, o rechazado) y lo de otras cuentas u organizaciones, que
// con esta sesión no se puede enviar.
func (s *Suite) SubirNotepads(ctx context.Context) int {
	captured, err := s.AutorizarSalida()
	if err != nil {
		return s.pendientesTrasSubir(ctx)
	}
	return s.SubirNotepadsAutorizado(ctx, captured)
}

// SubirNotepadsAutorizado retains the exact owner and HTTP witness captured by
// the dialog. A state response cannot turn A's queued text into a request from B.
func (s *Suite) SubirNotepadsAutorizado(ctx context.Context, captured AutorizacionSalida) int {
	if captured.suite != s || captured.Vigente() != nil {
		return s.pendientesTrasSubir(ctx)
	}
	identity := captured.action.Identity
	if identity.Subject == "" {
		return s.pendientesTrasSubir(ctx)
	}
	alm, err := AlmacenConContexto(ctx)
	if err != nil {
		return s.pendientesTrasSubir(ctx)
	}
	n := s.envioPrivadoNotepads(ctx, captured)
	if n == nil {
		return s.pendientesTrasSubir(ctx)
	}
	owner := identity.Subject + "/" + identity.Organization
	notas.Subir(ctx, alm, owner, notas.EnvioDe(n, time.Now().Format(time.DateOnly)), rand.Text)
	return s.pendientesTrasSubir(ctx)
}

func (s *Suite) pendientesTrasSubir(ctx context.Context) int {
	count, err := s.PendientesSalidaCon(ctx)
	if err != nil {
		return max(1, count)
	}
	return count
}

func (s *Suite) envioPrivadoNotepads(ctx context.Context, captured AutorizacionSalida) *api.Notes {
	identity := captured.action.Identity
	scope := storage.Scope{Tenant: identity.Organization, Subject: identity.Subject, Product: "notes"}
	if !scope.Valid() {
		return nil
	}
	bound, err := captured.sesionVinculada(ctx)
	if err != nil {
		return nil
	}
	legacy := api.NewNotes(s.URL("notes"), bound, nil)
	witness := captured.witness
	for _, work := range s.trabajosDe(witness) {
		if work.PrivateStorageScope() != scope {
			continue
		}
		data, ok := work.(api.NotesData)
		if !ok {
			continue
		}
		facade, err := legacy.WithPrivateData(data)
		if err != nil {
			return nil
		}
		return facade
	}
	// A closed private app cannot send queued plaintext through the former API.
	// Only an explicitly still-legacy account may use its existing capture queue.
	state, err := legacy.PrivateStorageState(ctx, scope)
	if err != nil || state.Mode != "legacy" {
		return nil
	}
	return legacy
}
