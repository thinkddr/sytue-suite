// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package scim implementa el aprovisionamiento SCIM 2.0 (RFC 7643/7644) de
// Sytue ID: recurso User con list/create/get/replace/patch/delete y los
// metadatos del proveedor. Clean-room sobre la especificación SCIM; no porta
// código AGPL (docs/CONVENTIONS.md §3, §12).
package scim

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Handler agrupa los endpoints SCIM.
type Handler struct {
	store  *store.Store
	issuer string
	// orgSlug liga el token SCIM (estático, del proveedor de aprovisionamiento) a
	// UNA organización: TODOS los recursos se acotan a ella (aislamiento por
	// tenant, no cruza) y el usuario aprovisionado por Users pasa a ser miembro de
	// ella. Vacío ⇒ SCIM NO se sirve: el router responde 503 igual que si faltara
	// el token (requireSCIMAuth) y estos handlers deniegan. Sin org ligada no hay
	// ámbito, y «sin ámbito» sobre un directorio significa TODAS las
	// organizaciones del despliegue.
	// ponytail: un token → una org (SYTUE_ID_SCIM_ORG_SLUG); es el modelo de una
	// conexión de aprovisionamiento de Entra/Okta. Si hiciera falta multi-org en un
	// mismo despliegue, promover a una tabla de tokens SCIM (hash→org, como
	// dcr_initial_tokens) — nunca a un token único sin acotar.
	orgSlug string
}

// New construye el Handler SCIM ligado a la org de orgSlug (obligatoria: sin
// ella el router no monta SCIM).
func New(st *store.Store, issuer, orgSlug string) *Handler {
	return &Handler{store: st, issuer: issuer, orgSlug: orgSlug}
}

const (
	schemaUser         = "urn:ietf:params:scim:schemas:core:2.0:User"
	schemaGroup        = "urn:ietf:params:scim:schemas:core:2.0:Group"
	schemaListResponse = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	schemaError        = "urn:ietf:params:scim:api:messages:2.0:Error"
	contentType        = "application/scim+json"
)

// errSCIMOrgUnset señala que el token SCIM no está ligado a ninguna organización
// (SYTUE_ID_SCIM_ORG_SLUG vacío). Lo exigen TODOS los recursos, Users incluido.
var errSCIMOrgUnset = errors.New("scim: token no asociado a una organización")

// resolveOrg devuelve la organización a la que está ligado el token SCIM. Es la
// verja de aislamiento del recurso Group: todo se acota a org.ID, jamás a un id
// del path. errSCIMOrgUnset si no hay slug configurado; ErrNotFound si el slug no
// existe (mala configuración).
func (h *Handler) resolveOrg(ctx context.Context) (domain.Organization, error) {
	// La organización del TOKEN manda sobre la de la configuración. Desde la mig
	// 0055 cada organización crea el suyo desde su consola, así que la misma
	// instalación sirve a varias a la vez; el middleware resuelve cuál es y la
	// deja en el contexto. El slug de configuración se conserva para el token
	// único del despliegue, que sigue valiendo mientras exista.
	if org, ok := OrgFrom(ctx); ok {
		return org, nil
	}
	if h.orgSlug == "" {
		return domain.Organization{}, errSCIMOrgUnset
	}
	return h.store.GetOrgBySlug(ctx, h.orgSlug)
}

// orgCtxKey transporta la organización del token SCIM desde el middleware.
type orgCtxKey struct{}

// WithOrg deja la organización del token en el contexto (lo llama el middleware
// de autenticación, que es quien lo ha resuelto).
func WithOrg(ctx context.Context, org domain.Organization) context.Context {
	return context.WithValue(ctx, orgCtxKey{}, org)
}

// OrgFrom recupera la organización del token, si el middleware la puso.
func OrgFrom(ctx context.Context) (domain.Organization, bool) {
	org, ok := ctx.Value(orgCtxKey{}).(domain.Organization)
	return org, ok && org.ID != ""
}

