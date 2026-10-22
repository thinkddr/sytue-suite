// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"net/http"
	"time"
)

// AbrirSSE abre un flujo `text/event-stream` en w: cabeceras, 200 y SIN los plazos
// del servidor para esta respuesta. Devuelve false, sin haber escrito nada, si w no
// sabe vaciar su búfer; entonces quien llama contesta el error que le toque.
//
// Los plazos son la razón de que exista. [NewServer] pone un WriteTimeout de 30 s a
// TODAS las respuestas, y un 0 en [ServerConfig] no lo quita: significa «el de por
// defecto». El Hub lo tenía así, con el comentario de que era para el SSE, y el flujo
// de avisos moría a los 30 s sin que se viera: una escritura pasado el plazo va al
// búfer y el vaciado falla callado. Medido en producción el 2026-09-24: de 746 flujos
// de la campana, ninguno pasó de 50 s —el segundo latido de 25, que es cuando el
// servidor se entera—, y un aviso creado entre el segundo 30 y el 50 no llegaba.
//
// Se quita también el de lectura: con él vencido, la lectura de fondo del servidor
// da error y cancela el contexto de la petición, que para un flujo es colgarlo.
func AbrirSSE(w http.ResponseWriter) (http.Flusher, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	// Un ResponseRecorder de test no los soporta, y no pasa nada: no tiene plazos.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	// Sin esto un proxy intermedio guarda el evento hasta llenar su búfer, y el
	// «tiempo real» llega tarde o nunca.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	f.Flush()
	return f, true
}
