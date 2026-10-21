// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// CUENTA DE GOOGLE EMULADA — la parte de identidad, bajo /compat/google:
//
//   - el discovery con la forma del de Google, que apunta a los endpoints de abajo;
//   - /o/oauth2/v2/auth, que entiende los parámetros de Google (access_type=offline,
//     scopes userinfo.*) y sigue al authorize de siempre;
//   - userinfo (v3, v2 y /userinfo/v2/me) y tokeninfo con los campos de Google,
//     incluido `hd`, el dominio de la organización;
//   - la Directory API de Admin SDK en lectura: usuarios, grupos y miembros.
//
// Una app escrita para Google apunta aquí su «base URL» o su discovery y funciona
// con las cuentas de Sytue. Google no participa: ni emite ni ve nada.

func (s *Server) mountGoogleCompat(r chi.Router, tokenRL func(http.Handler) http.Handler) {
	r.Route("/compat/google", func(r chi.Router) {
		r.Get("/.well-known/openid-configuration", s.handleGoogleDiscovery)
		r.Get("/o/oauth2/v2/auth", s.handleGoogleAuth)
		r.Get("/o/oauth2/auth", s.handleGoogleAuth)
		r.Get("/oauth2/v3/userinfo", s.handleGoogleUserinfo)
		r.Post("/oauth2/v3/userinfo", s.handleGoogleUserinfo)
		r.Get("/oauth2/v2/userinfo", s.handleGoogleUserinfoV2)
		r.Get("/userinfo/v2/me", s.handleGoogleUserinfoV2)
		r.With(tokenRL).Get("/oauth2/v3/tokeninfo", s.handleGoogleTokeninfo)
		r.With(tokenRL).Post("/oauth2/v3/tokeninfo", s.handleGoogleTokeninfo)
		r.Group(func(r chi.Router) {
			r.Use(s.requireAccessToken)
			r.Get("/admin/directory/v1/users", s.handleGoogleDirUsers)
			r.Get("/admin/directory/v1/users/{userKey}", s.handleGoogleDirUser)
			r.Get("/admin/directory/v1/groups", s.handleGoogleDirGroups)
			r.Get("/admin/directory/v1/groups/{groupKey}", s.handleGoogleDirGroup)
			r.Get("/admin/directory/v1/groups/{groupKey}/members", s.handleGoogleDirMembers)
		})
	})
}

// handleGoogleDiscovery sirve el discovery de siempre con los endpoints de Google.
func (s *Server) handleGoogleDiscovery(w http.ResponseWriter, r *http.Request) {
	issuer := strings.TrimRight(s.cfg.Issuer, "/")
	d := oidc.BuildDiscovery(issuer)
	d.AuthorizationEndpoint = issuer + "/compat/google/o/oauth2/v2/auth"
	d.UserinfoEndpoint = issuer + "/compat/google/oauth2/v3/userinfo"
	d.ScopesSupported = []string{"openid", "email", "profile"}
	for sc := range domain.GoogleScopes {
		d.ScopesSupported = append(d.ScopesSupported, sc)
	}
	sort.Strings(d.ScopesSupported[3:])
	if !slices.Contains(d.ClaimsSupported, "hd") {
		d.ClaimsSupported = append(d.ClaimsSupported, "hd")
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	httpx.JSON(w, r, http.StatusOK, d)
}

// handleGoogleAuth traduce la petición de autorización de Google a la de Sytue ID y
// la manda al authorize de siempre, que es quien valida client, redirect y scopes.
func (s *Server) handleGoogleAuth(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scope := domain.NormalizeGoogleScopes(q.Get("scope"))
	// Google pide el refresh token con access_type=offline; OpenID, con offline_access.
	if q.Get("access_type") == "offline" && !slices.Contains(strings.Fields(scope), "offline_access") {
		scope = strings.TrimSpace(scope + " offline_access")
	}
	q.Set("scope", scope)
	for _, sobra := range []string{"access_type", "include_granted_scopes", "hd", "enable_granular_consent"} {
		q.Del(sobra)
	}
	http.Redirect(w, r, "/oauth2/authorize?"+q.Encode(), http.StatusFound)
}

// verifyGoogleBearer valida el access token de la cabecera. Google admite también
// ?access_token= en el userinfo; aquí no, porque un token en la URL acaba en los
// registros de cualquier proxy del camino.
func (s *Server) verifyGoogleBearer(r *http.Request) (jwt.Token, error) {
	raw, ok := bearer(r)
	if !ok {
		return nil, errors.New("sin token")
	}
	set, err := s.keys.PublicJWKS()
	if err != nil {
		return nil, err
	}
	return oidc.Verify(raw, s.cfg.Issuer, s.cfg.ResourceAudience, set)
}

// googleUserinfo compone el userinfo v3 de Google para el sujeto del token.
func (s *Server) googleUserinfo(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	tok, err := s.verifyGoogleBearer(r)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		googleError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "Invalid Credentials")
		return nil, false
	}
	user, err := s.store.GetUserByID(r.Context(), tok.Subject())
	if err != nil {
		googleError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "Invalid Credentials")
		return nil, false
	}
	scopes := strings.Fields(domain.NormalizeGoogleScopes(strings.Join(claimStrings(tok, "scope"), " ")))
	// Sin `picture`: la URL solo sirve con la cookie de Sytue ID, que el sitio de un
	// tercero no tiene, y no se añade a lo que sale a terceros algo que no pueden usar.
	out := userinfoClaims(user, scopes, "")
	if hd := s.hostedDomain(r, tok); hd != "" {
		out["hd"] = hd
	}
	return out, true
}

