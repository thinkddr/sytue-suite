// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Recurso Group de SCIM 2.0 (RFC 7643 §4.2, RFC 7644): aprovisionamiento entrante
// de GRUPOS desde Entra/Okta hacia los grupos de Sytue ID, que alimentan el claim
// `groups` del JWT y, aguas abajo, las ACL por-grupo de Sytue Drive.
//
// Mapeo SCIM ↔ modelo Sytue:
//   - displayName            ↔ groups.name (único por-tenant; renombrar = 409 si choca)
//   - externalId             ↔ groups.external_id (id del grupo en el IdP origen)
//   - members[].value        ↔ user id de Sytue (los usuarios llegan por SCIM Users)
//   - members[]              ↔ filas group_members (la «membresía real» que ve
//     GroupsForUser ⇒ claim `groups` ⇒ shares de Drive)
//
// Aislamiento por tenant (docs/CONVENTIONS.md §0.7): el token SCIM está ligado a
// UNA org (resolveOrg); toda consulta se acota a org.ID y NUNCA a un id del path,
// así que un group_id/user_id de otro tenant da 404/400, jamás una acción
// cross-tenant. Añadir un miembro exige que el usuario sea MIEMBRO de esa org
// (mismo guard que el CRUD admin): un grupo no contiene a nadie ajeno al tenant.
// Los grupos NO otorgan rol de organización: añadir a alguien a un grupo llamado
// «admin» solo mete «admin» en su claim `groups` (un subject de share de Drive),
// no el rol de org `admin` (memberships.role / claim `roles`) — no hay escalada.
package scim

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// scimUUIDRe valida el formato de un user id antes de tocar columnas uuid (un
// value malformado ⇒ 400 legible, no un 500 por error de sintaxis de Postgres).
var scimUUIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// scimGroup es la representación SCIM de un grupo (petición y respuesta).
type scimGroup struct {
	Schemas     []string     `json:"schemas"`
	ID          string       `json:"id,omitempty"`
	ExternalID  string       `json:"externalId,omitempty"`
	DisplayName string       `json:"displayName"`
	Members     []scimMember `json:"members,omitempty"`
	Meta        *scimMeta    `json:"meta,omitempty"`
}

// scimMember es una entrada multivaluada de members: value es el user id.
type scimMember struct {
	Value   string `json:"value"`
	Ref     string `json:"$ref,omitempty"`
	Display string `json:"display,omitempty"`
	Type    string `json:"type,omitempty"`
}

func (h *Handler) toSCIMGroup(g domain.Group, members []store.GroupMember) scimGroup {
	sg := scimGroup{
		Schemas:     []string{schemaGroup},
		ID:          g.ID,
		ExternalID:  g.ExternalID,
		DisplayName: g.Name,
		Meta: &scimMeta{
			// groups no tiene updated_at; lastModified = created (suficiente para la
			// reconciliación de Entra/Okta, que compara por displayName/members).
			// ponytail: si algún proveedor exige lastModified real, añadir la columna.
			ResourceType: "Group",
			Created:      g.CreatedAt,
			LastModified: g.CreatedAt,
			Location:     h.issuer + "/scim/v2/Groups/" + g.ID,
		},
	}
	if len(members) > 0 {
		ms := make([]scimMember, 0, len(members))
		for _, m := range members {
			disp := m.Name
			if disp == "" {
				disp = m.Email
			}
			ms = append(ms, scimMember{
				Value:   m.UserID,
				Display: disp,
				Type:    "User",
				Ref:     h.issuer + "/scim/v2/Users/" + m.UserID,
			})
		}
		sg.Members = ms
	}
	return sg
}

