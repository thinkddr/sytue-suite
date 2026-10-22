// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// Claves con las que se guarda en el vault lo que devuelve el consentimiento. El
// access token va en la clave BEARER convencional a propósito: es lo que la pasarela
// inyecta ya en todo el egress (ver egressRuntime), así que los drivers SCIM y de datos
// funcionan sin enterarse de que la credencial viene de un consentimiento y no de un
// token pegado. Esa es la razón de que conectar por OAuth no obligue a tocar un driver.
// Los valores son NOMBRES DE CLAVE del vault, no credenciales: gosec G101 solo ve un
// identificador que suena a secreto asignado a una constante.
const (
	secretKeyRefresh = "oauth_refresh" //nolint:gosec // nombre de la clave, no el token

	// configKeyExpires deja el vencimiento del access token en la config PÚBLICA: no
	// es un secreto y el asistente lo enseña («caduca en 1 h, se renueva sola»).
	configKeyExpires = "_oauth_expires_at"
	// configKeyClientID permite el modo BYO: la organización registra su propia app
	// ante el proveedor cuando Sytue no tiene una. El client_id no es secreto (viaja
	// en la URL de autorización); su secret sí, y ese va al vault.
	configKeyClientID = "_oauth_client_id"
	secretKeyClientSc = "oauth_client_secret" //nolint:gosec // nombre de la clave, no el secreto
)

// oauthStartResp es lo que el asistente necesita para el paso «Conectar»: adónde mandar
// al admin y qué se le va a pedir que autorice.
type oauthStartResp struct {
	AuthorizeURL string   `json:"authorize_url"`
	Scopes       []string `json:"scopes"`
}

// oauthStartReq permite decir DESDE QUÉ asistente se conecta, para devolver a la
// persona a esa misma pantalla al volver del proveedor. Vacío ⇒ el catálogo de apps.
type oauthStartReq struct {
	ReturnTo string `json:"return_to"`
	// Purpose "out" pide también los permisos de escritura del manifiesto: es el
	// consentimiento para llevar contenido de Sytue a la cuenta del proveedor.
	Purpose string `json:"purpose"`
}

// consoleSections son las secciones de la consola a las que se puede volver tras el
// consentimiento. Es una ALLOWLIST y no un path libre: el destino acaba en un
// Location, y aceptar cualquiera sería una redirección abierta con el nombre de
// Sytue delante.
var consoleSections = map[string]bool{"apps": true, "migrations": true, "storage": true, "rooms": true, "external-chat": true, teamsMeetingSection: true, "native-storage": true, "native-rooms": true, "native-apps": true, "personal-storage": true, "native-personal-storage": true, "external-mail": true, "native-mail": true, "native-calendar": true, "native-contacts": true, "native-notes": true, "external-calendar": true, "external-contacts": true, "external-notes": true}

// seccionSalida marca, dentro de la sección sellada, que se autorizó la ESCRITURA: al
// volver, Migraciones abre el asistente de salida y no el de traer.
const seccionSalida = ":out"

