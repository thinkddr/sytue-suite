// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"cmp"
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

func (h *Handler) listPermissions(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner); err != nil {
		fail(w, r, err)
		return
	}
	perms, err := h.store.ListPermissions(r.Context(), docID(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"permissions": perms})
}

type upsertPermissionReq struct {
	PrincipalKind domain.PrincipalKind `json:"principal_kind"`
	PrincipalID   string               `json:"principal_id"`
	Role          domain.Role          `json:"role"`
}

func (h *Handler) upsertPermission(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner); err != nil {
		fail(w, r, err)
		return
	}
	var req upsertPermissionReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !req.PrincipalKind.Valid() || req.PrincipalKind == domain.PrincipalPublic {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid principal kind (user|group|tenant)."))
		return
	}
	if !req.Role.Valid() {
		fail(w, r, domain.ErrInvalidRole)
		return
	}
	perm := domain.Permission{
		DocumentID:    docID(r),
		PrincipalKind: req.PrincipalKind,
		PrincipalID:   req.PrincipalID,
		Role:          req.Role,
		GrantedBy:     subj.Sub,
	}
	if err := h.store.UpsertPermission(r.Context(), docID(r), perm); err != nil {
		fail(w, r, err)
		return
	}
	// Aviso al centro de notificaciones cuando lo compartido es con una
	// PERSONA: a grupos y a todo el tenant no hay buzón concreto al que avisar.
	// Cubre Docus, Calcus, Diapos y certiDocs, que son este mismo servicio.
	if req.PrincipalKind == domain.PrincipalUser {
		h.notifyShare(r.Context(), perm)
		// Si tiene el documento abierto, su franja de solo lectura cambia en vivo.
		// ponytail: solo concesiones a una persona; un grupo o la organización entera
		// se ven al recargar, y quitar un permiso también (puede quedarle el de un grupo).
		h.hub.PublishRole(r.Context(), perm.DocumentID, perm.PrincipalID, perm.Role)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"permission": perm})
}

// notifyShare deja en el Hub el aviso de «compartido contigo». Sin título no
// hay aviso (documento purgado a mitad de la concesión) y sin Hub configurado
// tampoco: compartir sigue siendo el hecho, el aviso es el extra.
func (h *Handler) notifyShare(ctx context.Context, perm domain.Permission) {
	// Una dirección en vez de un `sub` es la concesión que llega de la web tal cual se
	// escribió: el acceso se resuelve contra el `sub` y esa no abre nada, así que avisar
	// sería mandar a alguien a un documento que no puede leer.
	if h.hubNotifier == nil || strings.Contains(perm.PrincipalID, "@") {
		return
	}
	doc, err := h.store.GetDocument(ctx, perm.DocumentID)
	if err != nil || doc == nil {
		return
	}
	h.avisar(ctx, perm.PrincipalID, doc, "docs.shared", tituloCompartido(doc), "")
}

// avisar deja un aviso de un documento en el Hub. El origen es la app que lo abre
// —una hoja de cálculo la abre Calcus, no Docus— y el título, el respaldo de un
// cliente que no sepa componerlo con `params`.
func (h *Handler) avisar(ctx context.Context, destinatario string, doc *domain.Document, tipo, titulo, cuerpo string) {
	if h.hubNotifier == nil || destinatario == "" {
		return
	}
	t, _ := tenancy.From(ctx)
	actor := ""
	if c, ok := authn.FromContext(ctx); ok {
		actor = cmp.Or(strings.TrimSpace(c.Name), c.Email)
	}
	h.hubNotifier.SendAsync(ctx, avisoDeDocumento(h.publicURL, t.Slug, destinatario, doc, tipo, titulo, cuerpo, actor))
}

// Títulos de respaldo de los avisos de un documento (los clientes los componen en su
// idioma con `params`).
func tituloCompartido(doc *domain.Document) string { return "Compartido contigo: " + doc.Title }
func tituloComentario(doc *domain.Document) string { return "Comentario en «" + doc.Title + "»" }
func tituloMencion(doc *domain.Document) string    { return "Te han mencionado en «" + doc.Title + "»" }

// avisoDeDocumento es el aviso de un documento: el mismo en vivo que en la
// reconstrucción de avisos (AvisosHistoricos). Sin `actor`, sin ese parámetro.
func avisoDeDocumento(publicURL, tenant, destinatario string, doc *domain.Document, tipo, titulo, cuerpo, actor string) hubnotify.Notification {
	params := map[string]string{"name": doc.Title}
	if actor != "" {
		params["actor"] = actor
	}
	return hubnotify.Notification{
		Tenant:  tenant,
		Subject: destinatario,
		Source:  origenDe(doc.Kind),
		Type:    tipo,
		Level:   "info",
		Title:   titulo,
		Body:    cuerpo,
		URL:     hubnotify.EnlaceDocumento(publicURL, tenant, doc.ID, string(doc.Kind)),
		Params:  params,
	}
}

// origenDe es la clave de producto de la app nativa que abre un documento.
func origenDe(k domain.Kind) string {
	switch k {
	case domain.KindSpreadsheet:
		return "sheets"
	case domain.KindPresentation:
		return "slides"
	default:
		return "docs"
	}
}

