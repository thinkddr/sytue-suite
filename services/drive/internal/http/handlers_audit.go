// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Lectura admin del log de auditoría de compliance de Sytue Drive para la vista de
// «Auditoría unificada» de Sytue Admin (que agrega la traza de todos los
// servicios). Expone el audit_log que ya alimentan legal hold + eDiscovery
// (WriteAudit/writeAuditTx): un listado paginado por keyset (occurred_at, id) DESC
// con filtros por acción/actor/rango de fechas, y una exportación CSV del mismo
// conjunto filtrado apta para ingesta SIEM.
//
// ACCESO PRIVILEGIADO: la traza registra quién accedió a ficheros ajenos; el
// listado es owner/admin (RequireRole en el montaje + requireAdmin aquí) y va
// aislado por org — ListAudit del store corre bajo InTenantTx con predicado tenant
// EXPLÍCITO (RLS + defensa en profundidad), así que un admin de A jamás ve la traza
// de B (probado en el test de integración del store).
package drivehttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// listAuditLog — GET /api/v1/admin/audit-log — página de la traza de compliance
// de la org activa (o CSV con ?format=csv). Query: action (prefijo), actor
// (subcadena), since/until (RFC3339 o AAAA-MM-DD), cursor (opaco), limit
// (1..500). Responde {items, nextCursor}.
//
// Filtrar, paginar y exportar están en `platform/pkg/auditlog`: es el mismo
// código que usa Sytue Mail, y tenerlo dos veces ya había dejado dos mensajes de
// error distintos para la misma validación. Aquí se queda lo que de verdad es de
// este servicio: quién puede llamar, de dónde salen las filas y cómo se traduce
// su error de dominio.
func (a *complianceHandlers) listAuditLog(w http.ResponseWriter, r *http.Request) {
	if _, err := requireAdmin(r); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q, err := auditlog.LeerConsulta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	entries, err := a.store.ListAudit(r.Context(), auditlog.VentanaMax)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	entries = auditlog.Filtrar(entries, q)
	if q.CSV {
		auditlog.EscribirCSV(w, r, "drive", entries)
		return
	}
	page, next := auditlog.Paginar(entries, q.After, q.Limit)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": page, "nextCursor": next})
}