// conEscritura añade los permisos de escritura a los de siempre, sin repetir.
func conEscritura(scopes, escritura []string) []string {
	out := append([]string(nil), scopes...)
	for _, s := range escritura {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// sectionOr valida la sección pedida contra la allowlist ("apps" por defecto).
func sectionOr(raw string) string {
	s := strings.TrimSpace(raw)
	if consoleSections[s] {
		return s
	}
	return "apps"
}

func personalConsentTemplate(key string) bool {
	return externalChatTemplate(key) || externalMailTemplate(key)
}

func validateConsentTemplate(section, key string) error {
	switch section {
	case "external-mail", "native-mail", "native-calendar", "native-contacts", "native-notes", "external-calendar", "external-contacts", "external-notes":
		if !externalMailOAuth(key) {
			return httpx.ErrBadRequest
		}
	case teamsMeetingSection:
		if key != tmplTeamsChat {
			return httpx.ErrBadRequest
		}
	case "external-chat":
		if !externalChatTemplate(key) {
			return httpx.ErrBadRequest
		}
	case "native-storage":
		if !esPlantillaDeAlmacen(key) {
			return httpx.ErrBadRequest.WithMessage("Choose a supported cloud storage provider.")
		}
	case "native-rooms":
		if !esPlantillaDeRooms(key) {
			return httpx.ErrBadRequest.WithMessage("Choose a supported Rooms app.")
		}
	}
	return nil
}

// OAuthStart — POST /api/v1/instances/{id}/oauth/start — arranca el consentimiento.
// Devuelve la URL de autorización en vez de redirigir: quien navega es el NAVEGADOR del
// admin, no esta petición, y devolver la URL deja que el asistente enseñe antes los
// permisos que se van a conceder. Un consentimiento que no se entiende no es
// consentimiento. Escritura ⇒ owner/admin (router).
func (h *Handlers) OAuthStart(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	inst, m, cfg, err := h.loadInstanceManifest(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.plantillaEncendida(ctx, inst.TemplateKey); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if personalConsentTemplate(inst.TemplateKey) {
		claims, _ := authn.FromContext(ctx)
		if err := h.store.RequireExternalChatActor(ctx, id, claims.Subject); err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
	}
	if inst.TemplateKey == tmplWhatsAppBusiness {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Connect WhatsApp with its Embedded Signup flow."))
		return
	}
	if m.OAuth == nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"This app does not offer connection by consent; its credential is configured by hand.",
		))
		return
	}
	flow, err := h.resolveFlow(r.Context(), m, inst.TemplateKey, cfg)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cipher, err := h.cipher()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// El cuerpo es opcional: quien no diga nada vuelve al catálogo de apps.
	var body oauthStartReq
	_ = json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body)
	section := sectionOr(body.ReturnTo)
	if err := validateConsentTemplate(section, inst.TemplateKey); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if body.Purpose == "out" {
		section += seccionSalida
		if len(m.OAuth.WriteScopes) == 0 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
				"This app cannot receive content from Sytue.",
			))
			return
		}
		flow.Scopes = conEscritura(flow.Scopes, m.OAuth.WriteScopes)
	}

	claims, _ := authn.FromContext(ctx)
	st, err := consent.Begin(cipher, flow, inst.OrgID, claims.Subject, inst.ID, h.oauthRedirectURI(), section, time.Now())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// El estado vale UNA vez y solo el del último arranque: OAuthFinish lo consume.
	if err := h.store.MarcarConsentimiento(ctx, inst.ID, st.Challenge); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, oauthStartResp{AuthorizeURL: st.AuthorizeURL, Scopes: st.Scopes})
}

var nativeConsentDestinations = map[string]string{"native-storage": "sytue-drive://connect/callback", "native-personal-storage": "sytue-drive://connect/callback", "native-rooms": "sytue-rooms://connect/callback", "native-mail": "sytue-mail://connect/callback", "native-calendar": "sytue-calendar://connect/callback", "native-contacts": "sytue-contacts://connect/callback", "native-notes": "sytue-notes://connect/callback", "native-apps": "sytue-one://connect/callback"}

func (h *Handlers) consentDestination(section string) string {
	if target := nativeConsentDestinations[section]; target != "" {
		return target
	}
	switch section {
	case "storage":
		if h.drivePublicURL != "" {
			return strings.TrimRight(h.drivePublicURL, "/") + rutaFinEnDrive
		}
	case "personal-storage":
		if h.drivePublicURL != "" {
			return strings.TrimRight(h.drivePublicURL, "/") + rutaFinNubePersonal
		}
	case "external-calendar":
		if h.calendarPublicURL != "" {
			return strings.TrimRight(h.calendarPublicURL, "/") + "/calendar/api/linked-services-finish"
		}
	case "external-contacts":
		if h.contactsPublicURL != "" {
			return strings.TrimRight(h.contactsPublicURL, "/") + "/contacts/api/linked-services-finish"
		}
	case "external-notes":
		if h.notesPublicURL != "" {
			return strings.TrimRight(h.notesPublicURL, "/") + "/notes/api/linked-services-finish"
		}
	case "external-mail":
		if h.mailPublicURL != "" {
			return strings.TrimRight(h.mailPublicURL, "/") + "/mail/api/external-mail-finish"
		}
	case "rooms":
		if h.roomsPublicURL != "" {
			return strings.TrimRight(h.roomsPublicURL, "/") + rutaFinEnRooms
		}
	case "external-chat", teamsMeetingSection:
		if h.roomsPublicURL != "" {
			return strings.TrimRight(h.roomsPublicURL, "/") + "/rooms/api/external-conversations-finish"
		}
	}
	return strings.TrimRight(h.consoleURL, "/") + rutaFinEnLaConsola
}

