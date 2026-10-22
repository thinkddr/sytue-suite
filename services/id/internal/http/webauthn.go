// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/webauthn"
)

// webauthnUser resuelve de quién es la petición de passkeys, que llega por dos
// vías: el Bearer de la API (/api/v1/me/webauthn/…) y la cookie de sesión de la
// pantalla «Mi cuenta» (/account/passkeys/…). El registro es idéntico en ambas
// —mismo reto, misma attestation, mismo almacenamiento—, así que comparten
// handler en vez de mantener dos copias de la verificación WebAuthn.
//
// La vía cookie EXIGE un Origin permitido, y aquí sí que tiene que estar
// presente: estas rutas solo se invocan por fetch desde nuestra propia página,
// y un navegador siempre manda Origin en un fetch. Un POST sin Origin no puede
// venir del flujo legítimo, así que se rechaza en lugar de asumir —como sí hace
// sameOrigin para los formularios navegacionales, donde Chrome lo omite.
func (s *Server) webauthnUser(r *http.Request) (string, bool) {
	if c, ok := claimsFrom(r.Context()); ok {
		return c.Subject, true
	}
	sess, ok := s.currentSession(r)
	if !ok {
		return "", false
	}
	if o := r.Header.Get("Origin"); o == "" || !originAllowed(o, s.cfg.RPOrigins) {
		slog.WarnContext(r.Context(), "passkeys: origen rechazado", "origin", o)
		return "", false
	}
	return sess.UserID, true
}

// handleWebAuthnRegisterBegin inicia el registro de una passkey: genera el reto y
// devuelve PublicKeyCredentialCreationOptions (WebAuthn L2 §5.4).
func (s *Server) handleWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.webauthnUser(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	user, err := s.store.GetUserByID(r.Context(), subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}

	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if err := s.store.InsertChallenge(r.Context(), nil, challenge, user.ID, "register", time.Now().Add(5*time.Minute)); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}

	// Las que YA tiene, para que el autenticador no intente registrar otra vez la
	// misma. Sin esta lista, un dispositivo que ya guarda una passkey de esta
	// cuenta responde InvalidStateError y el alta falla SIEMPRE, sin más pista
	// que «no se pudo»: es la forma más común de que esto parezca roto cuando lo
	// que pasa es que ya estaba hecho. Un fallo al leerlas no impide registrar
	// —se pierde la protección contra duplicados, no el alta—.
	excluir := []map[string]any{}
	if yaTiene, e := s.store.ListCredentialsForUser(r.Context(), user.ID); e == nil {
		for _, c := range yaTiene {
			excluir = append(excluir, map[string]any{
				"type": "public-key",
				"id":   base64.RawURLEncoding.EncodeToString(c.CredID),
			})
		}
	} else {
		s.log.WarnContext(r.Context(), "passkeys: no se pudieron listar las existentes", "error", e)
	}

	b64 := base64.RawURLEncoding.EncodeToString
	opts := map[string]any{
		"challenge":          b64(challenge),
		"excludeCredentials": excluir,
		"rp":                 map[string]string{"id": s.cfg.RPID, "name": s.cfg.RPDisplayName},
		"user": map[string]string{
			"id":          b64([]byte(user.ID)),
			"name":        user.Email,
			"displayName": user.Name,
		},
		"pubKeyCredParams": []map[string]any{
			{"type": "public-key", "alg": -7},   // ES256
			{"type": "public-key", "alg": -257}, // RS256
		},
		"timeout":     300000,
		"attestation": "none",
		"authenticatorSelection": map[string]any{
			"residentKey":      "preferred",
			"userVerification": "preferred",
		},
	}
	httpx.JSON(w, r, http.StatusOK, opts)
}

// registerFinishRequest es el resultado de navigator.credentials.create() que el
// front reenvía (campos base64url).
type registerFinishRequest struct {
	ID                string `json:"id"`
	RawID             string `json:"rawId"`
	Type              string `json:"type"`
	Name              string `json:"name"`
	AttestationObject string `json:"attestationObject"`
	ClientDataJSON    string `json:"clientDataJSON"`
}

