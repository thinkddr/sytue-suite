// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// Almacenamiento conectado por CONSENTIMIENTO (ADR 0038): Google Drive, OneDrive y Dropbox
// se conectan como cualquier otra app —una instancia de plantilla y su flujo
// OAuth— y este servicio custodia el token. Sytue Drive, que es quien escribe
// los ficheros, lo pide aquí con el secreto interno; si está caducado se
// renueva con el refresh token antes de entregarlo. El token nunca sale por
// ninguna API pública.

const (
	tmplGoogleDrive = "google-drive"
	tmplOneDrive    = "onedrive"
	tmplDropbox     = "dropbox-storage"
	// Margen antes de la caducidad a partir del cual se renueva.
	tokenRenewMargin = 2 * time.Minute
)

func esPlantillaDeAlmacen(key string) bool {
	return key == tmplGoogleDrive || key == tmplOneDrive || key == tmplDropbox
}

// requireInternal autentica a otro servicio de la suite por secreto compartido
// (cabecera X-Sytue-Internal), en tiempo constante.
func requireInternal(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-Sytue-Internal")
			if secret == "" || subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// tenantDe fija en el contexto el tenant que dice la petición interna: no hay
// token de usuario del que sacarlo, y el store filtra por él.
func tenantDe(r *http.Request) (tenancy.Tenant, error) {
	t, err := tenancy.New(r.URL.Query().Get("tenant"))
	if err != nil {
		return tenancy.Tenant{}, httpx.ErrBadRequest.WithMessage("tenant is required")
	}
	return t, nil
}

// InternalStorageInstances — GET /internal/v1/storage/instances?tenant= — las
// instancias de Google Drive, OneDrive y Dropbox del tenant, con si tienen token.
func (h *Handlers) InternalStorageInstances(w http.ResponseWriter, r *http.Request) {
	t, err := tenantDe(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := tenancy.Into(r.Context(), t)
	list, err := h.store.ListInstances(ctx)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	off, err := h.store.DisabledIntegrations(ctx)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := make([]map[string]any, 0, 2)
	providers := map[string]bool{"gdrive": false, "onedrive": false, "dropbox": false}
	kind := map[string]string{tmplGoogleDrive: "gdrive", tmplOneDrive: "onedrive", tmplDropbox: "dropbox"}
	templates, err := h.store.ListTemplates(ctx)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	for _, tpl := range templates {
		if k := kind[tpl.Key]; k != "" {
			providers[k] = !off[proveedorDePlantilla(tpl.Key)] && h.oauthAvailable(ctx, tpl.Key, tpl.Manifest, nil, false)
		}
	}
	for _, inst := range list {
		if !esPlantillaDeAlmacen(inst.TemplateKey) {
			continue
		}
		available := !off[proveedorDePlantilla(inst.TemplateKey)] && h.instanceOAuthAvailable(ctx, inst)
		providers[kind[inst.TemplateKey]] = providers[kind[inst.TemplateKey]] || available
		items = append(items, map[string]any{
			"id": inst.ID, "template_key": inst.TemplateKey, "name": inst.Name,
			"connected":       inst.Status == domain.StatusActive && inst.HasOAuthToken,
			"oauth_available": available,
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items, "providers": providers})
}

// InternalStorageToken — GET /internal/v1/storage/token?tenant=&instance= — el
// access token vigente de la instancia, renovado si caducaba.
func (h *Handlers) InternalStorageToken(w http.ResponseWriter, r *http.Request) {
	t, err := tenantDe(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := tenancy.Into(r.Context(), t)
	conn, expires, err := h.oauthConnection(ctx, r.URL.Query().Get("instance"), esPlantillaDeAlmacen)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"access_token": conn.Secrets[store.SecretKeyBearer], "expires_at": expires, "template_key": conn.TemplateKey})
}

// oauthConnection es el único refresco de los consentimientos que consumen Drive
// y Rooms. La lista de plantillas la fija quien llama, nunca la petición interna.
func (h *Handlers) oauthConnection(ctx context.Context, id string, allowed func(string) bool) (connector.Instance, time.Time, error) {
	inst, m, cfg, err := h.loadInstanceManifest(ctx, id)
	if err != nil {
		return connector.Instance{}, time.Time{}, err
	}
	if err := h.plantillaEncendida(ctx, inst.TemplateKey); err != nil {
		return connector.Instance{}, time.Time{}, err
	}
	if !allowed(inst.TemplateKey) || inst.Status != domain.StatusActive {
		return connector.Instance{}, time.Time{}, httpx.ErrNotFound
	}
	conn, err := h.store.LoadInstanceWithSecrets(ctx, id)
	if err != nil {
		return connector.Instance{}, time.Time{}, toAPIError(err)
	}
	if conn.Secrets[store.SecretKeyBearer] == "" || m.OAuth == nil {
		return connector.Instance{}, time.Time{}, httpx.ErrConflict.WithMessage("The account is not connected: grant consent first.")
	}
	if externalMailOAuth(inst.TemplateKey) || inst.TemplateKey == tmplTeamsChat {
		cfg = conn.Config
	}
	expires, _ := time.Parse(time.RFC3339, cfg[configKeyExpires])
	if expires.IsZero() || time.Until(expires) > tokenRenewMargin {
		return conn, expires, nil
	}
	refresh := conn.Secrets[secretKeyRefresh]
	if refresh == "" {
		return connector.Instance{}, time.Time{}, httpx.ErrConflict.WithMessage("The consent expired: connect the account again.")
	}
	flow, err := h.resolveFlow(ctx, m, inst.TemplateKey, cfg)
	if err != nil {
		return connector.Instance{}, time.Time{}, err
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {flow.ClientID}}
	if secret := h.clientSecretFor(ctx, inst.TemplateKey, id, m.OAuth.TokenURL); secret != "" {
		form.Set("client_secret", secret)
	}
	tok, err := postTokenForm(ctx, h.egressRuntime(conn, log.From(ctx)), flow.TokenURL, form)
	if err != nil {
		return connector.Instance{}, time.Time{}, httpx.ErrUnavailable.WithMessage("The provider did not renew the account connection.")
	}
	if inst.TemplateKey == tmplSlackChat && tok.TokenType != "user" {
		return connector.Instance{}, time.Time{}, httpx.ErrConflict.WithCode("external_reconnect_required").WithMessage("Connect this personal account again.")
	}
	return h.saveOAuthRefresh(ctx, inst, conn, cfg, tok)
}

func (h *Handlers) saveOAuthRefresh(ctx context.Context, inst *domain.AppInstance, conn connector.Instance, cfg map[string]string, tok tokenResponse) (connector.Instance, time.Time, error) {
	if externalMailOAuth(inst.TemplateKey) {
		return h.saveMailOAuthRefresh(ctx, inst, conn, tok)
	}
	if inst.TemplateKey == tmplTeamsChat {
		patch, err := narrowedTeamsServices(conn.Config, tok.Scope)
		if err != nil {
			return connector.Instance{}, time.Time{}, err
		}
		return h.savePersonalOAuthRefresh(ctx, inst, conn, tok, patch)
	}
	expires, _ := time.Parse(time.RFC3339, cfg[configKeyExpires])
	secrets := map[string]string{store.SecretKeyBearer: tok.AccessToken}
	if tok.RefreshToken != "" {
		secrets[secretKeyRefresh] = tok.RefreshToken
	}
	if err := h.store.MergeSecrets(ctx, inst.ID, secrets); err != nil {
		return connector.Instance{}, time.Time{}, toAPIError(err)
	}
	for key, value := range secrets {
		conn.Secrets[key] = value
	}
	if tok.ExpiresIn > 0 {
		expires = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).UTC()
		cfg[configKeyExpires] = expires.Format(time.RFC3339)
		conn.Config = cfg
		merged, _ := json.Marshal(cfg)
		if _, err := h.store.UpdateInstance(ctx, inst.ID, inst.Name, merged, inst.Status, inst.LevelEnabled); err != nil {
			log.From(ctx).Warn("integrations: no se pudo anotar el vencimiento del token renovado", "instance", inst.ID, "error", err)
		}
	}
	return conn, expires, nil
}

func (h *Handlers) saveMailOAuthRefresh(ctx context.Context, inst *domain.AppInstance, conn connector.Instance, tok tokenResponse) (connector.Instance, time.Time, error) {
	patch, err := narrowedMailServices(externalMailProvider(inst.TemplateKey), conn.Config, tok.Scope)
	if err != nil {
		return connector.Instance{}, time.Time{}, err
	}
	return h.savePersonalOAuthRefresh(ctx, inst, conn, tok, patch)
}

// Personal refreshes preserve the atomic configuration/credential pair and reject
// a response that lost the CAS to reconnection, including when the actor is unchanged.
func (h *Handlers) savePersonalOAuthRefresh(ctx context.Context, inst *domain.AppInstance, conn connector.Instance, tok tokenResponse, patch map[string]string) (connector.Instance, time.Time, error) {
	patch[configKeyExpires] = ""
	if tok.ExpiresIn > 0 {
		patch[configKeyExpires] = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	secrets := map[string]string{store.SecretKeyBearer: tok.AccessToken}
	if tok.RefreshToken != "" {
		secrets[secretKeyRefresh] = tok.RefreshToken
	}
	applied, err := h.store.RefreshInstanceConnection(ctx, inst.ID, conn.Secrets[store.SecretKeyBearer], patch, secrets)
	if err != nil {
		return connector.Instance{}, time.Time{}, toAPIError(err)
	}
	if !applied {
		return connector.Instance{}, time.Time{}, httpx.ErrConflict.WithCode("external_reconnect_required").WithMessage("The account changed while this operation was in progress. Refresh the connected account before trying again.")
	}
	for key, value := range patch {
		conn.Config[key] = value
	}
	for key, value := range secrets {
		conn.Secrets[key] = value
	}
	expires, _ := time.Parse(time.RFC3339, conn.Config[configKeyExpires])
	return conn, expires, nil
}
