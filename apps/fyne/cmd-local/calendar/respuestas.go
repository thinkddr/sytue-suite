// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"slices"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
)

// Las respuestas de los invitados que llegan por correo (iMIP REPLY, A-11). Con un CalDAV
// que no planifica, la invitación sale por correo desde aquí y las respuestas llegan a
// Mail, que no abre el almacén de Calendar (no admite dos escritores): las deja en su
// cola (`<Datos>/Correo/respuestas-imip/`, `correo.RespuestasPorAplicar`). Calendar las
// recoge al abrirse y, abierta, en cada vuelta de la sincronización, ANTES de subir: las
// pone en sus eventos (`agenda.Registro.AplicarRespuestas`) y la vuelta las sube. Con la
// app abierta, una respuesta nueva en la cola pide la vuelta sin esperar a los diez
// minutos (se mira cada `miraRespuestas`, sin leer la red).

// miraRespuestas es cada cuánto Calendar abierta mira si Mail dejó una respuesta nueva.
const miraRespuestas = 20 * time.Second

// raizMail es la carpeta del correo de Mail, donde está su cola.
func (s *sincronia) raizMail() string { return filepath.Join(s.l.Espacio.Rutas.Datos, "Correo") }

// aplicarRespuestas pone en sus eventos lo que Mail dejó en la cola y lo quita de ella: lo
// aplicado y lo que no es de nada de aquí. Lo que espera (su evento aún no ha bajado, o
// cambió mientras) se queda, hasta que lleve un mes. Devuelve cuántas se aplicaron.
func (s *sincronia) aplicarRespuestas(ctx context.Context) int {
	return aplicarCola(ctx, s.reg, s.local, s.dir, s.raizMail())
}

// aplicador es quien pone las respuestas en sus eventos (`agenda.Registro`).
type aplicador interface {
	AplicarRespuestas(ctx context.Context, local agenda.Local, dir string, replies [][]byte) []agenda.EstadoRespuesta
}

func aplicarCola(ctx context.Context, reg aplicador, local agenda.Local, dir, raiz string) int {
	ps, err := correo.RespuestasPorAplicar(raiz)
	if err != nil {
		slog.Warn("calendar: no se pudo leer la cola de respuestas de Mail", "error", err)
		return 0
	}
	if len(ps) == 0 {
		return 0
	}
	ics := make([][]byte, len(ps))
	for i, p := range ps {
		ics[i] = p.ICS
	}
	aplicadas := 0
	for i, e := range reg.AplicarRespuestas(ctx, local, dir, ics) {
		switch {
		case e == agenda.RespuestaAplicada:
			aplicadas++
			_ = ps[i].Quitar()
		case e == agenda.RespuestaDescartada, time.Since(ps[i].Llegada) > correo.CaducidadRespuestas:
			_ = ps[i].Quitar()
		}
	}
	if aplicadas > 0 {
		slog.Info("calendar: respuestas de invitados aplicadas", "n", aplicadas)
	}
	return aplicadas
}

// vigilarRespuestas pide una vuelta en cuanto Mail deja una respuesta que no estaba (una
// que espera no la pide otra vez: espera a la vuelta siguiente).
func (s *sincronia) vigilarRespuestas(ctx context.Context) {
	go func() {
		t := time.NewTicker(miraRespuestas)
		defer t.Stop()
		// Lo que ya estaba al abrir lo recoge la primera vuelta, que sale a la vez.
		vistas := correo.RespuestasEnCola(s.raizMail())
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			ahora := correo.RespuestasEnCola(s.raizMail())
			for _, n := range ahora {
				if !slices.Contains(vistas, n) && len(s.reg.Cuentas()) > 0 {
					s.motor.Ya()
					break
				}
			}
			vistas = ahora
		}
	}()
}