func (s *Server) handleGoogleUserinfo(w http.ResponseWriter, r *http.Request) {
	if out, ok := s.googleUserinfo(w, r); ok {
		httpx.JSON(w, r, http.StatusOK, out)
	}
}

// handleGoogleUserinfoV2 es el formato antiguo (oauth2/v2/userinfo), que muchas apps
// siguen pidiendo: `id` y `verified_email` en vez de `sub` y `email_verified`.
func (s *Server) handleGoogleUserinfoV2(w http.ResponseWriter, r *http.Request) {
	v3, ok := s.googleUserinfo(w, r)
	if !ok {
		return
	}
	out := map[string]any{"id": v3["sub"]}
	for v3k, v2k := range map[string]string{
		"email": "email", "email_verified": "verified_email", "name": "name", "given_name": "given_name",
		"family_name": "family_name", "locale": "locale", "hd": "hd",
	} {
		if v, ok := v3[v3k]; ok {
			out[v2k] = v
		}
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// handleGoogleTokeninfo valida un access token o un ID token y lo describe como lo
// hace Google: todos los números como cadena, y `aud`/`azp` con el client.
func (s *Server) handleGoogleTokeninfo(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	set, err := s.keys.PublicJWKS()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	invalido := func() {
		httpx.JSON(w, r, http.StatusBadRequest, map[string]string{"error": "invalid_token", "error_description": "Invalid Value"})
	}
	now := time.Now()
	if raw := strings.TrimSpace(r.Form.Get("id_token")); raw != "" {
		tok, err := oidc.Verify(raw, s.cfg.Issuer, "", set)
		if err != nil {
			invalido()
			return
		}
		aud := strings.Join(tok.Audience(), " ")
		out := map[string]any{
			"iss": tok.Issuer(), "sub": tok.Subject(), "aud": aud, "azp": aud,
			"iat": strconv.FormatInt(tok.IssuedAt().Unix(), 10), "exp": strconv.FormatInt(tok.Expiration().Unix(), 10),
			"alg": "RS256",
		}
		for _, k := range []string{"email", "name", "given_name", "family_name", "locale", "hd", "nonce"} {
			if v, ok := tok.Get(k); ok {
				out[k] = v
			}
		}
		if v, ok := tok.Get("email_verified"); ok {
			out["email_verified"] = strconv.FormatBool(v == true)
		}
		httpx.JSON(w, r, http.StatusOK, out)
		return
	}
	raw := strings.TrimSpace(r.Form.Get("access_token"))
	if raw == "" {
		raw, _ = bearer(r)
	}
	tok, err := oidc.Verify(raw, s.cfg.Issuer, s.cfg.ResourceAudience, set)
	if err != nil {
		invalido()
		return
	}
	client := clientDeAudiencia(tok.Audience(), s.cfg.ResourceAudience)
	if client == domain.GoogleAPIsAudience {
		client = ""
	}
	scope := strings.Join(claimStrings(tok, "scope"), " ")
	out := map[string]any{
		"azp": client, "aud": client, "sub": tok.Subject(), "scope": scope,
		"exp":         strconv.FormatInt(tok.Expiration().Unix(), 10),
		"expires_in":  strconv.FormatInt(int64(tok.Expiration().Sub(now).Seconds()), 10),
		"access_type": "online",
	}
	if slices.Contains(strings.Fields(scope), "offline_access") {
		out["access_type"] = "offline"
	}
	if v, ok := tok.Get("email"); ok {
		out["email"] = v
		verificado, _ := tok.Get("email_verified")
		out["email_verified"] = strconv.FormatBool(verificado == true)
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// hostedDomain es el `hd` de Google: el primer dominio verificado de la organización
// activa del token. Una cuenta personal no lo tiene, como una de gmail.com.
func (s *Server) hostedDomain(r *http.Request, tok jwt.Token) string {
	if v, ok := tok.Get("hd"); ok {
		if hd, _ := v.(string); hd != "" {
			return hd
		}
	}
	tenant, _ := tok.Get("tenant")
	slug, _ := tenant.(string)
	if slug == "" {
		return ""
	}
	org, err := s.store.GetOrgBySlug(r.Context(), slug)
	if err != nil || (org.Kind != "" && org.Kind != domain.OrgKindOrg) {
		return ""
	}
	if ds, err := s.store.VerifiedOrgMailDomains(r.Context(), nil, org.ID); err == nil && len(ds) > 0 {
		return ds[0]
	}
	return ""
}

// --- Directory API (Admin SDK), solo lectura --------------------------------------

const googleDirPrefix = "https://www.googleapis.com/auth/admin.directory."

// directoryOrg comprueba lo mismo que Google antes de enseñar un directorio: el
// token lleva el scope, quien lo usa administra la organización y la app pertenece
// a ESA organización. Devuelve la organización o escribe el error.
func (s *Server) directoryOrg(w http.ResponseWriter, r *http.Request, scopes ...string) (domain.Organization, bool) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		googleError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "Invalid Credentials")
		return domain.Organization{}, false
	}
	permitido := false
	for _, sc := range scopes {
		if slices.Contains(c.Scopes, googleDirPrefix+sc) {
			permitido = true
		}
	}
	if !permitido {
		googleError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "Request had insufficient authentication scopes.")
		return domain.Organization{}, false
	}
	if !slices.Contains(c.Roles, string(domain.RoleOwner)) && !slices.Contains(c.Roles, string(domain.RoleAdmin)) {
		googleError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "Not Authorized to access this resource/api")
		return domain.Organization{}, false
	}
	org, err := s.store.GetOrgBySlug(r.Context(), c.Tenant)
	if err != nil {
		googleError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "Not Authorized to access this resource/api")
		return domain.Organization{}, false
	}
	// Una app registrada en OTRA organización no lee esta, aunque quien inicia sesión
	// sea administrador de las dos.
	if client, err := s.store.GetClient(r.Context(), c.ClientID); err != nil || client.OrgID != org.ID {
		googleError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "Not Authorized to access this resource/api")
		return domain.Organization{}, false
	}
	return org, true
}

