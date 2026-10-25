// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/operador"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// authzRequest captura los parámetros de la petición de autorización (RFC 6749
// §4.1.1 + OIDC + PKCE RFC 7636).
type authzRequest struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	Scope               string
	State               string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
	// MaxAge es `max_age` (OIDC Core §3.1.2.1): cuántos segundos vale como
	// mucho la última identificación de la persona. Nil = no se pidió.
	//
	// Es la otra mitad de la reautenticación fuerte (stepup.go). Sin esto, un
	// 401 `insufficient_user_authentication` sería un callejón sin salida: el
	// cliente volvería a /authorize, la sesión SSO seguiría valiendo y saldría
	// otro token con el MISMO `auth_time` viejo.
	MaxAge *int
}

func parseAuthz(r *http.Request) authzRequest {
	q := r.URL.Query()
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		q = r.Form
	}
	method := q.Get("code_challenge_method")
	if method == "" && q.Get("code_challenge") != "" {
		method = "S256"
	}
	var maxAge *int
	// Un `max_age` ilegible se IGNORA, no se toma por cero: leerlo como 0
	// obligaría a reidentificarse en cada vuelta por un parámetro mal escrito.
	if v := strings.TrimSpace(q.Get("max_age")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxAge = &n
		}
	}
	return authzRequest{
		MaxAge:              maxAge,
		ClientID:            q.Get("client_id"),
		RedirectURI:         q.Get("redirect_uri"),
		ResponseType:        q.Get("response_type"),
		Scope:               strings.TrimSpace(q.Get("scope")),
		State:               q.Get("state"),
		Nonce:               q.Get("nonce"),
		CodeChallenge:       q.Get("code_challenge"),
		CodeChallengeMethod: method,
	}
}

// handleAuthorize implementa el authorization endpoint (Authorization Code+PKCE).
// Si no hay sesión SSO válida, redirige a /login conservando la petición.
func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	req := parseAuthz(r)

	client, err := s.store.GetClient(r.Context(), req.ClientID)
	if err != nil {
		// client_id inválido: NO se redirige (riesgo de open redirect); error directo.
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("client_id desconocido."))
		return
	}
	// Validación EXACTA del redirect_uri (anti open-redirect, OAuth2 BCP).
	if req.RedirectURI == "" || !client.ValidRedirectURI(req.RedirectURI) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("redirect_uri is not registered for the client."))
		return
	}

	// A partir de aquí los errores se devuelven al redirect_uri (RFC 6749 §4.1.2.1).
	redirectErr := func(code, desc string) {
		u, _ := url.Parse(req.RedirectURI)
		v := u.Query()
		v.Set("error", code)
		if desc != "" {
			v.Set("error_description", desc)
		}
		if req.State != "" {
			v.Set("state", req.State)
		}
		u.RawQuery = v.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	}

	if code, desc, ok := validarAuthz(req, client); !ok {
		redirectErr(code, desc)
		return
	}

	// ¿Hay sesión SSO válida? Si no, ir a login conservando la petición original.
	sess, ok := s.currentSession(r)
	if !ok {
		loginURL := "/login/?" + url.Values{"return_to": {r.URL.RequestURI()}}.Encode()
		http.Redirect(w, r, loginURL, http.StatusFound)
		return
	}

	// Una app acotada a una organización (OIDC de terceros, mig 0017) NO sirve a una
	// sesión de OTRA organización: sin esto, cargar la dirección de autorización con
	// la sesión de una persona de otra organización emitía un código con el tenant de
	// esa sesión, y con él un token con alcance sobre sus datos (NEG-05). Un client
	// GLOBAL de la suite (OrgID "") pasa, que es la base de la sesión compartida entre
	// apps. Se responde al redirect_uri con `access_denied` (ya validado arriba), no un
	// error opaco: el cliente distingue «no autorizado» de un fallo. Es la misma
	// atadura que la Directory API ya exige (google_compat.go), traída al nacimiento
	// del código.
	if !client.UsableByOrg(sess.OrgID) {
		_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "oauth.authorize.cross_org_denied", client.ID, r))
		redirectErr("access_denied", "esta aplicación no pertenece a tu organización")
		return
	}

	// `max_age`: la sesión SSO existe, pero puede ser DEMASIADO VIEJA para lo
	// que el cliente va a hacer con el token. Volver a identificarse crea una
	// sesión nueva con `auth_time` de ahora (session.go), así que la vuelta a
	// esta misma URL ya cumple y no hay bucle.
	//
	// Va ANTES del step-up de MFA a propósito: si hay que reidentificarse
	// entera, el segundo factor se pide dentro de ese login y no dos veces.
	if req.MaxAge != nil && time.Since(sess.AuthTime) > time.Duration(*req.MaxAge)*time.Second {
		loginURL := "/login/?" + url.Values{"return_to": {r.URL.RequestURI()}}.Encode()
		http.Redirect(w, r, loginURL, http.StatusFound)
		return
	}

	// El MFA por rol protege las consolas de administración. Entrar en un producto
	// o en Mi cuenta no obliga a enrolarlo: Mi cuenta es precisamente donde se
	// activa. El login sigue exigiendo el factor ya habilitado por la persona y el
	// Acceso Condicional que haya establecido su organización.
	if s.requiresStepUp(r, sess, client) {
		if err := s.desviarAStepUp(w, r, sess, client.ID); err != nil {
			redirectErr("server_error", "")
		}
		return
	}

	// App nativa de la suite (pública): la primera vez en cada dispositivo, se pregunta
	// (NAT-02, authorize_nativa.go). El código queda atado al dispositivo.
	dispositivo := ""
	if client.EsNativa() {
		var sigue bool
		if dispositivo, sigue = s.consentimientoNativo(w, r, req, client, sess, redirectErr); !sigue {
			return
		}
	}

	// Emitir código de autorización (un solo uso). Se persiste solo el hash.
	code, err := domain.GenerateToken(32)
	if err != nil {
		redirectErr("server_error", "")
		return
	}
	now := time.Now()
	ac := domain.AuthCode{
		CodeHash:            domain.HashToken(code),
		ClientID:            client.ID,
		UserID:              sess.UserID,
		OrgID:               sess.OrgID,
		SessionID:           sess.ID,
		RedirectURI:         req.RedirectURI,
		Scope:               req.Scope,
		Nonce:               req.Nonce,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		AuthTime:            sess.AuthTime,
		ExpiresAt:           now.Add(s.cfg.AuthCodeTTL),
		Dispositivo:         dispositivo,
	}
	if err := s.store.InsertAuthCode(r.Context(), nil, ac); err != nil {
		redirectErr("server_error", "")
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "oauth.authorize", client.ID, r))

	// Redirigir con el código (y el state intacto).
	u, _ := url.Parse(req.RedirectURI)
	v := u.Query()
	v.Set("code", code)
	if req.State != "" {
		v.Set("state", req.State)
	}
	u.RawQuery = v.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// validarAuthz comprueba lo que se puede comprobar de la petición sin sesión:
