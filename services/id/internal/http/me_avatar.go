// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Avatar propio: predefinido o foto, mismo circuito que la PHOTO de un contacto
// (ver services/contacts/internal/domain/photo.go y platform/pkg/validation) —
// un data URI raster acotado, validado por el mismo helper compartido.
//
//	GET    /api/v1/me/avatar   bytes del avatar (cacheable, condicional por ETag)
//	PUT    /api/v1/me/avatar   {photo: data URI} — predefinido o foto ya recortada
//	DELETE /api/v1/me/avatar   vuelve a las iniciales

type putAvatarRequest struct {
	Photo string `json:"photo"`
}

func (s *Server) handleMeAvatarGet(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	s.sirveAvatar(w, r, c.Subject)
}

// sirveAvatar escribe los bytes del avatar del usuario (ETag por updated_at,
// caché privada revalidable). Común al acceso por JWT (/api/v1/me/avatar) y al
// acceso por COOKIE de sesión (/account/avatar), que es el que pueden usar los
// <img> de las demás apps — un <img> no manda cabeceras de autorización.
func (s *Server) sirveAvatar(w http.ResponseWriter, r *http.Request, userID string) {
	dataURI, updatedAt, err := s.store.GetAvatar(r.Context(), userID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	raw, mime, err := validation.RasterImageBytes(dataURI)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	tag := `"` + versionAvatar(updatedAt) + `"`
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, tag) {
		w.Header().Set("ETag", tag)
		w.Header().Set("Cache-Control", avatarCacheControl)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", avatarCacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// versionAvatar es la versión de la foto: su updated_at en base 36. Es el ETag de
// los bytes, el `avatar_version` de GET /me y el `?v=` del claim `picture`, y
// tienen que ser la misma cadena para que un cliente pueda compararlas.
func versionAvatar(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 36) }

// avatarCacheControl: privado (por-usuario) y SIEMPRE revalidado. Con el
// `max-age=3600` de antes, `must-revalidate` no servía de nada durante esa hora:
// la barra de otra app pintaba la foto vieja al recargar, porque su `?v=` sale del
// claim del token, que no cambia hasta el siguiente refresco. Revalidar cuesta un
// 304 sin cuerpo (el ETag es la versión).
const avatarCacheControl = "private, no-cache"

func (s *Server) handleMeAvatarPut(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req putAvatarRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	norm, err := validation.RasterImageDataURI(req.Photo)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid image."))
		return
	}
	ver, err := s.store.SetAvatar(r.Context(), c.Subject, norm)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "user.avatar.updated", "", r))
	s.avisarAvatar(r.Context(), c.Tenant, c.Subject, versionAvatar(ver))
	s.propagateAvatarToContacts(r.Context(), c.Subject, norm)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMeAvatarDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if err := s.store.ClearAvatar(r.Context(), c.Subject); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "user.avatar.removed", "", r))
	s.avisarAvatar(r.Context(), c.Tenant, c.Subject, "")
	s.propagateAvatarToContacts(r.Context(), c.Subject, "")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true})
}

// clienteHub es el emisor de eventos al Hub, o nil si falta la URL o el secreto.
func clienteHub(cfg Config) *hubnotify.Client {
	if cfg.HubInternalSecret == "" {
		return nil
	}
	return hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret)
}

// avisarAvatar cuenta a las barras que la persona tiene abiertas —las trece apps de
// la web y las de Fyne— que su foto cambió, para que la vuelvan a pedir sin
// recargar. `version` es la nueva (la del ETag y el `?v=`), vacía si la quitó.
// Es un evento de estado, no un aviso: no sale en la campana. Mejor esfuerzo y sin
// esperar, como el reflejo en Contactos: sin Hub, la foto nueva se ve al entrar.
func (s *Server) avisarAvatar(ctx context.Context, tenant, subject, version string) {
	if tenant == "" {
		return
	}
	s.hub.SendEventAsync(ctx, hubnotify.Evento{
		Tenant: tenant, Subject: subject, Event: hubnotify.EventoAvatar,
		Data: map[string]string{"version": version},
	})
}

// avatarSyncRequest es el cuerpo que recibe POST /internal/avatar-sync de
// Sytue Contacts (services/contacts/internal/http/internal.go).
type avatarSyncRequest struct {
	Tenant string   `json:"tenant"`
	Emails []string `json:"emails"`
	// Photo vacío BORRA el avatar de las fichas que coincidan (quitó el suyo).
	Photo string `json:"photo"`
}

// propagateAvatarToContacts refleja el avatar propio en las fichas de contacto
// que coincidan por correo, en TODA la organización: no solo la libreta
// «Organización» (que crea/api/directorio.ts en Contactos), también las
// añadidas a mano por cualquier compañero, porque son la misma persona vista
// desde otra agenda. Por eso lo resuelve Contacts, no aquí: solo Contacts
// puede escribir en agendas que no son las de quien cambió su avatar.
//
// MEJOR ESFUERZO. Si Contacts no responde, el avatar propio ya quedó guardado
// (es lo primero que hace el llamante): lo único que falta es el reflejo en
// las fichas de los demás, y sale solo en el próximo cambio. Fallar la petición
// entera por esto convertiría un efecto secundario en un bloqueo del guardado.
func (s *Server) propagateAvatarToContacts(ctx context.Context, userID, photo string) {
	c, ok := claimsFrom(ctx)
	if !ok || c.Tenant == "" {
		return
	}
	s.propagateAvatarToContactsIn(ctx, c.Tenant, userID, photo)
}

// propagateAvatarToContactsIn es lo mismo para una organización dada, sin token:
// lo usa «Borrar mi cuenta», que quita la foto de las fichas de TODAS las
// organizaciones de la persona y no solo de la de su sesión.
func (s *Server) propagateAvatarToContactsIn(ctx context.Context, tenant, userID, photo string) {
	if s.cfg.ContactsInternalURL == "" || s.cfg.ContactsInternalSecret == "" {
		return
	}
	org, err := s.store.GetOrgBySlug(ctx, tenant)
	if err != nil {
		s.log.Warn("propagar avatar: organización no resuelta", "error", err)
		return
	}
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		s.log.Warn("propagar avatar: usuario no resuelto", "error", err)
		return
	}
	buzones, err := s.store.ListOrgMailboxes(ctx, org.ID, userID)
	if err != nil {
		s.log.Warn("propagar avatar: buzones no resueltos", "error", err)
		return
	}
	verificados, err := s.store.VerifiedOrgMailDomains(ctx, nil, org.ID)
	if err != nil {
		s.log.Warn("propagar avatar: dominios verificados no resueltos", "error", err)
		return
	}
	correos := domain.DireccionesDeAgenda(buzones, user.Email, verificados)
	if len(correos) == 0 {
		return
	}
	body, err := json.Marshal(avatarSyncRequest{Tenant: tenant, Emails: correos, Photo: photo})
	if err != nil {
		return
	}
	// Contexto propio y corto: la petición del navegador no debe esperar a
	// Contacts, y esta llamada tampoco debe quedar atada a que el cliente siga
	// conectado cuando responda.
	ctxLlamada, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctxLlamada, http.MethodPost,
		strings.TrimRight(s.cfg.ContactsInternalURL, "/")+"/internal/avatar-sync", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+s.cfg.ContactsInternalSecret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.log.Warn("propagar avatar: Contacts inalcanzable", "error", err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		s.log.Warn("propagar avatar: Contacts rechazó la sincronización", "status", resp.StatusCode)
	}
}
