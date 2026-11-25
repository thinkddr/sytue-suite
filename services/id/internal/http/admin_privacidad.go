// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// DERECHOS RGPD DEL LADO DEL RESPONSABLE (mig 0130; ADR 0054, enmienda del
// 2026-11-25).
//
// La organización es la responsable del tratamiento de los datos de su gente:
// cuando alguien le pide acceso, rectificación o supresión —por correo, por
// carta, en persona—, quien la administra necesita dejarlo REGISTRADO, contestar
// en plazo (un mes, prorrogable dos: art. 12.3) y, si es una supresión, poder
// ejecutarla de verdad. Eso es lo que hay aquí:
//
//   - el registro de solicitudes, con su historia solo de añadir;
//   - la SUPRESIÓN FÍSICA de una cuenta de la organización: baja inmediata, sin
//     ventana de recuperación, credenciales borradas en el acto y la ficha
//     anonimizada en el siguiente barrido, junto con lo que los demás servicios
//     guardan de esa persona (merge.DatosDeLaPersona).
//
// Tres decisiones:
//
//   - RESERVADO A OWNER/ADMIN, como la gestión de roles: responder en nombre de
//     la organización no se delega con el catálogo fijo de permisos.
//   - SOLO LO SUYO. La supresión se niega si la cuenta es también de otra
//     organización o tiene su cuenta personal: esa parte no la decide quien
//     administra esta. La persona la borra ella misma (/account/delete).
//   - LO DE LA ORGANIZACIÓN SE QUEDA EN LA ORGANIZACIÓN (ADR 0054 §7): el buzón,
//     los archivos y los eventos que la persona creó son de la organización y se
//     borran, si procede, con las herramientas de cada producto. La respuesta lo
//     dice para que nadie dé por borrado lo que no lo está.
package http

import (
	"errors"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// recepcionMaxAtras acota la fecha de recepción que se puede declarar: una
// solicitud que llegó hace más de un año y nadie registró ya está fuera de plazo
// por once meses, y lo que hay que hacer con ella no es apuntarla con fecha.
const recepcionMaxAtras = 365 * 24 * time.Hour

// handleAdminListPrivacyRequests — GET /api/v1/admin/privacy/requests.
// Query: status (received|in_progress|completed|rejected|open), kind, overdue=1.
func (s *Server) handleAdminListPrivacyRequests(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := r.URL.Query()
	f := store.FiltroSolicitudesRGPD{Estado: q.Get("status"), Tipo: q.Get("kind"), Vencidas: q.Get("overdue") == "1"}
	if f.Estado != "" && f.Estado != "open" && !slices.Contains([]string{
		store.EstadoSolicitudRecibida, store.EstadoSolicitudEnCurso, store.EstadoSolicitudResuelta, store.EstadoSolicitudDenegada,
	}, f.Estado) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown status filter."))
		return
	}
	if f.Tipo != "" && !slices.Contains(store.TiposSolicitudRGPD, f.Tipo) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown request kind."))
		return
	}
	lista, err := s.store.ListarSolicitudesRGPD(r.Context(), org.ID, f)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	ahora := time.Now()
	abiertas, vencidas := 0, 0
	for _, sol := range lista {
		if sol.ClosedAt == nil {
			abiertas++
			if sol.DueAt.Before(ahora) {
				vencidas++
			}
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"requests": lista,
		"summary":  map[string]int{"open": abiertas, "overdue": vencidas},
		// Los plazos, dichos por el servidor: la consola no los recalcula.
		"response_months": store.PlazoRespuestaMesesRGPD, "extension_months": store.ProrrogaMesesRGPD,
	})
}

// cuerpoSolicitudRGPD es lo que la consola manda al registrar una solicitud.
type cuerpoSolicitudRGPD struct {
	SubjectEmail     string `json:"subject_email"`
	SubjectName      string `json:"subject_name"`
	UserID           string `json:"user_id"`
	Kind             string `json:"kind"`
	Channel          string `json:"channel"`
	ReceivedAt       string `json:"received_at"`
	IdentityVerified bool   `json:"identity_verified"`
	IdentityMethod   string `json:"identity_method"`
	Notes            string `json:"notes"`
}