// decodeSCIM decodifica un cuerpo SCIM de forma TOLERANTE: los proveedores
// (Entra/Okta) añaden atributos y esquemas libremente, de modo que —a diferencia
// de la API interna (httpx.DecodeJSON, campos estrictos)— no se rechazan campos
// desconocidos. Limita el tamaño del cuerpo (anti-DoS).
//
// Lo usan TAMBIÉN los endpoints de /Users: un payload real de Entra trae atributos que
// Sytue ID no persiste (nickName, phoneNumbers, userType, y employeeNumber/costCenter/
// manager de la extensión enterprise). Con el decodificador estricto, el alta entera
// respondía 400 y el aprovisionamiento no arrancaba (RFC 7644 §3.3: los atributos no
// soportados se IGNORAN, no invalidan la petición).
func decodeSCIM(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("scim: cuerpo vacío")
	}
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst)
}

// orgUnavailable escribe el 500 uniforme cuando el token SCIM no tiene org.
func orgUnavailable(w http.ResponseWriter) {
	scimErr(w, http.StatusInternalServerError, "SCIM no está asociado a una organización.")
}

// groupInOrg resuelve la org del token y el grupo {id} DE ella. Verja anti-IDOR
// compartida por las rutas de un grupo concreto: escribe el error SCIM y devuelve
// ok=false si el token no tiene org, el id no es uuid o el grupo no es del tenant
// (siempre 404, sin distinguir «no existe» de «es de otro» — sin fuga cross-tenant).
func (h *Handler) groupInOrg(w http.ResponseWriter, r *http.Request) (domain.Organization, domain.Group, bool) {
	org, err := h.resolveOrg(r.Context())
	if err != nil {
		orgUnavailable(w)
		return org, domain.Group{}, false
	}
	id := chi.URLParam(r, "id")
	if !scimUUIDRe.MatchString(id) {
		scimErr(w, http.StatusNotFound, "grupo no encontrado")
		return org, domain.Group{}, false
	}
	g, err := h.store.GetGroup(r.Context(), org.ID, id)
	if err != nil {
		scimErr(w, http.StatusNotFound, "grupo no encontrado")
		return org, domain.Group{}, false
	}
	return org, g, true
}

// ListGroups — GET /Groups — list + filtro `displayName eq "…"` + paginación
// startIndex/count (RFC 7644 §3.4.2). El listado omite members (sin N+1).
func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
	org, err := h.resolveOrg(r.Context())
	if err != nil {
		orgUnavailable(w)
		return
	}
	q := r.URL.Query()
	filter := parseDisplayNameFilter(q.Get("filter"))
	start, _ := strconv.Atoi(q.Get("startIndex"))
	if start < 1 {
		start = 1
	}
	count, _ := strconv.Atoi(q.Get("count"))
	if count <= 0 {
		count = 50
	}
	groups, total, err := h.store.ListGroupsPage(r.Context(), org.ID, filter, start-1, count)
	if err != nil {
		scimErr(w, http.StatusInternalServerError, "error al listar grupos")
		return
	}
	resources := make([]scimGroup, 0, len(groups))
	for _, g := range groups {
		resources = append(resources, h.toSCIMGroup(g, nil))
	}
	writeSCIM(w, http.StatusOK, map[string]any{
		"schemas":      []string{schemaListResponse},
		"totalResults": total,
		"startIndex":   start,
		"itemsPerPage": len(resources),
		"Resources":    resources,
	})
}

// GetGroup — GET /Groups/{id} — el grupo y sus miembros.
func (h *Handler) GetGroup(w http.ResponseWriter, r *http.Request) {
	org, g, ok := h.groupInOrg(w, r)
	if !ok {
		return
	}
	members, err := h.store.ListGroupMembers(r.Context(), org.ID, g.ID)
	if err != nil {
		scimErr(w, http.StatusInternalServerError, "error al leer miembros")
		return
	}
	writeSCIM(w, http.StatusOK, h.toSCIMGroup(g, members))
}