// OAuthCallback — GET /api/v1/oauth/callback — es donde aterriza el navegador al volver
// del proveedor. NO lleva sesión y por eso NO canjea nada: reenvía el navegador a la
// consola o a Drive según el estado sellado. El BFF —que sí sabe quién tiene
// delante— pide el canje por OAuthFinish.
//
// Antes canjeaba aquí mismo, con el `state` sellado como única autorización. El estado
// no se puede falsificar, pero sí llevar a otro navegador: quien completara una URL de
// autorización ajena dejaba su token del proveedor en la conexión de quien la generó.
func (h *Handlers) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	destination := strings.TrimRight(h.consoleURL, "/") + rutaFinEnLaConsola
	// Solo el estado sellado elige entre los destinos fijos. No se canjea nada
	// aquí: el BFF de destino termina con la sesión de quien inició el flujo.
	if raw := q.Get("state"); raw != "" && len(raw) <= 4096 {
		if cipher, err := h.cipher(); err == nil {
			rd, _ := consent.Callback(cipher, raw, q.Get("code"), q.Get("error"), time.Now())
			section, _ := strings.CutSuffix(rd.Section, seccionSalida)
			destination = h.consentDestination(section)
		}
	}
	u, err := url.Parse(destination)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	paso := url.Values{}
	for _, k := range []string{"state", "code", "error"} {
		// Tope de tamaño: son un estado sellado, un código y un nombre de error.
		if v := q.Get(k); v != "" && len(v) <= 4096 {
			paso.Set(k, v)
		}
	}
	u.RawQuery = paso.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// rutaFinEnLaConsola es el BFF de Sytue Admin que termina el consentimiento con la
// sesión de quien vuelve (apps/console, pages/api/admin/apps/oauth-finish.ts).
const rutaFinEnLaConsola = "/api/admin/apps/oauth-finish"

// La cuenta personal puede conectar su almacenamiento sin acceso a Admin.
const rutaFinEnDrive = "/drive/api/storage-external-finish"

// Las nubes personales vuelven a Drive, a la ubicación recién conectada.
const rutaFinNubePersonal = "/drive/api/personal-storage-finish"

const rutaFinEnRooms = "/rooms/api/app-connections-finish"

type oauthFinishReq struct {
	State string `json:"state"`
	Code  string `json:"code"`
	Error string `json:"error"`
}

// oauthFinishResp es el desenlace para la consola. `error` es un código corto y
// estable (denied|provider|state|exchange); vacío ⇒ conectada.
type oauthFinishResp struct {
	Instance string `json:"instance,omitempty"`
	Section  string `json:"section"`
	Out      bool   `json:"out,omitempty"`
	Error    string `json:"error,omitempty"`
}

