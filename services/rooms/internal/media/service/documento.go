// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

// DocumentGate pregunta a Sytue Docs si QUIEN LLAMA puede ver un documento. Es la verja de
// las salas que nacen en un documento («Llamar desde el documento»): la lista de personas
// con acceso no se copia a Rooms, así que no puede quedarse vieja. Quitar a alguien del
// documento le cierra la sala en la siguiente petición.
type DocumentGate interface {
	CanAccessDocument(ctx context.Context, docID string) error
}

type bearerCtxKey struct{}

// WithBearer lleva el token Bearer de quien llama en el contexto, para presentárselo a Docs:
// es Docs quien aplica su ACL (grupos, organización, enlaces), no una réplica de ella aquí.
func WithBearer(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, bearerCtxKey{}, token)
}

func bearerFrom(ctx context.Context) string {
	v, _ := ctx.Value(bearerCtxKey{}).(string)
	return v
}

// DocsClient es el DocumentGate real: GET /api/v1/documents/{id} de Sytue Docs con el
// token de quien llama. 200 = puede verlo; cualquier otra cosa, no.
type DocsClient struct {
	baseURL string
	http    *http.Client
}

// NewDocsClient construye el cliente. nil si no hay URL: sin verja configurada las salas de
// documento quedan CERRADAS (fail-closed), nunca abiertas.
func NewDocsClient(baseURL string) *DocsClient {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	return &DocsClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		// ponytail: sin caché; si la sonda aparece en los perfiles, un TTL de pocos
		// segundos por (documento, persona).
		http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

// CanAccessDocument devuelve nil si Docs deja ver el documento a quien llama y
// domain.ErrPermission en cualquier otro caso (sin token, 403, 404, red caída, 5xx):
// fail-closed, no se abre una llamada de un documento si no se puede comprobar.
func (c *DocsClient) CanAccessDocument(ctx context.Context, docID string) error {
	token := bearerFrom(ctx)
	if token == "" {
		return domain.ErrPermission
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/documents/"+url.PathEscape(docID), http.NoBody)
	if err != nil {
		return domain.ErrPermission
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.Join(domain.ErrPermission, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domain.ErrPermission
	}
	return nil
}

var _ DocumentGate = (*DocsClient)(nil)

// CalendarGate pregunta a Sytue Calendar si QUIEN LLAMA está invitado al evento de ese
// UID (organiza, figura como asistente o recibió su invitación). Es lo que abre la reunión
// de un evento a sus invitados aunque el enlace que tengan sea de antes de la clave
// (decidido por el dueño el 2026-10-16: «Invitados y quien tenga el enlace»).
type CalendarGate interface {
	EsInvitado(ctx context.Context, uid string) bool
}

// CalendarClient es el CalendarGate real: GET /api/v1/events/invited?uid=… de Sytue
// Calendar con el token de quien llama. 204 = invitado; cualquier otra cosa, no.
type CalendarClient struct {
	baseURL string
	http    *http.Client
}

// NewCalendarClient construye el cliente; nil sin URL (nadie entra por ser invitado).
func NewCalendarClient(baseURL string) *CalendarClient {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	return &CalendarClient{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 3 * time.Second}}
}

// EsInvitado: fail-closed, sin token o sin respuesta no se abre nada.
func (c *CalendarClient) EsInvitado(ctx context.Context, uid string) bool {
	token := bearerFrom(ctx)
	if token == "" || uid == "" {
		return false
	}
	// `org`: la organización de la sala, que para quien viene de fuera no es la de su token.
	q := url.Values{"uid": {uid}}
	if t, err := tenancy.From(ctx); err == nil {
		q.Set("org", t.Slug)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/events/invited?"+q.Encode(), http.NoBody)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusNoContent
}

var _ CalendarGate = (*CalendarClient)(nil)

// documentRef devuelve el id del documento ligado a la sala (source.ref_id) si su origen
// es un documento; "" en caso contrario.
func documentRef(room domain.Room) string {
	src := domain.SourceDeMetadata(room.Metadata)
	if src.Kind != domain.SourceDocument {
		return ""
	}
	return src.RefID
}

// CallAccess returns a versioned, time-bounded assertion for a native encrypted
// call. It is deliberately narrower than generic document viewing.
func (c *DocsClient) CallAccess(ctx context.Context, id string) (domain.DocumentCallAccess, error) {
	var proof domain.DocumentCallAccess
	token := bearerFrom(ctx)
	if token == "" {
		return proof, domain.ErrPermission
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/documents/"+url.PathEscape(id)+"/call-access", http.NoBody)
	if err != nil {
		return proof, domain.ErrPermission
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := c.http.Do(req)
	if err != nil {
		return proof, domain.ErrPermission
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return proof, domain.ErrPermission
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(&proof); err != nil {
		return proof, domain.ErrPermission
	}
	if proof.DocumentID != id || proof.Actor == "" || proof.Tenant == "" || proof.IdentityVersion < 0 || proof.Version < 0 || proof.ExpiresAtMS < 0 || proof.ExpiresAtMS > 0 && proof.ExpiresAtMS <= time.Now().UnixMilli() {
		return proof, domain.ErrPermission
	}
	return proof, nil
}
func (s *Service) SetDocumentEncryptionReady(ready bool) { s.documentEncryptionReady = ready }
func (s *Service) encryptedDocumentContext(ctx context.Context, room domain.Room, actor string) (context.Context, error) {
	id := documentRef(room)
	if id == "" {
		return ctx, nil
	}
	if !s.documentEncryptionReady {
		return ctx, domain.ErrEncryptedOperation
	}
	if proof, ok := domain.DocumentCallAccessFrom(ctx); ok && proof.DocumentID == id && proof.Actor == actor {
		return ctx, nil
	}

	gate, ok := s.docGate.(interface {
		CallAccess(context.Context, string) (domain.DocumentCallAccess, error)
	})
	if !ok {
		return ctx, domain.ErrPermission
	}
	proof, err := gate.CallAccess(ctx, id)
	if err != nil || proof.Actor != actor {
		return ctx, domain.ErrPermission
	}
	return domain.WithDocumentCallAccess(ctx, proof), nil
}

func (s *Service) encryptedRoomAccess(ctx context.Context, room domain.Room, actor string, admin bool) (context.Context, error) {
	if documentRef(room) != "" && s.encryption != nil {
		return s.encryptedDocumentContext(ctx, room, actor)
	}
	return ctx, s.requireRoomAccess(ctx, room, actor, admin)
}
