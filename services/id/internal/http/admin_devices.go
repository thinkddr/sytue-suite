// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Administración de dispositivos (mig 0022) desde Sytue Admin: visibilidad de los
// dispositivos registrados en la organización ACTIVA del solicitante y revocación. Todo
// se acota por org (aislamiento tenant, fail-closed): un dispositivo de otra org ⇒ 404,
// nunca se toca. La confianza self-service y el enforcement viven en otros ficheros
// (me_devices.go / access.go); esto es su plano de administración.
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// adminDeviceWire añade el dueño (email/nombre) al wire de dispositivo para la vista admin.
func adminDeviceWire(d domain.Device) map[string]any {
	w := deviceWire(d)
	w["user_id"] = d.UserID
	w["user_email"] = d.UserEmail
	w["user_name"] = d.UserName
	return w
}

// handleAdminListDevices — GET /api/v1/admin/devices — los dispositivos de la org activa,
// recientes primero, con el dueño de cada uno.
func (s *Server) handleAdminListDevices(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	list, err := s.store.ListDevicesForOrg(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, d := range list {
		items = append(items, adminDeviceWire(d))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"devices": items})
}

// handleAdminRevokeDevice — DELETE /api/v1/admin/devices/{id} — revoca (borra) un
// dispositivo de la org activa. Cross-org ⇒ 404 (aislamiento tenant, sin fuga).
func (s *Server) handleAdminRevokeDevice(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	id := chi.URLParam(r, "id")
	if _, perr := uuid.Parse(id); perr != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	deleted, err := s.store.DeleteDeviceForOrg(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !deleted {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "device.admin_revoked", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "deleted": true})
}