// handleAdminCreatePrivacyRequest — POST /api/v1/admin/privacy/requests.
func (s *Server) handleAdminCreatePrivacyRequest(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in cuerpoSolicitudRGPD
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sol, apiErr := s.solicitudDesdeCuerpo(r, org.ID, in)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	creada, err := s.store.CrearSolicitudRGPD(r.Context(), sol, c.Subject)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// Sin el correo de la persona en la auditoría: el registro ya lo guarda, y el
	// log de seguridad no tiene por qué repetir un dato personal.
	e := auditEntry(c.Subject, org.ID, "privacy.request.created", creada.ID, r)
	e.Detail["kind"] = creada.Kind
	_ = s.store.Audit(r.Context(), nil, e)
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"request": creada})
}

// solicitudDesdeCuerpo valida el alta. La persona se identifica por su cuenta
// (user_id, que tiene que ser de la organización) o por su correo; si el correo
// es el de una cuenta de la organización, se enlaza sola.
func (s *Server) solicitudDesdeCuerpo(r *http.Request, orgID string, in cuerpoSolicitudRGPD) (store.SolicitudRGPD, error) {
	sol := store.SolicitudRGPD{
		OrgID:            orgID,
		Kind:             strings.TrimSpace(in.Kind),
		Channel:          strings.TrimSpace(in.Channel),
		SubjectName:      validation.TruncateRunes(strings.TrimSpace(in.SubjectName), 200),
		IdentityVerified: in.IdentityVerified,
		IdentityMethod:   validation.TruncateRunes(strings.TrimSpace(in.IdentityMethod), 200),
		Notes:            validation.TruncateRunes(strings.TrimSpace(in.Notes), 4000),
	}
	if !slices.Contains(store.TiposSolicitudRGPD, sol.Kind) {
		return sol, httpx.ErrBadRequest.WithMessage("Say what the person is asking for: access, rectification, erasure, restriction, portability or objection.")
	}
	if !slices.Contains(store.CanalesSolicitudRGPD, sol.Channel) {
		return sol, httpx.ErrBadRequest.WithMessage("Say how the request arrived.")
	}
	ahora := time.Now()
	sol.ReceivedAt = ahora
	if v := strings.TrimSpace(in.ReceivedAt); v != "" {
		// La misma lectura de fechas que el filtro de la auditoría: RFC3339 o AAAA-MM-DD.
		t, err := parseAuditBound(v, false)
		if err != nil || t == nil {
			return sol, httpx.ErrBadRequest.WithMessage("The date the request arrived is not a valid date.")
		}
		// Una fecha de hoy sin hora no está «en el futuro».
		if t.After(ahora.Add(24*time.Hour)) || t.Before(ahora.Add(-recepcionMaxAtras)) {
			return sol, httpx.ErrBadRequest.WithMessage("The date the request arrived must be within the last year and not in the future.")
		}
		if t.Before(ahora) {
			sol.ReceivedAt = *t
		}
	}
	if id := strings.TrimSpace(in.UserID); id != "" {
		if !uuidRe.MatchString(id) {
			return sol, httpx.ErrBadRequest.WithMessage("Invalid user id.")
		}
		if _, err := s.store.GetMembership(r.Context(), orgID, id); err != nil {
			return sol, mapDomainErr(err)
		}
		u, err := s.store.GetUserByID(r.Context(), id)
		if err != nil {
			return sol, mapDomainErr(err)
		}
		sol.SubjectUserID, sol.SubjectEmail = id, u.Email
		if sol.SubjectName == "" {
			sol.SubjectName = u.Name
		}
		return sol, nil
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(in.SubjectEmail))
	if err != nil || len(addr.Address) > 320 {
		return sol, httpx.ErrBadRequest.WithMessage("Enter the email address of the person making the request.")
	}
	sol.SubjectEmail = domain.NormalizeEmail(addr.Address)
	if u, err := s.store.GetUserByEmail(r.Context(), sol.SubjectEmail); err == nil {
		if _, err := s.store.GetMembership(r.Context(), orgID, u.ID); err == nil {
			sol.SubjectUserID = u.ID
		}
	}
	return sol, nil
}