// el tipo de respuesta, el grant del client, PKCE y los scopes. Devuelve el par
// (código, descripción) del error de OAuth2 y si la petición vale.
//
// Está fuera de handleAuthorize para que el flujo de arriba quepa de un vistazo:
// lo que decide de verdad es el orden de sesión → max_age → segundo factor, y
// con estas cuatro validaciones en medio ese orden no se veía.
func validarAuthz(req authzRequest, client domain.Client) (code, desc string, ok bool) {
	if req.ResponseType != "code" {
		return "unsupported_response_type", "solo se soporta response_type=code", false
	}
	if !client.AllowsGrant("authorization_code") {
		return "unauthorized_client", "el client no permite authorization_code", false
	}
	// PKCE obligatorio para clients públicos SIEMPRE (OAuth2 Security BCP /
	// RFC 9700 §2.1.1) y para cualquier client que lo exija por configuración.
	// Solo se acepta el método S256 (nunca `plain`, que no protege ante fuga del
	// challenge — RFC 7636 §7.2 y OWASP ASVS L2 §3.5).
	if client.RequirePKCE || client.IsPublic() {
		if req.CodeChallenge == "" {
			return "invalid_request", "code_challenge requerido (PKCE)", false
		}
		if req.CodeChallengeMethod != "S256" {
			return "invalid_request", "code_challenge_method debe ser S256", false
		}
	}
	for _, sc := range strings.Fields(req.Scope) {
		if !client.AllowsScope(sc) {
			return "invalid_scope", "scope no permitido: " + sc, false
		}
	}
	return "", "", true
}

// desviarAStepUp manda a completar el segundo factor sobre la sesión que ya
// existe (no se vuelve a pedir la contraseña: lo que falta es el 2º factor).
//
// Emite el token de continuación que consume el formulario del paso 2. El
// step-up redirigía a ese formulario SIN emitirlo, así que el código correcto
// acababa siempre en «Vuelve a iniciar sesión» y volver a entrar traía otra vez
// aquí: un bucle cerrado, y la única forma de entrar a platform.sytue.eu.
func (s *Server) desviarAStepUp(w http.ResponseWriter, r *http.Request, sess domain.Session, clientID string) error {
	u, err := s.store.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		// Un fallo al leer la cuenta no es lo mismo que no tener segundo factor:
		// mezclarlos mandaba a enrolar MFA a quien ya lo tiene, cada vez que la
		// consulta fallara. Se propaga y lo trata el llamante.
		return err
	}
	if !u.MFAEnabled {
		// Sin segundo factor enrolado no hay nada que verificar: se le lleva a
		// darlo de alta, que es el único desenlace posible. Un callejón sin salida
		// sería peor que un rodeo explicado.
		http.Redirect(w, r, "/account?mfa=required", http.StatusFound)
		return nil
	}
	if err := s.beginMFAStepUp(w, r, sess.UserID, sess.OrgID, r.URL.RequestURI()); err != nil {
		return err
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "auth.mfa.challenge", clientID, r))
	stepURL := "/login/?" + url.Values{
		"return_to": {r.URL.RequestURI()},
		"step_up":   {"1"},
	}.Encode()
	http.Redirect(w, r, stepURL, http.StatusFound)
	return nil
}

