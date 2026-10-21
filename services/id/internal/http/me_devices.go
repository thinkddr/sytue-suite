// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Dispositivos de confianza self-service (API JSON, JWT bearer): mis dispositivos, con
// el ACTUAL marcado si el request trae la cookie de dispositivo; marcar de confianza el
// actual (prueba de posesión de la cookie firmada); y revocar cualquiera de los míos. El
// registro/identificación del dispositivo vive en el login (session.go/registerDevice);
// el enforcement de la condición `require_trusted_device` en el Acceso Condicional
// (access.go). Aquí solo el plano self-service, acotado por usuario en el store.
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// deviceWire es la forma JSON de un dispositivo para la Console.
func deviceWire(d domain.Device) map[string]any {
	return map[string]any{
		"id":         d.ID,
		"name":       d.Name,
		"user_agent": d.UserAgent,
		"trusted":    d.Trusted,
		"first_seen": d.FirstSeen.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"last_seen":  d.LastSeen.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"last_ip":    d.LastIP,
		"current":    d.Current,
	}
}

// currentDeviceID deriva el device_id del dispositivo desde el que llega el request, a
// partir de la cookie de dispositivo firmada. "" si no hay cookie o no verifica.
func (s *Server) currentDeviceID(r *http.Request, userID string) string {
	c, err := s.requestCookie(r, deviceCookieName)
	if err != nil || c.Value == "" {
		return ""
	}
	b, ok := domain.ParseDeviceCookie(s.deviceKey, c.Value)
	if !ok {
		return ""
	}
	return domain.DeriveDeviceID(s.deviceKey, userID, b)
}

// handleMeDevices — GET /api/v1/me/devices — los dispositivos del usuario del access
// token, recientes primero, con el actual marcado si el request trae la cookie de
// dispositivo (navegador same-site o reenviada por el BFF).
func (s *Server) handleMeDevices(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	list, err := s.store.ListDevicesForUser(r.Context(), c.Subject)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	cur := s.currentDeviceID(r, c.Subject)
	out := make([]map[string]any, 0, len(list))
	for _, d := range list {
		d.Current = cur != "" && d.DeviceID == cur
		out = append(out, deviceWire(d))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"devices": out})
}

// handleMeDeviceTrust — POST /api/v1/me/devices/{id}/trust — marca de confianza el
// dispositivo ACTUAL. Exige prueba de posesión: el {id} debe ser el dispositivo desde el
// que se hace la petición (su device_id deriva de la cookie firmada). Así una sesión no
// puede marcar de confianza un dispositivo en el que no está (p. ej. uno robado/remoto).
func (s *Server) handleMeDeviceTrust(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	cur := s.currentDeviceID(r, c.Subject)
	if cur == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("This device could not be identified. Sign in again from it to mark it as trusted."))
		return
	}
	dev, err := s.store.GetDeviceForUser(r.Context(), c.Subject, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if dev.DeviceID != cur {
		// Solo el dispositivo actual puede marcarse de confianza (prueba de posesión).
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("You can only mark as trusted the device you are signing in from."))
		return
	}
	updated, err := s.store.SetDeviceTrustedForUser(r.Context(), c.Subject, id, true)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "device.trusted", id, r))
	updated.Current = true
	httpx.JSON(w, r, http.StatusOK, deviceWire(updated))
}

// handleMeDeviceRevoke — DELETE /api/v1/me/devices/{id} — revoca (borra) un dispositivo
// del usuario. Puede ser cualquiera de los suyos (también uno perdido en el que no está):
// deja de reconocerse y pierde la confianza. En el próximo login desde él se registra de
// nuevo como desconocido (no de confianza).
func (s *Server) handleMeDeviceRevoke(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	deleted, err := s.store.DeleteDeviceForUser(r.Context(), c.Subject, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !deleted {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "device.revoked", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

// handleAccountDeviceTrust — POST /account/devices/trust — marca de confianza el
// dispositivo DESDE EL QUE se pide, por la cookie de sesión del IdP.
//
// Existe porque «Mi cuenta» no reenvía las cookies de Sytue ID (riesgo aceptado,
// docs/security.md#riesgo-aceptado-mi-cuenta-en-el-origen-del-idp): su BFF llama a
// `/api/v1/me/devices/{id}/trust` sin la de dispositivo, y sin ella no hay prueba de
// posesión posible. Aquí la cookie llega sola, porque es el navegador quien pide, y
// no hace falta decir qué dispositivo: es el que trae la cookie. Mismo guard que el
// resto de `/account/*`: sesión y Origin propio.
func (s *Server) handleAccountDeviceTrust(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.currentSession(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if !s.sameOrigin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	cur := s.currentDeviceID(r, sess.UserID)
	if cur == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("This device could not be identified. Sign in again from it to mark it as trusted."))
		return
	}
	dev, err := s.store.GetDeviceByDeviceID(r.Context(), sess.UserID, cur)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	updated, err := s.store.SetDeviceTrustedForUser(r.Context(), sess.UserID, dev.ID, true)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "device.trusted", dev.ID, r))
	updated.Current = true
	httpx.JSON(w, r, http.StatusOK, deviceWire(updated))
}