// userInScope es la verja anti-IDOR cross-tenant de /Users: solo es true si {id} es
// MIEMBRO de la org del token. Los handlers 404 cuando devuelve false — sin distinguir
// «no existe» de «otra org» (anti-enumeración). Espeja el aislamiento que /Groups ya
// hace con groupInOrg: un token de la org A no puede leer/suspender/borrar/reescribir
// usuarios de B. Sin org ligada NO hay ámbito que comprobar y se DENIEGA (el router ya
// no monta SCIM en ese caso; esto es la segunda cerradura de la misma puerta).
func (h *Handler) userInScope(ctx context.Context, id string) (bool, error) {
	org, err := h.resolveOrg(ctx)
	if err != nil {
		return false, err
	}
	_, mErr := h.store.GetMembership(ctx, org.ID, id)
	return mErr == nil, nil
}

// scimUser es la representación SCIM de un usuario. Además del núcleo lleva los
// atributos de perfil que alimentan las reglas de grupos dinámicos y la extensión
// enterprise que los transporta (ver attributes.go para el mapeo exacto).
type scimUser struct {
	Schemas     []string        `json:"schemas"`
	ID          string          `json:"id"`
	ExternalID  string          `json:"externalId,omitempty"`
	UserName    string          `json:"userName"`
	Name        *scimName       `json:"name,omitempty"`
	DisplayName string          `json:"displayName,omitempty"`
	Title       string          `json:"title,omitempty"`
	Emails      []scimEmail     `json:"emails,omitempty"`
	Addresses   []scimAddress   `json:"addresses,omitempty"`
	Enterprise  *scimEnterprise `json:"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User,omitempty"`
	Active      *bool           `json:"active,omitempty"`
	Meta        scimMeta        `json:"meta"`
}