func (h *Handler) revokePermission(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner); err != nil {
		fail(w, r, err)
		return
	}
	kind := domain.PrincipalKind(r.URL.Query().Get("principal_kind"))
	pid := r.URL.Query().Get("principal_id")
	if !kind.Valid() || pid == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("principal_kind/principal_id are missing."))
		return
	}
	if err := h.store.RevokePermission(r.Context(), docID(r), kind, pid); err != nil {
		fail(w, r, err)
		return
	}
	// Si lo tiene abierto, se le corta el canal en vivo (ADR 0052). Si aún puede
	// entrar por otro lado —un grupo, su organización—, al reconectar se autoriza de
	// nuevo y vuelve con lo que le quede.
	if kind == domain.PrincipalUser {
		h.hub.PublishRole(r.Context(), docID(r), pid, "")
	}
	httpx.NoContent(w)
}

// --- Enlaces de compartición ---

func (h *Handler) listShares(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner); err != nil {
		fail(w, r, err)
		return
	}
	links, err := h.store.ListShareLinks(r.Context(), docID(r))
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"shares": links})
}

type createShareReq struct {
	Role      domain.Role `json:"role"`
	ExpiresAt string      `json:"expires_at,omitempty"` // RFC3339
	// Password es opcional: con ella, el enlace exige además esa contraseña.
	Password string `json:"password,omitempty"`
}

func (h *Handler) createShare(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req createShareReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	exp, err := parseExpiry(req.ExpiresAt)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("expires_at must be an RFC3339 date."))
		return
	}
	link, err := h.svc.CreateShare(r.Context(), subj, docID(r), req.Role, exp, req.Password)
	if err != nil {
		fail(w, r, err)
		return
	}
	auditar(r, "enlace.creado", "documento", docID(r), "enlace", link.ID, "rol", string(link.Role), "caduca", link.ExpiresAt, "con_clave", link.HasPassword)
	// El token EN CLARO solo se devuelve aquí, una vez.
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"share": link})
}

func (h *Handler) revokeShare(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleOwner); err != nil {
		fail(w, r, err)
		return
	}
	linkID := chi.URLParam(r, "linkId")
	// Quién lo canjeó, ANTES de revocar: a quien tenga el documento abierto se le
	// corta el canal en vivo (ADR 0052); sin esto seguía recibiendo los cambios hasta
	// reconectar. Al reconectar se autoriza de nuevo, con lo que le quede.
	var canjearon []string
	if h.canje != nil {
		canjearon, _ = h.canje.CanjeadosDe(r.Context(), docID(r), linkID)
	}
	if err := h.store.RevokeShareLink(r.Context(), docID(r), linkID); err != nil {
		fail(w, r, err)
		return
	}
	for _, sub := range canjearon {
		h.hub.PublishRole(r.Context(), docID(r), sub, "")
	}
	auditar(r, "enlace.revocado", "documento", docID(r), "enlace", linkID)
	httpx.NoContent(w)
}

// resolveShare resuelve un token de enlace público SIN JWT. Devuelve el
// documento y el rol concedido.
func (h *Handler) resolveShare(w http.ResponseWriter, r *http.Request) {
	_, res, doc, ok := h.porEnlace(w, r)
	if !ok {
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"document": doc,
		"role":     res.Role,
		// El front intercambia este acceso por una sesión/cookie efímera vía el
		// BFF; el WS y las ops requieren credenciales emitidas por ese flujo.
	})
}

// viewShare es la vista de SOLO LECTURA de un enlace público: la página
// `/s/{token}` de la web. Sin JWT: la credencial es el token. El documento va
// como el HTML de «Descargar como HTML», que NO está saneado (es el del
// editor), así que la web lo pinta en un iframe con sandbox y sin scripts.
func (h *Handler) viewShare(w http.ResponseWriter, r *http.Request) {
	ctx, res, doc, ok := h.porEnlace(w, r)
	if !ok {
		return
	}
	state, err := h.svc.LoadShared(ctx, res, h.hydrateFromS3) //nolint:contextcheck // deriva de r.Context()
	if err != nil {
		fail(w, r, err)
		return
	}
	page, _, _, err := domain.Export(doc.Kind, doc.Title, state, domain.FormatHTML)
	if err != nil {
		fail(w, r, err)
		return
	}
	// «Cualquiera con el enlace» no es «cualquiera»: fuera de los buscadores.
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"id":    doc.ID,
		"title": doc.Title,
		"kind":  doc.Kind,
		"role":  res.Role,
		"html":  string(page),
	})
}

// porEnlace resuelve el token de la ruta y abre el documento bajo SU tenant, para
// que las lecturas siguientes respeten RLS. Si falla, ya ha contestado.
func (h *Handler) porEnlace(w http.ResponseWriter, r *http.Request) (context.Context, *domain.ShareResolution, *domain.Document, bool) {
	token := chi.URLParam(r, "token")
	if token == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return nil, nil, nil, false
	}
	// Un enlace con contraseña no se abre solo con el token.
	if err := h.exigirClave(r.Context(), token, r.Header.Get(cabeceraPrueba)); err != nil {
		fail(w, r, err)
		return nil, nil, nil, false
	}
	res, err := h.svc.ResolveShare(r.Context(), token)
	if err != nil {
		fail(w, r, err)
		return nil, nil, nil, false
	}
	t, err := tenancy.New(res.Tenant)
	if err != nil {
		fail(w, r, err)
		return nil, nil, nil, false
	}
	ctx := tenancy.Into(r.Context(), t)
	doc, err := h.store.GetDocument(ctx, res.DocumentID)
	if err != nil {
		fail(w, r, err)
		return nil, nil, nil, false
	}
	return ctx, res, doc, true
}