// handleAdminGetPrivacyRequest — GET /api/v1/admin/privacy/requests/{id}.
func (s *Server) handleAdminGetPrivacyRequest(w http.ResponseWriter, r *http.Request) {
	org, id, apiErr := s.solicitudDeLaOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	sol, eventos, err := s.store.SolicitudRGPDConHistoria(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if eventos == nil {
		eventos = []store.EventoSolicitudRGPD{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"request": sol, "events": eventos, "erasure": s.estadoSupresion(r, org.ID, sol),
	})
}

// estadoSupresion dice si la supresión física se puede ejecutar y, si no, por
// qué: la consola enseña el motivo en vez de un botón que luego falla.
func (s *Server) estadoSupresion(r *http.Request, orgID string, sol store.SolicitudRGPD) map[string]any {
	out := map[string]any{"available": false}
	switch {
	case sol.Kind != store.SolicitudSupresion:
		out["reason"] = "not_erasure"
	case sol.ErasureCompletedAt != nil:
		out["reason"] = "completed"
	case sol.ErasureStartedAt != nil:
		out["reason"] = "in_progress"
	case sol.ClosedAt != nil:
		out["reason"] = "closed"
	case sol.SubjectUserID == "":
		out["reason"] = "no_account"
	default:
		if _, err := s.store.GetMembership(r.Context(), orgID, sol.SubjectUserID); err != nil {
			out["reason"] = "not_member"
			return out
		}
		out["available"] = true
	}
	return out
}

// cuerpoCambiosRGPD son los cambios que admite PATCH.
type cuerpoCambiosRGPD struct {
	Status           *string `json:"status"`
	Resolution       *string `json:"resolution"`
	Notes            *string `json:"notes"`
	IdentityVerified *bool   `json:"identity_verified"`
	IdentityMethod   *string `json:"identity_method"`
}

// handleAdminUpdatePrivacyRequest — PATCH /api/v1/admin/privacy/requests/{id}.
func (s *Server) handleAdminUpdatePrivacyRequest(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, id, apiErr := s.solicitudDeLaOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	var in cuerpoCambiosRGPD
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Status != nil && !slices.Contains([]string{
		store.EstadoSolicitudRecibida, store.EstadoSolicitudEnCurso, store.EstadoSolicitudResuelta, store.EstadoSolicitudDenegada,
	}, *in.Status) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown status."))
		return
	}
	recortar := func(p *string, n int) *string {
		if p == nil {
			return nil
		}
		v := validation.TruncateRunes(strings.TrimSpace(*p), n)
		return &v
	}
	sol, err := s.store.ActualizarSolicitudRGPD(r.Context(), org.ID, id, store.CambiosSolicitudRGPD{
		Status: in.Status, Resolution: recortar(in.Resolution, 4000), Notes: recortar(in.Notes, 4000),
		IdentityVerified: in.IdentityVerified, IdentityMethod: recortar(in.IdentityMethod, 200),
	}, c.Subject)
	if err != nil {
		httpx.Fail(w, r, errorDeSolicitud(err))
		return
	}
	e := auditEntry(c.Subject, org.ID, "privacy.request.updated", id, r)
	e.Detail["status"] = sol.Status
	_ = s.store.Audit(r.Context(), nil, e)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"request": sol})
}

