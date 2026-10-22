// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// latido es cada cuánto se manda un comentario para que nadie corte la conexión
// por inactividad. Veinte segundos: por debajo del minuto que suelen aplicar los
// proxies, y muy por debajo del tiempo que un buzón puede pasar sin recibir nada.
const latido = 20 * time.Second

// handleStream abre el flujo SSE de llegadas de la cuenta autenticada:
// GET /jmap/eventos.
//
// LO QUE ARREGLA. Sytue Mail no tenía ningún transporte de tiempo real, así que
// con la bandeja abierta delante un correo nuevo no aparecía hasta volver a la
// pestaña. Aquí no viaja el correo: solo el aviso de que hay uno, con lo justo
// para pintar la fila.
//
// Protocolo (WHATWG EventSource):
//   - event: listo    -> el flujo está abierto.
//   - event: llegada  -> ha entrado un mensaje (JSON en data).
//   - ': latido'      -> comentario de mantenimiento.
func (a *API) handleStream(w http.ResponseWriter, r *http.Request) {
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The token is missing the tenant."))
		return
	}
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Sin los plazos del servidor: con su WriteTimeout el flujo moría a los 30 s y
	// el correo que entraba después no aparecía (ver httpx.AbrirSSE).
	flusher, ok := httpx.AbrirSSE(w)
	if !ok {
		httpx.Fail(w, r, httpx.ErrInternal.WithMessage("Streaming no soportado."))
		return
	}

	ch, cancelar := a.push.Subscribe(t.Slug, acc.ID)
	defer cancelar()

	// Se avisa de que el flujo está abierto ANTES de la primera llegada: así el
	// cliente distingue «conectado y sin correo» de «no me he conectado».
	escribirSSE(w, "listo", map[string]any{"account_id": acc.ID})
	flusher.Flush()

	tic := time.NewTicker(latido)
	defer tic.Stop()
	// El flujo no vive más que su token (COM-08): al reconectar, el nuevo dice qué ve.
	claims, _ := authn.FromContext(r.Context())
	ctx, cancelar := authn.HastaQueCaduque(r.Context(), claims)
	defer cancelar()
	for {
		select {
		case <-ctx.Done():
			return
		case l, abierto := <-ch:
			if !abierto {
				return
			}
			if !escribirSSE(w, "llegada", l) {
				return
			}
			flusher.Flush()
		case <-tic.C:
			if _, err := fmt.Fprint(w, ": latido\n\n"); err != nil {
				log.From(ctx).Debug("mail SSE: cliente desconectado", "error", err)
				return
			}
			flusher.Flush()
		}
	}
}

// escribirSSE serializa v como un evento con nombre. Devuelve false si la
// escritura falla, que es como se entera uno de que el cliente se fue.
func escribirSSE(w http.ResponseWriter, evento string, v any) bool {
	datos, err := json.Marshal(v)
	if err != nil {
		return false
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evento, datos)
	return err == nil
}