// CreateGroup — POST /Groups — crea el grupo (+ members iniciales) en la org del
// token. displayName obligatorio y único por-tenant (409 en duplicado).
func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	org, err := h.resolveOrg(r.Context())
	if err != nil {
		orgUnavailable(w)
		return
	}
	var in scimGroup
	if err := decodeSCIM(w, r, &in); err != nil {
		scimErr(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	name := domain.SanitizeDisplayName(in.DisplayName)
	if name == "" {
		scimErr(w, http.StatusBadRequest, "displayName obligatorio")
		return
	}
	userIDs, bad := h.validateMembers(r.Context(), org.ID, in.Members)
	if bad != "" {
		scimErr(w, http.StatusBadRequest, "miembro inválido o ajeno a la organización: "+bad)
		return
	}
	var created domain.Group
	ctx := r.Context()
	err = h.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		if created, e = h.store.CreateGroupSCIM(ctx, q, org.ID, name, in.ExternalID); e != nil {
			return e
		}
		if len(userIDs) > 0 {
			return h.store.ReplaceGroupMembers(ctx, q, org.ID, created.ID, userIDs)
		}
		return nil
	})
	if err != nil {
		if isConflict(err) {
			scimErr(w, http.StatusConflict, "el grupo ya existe")
			return
		}
		scimErr(w, http.StatusInternalServerError, "error al crear grupo")
		return
	}
	members, _ := h.store.ListGroupMembers(r.Context(), org.ID, created.ID)
	_ = h.store.Audit(r.Context(), nil, store.AuditEntry{OrgID: org.ID, Action: "scim.group.created", Target: created.ID, IP: clientIP(r)})
	w.Header().Set("Location", h.issuer+"/scim/v2/Groups/"+created.ID)
	writeSCIM(w, http.StatusCreated, h.toSCIMGroup(created, members))
}