// handleAdminExtendPrivacyRequest — POST /api/v1/admin/privacy/requests/{id}/extend.
func (s *Server) handleAdminExtendPrivacyRequest(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, id, apiErr := s.solicitudDeLaOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	motivo := validation.TruncateRunes(strings.TrimSpace(in.Reason), 1000)
	if motivo == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"Say why more time is needed: the person has to be told the reasons (GDPR art. 12.3)."))
		return
	}
	sol, err := s.store.ProrrogarSolicitudRGPD(r.Context(), org.ID, id, motivo, c.Subject)
	if err != nil {
		httpx.Fail(w, r, errorDeSolicitud(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "privacy.request.extended", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"request": sol})
}

// handleAdminErasePrivacyRequest — POST /api/v1/admin/privacy/requests/{id}/erase.
// Pide reautenticación (requireStepUp) y que se escriba el correo de la persona:
// no se deshace.
func (s *Server) handleAdminErasePrivacyRequest(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, id, apiErr := s.solicitudDeLaOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actual, _, err := s.store.SolicitudRGPDConHistoria(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if actual.SubjectUserID == "" || actual.Kind != store.SolicitudSupresion {
		httpx.Fail(w, r, errorDeSolicitud(store.ErrSupresionNoAdmitida))
		return
	}
	if domain.NormalizeEmail(in.Confirm) != actual.SubjectEmail {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Type the email address of the person to confirm the erasure."))
		return
	}
	mem, err := s.store.GetMembership(r.Context(), org.ID, actual.SubjectUserID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	// Los mismos guardas que dar de baja: ni la propia cuenta ni, siendo admin, la
	// de un owner.
	if e := adminMayManage(c.Subject, actual.SubjectUserID, c.Roles, mem.Role); e != nil {
		httpx.Fail(w, r, e)
		return
	}
	sol, err := s.store.IniciarSupresionRGPD(r.Context(), org.ID, id, c.Subject)
	if err != nil {
		httpx.Fail(w, r, errorDeSolicitud(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "privacy.request.erasure_started", id, r))
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "org.user.erased", actual.SubjectUserID, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"request": sol,
		// Lo que se hace y lo que no, en la misma respuesta (ver el encabezado).
		"note": "The account is closed and its sign-in methods are gone; it is anonymised, with what other services keep about the person, " +
			"within minutes. Content the person created for the organisation (mailbox, files, events) stays with the organisation.",
	})
}

// solicitudDeLaOrg resuelve la organización activa y valida el {id}.
func (s *Server) solicitudDeLaOrg(r *http.Request) (domain.Organization, string, error) {
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		return domain.Organization{}, "", httpx.ErrBadRequest.WithMessage("Invalid request id.")
	}
	org, err := s.activeOrg(r)
	return org, id, err
}

// errorDeSolicitud traduce los errores del registro a respuestas con su motivo.
func errorDeSolicitud(err error) error {
	switch {
	case errors.Is(err, store.ErrSolicitudCerrada):
		return httpx.ErrConflict.WithMessage("This request is closed and can no longer change.")
	case errors.Is(err, store.ErrProrrogaNoAdmitida):
		return httpx.ErrConflict.WithMessage("The deadline can be extended only once, and before it expires.")
	case errors.Is(err, store.ErrSupresionNoAdmitida):
		return httpx.ErrConflict.WithMessage("This request does not allow erasing an account of the organisation, or the erasure is still running.")
	case errors.Is(err, store.ErrCuentaDeFuera):
		return httpx.ErrConflict.WithMessage("This account is also used outside your organisation (another organisation or the person's own account). " +
			"Remove the person from your organisation; they can delete the account themselves.")
	case errors.Is(err, store.ErrUnicoAdministrador):
		return httpx.ErrConflict.WithMessage("This person is the only one who administers an organisation: name someone else first.")
	case errors.Is(err, domain.ErrInvalidInput):
		return httpx.ErrBadRequest.WithMessage("Closing a request needs a written resolution: it is what the person is told.")
	default:
		return mapDomainErr(err)
	}
}
