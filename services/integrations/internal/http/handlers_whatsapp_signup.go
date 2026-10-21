// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

const (
	tmplWhatsAppBusiness = "whatsapp-business"
	whatsAppGraphOrigin  = "https://graph.facebook.com"
	whatsAppStateSection = "rooms-whatsapp"
	secretWAAccounts     = "whatsapp_accounts"   //nolint:gosec // clave del vault
	secretWARegistered   = "whatsapp_registered" //nolint:gosec // clave del vault
	secretWAAppSecret    = "whatsapp_app_secret" //nolint:gosec // clave del vault, no una credencial
)

var (
	whatsAppGraphVersion = regexp.MustCompile(`^v\d{1,3}\.\d{1,2}$`)
	whatsAppIdentifier   = regexp.MustCompile(`^\d{1,32}$`)
)

func (h *Handlers) whatsAppPlatform(ctx context.Context) (config.OAuthClient, string, bool) {
	version := h.whatsAppGraphVersion
	if version == "" {
		version = "v26.0"
	}
	if h.oauthClients == nil || !whatsAppGraphVersion.MatchString(version) {
		return config.OAuthClient{}, "", false
	}
	client := h.oauthClients.get(ctx, tmplWhatsAppBusiness)
	return client, whatsAppGraphOrigin + "/" + version, h.whatsAppConfigID != "" && h.whatsAppWebhookVerifyToken != "" && client.ClientID != "" && client.ClientSecret != "" && h.oauthCipher != nil && h.oauthCipher.Enabled()
}