type scimName struct {
	Formatted  string `json:"formatted,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

type scimEmail struct {
	Value   string `json:"value"`
	Primary bool   `json:"primary,omitempty"`
}

type scimMeta struct {
	ResourceType string    `json:"resourceType"`
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	Location     string    `json:"location"`
}

func (h *Handler) toSCIM(u domain.User) scimUser {
	active := u.Status == domain.StatusActive
	su := scimUser{
		Schemas:    []string{schemaUser},
		ID:         u.ID,
		ExternalID: u.ExternalID,
		UserName:   u.Email,
		Active:     &active,
		Emails:     []scimEmail{{Value: u.Email, Primary: true}},
		Meta: scimMeta{
			ResourceType: "User",
			Created:      u.CreatedAt,
			LastModified: u.UpdatedAt,
			Location:     h.issuer + "/scim/v2/Users/" + u.ID,
		},
	}
	if u.Name != "" || u.GivenName != "" || u.FamilyName != "" {
		su.Name = &scimName{Formatted: u.Name, GivenName: u.GivenName, FamilyName: u.FamilyName}
		su.DisplayName = u.Name
	}
	// Los atributos de perfil se DEVUELVEN en su sitio del esquema: title en el núcleo,
	// department en la extensión enterprise (declarándola en `schemas`, como exige el
	// §3.3) y location como la localidad de la dirección de trabajo. Así un cliente que
	// lee-modifica-escribe (PUT) no los pierde por el camino.
	su.Title = u.Title
	if u.Department != "" {
		su.Schemas = append(su.Schemas, schemaEnterpriseUser)
		su.Enterprise = &scimEnterprise{Department: u.Department}
	}
	if u.Location != "" {
		su.Addresses = []scimAddress{{Type: "work", Locality: u.Location, Primary: true}}
	}
	return su
}

func writeSCIM(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = jsonEncode(w, v)
}

func jsonEncode(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}

func scimErr(w http.ResponseWriter, status int, detail string) {
	writeSCIM(w, status, map[string]any{
		"schemas": []string{schemaError},
		"status":  strconv.Itoa(status),
		"detail":  detail,
	})
}

// ListUsers implementa GET /Users con filtro básico userName eq y paginación.
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filterEmail := parseUserNameFilter(q.Get("filter"))
	start, _ := strconv.Atoi(q.Get("startIndex"))
	if start < 1 {
		start = 1
	}
	count, _ := strconv.Atoi(q.Get("count"))
	if count <= 0 {
		count = 50
	}
	// SOLO los miembros de la org del token: sin ese filtro esto enumeraría a
	// TODOS los usuarios del despliegue, de todas las organizaciones.
	org, oerr := h.resolveOrg(r.Context())
	if oerr != nil {
		scimErr(w, http.StatusInternalServerError, "SCIM: organización no resoluble")
		return
	}
	users, total, err := h.store.ListOrgUsers(r.Context(), org.ID, filterEmail, start-1, count)
	if err != nil {
		scimErr(w, http.StatusInternalServerError, "error al listar usuarios")
		return
	}
	resources := make([]scimUser, 0, len(users))
	for _, u := range users {
		resources = append(resources, h.toSCIM(u))
	}
	writeSCIM(w, http.StatusOK, map[string]any{
		"schemas":      []string{schemaListResponse},
		"totalResults": total,
		"startIndex":   start,
		"itemsPerPage": len(resources),
		"Resources":    resources,
	})
}

// GetUser implementa GET /Users/{id}.
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if ok, err := h.userInScope(r.Context(), id); err != nil {
		scimErr(w, http.StatusInternalServerError, "SCIM: organización no resoluble")
		return
	} else if !ok {
		scimErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	u, err := h.store.GetUserByID(r.Context(), id)
	if err != nil {
		scimErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	writeSCIM(w, http.StatusOK, h.toSCIM(u))
}

// CreateUser implementa POST /Users (provisioning idempotente por externalId/email).
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var in scimUser
	if err := decodeSCIM(w, r, &in); err != nil {
		scimErr(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	email := strings.TrimSpace(in.UserName)
	if email == "" && len(in.Emails) > 0 {
		email = in.Emails[0].Value
	}
	if email == "" {
		scimErr(w, http.StatusBadRequest, "userName/email obligatorio")
		return
	}
	u := domain.User{
		Email:      email,
		ExternalID: in.ExternalID,
		Status:     activeToStatus(boolOrTrue(in.Active)),
	}
	applySCIMAttrs(&u, in)
	// El usuario aprovisionado ES miembro de la org del token (rol member, el
	// mínimo) en la MISMA tx que su alta: sin membresía no obtendría token de la
	// org y su pertenencia a grupos (claim `groups` / ACL) quedaría inerte. NO
	// escala privilegios: el rol de org lo fija esto (member), nunca los grupos
	// SCIM (arbitrarios, alimentan shares de Drive, no roles). Sin org ligada no
	// se crea nada: un usuario sin membresía no es de nadie.
	org, orgErr := h.resolveOrg(r.Context())
	if orgErr != nil {
		scimErr(w, http.StatusInternalServerError, "SCIM: organización no resoluble")
		return
	}
	var created domain.User
	ctx := r.Context()
	err := h.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		if created, e = h.store.CreateUser(ctx, q, u); e != nil {
			return e
		}
		if mErr := h.store.UpsertMembership(ctx, q, domain.Membership{
			OrgID: org.ID, UserID: created.ID, Role: domain.RoleMember,
		}); mErr != nil {
			return mErr
		}
		// El alta que llega del directorio corporativo entra SOLA en los grupos
		// dinámicos que le corresponden por department/title/location, en la MISMA
		// tx: su primer token ya lleva el claim `groups` correcto.
		return h.store.ReevaluateUserDynamicGroups(ctx, q, org.ID, created.ID)
	})
	if err != nil {
		if isConflict(err) {
			scimErr(w, http.StatusConflict, "el usuario ya existe")
			return
		}
		scimErr(w, http.StatusInternalServerError, "error al crear usuario")
		return
	}
	_ = h.store.Audit(r.Context(), nil, store.AuditEntry{OrgID: org.ID, Action: "scim.user.created", Target: created.ID, IP: clientIP(r)})
	w.Header().Set("Location", h.issuer+"/scim/v2/Users/"+created.ID)
	writeSCIM(w, http.StatusCreated, h.toSCIM(created))
}

// ReplaceUser implementa PUT /Users/{id}.
func (h *Handler) ReplaceUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if ok, err := h.userInScope(r.Context(), id); err != nil {
		scimErr(w, http.StatusInternalServerError, "SCIM: organización no resoluble")
		return
	} else if !ok {
		scimErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	existing, err := h.store.GetUserByID(r.Context(), id)
	if err != nil {
		scimErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	var in scimUser
	if err := decodeSCIM(w, r, &in); err != nil {
		scimErr(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	existing.Email = orDefault(in.UserName, existing.Email)
	existing.Status = activeToStatus(boolOrTrue(in.Active))
	applySCIMAttrs(&existing, in)
	updated, err := h.saveUser(r.Context(), existing)
	if err != nil {
		scimErr(w, http.StatusInternalServerError, "error al actualizar usuario")
		return
	}
	writeSCIM(w, http.StatusOK, h.toSCIM(updated))
}

// saveUser persiste los cambios de un usuario y —si el token SCIM está ligado a una org—
// REEVALÚA sus grupos dinámicos en la MISMA tx. Es el disparador que faltaba: un cambio de
// department/title/location que llega del directorio corporativo mueve la membresía SIN
// intervención de nadie, igual que ya hacía el PATCH de perfil de la Console. Sin org
// ligada (modo global, compat) no hay tenant contra el que evaluar y solo se persiste.
func (h *Handler) saveUser(ctx context.Context, u domain.User) (domain.User, error) {
	org, orgErr := h.resolveOrg(ctx)
	if orgErr != nil {
		// El handler ya pasó por userInScope: aquí orgErr solo puede ser «sin org ligada».
		return h.store.UpdateUser(ctx, nil, u)
	}
	var updated domain.User
	err := h.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		if updated, e = h.store.UpdateUser(ctx, q, u); e != nil {
			return e
		}
		return h.store.ReevaluateUserDynamicGroups(ctx, q, org.ID, u.ID)
	})
	return updated, err
}

// patchBody es el cuerpo de PATCH (RFC 7644 §3.5.2), soporte de replace de active.
type patchBody struct {
	Schemas    []string `json:"schemas"`
	Operations []struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value any    `json:"value"`
	} `json:"Operations"`
}

// PatchUser implementa PATCH /Users/{id}: active, userName, name/displayName y los
// atributos de perfil que gobiernan los grupos dinámicos (title, department de la
// extensión enterprise, localidad de la dirección de trabajo). Ver attributes.go.
func (h *Handler) PatchUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if ok, err := h.userInScope(r.Context(), id); err != nil {
		scimErr(w, http.StatusInternalServerError, "SCIM: organización no resoluble")
		return
	} else if !ok {
		scimErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	existing, err := h.store.GetUserByID(r.Context(), id)
	if err != nil {
		scimErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	var body patchBody
	if err := decodeSCIM(w, r, &body); err != nil {
		scimErr(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	for _, op := range body.Operations {
		switch {
		case strings.EqualFold(op.Op, "replace"), strings.EqualFold(op.Op, "add"):
			applySCIMPatchOp(&existing, op.Path, op.Value)
		case strings.EqualFold(op.Op, "remove"):
			// remove viene sin value ⇒ el atributo queda vacío. Es como el directorio
			// dice «este usuario ya no tiene departamento» y sale del grupo dinámico.
			applySCIMPatchOp(&existing, op.Path, nil)
		}
	}
	updated, err := h.saveUser(r.Context(), existing)
	if err != nil {
		scimErr(w, http.StatusInternalServerError, "error al actualizar usuario")
		return
	}
	writeSCIM(w, http.StatusOK, h.toSCIM(updated))
}

// DeleteUser implementa DELETE /Users/{id} (desaprovisionamiento => borrado lógico).
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if ok, err := h.userInScope(r.Context(), id); err != nil {
		scimErr(w, http.StatusInternalServerError, "SCIM: organización no resoluble")
		return
	} else if !ok {
		scimErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	if err := h.store.DeleteUser(r.Context(), nil, id); err != nil {
		scimErr(w, http.StatusInternalServerError, "error al eliminar usuario")
		return
	}
	_ = h.store.Audit(r.Context(), nil, store.AuditEntry{Action: "scim.user.deleted", Target: id, IP: clientIP(r)})
	w.WriteHeader(http.StatusNoContent)
}

func parseUserNameFilter(filter string) string {
	// Soporta: userName eq "valor"
	f := strings.TrimSpace(filter)
	low := strings.ToLower(f)
	if !strings.HasPrefix(low, "username eq ") {
		return ""
	}
	rest := strings.TrimSpace(f[len("userName eq "):])
	return strings.Trim(rest, `"`)
}

