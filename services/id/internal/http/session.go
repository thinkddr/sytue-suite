// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/sesscrypto"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
)

// Los nombres base se conservan en dev HTTP. En HTTPS se emiten con __Host-
// para que el navegador impida Domain y cookie tossing desde subdominios.
const sessionCookieName = "sytue_id_session"

// mfaCookieName es la cookie httpOnly del token de continuación MFA (paso 1→2).
// Ámbito acotado a /login (no se envía al resto de la suite) y vida muy corta.
const mfaCookieName = "sytue_id_mfa"

// deviceCookieName es la cookie httpOnly, firmada y long-lived que identifica el
// dispositivo (navegador) en logins sucesivos (dispositivos de confianza, mig 0022).
const deviceCookieName = "sytue_device"

// deviceCookieTTL es la vida de la cookie de dispositivo: larga por diseño (identifica
// el navegador entre sesiones). 1 año — el device_id derivado es estable, así que un
// dispositivo de confianza sigue siéndolo mientras el navegador conserve la cookie.
const deviceCookieTTL = 365 * 24 * time.Hour

func secureCookieName(base string, secure bool) string {
	if secure {
		return "__Host-" + base
	}
	return base
}

func (s *Server) requestCookie(r *http.Request, base string) (*http.Cookie, error) {
	return r.Cookie(secureCookieName(base, s.cfg.CookieSecure))
}

