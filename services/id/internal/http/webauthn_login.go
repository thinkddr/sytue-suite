// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Ceremonia de LOGIN por passkey (WebAuthn assertion, L2 §7.2). El registro de
// passkeys vive en webauthn.go (API JWT, usuario ya autenticado); aquí el usuario
// AÚN NO tiene sesión, así que begin/finish son públicos (bajo /login) y van
// rate-limitados como el login por contraseña. begin emite un reto de un solo uso
// y devuelve allowCredentials (pista, sin enumerar cuentas); finish verifica la
// assertion contra la clave pública COSE registrada, comprueba el signCount
// anti-clonación y, si todo cuadra, abre la MISMA sesión SSO que el login normal.
package http

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/webauthn"
)

// b64u es la codificación base64url sin padding que usan todas las estructuras
// WebAuthn (retos, credential ids, authenticatorData, firmas).
var b64u = base64.RawURLEncoding

// loginBeginRequest es el cuerpo (opcional) de /login/webauthn/begin: el email
// permite el flujo "usuario-primero" (devolver allowCredentials de esa cuenta);
// vacío ⇒ passkey discoverable/usernameless (el autenticador elige la credencial).
type loginBeginRequest struct {
	Email string `json:"email"`
}

// handleWebAuthnLoginBegin — POST /login/webauthn/begin — emite un reto de login.
func (s *Server) handleWebAuthnLoginBegin(w http.ResponseWriter, r *http.Request) {
	// Origin verificado (CSRF/anti-cross-origin) igual que el resto de POST de navegador.
	if !s.sameOrigin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Origen no permitido."))
		return
	}
	var req loginBeginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// Reto de un solo uso, corta vida y SIN binding de usuario (user_id NULL): al
	// usuario lo identifica la credencial presentada en finish (modelo discoverable).
	if err := s.store.InsertChallenge(r.Context(), nil, challenge, "", "login", time.Now().Add(5*time.Minute)); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}

	// allowCredentials es una PISTA para el autenticador en el flujo usuario-primero
	// (necesaria para passkeys no-descubribles). El endpoint SIEMPRE responde 200 con
	// un reto —no confirma la existencia de la cuenta por éxito/fallo—; la lista va
	// vacía para emails desconocidos o sin passkeys. Que un email CON passkeys
	// devuelva una lista no vacía es el filtrado de existencia inherente y aceptado
	// del username-first WebAuthn; la no-enumeración estricta se garantiza donde más
	// importa, en la recuperación de contraseña (respuesta uniforme). Sin email ⇒
	// lista vacía ⇒ passkey discoverable/usernameless (sin escribir usuario).
	allow := make([]map[string]string, 0)
	if email := domain.NormalizeLogin(req.Email); email != "" {
		if user, err := s.userByLogin(r.Context(), email); err == nil && user.Active() {
			if creds, err := s.store.ListCredentialsForUser(r.Context(), user.ID); err == nil {
				for _, c := range creds {
					allow = append(allow, map[string]string{
						"type": "public-key",
						"id":   b64u.EncodeToString(c.CredID),
					})
				}
			}
		}
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"challenge":        b64u.EncodeToString(challenge),
		"rpId":             s.cfg.RPID,
		"allowCredentials": allow,
		"timeout":          300000,
		"userVerification": "preferred",
	})
}

// assertionRequest es el resultado de navigator.credentials.get() que reenvía el
// front (campos base64url) más el return_to del flujo OIDC/authorize en curso.
type assertionRequest struct {
	ID       string `json:"id"`
	RawID    string `json:"rawId"`
	Type     string `json:"type"`
	Response struct {
		AuthenticatorData string `json:"authenticatorData"`
		ClientDataJSON    string `json:"clientDataJSON"`
		Signature         string `json:"signature"`
		UserHandle        string `json:"userHandle"`
	} `json:"response"`
	ReturnTo string `json:"return_to"`
}