// boolOrTrue devuelve el valor del puntero, o true si es nil (SCIM: active
// ausente => activo).
func boolOrTrue(b *bool) bool {
	if b == nil {
		return true
	}
	return *b
}

func activeToStatus(active bool) domain.Status {
	if active {
		return domain.StatusActive
	}
	return domain.StatusSuspended
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func isConflict(err error) bool {
	return errors.Is(err, domain.ErrConflict)
}

// ServiceProviderConfig describe las capacidades SCIM del proveedor (§4 RFC 7643).
func (h *Handler) ServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	writeSCIM(w, http.StatusOK, map[string]any{
		"schemas":          []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"documentationUri": h.issuer + "/docs/scim",
		"patch":            map[string]bool{"supported": true},
		"bulk":             map[string]any{"supported": false, "maxOperations": 0},
		"filter":           map[string]any{"supported": true, "maxResults": 200},
		"changePassword":   map[string]bool{"supported": false},
		"sort":             map[string]bool{"supported": false},
		"etag":             map[string]bool{"supported": false},
		"authenticationSchemes": []map[string]any{{
			"type":        "oauthbearertoken",
			"name":        "Bearer Token",
			"description": "Token bearer estático del proveedor de aprovisionamiento.",
		}},
	})
}

// ResourceTypes lista los tipos de recurso soportados (User y Group).
func (h *Handler) ResourceTypes(w http.ResponseWriter, r *http.Request) {
	rt := "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	writeSCIM(w, http.StatusOK, []map[string]any{
		{
			"schemas": []string{rt}, "id": "User", "name": "User", "endpoint": "/Users", "schema": schemaUser,
			// La extensión enterprise se DECLARA (§6): es lo que un cliente
			// conformante consulta para saber que puede mandar department.
			"schemaExtensions": []map[string]any{{"schema": schemaEnterpriseUser, "required": false}},
		},
		{"schemas": []string{rt}, "id": "Group", "name": "Group", "endpoint": "/Groups", "schema": schemaGroup},
	})
}

