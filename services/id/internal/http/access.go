// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Acceso Condicional (paridad Entra Conditional Access): evaluación de las políticas
// de la org en el LOGIN — TRAS verificar la credencial y ANTES de emitir la sesión.
// El motor de decisión es puro (domain.EvaluateAccess, testeable offline); aquí vive
// solo el CABLEADO: cargar las políticas habilitadas de la org y los grupos del
// usuario, y traducir la decisión.
//
// ANTI-LOCKOUT (crítico): sin políticas ⇒ permitir. Un error transitorio cargando
// políticas (BD) NO debe dejar a toda la org fuera: se FALLA ABIERTO (permitir) y se
// audita. El bloqueo real solo ocurre con una política habilitada que aplique y no
// exima al usuario. La verja anti-bypass vive en createSession (se evalúa el `block`
// para TODOS los métodos de login: contraseña, passkey, federado, SAML).
package http

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// resolveAccessDecision carga las políticas habilitadas de la org y los grupos del
// usuario, y evalúa el intento. Fail-open: ante error de BD o sin org, devuelve
// AccessAllow (nunca cierra la org por un fallo transitorio) y lo audita. clientID
// puede ir vacío (flujos donde la app pendiente no se conoce todavía).
func (s *Server) resolveAccessDecision(ctx context.Context, r *http.Request, userID, orgID, clientID string) domain.AccessDecision {
	allow := domain.AccessDecision{Action: domain.AccessAllow}
	if orgID == "" {
		return allow // sin contexto de org no hay Acceso Condicional que aplicar.
	}
	policies, err := s.store.ListEnabledPolicies(ctx, orgID)
	if err != nil {
		s.log.Warn("acceso condicional: no se pudieron cargar las políticas; se permite (fail-open)",
			"org_id", orgID, "user_id", userID, "error", err)
		_ = s.store.Audit(ctx, nil, auditEntry(userID, orgID, "auth.ca.error", "", r))
		return allow
	}
	if len(policies) == 0 {
		return allow // sin políticas: nadie se queda fuera.
	}
	// Grupos del usuario en la org (para include/exclude por grupo). Un error aquí no
	// debe bloquear: se evalúa sin grupos (las condiciones por usuario/IP/app siguen).
	groups, gerr := s.store.GroupIDsForUser(ctx, orgID, userID)
	if gerr != nil {
		s.log.Warn("acceso condicional: no se pudieron leer los grupos del usuario; se evalúa sin grupos",
			"org_id", orgID, "user_id", userID, "error", gerr)
		groups = nil
	}
	actx := domain.AccessContext{
		UserID:   userID,
		GroupIDs: groups,
		IP:       clientIP(r),
		ClientID: clientID,
		Now:      time.Now(),
	}
	// Solo resolvemos el dispositivo si alguna política lo exige (evita una lectura de
	// BD en el caso común sin condición de dispositivo).
	if domain.PoliciesNeedDevice(policies) {
		actx.TrustedDevice = s.deviceTrusted(ctx, r, userID)
	}
	return domain.EvaluateAccess(policies, actx)
}

// deviceTrusted resuelve si el request viene de un dispositivo de CONFIANZA del usuario:
// lee la cookie de dispositivo firmada, deriva el device_id (ligado al usuario) y
// consulta su marca `trusted`. Sin cookie o firma inválida ⇒ no es un dispositivo
// conocido (false, el default seguro para "exigir dispositivo gestionado"). Un dispositivo
// no registrado ⇒ false (caso normal de primer login). Un ERROR transitorio de BD ⇒ true
// (FAIL-OPEN, coherente con el anti-lockout de CA: un fallo puntual no cierra la sesión;
// el break-glass de exclusiones y "sin política ⇒ allow" siguen intactos).
func (s *Server) deviceTrusted(ctx context.Context, r *http.Request, userID string) bool {
	c, err := s.requestCookie(r, deviceCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	b, ok := domain.ParseDeviceCookie(s.deviceKey, c.Value)
	if !ok {
		return false
	}
	dev, err := s.store.GetDeviceByDeviceID(ctx, userID, domain.DeriveDeviceID(s.deviceKey, userID, b))
	if errors.Is(err, domain.ErrNotFound) {
		return false
	}
	if err != nil {
		s.log.Warn("dispositivos de confianza: fallo leyendo el dispositivo; se trata como de confianza (fail-open, anti-lockout)",
			"user_id", userID, "error", err)
		return true
	}
	return dev.Trusted
}

// auditAccessDecision registra una decisión NO trivial (block / require_mfa) con la
// política decisoria, para trazabilidad (docs/CONVENTIONS.md §10). allow no se audita
// (ruido: el login normal ya deja su propia entrada).
func (s *Server) auditAccessDecision(ctx context.Context, r *http.Request, userID, orgID, action string, d domain.AccessDecision) {
	e := auditEntry(userID, orgID, action, d.PolicyID, r)
	e.Detail["policy_id"] = d.PolicyID
	e.Detail["policy_name"] = d.PolicyName
	_ = s.store.Audit(ctx, nil, e)
}

// clientIDFromReturnTo extrae el client_id de la petición /authorize pendiente que el
// login conserva en return_to (solo rutas internas). Permite que las condiciones por
// app casen ya en el login interactivo. Vacío si no hay app pendiente o no se parsea.
func clientIDFromReturnTo(returnTo string) string {
	if returnTo == "" {
		return ""
	}
	u, err := url.Parse(returnTo)
	if err != nil {
		return ""
	}
	return u.Query().Get("client_id")
}

// Mensajes de denegación por Acceso Condicional (sobrios, accionables).
const (
	msgAccessBlocked = "Tu organización no permite el acceso en estas condiciones. Contacta con un administrador."
	// require_mfa sobre un usuario SIN segundo factor: no se puede enrolar antes de la
	// sesión, así que se deniega con indicación clara (un admin excluido hace break-glass).
	msgMFAEnrollRequired = "Tu organización exige verificación en dos pasos. Pide a un administrador que la habilite en tu cuenta."
)