func (s *Server) clearLegacyDomainCookie(w http.ResponseWriter, base, path string) {
	// Las cookies compartidas históricas solo se emitieron en HTTPS. En dev HTTP
	// no hay nada que migrar y escribir una cookie Domain adicional confunde a
	// clientes simples que no mantienen un jar completo.
	if !s.cfg.CookieSecure {
		return
	}
	suite := suiteDomain(s.cfg.Issuer)
	if suite == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Cookie de borrado; Secure sigue la configuración del entorno.
		Name: base, Value: "", Path: path, Domain: "." + suite, MaxAge: -1,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

// setSessionCookie escribe la cookie de sesión con los flags de seguridad
// (host-only, HttpOnly, Secure, SameSite=Lax).
func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	s.clearLegacyDomainCookie(w, sessionCookieName, "/")
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure es config (CookieSecure, true salvo dev http); HttpOnly y SameSite=Lax siempre fijos.
		Name:     secureCookieName(sessionCookieName, s.cfg.CookieSecure),
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// setMFACookie escribe la cookie de continuación MFA (paso 1→2). Usa Path=/
// porque el prefijo __Host- lo exige; el nombre propio, HttpOnly y su vida corta
// impiden que se confunda con la sesión normal.
func (s *Server) setMFACookie(w http.ResponseWriter, token string, expires time.Time) {
	s.clearLegacyDomainCookie(w, mfaCookieName, "/login")
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure es config (CookieSecure, true salvo dev http); HttpOnly y SameSite=Lax siempre fijos.
		Name:     secureCookieName(mfaCookieName, s.cfg.CookieSecure),
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// setDeviceCookie escribe la cookie host-only de dispositivo firmada.
func (s *Server) setDeviceCookie(w http.ResponseWriter, value string) {
	s.clearLegacyDomainCookie(w, deviceCookieName, "/")
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure es config (CookieSecure, true salvo dev http); HttpOnly y SameSite=Lax siempre fijos.
		Name:     secureCookieName(deviceCookieName, s.cfg.CookieSecure),
		Value:    value,
		Path:     "/",
		Expires:  time.Now().Add(deviceCookieTTL),
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// registerDevice identifica (o registra) el dispositivo del login y refresca su última
// actividad. Lee la cookie de dispositivo firmada; si falta o no verifica, acuña un
// secreto de navegador nuevo y emite la cookie. El device_id se DERIVA de (usuario,
// secreto), así que es estable por (usuario, navegador) y único global. Best-effort:
// un fallo registrando NO rompe el login (solo se pierde el reconocimiento del
// dispositivo hasta el próximo login). El dispositivo nace `trusted=false`.
func (s *Server) registerDevice(w http.ResponseWriter, r *http.Request, userID, orgID string) {
	b, ok := "", false
	if c, err := s.requestCookie(r, deviceCookieName); err == nil && c.Value != "" {
		b, ok = domain.ParseDeviceCookie(s.deviceKey, c.Value)
	}
	if !ok {
		var err error
		if b, err = domain.GenerateToken(32); err != nil {
			s.log.Warn("dispositivos: no se pudo acuñar el secreto de dispositivo", "error", err)
			return
		}
		s.setDeviceCookie(w, domain.SignDeviceCookie(s.deviceKey, b))
	}
	dev := domain.Device{
		OrgID:     orgID,
		UserID:    userID,
		DeviceID:  domain.DeriveDeviceID(s.deviceKey, userID, b),
		Name:      deviceES(i18n.Default, r.UserAgent()),
		UserAgent: r.UserAgent(),
		LastIP:    clientIP(r),
	}
	// Un dispositivo que la persona no había usado nunca es un «nuevo inicio de
	// sesión» en su campana. El primero de todos no: es el alta, no una alarma.
	nuevo := false
	if ds, err := s.store.ListDevicesForUser(r.Context(), userID); err == nil && len(ds) > 0 {
		nuevo = !slices.ContainsFunc(ds, func(d domain.Device) bool { return d.DeviceID == dev.DeviceID })
	}
	if _, err := s.store.UpsertDevice(r.Context(), nil, dev); err != nil {
		s.log.Warn("dispositivos: no se pudo registrar el dispositivo", "user_id", userID, "error", err)
		return
	}
	if nuevo {
		s.avisarSeguridad(r.Context(), orgID, userID, "security.signin.new", map[string]string{"device": dev.Name})
	}
}

// clearMFACookie elimina la cookie de continuación MFA.
func (s *Server) clearMFACookie(w http.ResponseWriter) {
	s.clearLegacyDomainCookie(w, mfaCookieName, "/login")
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure es config (CookieSecure, true salvo dev http); HttpOnly y SameSite=Lax siempre fijos.
		Name:     secureCookieName(mfaCookieName, s.cfg.CookieSecure),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie elimina la cookie de sesión (logout).
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	s.clearLegacyDomainCookie(w, sessionCookieName, "/")
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure es config (CookieSecure, true salvo dev http); HttpOnly y SameSite=Lax siempre fijos.
		Name:     secureCookieName(sessionCookieName, s.cfg.CookieSecure),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// currentSession resuelve la sesión SSO vigente desde la cookie, o (zero,false).
func (s *Server) currentSession(r *http.Request) (domain.Session, bool) {
	c, err := s.requestCookie(r, sessionCookieName)
	if err != nil || c.Value == "" {
		return domain.Session{}, false
	}
	sess, err := s.store.GetSessionByToken(r.Context(), domain.HashToken(c.Value))
	if err != nil {
		return domain.Session{}, false
	}
	if !sess.Valid(time.Now()) {
		return domain.Session{}, false
	}
	// La sesión NO sobrevive a la baja de la cuenta. Antes solo se comprobaba su propia
	// vigencia, así que un usuario desactivado o dado de baja —incluido el
	// desaprovisionamiento por SCIM, que es el offboarding estándar de empresa— seguía
	// con su navegador dentro: pasaba por /authorize obteniendo códigos y tokens
	// frescos, y por /saml/idp/sso obteniendo Assertions FIRMADAS hacia SPs de terceros,
	// donde la caducidad la manda el SP (acceso efectivamente ilimitado aguas abajo).
	// Este es el punto por el que pasan TODOS los flujos con sesión, así que una sola
	// comprobación los cubre. Fail-CLOSED: si el usuario no se puede leer, no hay sesión.
	u, uerr := s.store.GetUserByID(r.Context(), sess.UserID)
	if uerr != nil || !u.Active() {
		return domain.Session{}, false
	}
	if sess.OrgID != "" {
		org, oerr := s.store.GetOrgByID(r.Context(), sess.OrgID)
		if oerr != nil || org.Status != domain.StatusActive || !s.orgEntitled(r.Context(), sess.OrgID) {
			return domain.Session{}, false
		}
	}
	return sess, true
}

// createSession crea una sesión SSO y emite su cookie.
func (s *Server) createSession(w http.ResponseWriter, r *http.Request, userID, orgID string, amr []string) error {
	// Bloqueo de emergencia (mig 0035): con el interruptor levantado no se abre
	// NINGUNA sesión nueva. Va lo primero, antes incluso del Acceso Condicional,
	// porque el supuesto del bloqueo es que la credencial ya está en manos
	// ajenas: rotar la clave de firma invalida lo emitido, pero sin esto quien
	// tenga la contraseña vuelve a entrar un segundo después y deshace el corte.
	//
	// Se consulta la BASE y no un flag del proceso, por lo mismo que
	// `platform_admins`: levantarlo y bajarlo tiene que ser inmediato y global,
	// sin reiniciar réplicas. Un error de consulta DENIEGA (fail-closed).
	if cerrado, err := s.store.IsLockedDown(r.Context()); err != nil || cerrado {
		if err != nil {
			s.log.Error("no se pudo comprobar el bloqueo de emergencia", "error", err)
		}
		_ = s.store.Audit(r.Context(), nil, auditEntry(userID, orgID, "auth.lockdown.denied", "", r))
		return domain.ErrAccessDenied
	}
	// Verja anti-bypass del Acceso Condicional: el `block` se aplica AQUÍ, el punto
	// por el que pasan TODOS los métodos de login (contraseña, passkey, federado,
	// SAML), para que ninguno lo esquive. El login interactivo ya trata block y
	// require_mfa con su UX (mensaje/step-up) ANTES de llegar aquí; esta es la red de
	// seguridad para los demás flujos. client_id no se conoce en este punto ("") — las
	// condiciones por-app se evalúan en el login interactivo. Sin políticas ⇒ allow
	// (no cambia nada para nadie): anti-lockout.
	if d := s.resolveAccessDecision(r.Context(), r, userID, orgID, ""); d.Action != domain.AccessAllow {
		if d.Action == domain.AccessBlock {
			s.auditAccessDecision(r.Context(), r, userID, orgID, "auth.ca.blocked", d)
			return domain.ErrAccessDenied
		}
		// require_mfa TAMBIÉN se aplica aquí, y no solo en el login interactivo. Antes
		// esta verja solo miraba `block`, así que una política que exigía segundo factor
		// se cumplía al entrar con contraseña y se IGNORABA en silencio al entrar por
		// SSO federado o SAML: esos caminos crean la sesión directamente con
		// amr=["federated"]/["saml"]. Quien estuviera en el grupo con MFA obligatorio
		// entraba sin segundo factor sin más que usar el botón de su IdP corporativo.
		//
		// La prueba de que hubo segundo factor es el `amr`, que ya lo transporta. Si no
		// lo trae, se DENIEGA en vez de dejar pasar: la política dice explícitamente que
		// hace falta MFA y aquí no hay forma de pedirlo (el IdP de arriba ya terminó su
		// flujo). Fail-closed a propósito — y el evento de auditoría lo nombra aparte
		// para que el administrador vea por qué, en vez de un «acceso denegado» opaco.
		if !amrHasSecondFactor(amr) {
			s.auditAccessDecision(r.Context(), r, userID, orgID, "auth.ca.mfa_missing", d)
			return domain.ErrAccessDenied
		}
	}
	// El USUARIO tiene que seguir activo EN ESTE INSTANTE, no cuando empezó el login.
	// Entre el paso 1 (contraseña correcta, se emite el token de continuación) y el paso
	// 2 (código TOTP) hay una ventana de minutos, y el paso 2 abría la sesión con el
	// `cont.UserID` sin releer nada: suspender o borrar la cuenta durante esa ventana no
	// impedía que el login terminara. Lo mismo valía para SAML, que crea la sesión
	// directamente. Va AQUÍ, el punto por el que pasan los cinco métodos, en vez de en
	// cada uno: es el mismo motivo por el que están aquí el Acceso Condicional y el
	// estado de la organización. Fail-CLOSED: si el usuario no se puede leer, no hay
	// sesión.
	u, uerr := s.store.GetUserByID(r.Context(), userID)
	if uerr != nil || !u.Active() {
		return domain.ErrAccessDenied
	}

	// Una organización SUSPENDIDA no admite inicios de sesión. `organizations.status` se
	// escribía y NO se leía en ninguna verja de autorización (solo al serializar
	// respuestas), así que el botón de cortar un tenant —impago, incidente, cuenta
	// comprometida— era puramente cosmético: sus miembros seguían entrando y recibiendo
	// tokens con el claim `tenant` de esa org, del que TODA la suite deriva su
	// aislamiento. Y no hay otra palanca: no existe borrado de organización.
	//
	// Se comprueba AQUÍ por el mismo motivo que el Acceso Condicional: es el punto por
	// el que pasan los cinco métodos de login. A propósito NO se filtra por estado en
	// GetOrgBySlug/GetOrgByID: esas lecturas se usan también para REACTIVAR la org, y
	// filtrarlas ahí sería un autobloqueo. Fail-CLOSED: si la org no se puede leer, no
	// se crea la sesión.
	if orgID != "" {
		org, oerr := s.store.GetOrgByID(r.Context(), orgID)
		if oerr != nil || org.Status != domain.StatusActive {
			return domain.ErrAccessDenied
		}
		// Un miembro sin suscripción pagada puede autenticarse solo en modo de
		// bootstrap (sin tenant) para completar el checkout del onboarding. Ese
		// token no contiene claim tenant y ningún servicio de la suite lo acepta
		// para datos de organización.
		if !s.orgEntitled(r.Context(), orgID) {
			orgID = ""
		}
	}
	token, err := domain.GenerateToken(32)
	if err != nil {
		return err
	}
	now := time.Now()
	exp := now.Add(s.cfg.SessionTTL)
	// La IP se usa AQUÍ y no vuelve a tocar el disco: se resuelve la ubicación
	// aproximada y se cifra en el acto con la clave derivada de este token. Lo que
	// se persiste es solo ese texto cifrado; ni la IP ni el token quedan en la
	// base de datos (de él solo su hash), así que nadie —tampoco nosotros— puede
	// leer de dónde vino este acceso sin la cookie de su dueño.
	ip := clientIP(r)
	lugar := ""
	if ip != "" {
		if u, ok := s.store.LookupIPs(r.Context(), []string{ip})[ip]; ok {
			lugar = u.String()
		}
	}
	lugarCifrado, err := sesscrypto.Cifrar(token, lugar)
	if err != nil {
		return err
	}
	sess := domain.Session{
		TokenHash:    domain.HashToken(token),
		UserID:       userID,
		OrgID:        orgID,
		AuthTime:     now,
		AMR:          amr,
		UserAgent:    r.UserAgent(),
		LugarCifrado: lugarCifrado,
		ExpiresAt:    exp,
	}
	if _, err := s.store.InsertSession(r.Context(), nil, sess); err != nil {
		return err
	}
	s.setSessionCookie(w, token, exp)
	// Registrar/identificar el dispositivo (mig 0022): choke point de TODOS los métodos
	// de login (contraseña, passkey, federado, SAML), como la verja anti-bypass de CA.
	// Requiere org (FK); sin org no hay contexto de dispositivo que persistir.
	if orgID != "" {
		s.registerDevice(w, r, userID, orgID)
	}
	return nil
}

// clientIP resuelve la IP del cliente CREYENDO la cabecera X-Forwarded-For solo
// si quien conecta es un proxy de la lista de confianza (ver
// domain.TrustedProxies, que la lleva el contexto desde el router).
//
// Antes se leía la PRIMERA entrada de la cabecera sin mirar nada más, y esa
// entrada la escribe el cliente: bastaba `X-Forwarded-For: <lo que sea>` para
// estrenar un bucket de fuerza bruta en cada intento de login, para afirmar una
// IP de dentro de la oficina ante una política de Acceso Condicional que bloquea
// por red, y para dejar la IP que quisiera en el registro de auditoría.
func clientIP(r *http.Request) string {
	return domain.TrustedProxiesFrom(r.Context()).
		ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
}

// trim recorta TODO el espacio en blanco, no solo el carácter espacio.
//
// La versión anterior comparaba con ' ' a mano, así que un tabulador o un salto
// de línea sobrevivían al recorte. Eso no es cosmético en los dos sitios donde
// se usa para VALIDAR: el motivo de suspender una organización (platform.go) y
// la justificación de elevarse a plataforma (platform_pim.go) se rechazan
// cuando quedan vacíos, y con `"\t\n"` no quedaban vacíos — se suspendía a una
// empresa entera dejando en el registro un motivo que es un tabulador.
func trim(s string) string { return strings.TrimSpace(s) }

// amrHasSecondFactor dice si los métodos de autenticación de esta sesión incluyen un
// segundo factor, según los valores que emiten los cinco caminos de login.
//
// `webauthn` cuenta por sí solo: una passkey verifica al usuario en el dispositivo
// (posesión + biometría o PIN) y es resistente a phishing, así que exigirle además un
// TOTP sería pedir un factor MÁS DÉBIL encima de uno más fuerte. `pwd`, `federated` y
// `saml` no cuentan: son un único factor, o uno cuya fuerza la decide un tercero que no
// nos la cuenta.
func amrHasSecondFactor(amr []string) bool {
	for _, m := range amr {
		if m == "mfa" || m == "webauthn" {
			return true
		}
	}
	return false
}