// OAuthFinish — POST /api/v1/oauth/finish — canjea el código y guarda los tokens, y
// solo para QUIEN EMPEZÓ: la sesión que llama tiene que ser de la misma persona y la
// misma organización que selló el estado. Las cuentas personales exigen su actor;
// las conexiones organizativas conservan owner/admin.
//
// Responde siempre 200 con el desenlace: que el proveedor diga que no, o que el estado
// haya caducado, no es un fallo de esta llamada sino su resultado.
func (h *Handlers) OAuthFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req oauthFinishReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cipher, err := h.cipher()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rd, err := consent.Callback(cipher, req.State, req.Code, req.Error, time.Now())
	if err != nil {
		// El motivo se registra pero NO vuelve con detalle: a quien manipula un
		// callback no se le explica en qué paso ha fallado.
		log.From(ctx).Warn("integrations: consentimiento rechazado", "error", err)
		out := oauthFinishResp{Section: "apps", Error: "state"}
		// Solo un estado autenticado y la sesión que lo inició deciden la vuelta.
		if esDeQuienEmpezo(ctx, rd) {
			section, salida := strings.CutSuffix(rd.Section, seccionSalida)
			out.Section, out.Out, out.Instance = sectionOr(section), salida, rd.Instance
		}
		switch {
		case errors.Is(err, consent.ErrDenied) && esDeQuienEmpezo(ctx, rd):
			out.Error = "denied"
		case errors.Is(err, consent.ErrProvider) && esDeQuienEmpezo(ctx, rd):
			out.Error = "provider"
		}
		httpx.JSON(w, r, http.StatusOK, out)
		return
	}
	if rd.Section == whatsAppStateSection {
		httpx.JSON(w, r, http.StatusOK, oauthFinishResp{Section: "rooms", Error: "state"})
		return
	}
	section, salida := strings.CutSuffix(rd.Section, seccionSalida)
	out := oauthFinishResp{Section: sectionOr(section), Out: salida}
	if !esDeQuienEmpezo(ctx, rd) {
		log.From(ctx).Warn("integrations: consentimiento terminado por otra sesión que la que lo empezó",
			"instance", rd.Instance)
		out.Error = "state"
		httpx.JSON(w, r, http.StatusOK, out)
		return
	}
	out.Instance = rd.Instance
	if err := h.authorizeOAuthFinish(ctx, rd.Instance); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// Antes del canje y no después: dos llamadas con el mismo estado no canjean dos
	// veces, y un estado de un arranque anterior ya no vale.
	if ok, err := h.store.ConsumirConsentimiento(ctx, rd.Instance, rd.Challenge()); err != nil || !ok {
		log.From(ctx).Warn("integrations: consentimiento ya usado o sustituido por otro arranque",
			"instance", rd.Instance, "error", err)
		out.Error = "state"
		httpx.JSON(w, r, http.StatusOK, out)
		return
	}
	if err := h.redeemAndStore(r, rd); err != nil {
		log.From(ctx).Error("integrations: canje del código de autorización falló",
			"instance", rd.Instance, "error", err)
		out.Error = "exchange"
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// esDeQuienEmpezo dice si la sesión que termina el consentimiento es la que lo empezó:
// misma persona y misma organización que las selladas en el estado.
func esDeQuienEmpezo(ctx context.Context, rd consent.Redeem) bool {
	claims, ok := authn.FromContext(ctx)
	t, err := tenancy.From(ctx)
	return ok && err == nil && rd.Sub != "" && rd.Sub == claims.Subject && rd.Tenant == t.Slug
}

// redeemAndStore canjea el código y guarda los tokens.
//
// El POST sale por el runtime de egress de la instancia, que es el ÚNICO cliente
// autorizado a hablar con terceros: destino SSRF-safe, rate-limit por organización,
// auditoría y política. Pedir un token es tráfico saliente como cualquier otro; abrir
// aquí un http.Client propio habría sido el atajo fácil y una puerta sin vigilar.
//
//nolint:gocyclo // OAuth token exchange and storage pipeline validates state, manifest, egress client, PKCE, token response and secret storage in one secure flow
func (h *Handlers) redeemAndStore(r *http.Request, rd consent.Redeem) error {
	// El tenant sale del ESTADO SELLADO. Quien llama (OAuthFinish) ya comprobó que es
	// el de la sesión, pero se fija desde el estado para que el canje no dependa de
	// esa comprobación: un estado sin organización para aquí, antes del store.
	t, err := tenancy.New(rd.Tenant)
	if err != nil {
		return fmt.Errorf("tenant del estado sellado: %w", err)
	}
	ctx := tenancy.Into(r.Context(), t)
	inst, m, cfg, err := h.loadInstanceManifest(ctx, rd.Instance)
	if err != nil {
		return err
	}
	if err := h.plantillaEncendida(ctx, inst.TemplateKey); err != nil {
		return err
	}
	if m.OAuth == nil {
		return fmt.Errorf("la plantilla %q no declara consentimiento", inst.TemplateKey)
	}
	flow, err := h.resolveFlow(r.Context(), m, inst.TemplateKey, cfg)
	if err != nil {
		return err
	}
	secret := h.clientSecretFor(ctx, inst.TemplateKey, rd.Instance, m.OAuth.TokenURL)

	conn, err := h.loadInstance(ctx, rd.Instance)
	if err != nil {
		return err
	}
	rt := h.egressRuntime(conn, log.From(ctx))

	body := strings.NewReader(rd.Form(flow.ClientID, secret).Encode())
	req, err := http.NewRequestWithContext(policy.SinCredencial(ctx), http.MethodPost, flow.TokenURL, body)
	if err != nil {
		return fmt.Errorf("construir el canje: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := rt.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("canjear el código: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// 64 KiB: una respuesta de token son cientos de bytes; más que eso es un proveedor
	// mal configurado o un intento de agotar memoria.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("leer la respuesta del canje: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// El cuerpo NO se propaga: puede llevar eco de la credencial enviada.
		return fmt.Errorf("el proveedor rechazó el canje con %d", resp.StatusCode)
	}
	var tok tokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return fmt.Errorf("respuesta de token ilegible: %w", err)
	}
	if inst.TemplateKey == tmplSlackChat {
		tok, err = externalSlackUserToken(tok)
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(tok.AccessToken) == "" {
		return errors.New("el proveedor no devolvió access_token")
	}

	// El access token va a la clave BEARER: a partir de aquí los drivers lo usan sin
	// saber de dónde vino. El refresh, a su propia clave, ambos sellados con la KEK.
	secrets := map[string]string{store.SecretKeyBearer: tok.AccessToken}
	if tok.RefreshToken != "" {
		secrets[secretKeyRefresh] = tok.RefreshToken
	}
	if err := applyRoomsConsentGrant(inst.TemplateKey, tok, cfg, secrets); err != nil {
		return err
	}
	if externalChatTemplate(inst.TemplateKey) {
		conn.Secrets[store.SecretKeyBearer] = tok.AccessToken
		if err := h.applyPersonalChatConsent(ctx, conn, tok, cfg, rd.RequestedScopes, flow.Scopes); err != nil {
			return err
		}
	}
	if externalMailOAuth(inst.TemplateKey) {
		conn.Secrets[store.SecretKeyBearer] = tok.AccessToken
		if err := applyMailServicesGrant(inst.TemplateKey, cfg, tok.Scope, rd.RequestedScopes, flow.Scopes, mailConsentService(rd.Section)); err != nil {
			return err
		}
		if err := h.applyMailConsentMetadata(ctx, conn, cfg); err != nil {
			return err
		}
	}

	// El vencimiento va a la config pública para que el asistente pueda enseñarlo. Si
	// el proveedor no lo dice, no se inventa: se deja la clave fuera.
	newStatus := inst.Status
	if newStatus == domain.StatusDraft || newStatus == "" {
		newStatus = domain.StatusActive
	}
	if tok.ExpiresIn > 0 {
		cfg[configKeyExpires] = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	if externalMailOAuth(inst.TemplateKey) {
		return h.storeMailConsent(ctx, inst, cfg, tok, newStatus, secrets, rd.Challenge())
	}
	merged, _ := json.Marshal(cfg)
	if _, err := h.store.CommitInstanceConsent(ctx, rd.Instance, inst.Name, merged, newStatus, inst.LevelEnabled, secrets, rd.Challenge()); err != nil {
		return fmt.Errorf("guardar consentimiento: %w", err)
	}
	return nil
}

// tokenResponse es la respuesta del token endpoint (RFC 6749 §5.1). Solo se lee lo que
// se usa: los proveedores añaden campos propios y `scope` puede venir como string o
// como lista según el proveedor, así que no se toca.
type tokenResponse struct {
	Scope           json.RawMessage    `json:"scope,omitempty"`
	AuthedUser      *slackUserGrant    `json:"authed_user,omitempty"`
	AccessToken     string             `json:"access_token"`
	RefreshToken    string             `json:"refresh_token"`
	ExpiresIn       int64              `json:"expires_in"`
	TokenType       string             `json:"token_type"`
	IncomingWebhook *roomsWebhookGrant `json:"incoming_webhook,omitempty"`
	Team            *roomsTeamGrant    `json:"team,omitempty"`
}

// UnmarshalJSON admite los segundos como número o cadena: ambos formatos los
// devuelven proveedores OAuth. El vencimiento se conserva en el mismo contrato.
func (t *tokenResponse) UnmarshalJSON(body []byte) error {
	var raw struct {
		AuthedUser      *slackUserGrant    `json:"authed_user"`
		AccessToken     string             `json:"access_token"`
		RefreshToken    string             `json:"refresh_token"`
		ExpiresIn       json.Number        `json:"expires_in"`
		TokenType       string             `json:"token_type"`
		IncomingWebhook *roomsWebhookGrant `json:"incoming_webhook"`
		Team            *roomsTeamGrant    `json:"team"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	var seconds int64
	if raw.ExpiresIn != "" {
		var err error
		seconds, err = raw.ExpiresIn.Int64()
		if err != nil {
			return fmt.Errorf("oauth: expires_in inválido: %w", err)
		}
	}
	*t = tokenResponse{AuthedUser: raw.AuthedUser, AccessToken: raw.AccessToken, RefreshToken: raw.RefreshToken, ExpiresIn: seconds, TokenType: raw.TokenType, IncomingWebhook: raw.IncomingWebhook, Team: raw.Team}
	return nil
}

// resolveFlow materializa el flujo del manifiesto con los valores de la instancia
// (plantillas {campo}) y decide con qué client_id se presenta Sytue: el de la app
// registrada por Sytue —el camino de 1 clic— o el que haya traído la organización.
func (h *Handlers) resolveFlow(ctx context.Context, m *manifest.Manifest, templateKey string, cfg map[string]string) (consent.Flow, error) {
	rendered, err := m.RenderOAuth(cfg)
	if err != nil {
		return consent.Flow{}, httpx.ErrBadRequest.WithMessage(
			"The connection is missing details needed to ask for consent: " + err.Error(),
		)
	}
	clientID := h.clienteDePlataforma(ctx, templateKey, m.OAuth.TokenURL).ClientID
	if clientID == "" {
		clientID = cfg[configKeyClientID] // BYO: la organización registró la suya.
	}
	if clientID == "" {
		return consent.Flow{}, httpx.ErrBadRequest.WithMessage(
			"There is no app registered with this provider yet. Register one and supply its client_id, or ask the Sytue operator to configure the suite one.",
		)
	}
	rendered.ClientID = clientID
	return rendered, nil
}

// clienteDePlataforma devuelve la credencial con la que SYTUE se presenta ante el
// proveedor, y solo si la URL a la que va la fija el manifiesto (manifest.HostFijo).
//
// En Salesforce, ServiceNow o Databricks el endpoint cuelga de un campo de la instancia
// («https://{subdominio}.my.salesforce.com/…», «{workspace_url}/oidc/v1/token»). Ese
// campo lo escribe quien administra CUALQUIER organización, y el secreto de la app de
// plataforma es de todas: no puede viajar a un servidor que elija una de ellas. Donde
// el host no es fijo, la conexión va con la app de la propia organización o no va.
func (h *Handlers) clienteDePlataforma(ctx context.Context, templateKey, plantillaDestino string) config.OAuthClient {
	if !manifest.HostFijo(plantillaDestino) || h.oauthClients == nil {
		return config.OAuthClient{}
	}
	return h.oauthClients.get(ctx, templateKey)
}

// clientSecretFor devuelve el secret del cliente OAuth para una llamada a
// `plantillaDestino` (la URL del manifiesto, sin resolver): el de Sytue si lo hay y el
// destino es fijo, si no el que la organización guardó al registrar su app. Vacío es
// válido (cliente público que se apoya solo en PKCE), así que un fallo al leer el vault
// NO se propaga.
func (h *Handlers) clientSecretFor(ctx context.Context, templateKey, instanceID, plantillaDestino string) string {
	if s := h.clienteDePlataforma(ctx, templateKey, plantillaDestino).ClientSecret; s != "" {
		return s
	}
	inst, err := h.store.LoadInstanceWithSecrets(ctx, instanceID)
	if err != nil {
		return ""
	}
	return inst.Secrets[secretKeyClientSc]
}

// cipher devuelve el cifrador con el que se sella el `state`. Sin KEK no se arranca el
// flujo: firmar el estado «a medias» sería peor que no ofrecer el consentimiento.
func (h *Handlers) cipher() (*keycrypto.Cipher, error) {
	if h.oauthCipher == nil || !h.oauthCipher.Enabled() {
		return nil, httpx.NewError(http.StatusServiceUnavailable, "no_kek",
			"Este despliegue no tiene configurada la clave de cifrado, así que no puede conectar por consentimiento.")
	}
	return h.oauthCipher, nil
}

// oauthRedirectURI es la URL que el proveedor tiene registrada. Sale de la config del
// servicio y NUNCA de la petición: derivarla de Host o de X-Forwarded-Host dejaría que
// una cabecera falsificada desviara el código de autorización a otro sitio.
func (h *Handlers) oauthRedirectURI() string {
	return strings.TrimRight(h.publicURL, "/") + "/api/v1/oauth/callback"
}

// revokeAtProvider avisa al proveedor de que el token deja de valer (RFC 7009), para que
// desconectar no sea solo un cambio de estado local.
//
// Es BEST-EFFORT y no bloquea la desconexión, a propósito: si el proveedor está caído o
// no publica endpoint de revocación, lo peor que puede pasar es que el token siga vivo
// AL OTRO LADO hasta que caduque —y aun así aquí ya no queda—. Negarse a desconectar
// porque un tercero no contesta dejaría a la organización con la integración puesta,
// que es peor. Lo que sí se hace es dejar constancia en el log.
//
// Sin revoke_url en el manifiesto no hay nada que llamar: hay proveedores grandes que no
// lo publican (Atlassian, comprobado en su documentación). En ese caso la desconexión es
// necesariamente local y la UI debe decirlo en vez de prometer lo que no hace.
func (h *Handlers) revokeAtProvider(ctx context.Context, id, templateKey string, m *manifest.Manifest, cfg map[string]string) {
	if m.OAuth == nil || strings.TrimSpace(m.OAuth.RevokeURL) == "" {
		return
	}
	revokeURL, err := manifest.RenderURL(m.OAuth.RevokeURL, cfg)
	if err != nil {
		log.From(ctx).Warn("integrations: revoke_url sin resolver, no se revoca", "instance", id, "error", err)
		return
	}
	conn, err := h.loadInstance(ctx, id)
	if err != nil {
		return
	}
	token := conn.Secrets[store.SecretKeyBearer]
	if token == "" {
		return // nada que revocar
	}
	form := url.Values{"token": {token}, "token_type_hint": {"access_token"}}
	if cid := h.clienteDePlataforma(ctx, templateKey, m.OAuth.RevokeURL).ClientID; cid != "" {
		form.Set("client_id", cid)
	} else if cid := cfg[configKeyClientID]; cid != "" {
		form.Set("client_id", cid)
	}
	if sec := h.clientSecretFor(ctx, templateKey, id, m.OAuth.RevokeURL); sec != "" {
		form.Set("client_secret", sec)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Por el mismo egress vigilado que todo lo demás: revocar es tráfico saliente.
	resp, err := h.egressRuntime(conn, log.From(ctx)).HTTP.Do(req)
	if err != nil {
		log.From(ctx).Warn("integrations: no se pudo revocar el token en el proveedor",
			"instance", id, "error", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	// La RFC 7009 §2.2 pide 200 incluso para un token ya inválido; cualquier otra cosa
	// se registra para que quede rastro de que el tercero pudo quedarse el permiso.
	if resp.StatusCode != http.StatusOK {
		log.From(ctx).Warn("integrations: el proveedor no confirmó la revocación",
			"instance", id, "status", resp.StatusCode)
	}
}

// postTokenForm habla con el token endpoint del proveedor para el REFRESCO de
// un token (ver handlers_internal_storage.go). El cuerpo va en el formulario,
// nunca en la URL. ponytail: redeemAndStore hace su propio canje con el mismo
// esquema; unificarlos cuando se toque el canje por otra razón.
func postTokenForm(ctx context.Context, rt connector.Runtime, tokenURL string, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(policy.SinCredencial(ctx), http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, fmt.Errorf("construir el canje: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := rt.HTTP.Do(req)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("canjear el código: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return tokenResponse{}, fmt.Errorf("leer la respuesta del canje: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return tokenResponse{}, fmt.Errorf("el proveedor rechazó el canje con %d", resp.StatusCode)
	}
	var tok tokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return tokenResponse{}, fmt.Errorf("respuesta de token ilegible: %w", err)
	}
	if strings.TrimSpace(tok.AccessToken) == "" {
		return tokenResponse{}, errors.New("el proveedor no devolvió access_token")
	}
	return tok, nil
}