// handleWebAuthnLoginFinish — POST /login/webauthn/finish — verifica la assertion
// y abre sesión. Mensajes de error genéricos (no enumeran ni distinguen el motivo).
//
//nolint:gocyclo // verificación WebAuthn: la secuencia de comprobaciones ES la del §7.2 de la spec; trocearla la haría auditables a medias.
func (s *Server) handleWebAuthnLoginFinish(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Origen no permitido."))
		return
	}
	var req assertionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rawID, err1 := b64u.DecodeString(req.RawID)
	authDataRaw, err2 := b64u.DecodeString(req.Response.AuthenticatorData)
	clientDataRaw, err3 := b64u.DecodeString(req.Response.ClientDataJSON)
	sig, err4 := b64u.DecodeString(req.Response.Signature)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || len(rawID) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid passkey response."))
		return
	}

	// Extraer el reto del clientDataJSON y consumirlo (atómico, un solo uso): esto
	// prueba que lo emitió el servidor y no está caducado ni reusado.
	var cd webauthn.ClientData
	if err := json.Unmarshal(clientDataRaw, &cd); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("clientData no parseable."))
		return
	}
	challengeBytes, err := b64u.DecodeString(cd.Challenge)
	if err != nil {
		loginFailJSON(w, r)
		return
	}
	_, purpose, err := s.store.ConsumeChallenge(r.Context(), nil, challengeBytes, time.Now())
	if err != nil || purpose != "login" {
		loginFailJSON(w, r)
		return
	}

	// Localizar la credencial por su id y resolver el usuario dueño.
	cred, err := s.store.GetCredentialByID(r.Context(), rawID)
	if err != nil {
		loginFailJSON(w, r)
		return
	}
	// userHandle (si el autenticador lo devuelve) debe coincidir con el dueño.
	if uh := req.Response.UserHandle; uh != "" {
		if b, err := b64u.DecodeString(uh); err != nil || string(b) != cred.UserID {
			loginFailJSON(w, r)
			return
		}
	}
	user, err := s.store.GetUserByID(r.Context(), cred.UserID)
	if err != nil || !user.Active() {
		loginFailJSON(w, r)
		return
	}

	res, err := webauthn.VerifyAssertion(webauthn.AssertionInput{
		COSEPublicKey:     cred.PublicKey,
		AuthenticatorData: authDataRaw,
		ClientDataJSON:    clientDataRaw,
		Signature:         sig,
		StoredSignCount:   cred.SignCount,
		ExpectedChallenge: challengeBytes,
		RPID:              s.cfg.RPID,
		AllowedOrigins:    s.cfg.RPOrigins,
	})
	if err != nil {
		action := "auth.webauthn.failed"
		if errors.Is(err, webauthn.ErrSignCountRegression) {
			// Señal de posible clonación del autenticador: se audita aparte.
			action = "auth.webauthn.clone_suspected"
		}
		_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, "", action, "", r))
		loginFailJSON(w, r)
		return
	}

	// Persistir el nuevo signCount (anti-clonación). ponytail: dos assertions
	// concurrentes de la misma passkey podrían pisarse el signCount; es la carrera
	// inherente al RP en WebAuthn y no habilita un login no autorizado.
	if err := s.store.UpdateSignCount(r.Context(), nil, cred.CredID, res.SignCount); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}

	// Resolver la organización activa (misma lógica que el login por contraseña).
	orgID := ""
	if mems, err := s.store.ListMembershipsForUser(r.Context(), user.ID); err == nil && len(mems) > 0 {
		orgID = mems[0].OrgID
	}
	// Una passkey es un factor fuerte y resistente a phishing: abre sesión directa,
	// sin step-up TOTP. amr=webauthn (+mfa si el autenticador verificó al usuario).
	amr := []string{"webauthn"}
	if res.UserVerified {
		amr = append(amr, "mfa")
	}
	if err := s.createSession(w, r, user.ID, orgID, amr); err != nil {
		// mapDomainErr traduce el block del Acceso Condicional (ErrAccessDenied) a 403;
		// cualquier otro error sigue siendo 500.
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, orgID, "auth.webauthn.success", "", r))

	// El front navega a este destino (continúa el flujo OIDC/authorize en curso).
	// Solo rutas internas relativas (anti open-redirect), como redirectAfterLogin.
	redirect := "/"
	if rutaInternaSegura(req.ReturnTo) {
		redirect = req.ReturnTo
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"redirect": redirect})
}

// loginFailJSON responde un fallo de login por passkey genérico (401), sin revelar
// el motivo (reto, credencial, firma o cuenta): no enumera ni distingue causas.
func loginFailJSON(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("The passkey could not be verified."))
}