type googleDirCursor struct {
	T time.Time `json:"t"`
	I string    `json:"i"`
}

func (s *Server) handleGoogleDirUsers(w http.ResponseWriter, r *http.Request) {
	org, ok := s.directoryOrg(w, r, "user.readonly")
	if !ok {
		return
	}
	q := r.URL.Query()
	limite := 100
	if n, err := strconv.Atoi(q.Get("maxResults")); err == nil && n > 0 {
		limite = min(n, 200)
	}
	var tras *store.UserCursor
	if pt := q.Get("pageToken"); pt != "" {
		var cur googleDirCursor
		raw, err := base64.RawURLEncoding.DecodeString(pt)
		if err != nil || json.Unmarshal(raw, &cur) != nil {
			googleError(w, r, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid page token")
			return
		}
		tras = &store.UserCursor{CreatedAt: cur.T, ID: cur.I}
	}
	usuarios, mas, err := s.store.ListOrgUsersPage(r.Context(), org.ID, googleQuery(q.Get("query")), tras, limite)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	lista, err := s.googleUsers(r, org.ID, usuarios)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	out := map[string]any{"kind": "admin#directory#users", "users": lista}
	if mas && len(usuarios) > 0 {
		ultimo := usuarios[len(usuarios)-1]
		raw, _ := json.Marshal(googleDirCursor{T: ultimo.CreatedAt, I: ultimo.ID})
		out["nextPageToken"] = base64.RawURLEncoding.EncodeToString(raw)
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

func (s *Server) handleGoogleDirUser(w http.ResponseWriter, r *http.Request) {
	org, ok := s.directoryOrg(w, r, "user.readonly")
	if !ok {
		return
	}
	id, err := s.store.OrgUserIDForKey(r.Context(), org.ID, strings.TrimSpace(chi.URLParam(r, "userKey")))
	if err != nil {
		googleError(w, r, http.StatusNotFound, "NOT_FOUND", "Resource Not Found: userKey")
		return
	}
	u, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		googleError(w, r, http.StatusNotFound, "NOT_FOUND", "Resource Not Found: userKey")
		return
	}
	m, err := s.store.GetMembership(r.Context(), org.ID, id)
	if err != nil {
		googleError(w, r, http.StatusNotFound, "NOT_FOUND", "Resource Not Found: userKey")
		return
	}
	lista, err := s.googleUsers(r, org.ID, []store.OrgUser{{
		ID: u.ID, Email: u.Email, EmailVerified: u.EmailVerified, Name: u.Name, MFAEnabled: u.MFAEnabled,
		Status: u.Status, CreatedAt: u.CreatedAt, Role: string(m.Role),
		Department: u.Department, Title: u.Title, Location: u.Location,
	}})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, lista[0])
}

// googleUsers pasa filas del directorio al recurso User de Google. La dirección
// principal es el buzón de la organización, que es lo que en Workspace se llama
// primaryEmail; el correo de registro va como dirección adicional.
func (s *Server) googleUsers(r *http.Request, orgID string, usuarios []store.OrgUser) ([]map[string]any, error) {
	ids := make([]string, len(usuarios))
	for i, u := range usuarios {
		ids[i] = u.ID
	}
	buzones, err := s.store.ListOrgMailboxesDe(r.Context(), orgID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(usuarios))
	for _, u := range usuarios {
		principal := u.Email
		for _, b := range buzones[u.ID] {
			if b.Primary || principal == u.Email {
				principal = b.Address
			}
		}
		direcciones := []map[string]any{{"address": principal, "primary": true}}
		if !strings.EqualFold(principal, u.Email) {
			direcciones = append(direcciones, map[string]any{"address": u.Email})
		}
		nombre, apellidos, _ := strings.Cut(strings.TrimSpace(u.Name), " ")
		admin := u.Role == string(domain.RoleOwner) || u.Role == string(domain.RoleAdmin)
		usuario := map[string]any{
			"kind": "admin#directory#user", "id": u.ID, "primaryEmail": principal,
			"name":             map[string]any{"fullName": u.Name, "givenName": nombre, "familyName": apellidos},
			"isAdmin":          admin,
			"isDelegatedAdmin": false,
			"suspended":        u.Status != domain.StatusActive,
			"orgUnitPath":      "/",
			"emails":           direcciones,
			"isEnrolledIn2Sv":  u.MFAEnabled,
			"creationTime":     u.CreatedAt.UTC().Format(time.RFC3339),
		}
		if u.Department != "" || u.Title != "" {
			usuario["organizations"] = []map[string]any{{"department": u.Department, "title": u.Title, "primary": true}}
		}
		if u.Location != "" {
			usuario["locations"] = []map[string]any{{"area": u.Location, "type": "desk"}}
		}
		out = append(out, usuario)
	}
	return out, nil
}

// googleQuery reduce la sintaxis de búsqueda de Google (`email:ana*`, `name:'Ana'`)
// a la subcadena que entiende el directorio.
func googleQuery(q string) string {
	q = strings.TrimSpace(q)
	for _, prefijo := range []string{"email:", "name:", "givenName:", "familyName:"} {
		q = strings.TrimPrefix(q, prefijo)
	}
	return strings.Trim(q, "'\"*")
}

func (s *Server) handleGoogleDirGroups(w http.ResponseWriter, r *http.Request) {
	org, ok := s.directoryOrg(w, r, "group.readonly")
	if !ok {
		return
	}
	grupos, err := s.store.ListGroups(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// userKey: solo los grupos de esa persona.
	var suyos []string
	if clave := strings.TrimSpace(r.URL.Query().Get("userKey")); clave != "" {
		id, err := s.store.OrgUserIDForKey(r.Context(), org.ID, clave)
		if err != nil {
			googleError(w, r, http.StatusNotFound, "NOT_FOUND", "Resource Not Found: userKey")
			return
		}
		if suyos, err = s.store.GroupsForUser(r.Context(), org.ID, id); err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
	}
	lista := make([]map[string]any, 0, len(grupos))
	for _, g := range grupos {
		if r.URL.Query().Get("userKey") != "" && !slices.Contains(suyos, g.Name) {
			continue
		}
		lista = append(lista, googleGroup(g.Group, g.MemberCount))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"kind": "admin#directory#groups", "groups": lista})
}

func (s *Server) handleGoogleDirGroup(w http.ResponseWriter, r *http.Request) {
	org, ok := s.directoryOrg(w, r, "group.readonly")
	if !ok {
		return
	}
	g, err := s.store.GetGroup(r.Context(), org.ID, chi.URLParam(r, "groupKey"))
	if err != nil {
		googleError(w, r, http.StatusNotFound, "NOT_FOUND", "Resource Not Found: groupKey")
		return
	}
	miembros, _ := s.store.ListGroupMembers(r.Context(), org.ID, g.ID)
	httpx.JSON(w, r, http.StatusOK, googleGroup(g, len(miembros)))
}

func (s *Server) handleGoogleDirMembers(w http.ResponseWriter, r *http.Request) {
	org, ok := s.directoryOrg(w, r, "group.readonly", "group.member.readonly")
	if !ok {
		return
	}
	g, err := s.store.GetGroup(r.Context(), org.ID, chi.URLParam(r, "groupKey"))
	if err != nil {
		googleError(w, r, http.StatusNotFound, "NOT_FOUND", "Resource Not Found: groupKey")
		return
	}
	miembros, err := s.store.ListGroupMembers(r.Context(), org.ID, g.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	lista := make([]map[string]any, 0, len(miembros))
	for _, m := range miembros {
		lista = append(lista, map[string]any{
			"kind": "admin#directory#member", "id": m.UserID, "email": m.Email,
			"role": "MEMBER", "type": "USER", "status": "ACTIVE",
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"kind": "admin#directory#members", "members": lista})
}

// googleGroup es el recurso Group de Google. Sin `email`: los grupos de Sytue no
// tienen dirección, y una inventada haría que alguien escribiera a un buzón que no
// existe.
func googleGroup(g domain.Group, miembros int) map[string]any {
	return map[string]any{
		"kind": "admin#directory#group", "id": g.ID, "name": g.Name, "description": g.Description,
		"directMembersCount": strconv.Itoa(miembros), "adminCreated": true,
	}
}

// googleError escribe el sobre de error de las APIs de Google.
func googleError(w http.ResponseWriter, r *http.Request, code int, status, message string) {
	razon := map[string]string{
		"UNAUTHENTICATED": "authError", "PERMISSION_DENIED": "forbidden",
		"NOT_FOUND": "notFound", "INVALID_ARGUMENT": "invalid",
	}[status]
	httpx.JSON(w, r, code, map[string]any{"error": map[string]any{
		"code": code, "message": message, "status": status,
		"errors": []map[string]string{{"message": message, "domain": "global", "reason": razon}},
	}})
}