// handleWebAuthnRegisterFinish verifica la attestation y registra la passkey.
func (s *Server) handleWebAuthnRegisterFinish(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.webauthnUser(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req registerFinishRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	clientDataRaw, err := base64.RawURLEncoding.DecodeString(req.ClientDataJSON)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid clientDataJSON."))
		return
	}
	attObjRaw, err := base64.RawURLEncoding.DecodeString(req.AttestationObject)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid attestationObject."))
		return
	}

	// Validar clientDataJSON (tipo, reto, origen) — WebAuthn L2 §7.1.
	var cd webauthn.ClientData
	if err := json.Unmarshal(clientDataRaw, &cd); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("clientData could not be parsed."))
		return
	}
	if cd.Type != "webauthn.create" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unexpected operation type."))
		return
	}
	challengeBytes, err := base64.RawURLEncoding.DecodeString(cd.Challenge)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The challenge could not be decoded."))
		return
	}
	if !originAllowed(cd.Origin, s.cfg.RPOrigins) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Origin not allowed."))
		return
	}

	// Consumir el reto (atómico, un solo uso) y comprobar que es de este usuario.
	uid, purpose, err := s.store.ConsumeChallenge(r.Context(), nil, challengeBytes, time.Now())
	if err != nil || purpose != "register" || uid != subject {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid or expired challenge."))
		return
	}

	// Parsear attestationObject (CBOR) -> authData -> credential id + clave COSE.
	att, err := webauthn.ParseAttestationObject(attObjRaw)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The attestation could not be parsed.").Wrap(err))
		return
	}
	// Verificar rpIdHash == SHA-256(rpId) y flags (UP + AT) — WebAuthn L2 §7.1.
	rpHash := sha256.Sum256([]byte(s.cfg.RPID))
	if !att.AuthData.RPIDHashEquals(rpHash[:]) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("rpIdHash does not match."))
		return
	}
	if !att.AuthData.UserPresent() || !att.AuthData.HasAttestedCredential() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid authenticator flags."))
		return
	}

	cred := domain.WebAuthnCredential{
		UserID:    subject,
		CredID:    att.AuthData.CredentialID,
		PublicKey: att.AuthData.COSEPublicKey,
		AAGUID:    att.AuthData.AAGUID,
		SignCount: att.AuthData.SignCount,
		Name:      req.Name,
	}
	if err := s.store.InsertCredential(r.Context(), nil, cred); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if err := s.store.SetMFAEnabled(r.Context(), nil, subject, true); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(subject, "", "webauthn.credential.registered", "", r))
	s.avisarSeguridad(r.Context(), "", subject, "security.passkey.added", nil)
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"credential_id": base64.RawURLEncoding.EncodeToString(att.AuthData.CredentialID),
		"name":          defaultName(req.Name),
	})
}

// handleWebAuthnList lista las passkeys del usuario.
func (s *Server) handleWebAuthnList(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.webauthnUser(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	creds, err := s.store.ListCredentialsForUser(r.Context(), subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	out := make([]map[string]any, 0, len(creds))
	for _, cr := range creds {
		out = append(out, map[string]any{
			// `id` ES EL UUID DE LA FILA, no el CredID de WebAuthn. Antes era el
			// segundo, y con él no se podía borrar nada: el borrado acota por
			// `webauthn_credentials.id`. Quien listara por esta API recibía
			// identificadores que ninguna otra ruta acepta. Nadie la consumía
			// todavía —lo comprobé—, así que se corrige en vez de arrastrarlo.
			// Es además el mismo criterio que `/me/devices/{id}` y `/me/sessions/{id}`.
			"id": cr.ID,
			// Y el de WebAuthn aparte, con su nombre: lo necesita el navegador para
			// excluir una llave ya registrada al dar de alta otra.
			"credential_id": base64.RawURLEncoding.EncodeToString(cr.CredID),
			"name":          cr.Name,
			"created_at":    cr.CreatedAt,
			"last_used":     cr.LastUsedAt,
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"credentials": out})
}

func originAllowed(origin string, allowed []string) bool {
	for _, a := range allowed {
		if a == origin {
			return true
		}
	}
	return false
}

func defaultName(n string) string {
	if n == "" {
		return "Passkey"
	}
	return n
}
