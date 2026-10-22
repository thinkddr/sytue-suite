// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// handleIntrospect implementa la introspección de tokens (RFC 7662). EXIGE
// autenticación de client CONFIDENCIAL (client_secret_basic/post): un llamante
// no autenticado —o uno público, que solo presenta un client_id público— recibe
// 401; nunca se puede escanear tokens de forma anónima. Devuelve {active:true, ...claims mínimos} para un token válido (access
// JWT o refresh no revocado/rotado/expirado) y {active:false} y NADA MÁS para
// cualquier token inválido/expirado/revocado/desconocido, sin distinguir el motivo
// (anti-enumeración).
func (s *Server) handleIntrospect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		tokenError(w, r, http.StatusBadRequest, "invalid_request", "formulario inválido")
		return
	}
	client, ok := s.authenticateClient(r)
	// Un client PÚBLICO «autentica» presentando solo su client_id, que viaja
	// dentro del binario de la app o del bundle de la SPA: aceptarlo aquí sería
	// dejar el endpoint abierto a quien lo copie de ahí. RFC 7662 §2.1 pide
	// justamente lo contrario. Introspeccionar es cosa de resource servers, que
	// son confidenciales; la revocación (RFC 7009) sí admite públicos, porque un
	// móvil tiene que poder anular su propio refresh token al cerrar sesión.
	if !ok || client.IsPublic() {
		w.Header().Set("WWW-Authenticate", `Basic realm="sytue-id"`)
		tokenError(w, r, http.StatusUnauthorized, "invalid_client", "autenticación de client fallida")
		return
	}
	token := r.Form.Get("token")
	if token == "" {
		tokenError(w, r, http.StatusBadRequest, "invalid_request", "falta token")
		return
	}

	// token_type_hint (opcional) solo decide el ORDEN de comprobación; ambos caminos
	// caen a {active:false} si no aciertan, así que un hint incorrecto no filtra nada
	// ni cambia el resultado (RFC 7662 §2.1).
	probes := []func(*http.Request, domain.Client, string) (map[string]any, bool){
		s.introspectAccess, s.introspectRefresh,
	}
	if r.Form.Get("token_type_hint") == "refresh_token" {
		probes[0], probes[1] = probes[1], probes[0]
	}
	for _, probe := range probes {
		if out, ok := probe(r, client, token); ok {
			introspectRespond(w, r, out)
			return
		}
	}
	introspectRespond(w, r, map[string]any{"active": false})
}

// introspectAccess valida un access token JWT reusando la misma verificación que
// el resto del servicio (firma RS256 + iss + aud + exp).
//
// Alcance por client (RFC 7662): un client GLOBAL de la suite (OrgID "") puede
// introspeccionar cualquier access token válido —los resource servers (Drive,
// Mail, …) los validan sin ser el `aud`, y la auth de client ya impide el escaneo
// anónimo—. Un client de TERCEROS (acotado a una organización) solo puede
// introspeccionar SUS tokens: los que se emitieron con su client_id en el `aud`.
// Sin esto, una app que un administrador registró en su organización podía
// preguntar por tokens ajenos y leer su `tenant`/`roles`/`perms` (ID-03).
func (s *Server) introspectAccess(r *http.Request, client domain.Client, token string) (map[string]any, bool) {
	set, err := s.keys.PublicJWKS()
	if err != nil {
		return nil, false
	}
	tok, err := oidc.Verify(token, s.cfg.Issuer, s.cfg.ResourceAudience, set)
	if err != nil {
		return nil, false
	}
	// Un client de terceros solo ve lo que se emitió para él. Mismo veredicto
	// {active:false} que un token inválido: no se distingue «no es tuyo» de «no
	// existe» (anti-enumeración, RFC 7662 §4).
	if client.OrgID != "" && !audienceContains(tok.Audience(), client.ID) {
		return nil, false
	}
	// De una cuenta que ya no está activa (suspendida, dada de baja, borrada por su
	// titular) el token deja de valer al instante, no cuando caduca.
	if fuera, uerr := s.cuentaNoActiva(r.Context(), tok.Subject()); uerr != nil || fuera {
		return nil, false
	}
	out := map[string]any{
		"active":     true,
		"token_type": "Bearer",
		"sub":        tok.Subject(),
		"iss":        s.cfg.Issuer,
		"exp":        tok.Expiration().Unix(),
		"iat":        tok.IssuedAt().Unix(),
	}
	if aud := tok.Audience(); len(aud) > 0 {
		out["aud"] = aud
	}
	if v, ok := tok.Get("scope"); ok {
		if sv, _ := v.(string); sv != "" {
			out["scope"] = sv
		}
	}
	if cid := clientIDFromAud(tok.Audience(), s.cfg.ResourceAudience, tok.Subject()); cid != "" {
		out["client_id"] = cid
	}
	if v, ok := tok.Get("tenant"); ok {
		out["tenant"] = v
	}
	if roles := claimStrings(tok, "roles"); len(roles) > 0 {
		out["roles"] = roles
	}
	if perms := claimStrings(tok, "perms"); len(perms) > 0 {
		out["perms"] = perms
	}
	return out, true
}

// introspectRefresh valida un refresh token opaco contra el store. Solo el client
// DUEÑO del refresh puede introspeccionarlo (igual que la revocación, RFC 7009):
// un client distinto recibe {active:false}, sin revelar que el token existe.
func (s *Server) introspectRefresh(r *http.Request, client domain.Client, token string) (map[string]any, bool) {
	rt, err := s.store.GetRefreshToken(r.Context(), s.store.DB(), domain.HashToken(token))
	if err != nil {
		return nil, false
	}
	if rt.ClientID != client.ID {
		return nil, false
	}
	if rt.RevokedAt != nil || rt.RotatedTo != "" || time.Now().After(rt.ExpiresAt) {
		return nil, false
	}
	out := map[string]any{
		"active":     true,
		"token_type": "refresh_token",
		"sub":        rt.UserID,
		"client_id":  rt.ClientID,
		"exp":        rt.ExpiresAt.Unix(),
	}
	if rt.Scope != "" {
		out["scope"] = rt.Scope
	}
	return out, true
}

// audienceContains indica si `aud` incluye el client dado.
func audienceContains(aud []string, clientID string) bool {
	for _, a := range aud {
		if a == clientID {
			return true
		}
	}
	return false
}

// clientIDFromAud deduce el client destinatario del access token: la entrada del
// `aud` distinta del resource server; si no la hay (grant client_credentials), el
// propio `sub` (que es el client_id). "" si no se puede deducir.
func clientIDFromAud(aud []string, resourceAud, sub string) string {
	for _, a := range aud {
		if a != resourceAud {
			return a
		}
	}
	if len(aud) > 0 && aud[0] == resourceAud && sub != "" {
		return sub
	}
	return ""
}

// introspectRespond escribe la respuesta con las cabeceras no-cache exigidas por
// RFC 7662 §4 (no cachear el estado de un token).
func introspectRespond(w http.ResponseWriter, r *http.Request, out map[string]any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	httpx.JSON(w, r, http.StatusOK, out)
}