// requiresStepUp indica si la sesión necesita MFA por su rol al entrar en una
// consola. El client ya fue leído y validado antes de llegar aquí: no se confía en
// un client_id sin validar de la query.
func (s *Server) requiresStepUp(r *http.Request, sess domain.Session, client domain.Client) bool {
	if client.ID != "sytue-console" && client.ID != operador.Audiencia {
		return false
	}
	// Si la sesión ya incluye MFA/WebAuthn en AMR, no hace falta step-up.
	for _, m := range sess.AMR {
		if m == "mfa" || m == "webauthn" || m == "otp" {
			return false
		}
	}
	// La administración de plataforma exige MFA SIEMPRE (ver platform.go), y su
	// alcance no es un rol de organización: va antes del corte por org, porque un
	// admin global puede no administrar ninguna. Sin esto entraba en
	// platform.sytue.eu sin que nadie le pidiera el segundo factor, y la consola
	// respondía 403 en las cuatro llamadas sin decir cómo arreglarlo.
	// Fail-closed: si no se puede comprobar, se exige el segundo factor.
	if esAdmin, err := s.store.IsPlatformAdmin(r.Context(), sess.UserID); err != nil || esAdmin {
		if err != nil {
			s.log.Warn("requiresStepUp: no se pudo comprobar el alcance de plataforma; se fuerza step-up",
				"user_id", sess.UserID, "error", err)
		}
		// Llegar aquí con la sesión recién elevada es un BUCLE: el segundo factor
		// acaba de darse, `handleMFAVerify` abre la sesión con amr=["pwd","mfa"] y
		// aun así se vuelve a pedir. Se registra el `amr` y el id de la sesión que
		// se ha leído, que es la única pregunta abierta: si `currentSession`
		// devuelve la sesión nueva o una anterior.
		//
		// Medido el 2026-08-17: el ciclo entero pasa en integración
		// (`mfa_ciclo_integration_test.go`), así que lo que difiere está en el dato
		// o en el entorno de producción, y desde fuera los dos caminos que salen de
		// aquí son indistinguibles. Va en INFO y solo en esta rama —la que fuerza el
		// step-up—, así que no habla salvo cuando hay algo que contar.
		s.log.Info("requiresStepUp: se exige segundo factor por alcance de plataforma",
			"user_id", sess.UserID, "sesion_id", sess.ID, "amr", sess.AMR,
			"auth_time", sess.AuthTime, "org_id", sess.OrgID)
		return true
	}
	if sess.OrgID == "" {
		return false
	}
	// Una cuenta personal es una persona sola (consumidor); no administra
	// ninguna organización corporativa y no está obligada a enrolar MFA
	// solo por ser el owner de su propio espacio (ADR 0038).
	if org, err := s.store.GetOrgByID(r.Context(), sess.OrgID); err == nil && org.Kind == domain.OrgKindPersonal {
		return false
	}
	roles, err := s.store.RolesForUserInOrg(r.Context(), sess.OrgID, sess.UserID)
	if err != nil {
		// Fail-closed: si no se puede determinar el rol de forma fiable (timeout de
		// BD, contención...), exigir step-up MFA en vez de emitir tokens sin segundo
		// factor. La política MFA de owner/admin no debe saltarse por un error
		// transitorio (docs/CONVENTIONS.md §8).
		s.log.Warn("requiresStepUp: no se pudieron leer los roles; se fuerza step-up (fail-closed)",
			"user_id", sess.UserID, "org_id", sess.OrgID, "error", err)
		return true
	}
	for _, role := range roles {
		if domain.Role(role).RequiresMFA() {
			// MISMO LOG QUE LA RAMA DE PLATAFORMA, y por el mismo motivo. Esa se
			// instrumentó tras el bucle de agosto de 2026 y esta se quedó muda, así
			// que cuando el bucle volvió —cuenta de auditoría, septiembre— el
			// `grep requiresStepUp` salió VACÍO y pareció que la función ni se
			// llamaba. Las dos salidas tienen que contar lo mismo o la próxima vez
			// se vuelve a diagnosticar a ciegas.
			//
			// Llegar aquí significa que la sesión leída NO trae segundo factor en su
			// `amr` —la primera condición de esta función habría devuelto false—, así
			// que lo que hay que mirar es qué sesión ha devuelto `currentSession`.
			s.log.Info("requiresStepUp: se exige segundo factor por rol",
				"user_id", sess.UserID, "sesion_id", sess.ID, "amr", sess.AMR,
				"auth_time", sess.AuthTime, "org_id", sess.OrgID, "rol", role)
			return true
		}
	}
	return false
}
