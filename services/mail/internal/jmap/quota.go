// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// getQuota expone el uso de almacenamiento de la cuenta del usuario autenticado:
// bytes usados y cuota total (GET de solo lectura, para el indicador del sidebar).
//
// used_bytes es el CONTADOR que el store mantiene de forma incremental (accounts.
// used_bytes): +tamaño al insertar/copiar un mensaje, −tamaño al borrarlo (soft-
// delete/expunge), acotado a ≥0. Es por tanto la SUMA agregada de los tamaños de
// los mensajes no borrados de la cuenta, ya precomputada: leerla es UNA fila
// indexada (accounts_tenant_subject_idx), O(1), sin recorrer mensajes ni N+1
// —a diferencia de un SUM(size_bytes) en vivo, que a la escala objetivo (cientos de
// miles de cuentas, millones de mensajes) escanearía todo el historial en cada
// carga—. quota_bytes sale de la columna de la cuenta (default defaultQuotaBytes).
//
// Owner-scoped: a.account resuelve la cuenta por el subject del token bajo el
// tenant del token (RLS + predicado explícito por tenant), así que solo devuelve la
// cuota del propio usuario; nunca la de otra cuenta o tenant.
func (a *API) getQuota(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"used_bytes":  acc.UsedBytes,
		"quota_bytes": acc.QuotaBytes,
	})
}