// WhatsAppSignupInfo expone solo configuración pública del SDK; nunca credenciales.
func (h *Handlers) WhatsAppSignupInfo(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("state") {
		if err := h.previewWhatsAppState(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("instance")); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	client, base, available := h.whatsAppPlatform(r.Context())
	out := map[string]any{"available": available}
	if available {
		out["app_id"], out["config_id"], out["graph_version"] = client.ClientID, h.whatsAppConfigID, strings.TrimPrefix(base, whatsAppGraphOrigin+"/")
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// previewWhatsAppState permite al navegador comprobar la sesión ANTES de abrir
// Meta. Leer la configuración del SDK no gasta el nonce de un solo uso.
func (h *Handlers) previewWhatsAppState(ctx context.Context, state, instance string) error {
	cipher, err := h.cipher()
	if err != nil {
		return err
	}
	if len(state) > 4096 || state == "" || instance == "" {
		return httpx.ErrForbidden
	}
	rd, err := consent.Callback(cipher, state, "preview", "", time.Now())
	if err != nil || rd.Section != whatsAppStateSection || rd.Instance != instance || !esDeQuienEmpezo(ctx, rd) {
		return httpx.ErrForbidden.WithMessage("This connection belongs to another session or has expired. Return to the app and start again.")
	}
	inst, err := h.store.GetInstance(ctx, instance)
	if err != nil || inst.TemplateKey != tmplWhatsAppBusiness {
		return httpx.ErrForbidden
	}
	if inst.ConfigMap()[store.ConfigKeyConsentimiento] != rd.Challenge() {
		return httpx.ErrForbidden
	}
	return nil
}

// WhatsAppSignupStart crea un consentimiento ligado a la sesión antes de abrir el SDK.
func (h *Handlers) WhatsAppSignupStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	client, base, available := h.whatsAppPlatform(ctx)
	if !available {
		httpx.Fail(w, r, whatsAppNotConfigured())
		return
	}
	cipher, err := h.cipher()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req struct {
		Instance string `json:"instance_id"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	inst, err := h.whatsAppDraft(ctx, req.Instance)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	claims, _ := authn.FromContext(ctx)
	state, nonce, err := consent.BeginEmbedded(cipher, inst.OrgID, claims.Subject, inst.ID, whatsAppStateSection, time.Now())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	if err := h.store.MarcarConsentimiento(ctx, inst.ID, nonce); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]string{"instance_id": inst.ID, "state": state, "app_id": client.ClientID, "config_id": h.whatsAppConfigID, "graph_version": strings.TrimPrefix(base, whatsAppGraphOrigin+"/")})
}

func whatsAppNotConfigured() error {
	return httpx.NewError(http.StatusServiceUnavailable, "whatsapp_not_configured", "WhatsApp Business is not configured for this installation yet.")
}

func (h *Handlers) whatsAppDraft(ctx context.Context, id string) (*domain.AppInstance, error) {
	if id != "" {
		inst, err := h.store.GetInstance(ctx, id)
		if err != nil {
			return nil, toAPIError(err)
		}
		if inst.TemplateKey != tmplWhatsAppBusiness {
			return nil, httpx.ErrBadRequest.WithMessage("Choose a WhatsApp Business connection.")
		}
		return inst, nil
	}
	// Verificar que el operador no haya retirado esta plantilla.
	if _, err := h.store.GetTemplate(ctx, tmplWhatsAppBusiness); err != nil {
		return nil, toAPIError(err)
	}
	inst, err := h.store.CreateInstance(ctx, tmplWhatsAppBusiness, "WhatsApp Business", json.RawMessage(`{}`), nil)
	if err != nil {
		return nil, toAPIError(err)
	}
	return inst, nil
}

type whatsAppFinishRequest struct {
	State string `json:"state"`
	Code  string `json:"code"`
	WABA  string `json:"waba_id"`
	Phone string `json:"phone_number_id"`
	Flow  string `json:"flow"`
}

// WhatsAppSignupFinish canjea el código de 30 s inmediatamente. Los IDs del SDK
// son solo una selección: los WABA autorizados se comprueban con Graph en servidor.
func (h *Handlers) WhatsAppSignupFinish(w http.ResponseWriter, r *http.Request) {
	var req whatsAppFinishRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := h.finishWhatsAppSignup(r.Context(), req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}

type whatsAppFinishResult struct {
	InstanceID string           `json:"instance_id"`
	Numbers    []whatsAppNumber `json:"numbers"`
}

func (h *Handlers) consumeWhatsAppState(ctx context.Context, req whatsAppFinishRequest) (*domain.AppInstance, error) {
	cipher, err := h.cipher()
	if err != nil {
		return nil, err
	}
	rd, err := consent.Callback(cipher, req.State, req.Code, "", time.Now())
	if err != nil || rd.Section != whatsAppStateSection || !esDeQuienEmpezo(ctx, rd) {
		return nil, httpx.ErrBadRequest.WithMessage("The connection has expired. Start again.")
	}
	inst, err := h.store.GetInstance(ctx, rd.Instance)
	if err != nil || inst.TemplateKey != tmplWhatsAppBusiness {
		return nil, httpx.ErrNotFound
	}
	if ok, err := h.store.ConsumirConsentimiento(ctx, rd.Instance, rd.Challenge()); err != nil || !ok {
		return nil, httpx.ErrBadRequest.WithMessage("The connection has expired. Start again.")
	}
	return inst, nil
}

func (h *Handlers) finishWhatsAppSignup(ctx context.Context, req whatsAppFinishRequest) (whatsAppFinishResult, error) {
	client, base, available := h.whatsAppPlatform(ctx)
	if !available {
		return whatsAppFinishResult{}, whatsAppNotConfigured()
	}
	if len(req.Code) > 8192 || len(req.State) > 4096 {
		return whatsAppFinishResult{}, httpx.ErrBadRequest
	}
	inst, err := h.consumeWhatsAppState(ctx, req)
	if err != nil {
		return whatsAppFinishResult{}, err
	}
	conn := connector.Instance{ID: inst.ID, OrgID: inst.OrgID, TemplateKey: tmplWhatsAppBusiness, Secrets: map[string]string{}}
	tok, err := exchangeWhatsAppCode(ctx, h.egressRuntime(conn, log.From(ctx)), base, client, req.Code)
	if err != nil {
		return whatsAppFinishResult{}, err
	}
	accounts, expires, err := h.whatsAppGrantedAccounts(ctx, conn, base, client, tok.AccessToken)
	if err != nil {
		return whatsAppFinishResult{}, err
	}
	if req.WABA != "" && !slices.Contains(accounts, req.WABA) {
		return whatsAppFinishResult{}, httpx.ErrForbidden
	}
	encoded, _ := json.Marshal(accounts)
	conn.Secrets = map[string]string{store.SecretKeyBearer: tok.AccessToken, secretWAAccounts: string(encoded), secretWAAppSecret: client.ClientSecret, secretWARegistered: "[]"}
	numbers, err := h.whatsAppNumbers(ctx, conn)
	if err != nil {
		return whatsAppFinishResult{}, err
	}
	if req.Phone != "" && !slices.ContainsFunc(numbers, func(n whatsAppNumber) bool { return n.ID == req.Phone }) {
		return whatsAppFinishResult{}, httpx.ErrForbidden
	}
	if err := h.saveWhatsAppGrant(ctx, inst, conn, tok, expires); err != nil {
		return whatsAppFinishResult{}, err
	}
	return whatsAppFinishResult{InstanceID: inst.ID, Numbers: numbers}, nil
}

func exchangeWhatsAppCode(ctx context.Context, rt connector.Runtime, base string, client config.OAuthClient, code string) (tokenResponse, error) {
	q := url.Values{"client_id": {client.ClientID}, "client_secret": {client.ClientSecret}, "code": {code}}
	var tok tokenResponse
	if err := roomsProviderJSON(ctx, rt, http.MethodGet, base+"/oauth/access_token?"+q.Encode(), nil, &tok); err != nil {
		return tokenResponse{}, err
	}
	if tok.AccessToken == "" {
		return tokenResponse{}, httpx.NewError(http.StatusBadGateway, "whatsapp_grant_rejected", "Meta did not authorize this account.")
	}
	return tok, nil
}

func (h *Handlers) saveWhatsAppGrant(ctx context.Context, inst *domain.AppInstance, conn connector.Instance, tok tokenResponse, expires time.Time) error {
	cfg := inst.ConfigMap()
	if cfg == nil {
		cfg = map[string]string{}
	}
	delete(cfg, configKeyExpires)
	delete(cfg, store.ConfigKeyConsentimiento)
	if tok.ExpiresIn > 0 {
		expires = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).UTC()
	}
	if !expires.IsZero() {
		cfg[configKeyExpires] = expires.Format(time.RFC3339)
	}
	public, _ := json.Marshal(cfg)
	if _, err := h.store.CommitInstanceConsent(ctx, inst.ID, inst.Name, public, domain.StatusActive, inst.LevelEnabled, conn.Secrets, cfg[store.ConfigKeyConsentGeneration]); err != nil {
		return toAPIError(err)
	}
	return nil
}

func (h *Handlers) whatsAppGrantedAccounts(ctx context.Context, conn connector.Instance, base string, client config.OAuthClient, token string) ([]string, time.Time, error) {
	// Graph permite el app access token para inspeccionar un token de su app. El
	// bearer del cliente nunca se manda como credencial de esta inspección.
	conn.Secrets = map[string]string{store.SecretKeyBearer: client.ClientID + "|" + client.ClientSecret}
	var out struct {
		Data whatsAppDebugGrant `json:"data"`
	}
	q := url.Values{"input_token": {token}}
	if err := roomsProviderJSON(ctx, h.egressRuntime(conn, log.From(ctx)), http.MethodGet, base+"/debug_token?"+q.Encode(), nil, &out); err != nil {
		return nil, time.Time{}, err
	}
	return validateWhatsAppGrant(out.Data, client.ClientID)
}

type whatsAppDebugGrant struct {
	AppID   string   `json:"app_id"`
	Valid   bool     `json:"is_valid"`
	Expires int64    `json:"expires_at"`
	Scopes  []string `json:"scopes"`
	Grants  []struct {
		Scope   string   `json:"scope"`
		Targets []string `json:"target_ids"`
	} `json:"granular_scopes"`
}

func validateWhatsAppGrant(grant whatsAppDebugGrant, appID string) ([]string, time.Time, error) {
	if !grant.Valid || grant.AppID != appID || !slices.Contains(grant.Scopes, "whatsapp_business_management") || !slices.Contains(grant.Scopes, "whatsapp_business_messaging") {
		return nil, time.Time{}, httpx.ErrForbidden
	}
	if grant.Expires > 0 && grant.Expires <= time.Now().Unix() {
		return nil, time.Time{}, httpx.ErrConflict.WithMessage("The consent expired. Connect WhatsApp Business again.")
	}
	accounts := make([]string, 0, 8)
	for _, scope := range grant.Grants {
		if scope.Scope != "whatsapp_business_management" {
			continue
		}
		for _, id := range scope.Targets {
			if whatsAppIdentifier.MatchString(id) && !slices.Contains(accounts, id) {
				accounts = append(accounts, id)
			}
		}
	}
	if len(accounts) == 0 || len(accounts) > 25 {
		return nil, time.Time{}, httpx.ErrConflict.WithMessage("Meta did not share a WhatsApp Business account. Connect again and choose one.")
	}
	var expires time.Time
	if grant.Expires > 0 {
		expires = time.Unix(grant.Expires, 0).UTC()
	}
	return accounts, expires, nil
}

type whatsAppNumber struct {
	ID                   string `json:"id"`
	DisplayPhone         string `json:"display_phone"`
	VerifiedName         string `json:"verified_name"`
	RegistrationRequired bool   `json:"registration_required"`
	waba                 string
}

// WhatsAppNumbers devuelve únicamente números que Graph permite usar con el grant.
func (h *Handlers) WhatsAppNumbers(w http.ResponseWriter, r *http.Request) {
	conn, _, err := h.oauthConnection(r.Context(), r.URL.Query().Get("instance"), func(key string) bool { return key == tmplWhatsAppBusiness })
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	numbers, err := h.whatsAppNumbers(r.Context(), conn)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"instance_id": conn.ID, "numbers": numbers})
}

func (h *Handlers) whatsAppNumbers(ctx context.Context, conn connector.Instance) ([]whatsAppNumber, error) {
	_, base, ok := h.whatsAppPlatform(ctx)
	if !ok {
		return nil, whatsAppNotConfigured()
	}
	var accounts, registered []string
	if json.Unmarshal([]byte(conn.Secrets[secretWAAccounts]), &accounts) != nil || len(accounts) == 0 || len(accounts) > 25 {
		return nil, httpx.ErrConflict.WithMessage("Connect WhatsApp Business again.")
	}
	_ = json.Unmarshal([]byte(conn.Secrets[secretWARegistered]), &registered)
	items := make([]whatsAppNumber, 0, 20)
	for _, waba := range accounts {
		if !whatsAppIdentifier.MatchString(waba) {
			return nil, httpx.ErrForbidden
		}
		numbers, err := listWhatsAppNumbers(ctx, h.egressRuntime(conn, log.From(ctx)), base, waba, registered)
		if err != nil {
			return nil, err
		}
		items = append(items, numbers...)
		if len(items) > 500 {
			return nil, httpx.ErrConflict.WithMessage("Select a smaller set of WhatsApp Business accounts.")
		}
	}
	return items, nil
}

func listWhatsAppNumbers(ctx context.Context, rt connector.Runtime, base, waba string, registered []string) ([]whatsAppNumber, error) {
	items := make([]whatsAppNumber, 0, 20)
	after := ""
	for page := 0; page < 10; page++ {
		var out struct {
			Data []struct {
				ID      string `json:"id"`
				Display string `json:"display_phone_number"`
				Name    string `json:"verified_name"`
				Status  string `json:"status"`
			} `json:"data"`
			Paging struct {
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
				Next string `json:"next"`
			} `json:"paging"`
		}
		q := url.Values{"fields": {"id,display_phone_number,verified_name,status"}, "limit": {"100"}}
		if after != "" {
			q.Set("after", after)
		}
		if err := roomsProviderJSON(ctx, rt, http.MethodGet, base+"/"+waba+"/phone_numbers?"+q.Encode(), nil, &out); err != nil {
			return nil, err
		}
		for _, n := range out.Data {
			if whatsAppIdentifier.MatchString(n.ID) {
				items = append(items, whatsAppNumber{ID: n.ID, DisplayPhone: n.Display, VerifiedName: n.Name, RegistrationRequired: n.Status != "CONNECTED" && !slices.Contains(registered, n.ID), waba: waba})
			}
		}
		if out.Paging.Next == "" || out.Paging.Cursors.After == "" || out.Paging.Cursors.After == after {
			break
		}
		after = out.Paging.Cursors.After
	}
	return items, nil
}

func whatsAppTenant(ctx context.Context, tenant string) (context.Context, error) {
	t, err := tenancy.New(tenant)
	if err != nil {
		return nil, httpx.ErrBadRequest.WithMessage("tenant is required")
	}
	return tenancy.Into(ctx, t), nil
}