// ReplaceGroup — PUT /Groups/{id} — reemplaza displayName, externalId y el
// conjunto COMPLETO de members.
func (h *Handler) ReplaceGroup(w http.ResponseWriter, r *http.Request) {
	org, g, ok := h.groupInOrg(w, r)
	if !ok {
		return
	}
	var in scimGroup
	if err := decodeSCIM(w, r, &in); err != nil {
		scimErr(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	name := domain.SanitizeDisplayName(in.DisplayName)
	if name == "" {
		scimErr(w, http.StatusBadRequest, "displayName obligatorio")
		return
	}
	userIDs, bad := h.validateMembers(r.Context(), org.ID, in.Members)
	if bad != "" {
		scimErr(w, http.StatusBadRequest, "miembro inválido o ajeno a la organización: "+bad)
		return
	}
	var updated domain.Group
	ctx := r.Context()
	err := h.store.InTx(ctx, func(q pg.Querier) error {
		var e error
		if updated, e = h.store.UpdateGroupSCIM(ctx, q, org.ID, g.ID, name, in.ExternalID); e != nil {
			return e
		}
		return h.store.ReplaceGroupMembers(ctx, q, org.ID, g.ID, userIDs)
	})
	if err != nil {
		if isConflict(err) {
			scimErr(w, http.StatusConflict, "displayName en uso")
			return
		}
		scimErr(w, http.StatusInternalServerError, "error al actualizar grupo")
		return
	}
	members, _ := h.store.ListGroupMembers(r.Context(), org.ID, g.ID)
	_ = h.store.Audit(r.Context(), nil, store.AuditEntry{OrgID: org.ID, Action: "scim.group.replaced", Target: g.ID, IP: clientIP(r)})
	writeSCIM(w, http.StatusOK, h.toSCIMGroup(updated, members))
}

// PatchGroup — PATCH /Groups/{id} (RFC 7644 §3.5.2) — el camino de Entra/Okta
// para añadir/quitar miembros y renombrar. Soporta add/remove/replace de
// `members` (incluido el filtro `members[value eq "id"]` de Entra) y replace de
// displayName/externalId.
//
//nolint:gocognit,gocyclo // PATCH SCIM 2.0: la matriz op×path (add/replace/remove × members/displayName) es el contrato del RFC 7644.
func (h *Handler) PatchGroup(w http.ResponseWriter, r *http.Request) {
	org, g, ok := h.groupInOrg(w, r)
	if !ok {
		return
	}
	var body patchBody
	if err := decodeSCIM(w, r, &body); err != nil {
		scimErr(w, http.StatusBadRequest, "cuerpo inválido")
		return
	}
	name, externalID := g.Name, g.ExternalID
	metaChanged := false
	for _, op := range body.Operations {
		opName := strings.ToLower(strings.TrimSpace(op.Op))
		path := strings.TrimSpace(op.Path)
		switch {
		case strings.HasPrefix(strings.ToLower(path), "members"):
			if err := h.patchMembers(r.Context(), org.ID, g.ID, opName, path, op.Value); err != nil {
				code, msg := memberOpError(err)
				scimErr(w, code, msg)
				return
			}
		case strings.EqualFold(path, "displayName"):
			if sv, ok := op.Value.(string); ok {
				name, metaChanged = domain.SanitizeDisplayName(sv), true
			}
		case strings.EqualFold(path, "externalId"):
			if sv, ok := op.Value.(string); ok {
				externalID, metaChanged = sv, true
			}
		case path == "" && opName == "replace":
			// replace sin path: value es un objeto {displayName?, externalId?, members?}.
			mp, ok := op.Value.(map[string]any)
			if !ok {
				continue
			}
			if sv, ok := mp["displayName"].(string); ok {
				name, metaChanged = domain.SanitizeDisplayName(sv), true
			}
			if sv, ok := mp["externalId"].(string); ok {
				externalID, metaChanged = sv, true
			}
			if mv, ok := mp["members"]; ok {
				if err := h.patchMembers(r.Context(), org.ID, g.ID, "replace", "members", mv); err != nil {
					code, msg := memberOpError(err)
					scimErr(w, code, msg)
					return
				}
			}
		}
	}
	if metaChanged {
		if name == "" {
			scimErr(w, http.StatusBadRequest, "displayName no puede quedar vacío")
			return
		}
		if _, err := h.store.UpdateGroupSCIM(r.Context(), nil, org.ID, g.ID, name, externalID); err != nil {
			if isConflict(err) {
				scimErr(w, http.StatusConflict, "displayName en uso")
				return
			}
			scimErr(w, http.StatusInternalServerError, "error al actualizar grupo")
			return
		}
	}
	g2, err := h.store.GetGroup(r.Context(), org.ID, g.ID)
	if err != nil {
		scimErr(w, http.StatusInternalServerError, "error al leer grupo")
		return
	}
	members, _ := h.store.ListGroupMembers(r.Context(), org.ID, g.ID)
	_ = h.store.Audit(r.Context(), nil, store.AuditEntry{OrgID: org.ID, Action: "scim.group.patched", Target: g.ID, IP: clientIP(r)})
	writeSCIM(w, http.StatusOK, h.toSCIMGroup(g2, members))
}

// DeleteGroup — DELETE /Groups/{id} — borra el grupo; group_members cae en
// cascada (FK). El claim `groups` de sus antiguos miembros queda sin ese nombre.
func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	org, g, ok := h.groupInOrg(w, r)
	if !ok {
		return
	}
	if _, err := h.store.DeleteGroup(r.Context(), nil, org.ID, g.ID); err != nil {
		scimErr(w, http.StatusInternalServerError, "error al eliminar grupo")
		return
	}
	_ = h.store.Audit(r.Context(), nil, store.AuditEntry{OrgID: org.ID, Action: "scim.group.deleted", Target: g.ID, IP: clientIP(r)})
	w.WriteHeader(http.StatusNoContent)
}

// patchMembers aplica una operación PATCH sobre `members`. add/replace validan
// que cada usuario sea miembro de la org (aislamiento); remove es idempotente.
// Soporta el filtro de Entra/Okta `members[value eq "id"]` y value=[{value:id}].
func (h *Handler) patchMembers(ctx context.Context, orgID, groupID, op, path string, value any) error {
	switch op {
	case "add":
		ids, bad := h.validateMembers(ctx, orgID, membersFromValue(value))
		if bad != "" {
			return &memberErr{bad}
		}
		for _, id := range ids {
			// Duplicado ⇒ idempotente (SCIM add de un miembro ya presente no falla).
			if err := h.store.AddGroupMember(ctx, nil, orgID, groupID, id); err != nil && !isConflict(err) {
				return err
			}
		}
		return nil
	case "remove":
		if id, ok := memberIDFromPath(path); ok {
			return h.store.RemoveGroupMember(ctx, nil, orgID, groupID, id)
		}
		vals := membersFromValue(value)
		if len(vals) == 0 {
			// `remove` de members sin filtro ni value ⇒ vaciar el grupo.
			return h.store.ReplaceGroupMembers(ctx, nil, orgID, groupID, nil)
		}
		for _, m := range vals {
			if err := h.store.RemoveGroupMember(ctx, nil, orgID, groupID, strings.TrimSpace(m.Value)); err != nil {
				return err
			}
		}
		return nil
	case "replace":
		ids, bad := h.validateMembers(ctx, orgID, membersFromValue(value))
		if bad != "" {
			return &memberErr{bad}
		}
		return h.store.ReplaceGroupMembers(ctx, nil, orgID, groupID, ids)
	}
	return nil
}

// validateMembers extrae los user id de members[].value, exige uuid válido y que
// el usuario sea MIEMBRO de la org (aislamiento: un grupo solo contiene usuarios
// del tenant). Devuelve los ids válidos, o el primer value inválido (para el 400).
func (h *Handler) validateMembers(ctx context.Context, orgID string, members []scimMember) ([]string, string) {
	ids := make([]string, 0, len(members))
	for _, m := range members {
		v := strings.TrimSpace(m.Value)
		if !scimUUIDRe.MatchString(v) {
			return nil, v
		}
		if _, err := h.store.GetMembership(ctx, orgID, v); err != nil {
			return nil, v
		}
		ids = append(ids, v)
	}
	return ids, ""
}

// memberErr marca un value de miembro inválido/ajeno a la org (⇒ 400 SCIM).
type memberErr struct{ value string }

func (e *memberErr) Error() string { return "scim: miembro inválido: " + e.value }

func memberOpError(err error) (int, string) {
	var me *memberErr
	if errors.As(err, &me) {
		return http.StatusBadRequest, "miembro inválido o ajeno a la organización: " + me.value
	}
	return http.StatusInternalServerError, "error al modificar miembros"
}

// membersFromValue normaliza el value de una operación PATCH de members a
// []scimMember: acepta un array de {value,…} o un único objeto {value,…}.
func membersFromValue(v any) []scimMember {
	switch t := v.(type) {
	case []any:
		out := make([]scimMember, 0, len(t))
		for _, e := range t {
			if m, ok := e.(map[string]any); ok {
				if s, ok := m["value"].(string); ok {
					out = append(out, scimMember{Value: s})
				}
			}
		}
		return out
	case map[string]any:
		if s, ok := t["value"].(string); ok {
			return []scimMember{{Value: s}}
		}
	}
	return nil
}

// memberIDFromPath extrae el id de `members[value eq "id"]` (el formato de
// Entra/Okta para quitar un miembro concreto). ok=false si el path no lleva filtro.
func memberIDFromPath(path string) (string, bool) {
	i := strings.IndexByte(path, '[')
	if i < 0 {
		return "", false
	}
	inner := strings.TrimSuffix(path[i+1:], "]")
	a := strings.IndexByte(inner, '"')
	b := strings.LastIndexByte(inner, '"')
	if a < 0 || b <= a {
		return "", false
	}
	return inner[a+1 : b], true
}

// parseDisplayNameFilter soporta `displayName eq "valor"`, el único filtro que
// Entra/Okta emiten al reconciliar grupos. "" si no aplica.
func parseDisplayNameFilter(filter string) string {
	f := strings.TrimSpace(filter)
	if !strings.HasPrefix(strings.ToLower(f), "displayname eq ") {
		return ""
	}
	return strings.Trim(strings.TrimSpace(f[len("displayName eq "):]), `"`)
}