// Schemas devuelve el esquema de los recursos User y Group (subconjunto descriptivo).
func (h *Handler) Schemas(w http.ResponseWriter, r *http.Request) {
	writeSCIM(w, http.StatusOK, []map[string]any{
		{
			"id":          schemaUser,
			"name":        "User",
			"description": "Usuario de Sytue ID",
			"attributes": []map[string]any{
				{"name": "userName", "type": "string", "required": true, "uniqueness": "server"},
				{"name": "active", "type": "boolean", "required": false},
				{"name": "emails", "type": "complex", "multiValued": true},
				{"name": "name", "type": "complex", "required": false},
				{"name": "displayName", "type": "string", "required": false},
				{"name": "title", "type": "string", "required": false},
				{"name": "addresses", "type": "complex", "multiValued": true, "required": false},
			},
		},
		{
			// Extensión enterprise: solo declaramos department, que es el único campo
			// que Sytue ID PERSISTE (alimenta las reglas de grupos dinámicos).
			"id":          schemaEnterpriseUser,
			"name":        "EnterpriseUser",
			"description": "Extensión enterprise del usuario (atributos de directorio corporativo)",
			"attributes": []map[string]any{
				{"name": "department", "type": "string", "required": false},
			},
		},
		{
			"id":          schemaGroup,
			"name":        "Group",
			"description": "Grupo de Sytue ID (aprovisionamiento SCIM)",
			"attributes": []map[string]any{
				{"name": "displayName", "type": "string", "required": true, "uniqueness": "server"},
				{"name": "members", "type": "complex", "multiValued": true, "required": false},
			},
		},
	})
}

// clientIP es la misma resolución que la del IdP: X-Forwarded-For solo se cree
// cuando quien conecta es un proxy de confianza, y dentro de la cabecera se lee
// de derecha a izquierda (ver domain.TrustedProxies). La IP acaba en una columna
// `inet` del registro de auditoría, así que se normaliza.
func clientIP(r *http.Request) string {
	return domain.TrustedProxiesFrom(r.Context()).
		ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
}
