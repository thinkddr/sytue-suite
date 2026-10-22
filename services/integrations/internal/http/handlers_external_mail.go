// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/linkedservices"
	"git.sytue.eu/sytue/one/services/integrations/internal/mailbox"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

var mailProviderTemplates = map[string]string{
	"gmail": "gmail-mail", "microsoft": "microsoft-mail", "yahoo": "yahoo-mail",
	"imap": "mail-imap", "pop3": "mail-pop3",
}

const (
	mailPasswordKey     = "mail_password"      //nolint:gosec // vault key name, not a credential
	mailSMTPPasswordKey = "mail_smtp_password" //nolint:gosec // vault key name, not a credential
)

func externalMailProvider(key string) string {
	for provider, template := range mailProviderTemplates {
		if template == key {
			return provider
		}
	}
	return ""
}

func externalMailTemplate(key string) bool { return externalMailProvider(key) != "" }
func externalMailOAuth(key string) bool {
	p := externalMailProvider(key)
	return p == "gmail" || p == "microsoft" || p == "yahoo"
}

type mailConfiguration struct {
	Address  string         `json:"address"`
	Incoming mailbox.Server `json:"incoming"`
	Outgoing mailbox.Server `json:"outgoing"`
}

func mailConfig(cfg map[string]string) (mailConfiguration, error) {
	var result mailConfiguration
	if json.Unmarshal([]byte(cfg["_mail_config"]), &result) != nil {
		return result, mailbox.ErrInvalid
	}
	return result, nil
}

func mailCapabilities(provider string) map[string]bool {
	return map[string]bool{"read": true, "send": true, "actions": provider != "pop3", "attachments": true, "continuous_sync": false}
}

func connectedMailCapabilities(provider string, cfg map[string]string) map[string]bool {
	caps := mailCapabilities(provider)
	if !slices.Contains(connectedMailServices(cfg), linkedservices.Mail) {
		for key := range caps {
			caps[key] = false
		}
	}
	return caps
}

// ExternalMailAccounts lists only the caller's connections. Provider readiness
// reflects real configured credentials, never the mere existence of a template.
func (h *Handlers) ExternalMailAccounts(w http.ResponseWriter, r *http.Request) {
	actor, _ := authn.FromContext(r.Context())
	instances, err := h.store.ListExternalChatInstances(r.Context(), actor.Subject)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := []map[string]any{}
	for _, inst := range instances {
		provider := externalMailProvider(inst.TemplateKey)
		if provider == "" {
			continue
		}
		var cfg map[string]string
		_ = json.Unmarshal(inst.Config, &cfg)
		configuration, _ := mailConfig(cfg)
		items = append(items, map[string]any{"instance_id": inst.ID, "provider": provider, "name": inst.Name, "address": configuration.Address, "connected": inst.Status == domain.StatusActive && inst.HasSecrets, "configuration": configuration, "capabilities": connectedMailCapabilities(provider, cfg), "services": connectedMailServices(cfg)})
	}
	providers := []map[string]any{}
	for _, provider := range []string{"gmail", "microsoft", "yahoo", "imap", "pop3"} {
		available := h.oauthCipher.Enabled()
		mode := "password"
		template, templateErr := h.store.GetTemplate(r.Context(), mailProviderTemplates[provider])
		available = available && templateErr == nil && template.Enabled
		if policyErr := h.integracionEncendida(r.Context(), provider); policyErr != nil {
			available = false
		}
		if provider != "imap" && provider != "pop3" {
			mode = "oauth2"
			_, _, err := h.externalChatFlow(r.Context(), mailProviderTemplates[provider])
			available = available && err == nil
		}
		providers = append(providers, map[string]any{"provider": provider, "available": available, "authorization": mode, "services": linkedservices.Offered(provider)})
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items, "providers": providers})
}

func (h *Handlers) ExternalMailConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string   `json:"provider"`
		Instance string   `json:"instance_id,omitempty"`
		ReturnTo string   `json:"return_to,omitempty"`
		Services []string `json:"services"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	key := mailProviderTemplates[req.Provider]
	if !externalMailOAuth(key) || !mailConsentReturn(req.ReturnTo) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if err := h.integracionEncendida(r.Context(), req.Provider); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, flow, err := h.externalChatFlow(r.Context(), key)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	plan, err := linkedservices.ConsentPlan(req.Provider, flow.Scopes, req.Services, mailConsentService(req.ReturnTo))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	flow.Scopes = plan.Scopes
	cipher, err := h.cipher()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := authn.FromContext(r.Context())
	inst, err := h.externalChatInstance(r.Context(), key, name, req.Instance, actor.Subject)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	section := "external-mail"
	if strings.HasPrefix(req.ReturnTo, "native-") || strings.HasPrefix(req.ReturnTo, "external-") {
		section = req.ReturnTo
	}
	start, err := consent.Begin(cipher, flow, tenancy.MustFrom(r.Context()).Slug, actor.Subject, inst.ID, h.oauthRedirectURI(), section, time.Now())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	if err := h.store.MarcarConsentimiento(r.Context(), inst.ID, start.Challenge); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"instance_id": inst.ID, "authorize_url": start.AuthorizeURL, "scopes": start.Scopes})
}

// ExternalMailConfigure is the explicit manual path for secure generic servers.
// Both incoming and SMTP authentication are tested before storing a connection.
func (h *Handlers) requireMailOwner(ctx context.Context, id, actor, template string) error {
	if id == "" {
		return nil
	}
	if _, err := uuid.Parse(id); err != nil {
		return httpx.ErrBadRequest
	}
	if err := h.store.RequireExternalChatActor(ctx, id, actor); err != nil {
		return toAPIError(err)
	}
	existing, err := h.store.GetInstance(ctx, id)
	if err != nil || existing.TemplateKey != template {
		return httpx.ErrNotFound
	}
	return nil
}

type mailConfigureRequest struct {
	Provider string `json:"provider"`
	Instance string `json:"instance_id,omitempty"`
	Name     string `json:"name"`
	mailConfiguration
	Password     string `json:"password"`
	SMTPPassword string `json:"smtp_password,omitempty"`
}

func (r mailConfigureRequest) valid() bool {
	address, err := mail.ParseAddress(r.Address)
	return (r.Provider == "imap" || r.Provider == "pop3") && r.Password != "" && len(r.Password) <= 4096 && len(r.SMTPPassword) <= 4096 && len(r.Name) <= maxNameLen && err == nil && address.Address == r.Address
}

func (h *Handlers) storeMailConsent(ctx context.Context, inst *domain.AppInstance, cfg map[string]string, tok tokenResponse, status domain.Status, secrets map[string]string, challenge string) error {
	secrets[secretKeyRefresh] = tok.RefreshToken
	if tok.ExpiresIn <= 0 {
		delete(cfg, configKeyExpires)
	}
	encoded, _ := json.Marshal(cfg)
	if _, err := h.store.CommitInstanceConsent(ctx, inst.ID, inst.Name, encoded, status, inst.LevelEnabled, secrets, challenge); err != nil {
		return fmt.Errorf("guardar conexión de correo: %w", err)
	}
	return nil
}

func (h *Handlers) ExternalMailConfigure(w http.ResponseWriter, r *http.Request) {
	var req mailConfigureRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !req.valid() {
		failExternalMail(w, r, mailbox.ErrInvalid)
		return
	}
	if err := h.integracionEncendida(r.Context(), req.Provider); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := authn.FromContext(r.Context())
	if err := h.requireMailOwner(r.Context(), req.Instance, actor.Subject, mailProviderTemplates[req.Provider]); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if _, err := h.cipher(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	template, err := h.store.GetTemplate(r.Context(), mailProviderTemplates[req.Provider])
	if err != nil || !template.Enabled {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("external_provider_unavailable"))
		return
	}
	if req.Incoming.Username == "" {
		req.Incoming.Username = req.Address
	}
	if req.Outgoing.Username == "" {
		req.Outgoing.Username = req.Incoming.Username
	}
	if req.SMTPPassword == "" {
		req.SMTPPassword = req.Password
	}
	account := mailbox.Account{Provider: req.Provider, Address: req.Address, Incoming: req.Incoming, Outgoing: req.Outgoing, Password: req.Password, SMTPPassword: req.SMTPPassword}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if _, err := mailbox.NewProtocolClient(account).Test(ctx); err != nil {
		failExternalMail(w, r, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = req.Address
	}
	inst, err := h.externalChatInstance(r.Context(), mailProviderTemplates[req.Provider], name, req.Instance, actor.Subject)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}

	encoded, _ := json.Marshal(req.mailConfiguration)
	cfg, _ := json.Marshal(map[string]string{"_mail_config": string(encoded)})
	if _, err := h.store.CommitInstanceConnection(r.Context(), inst.ID, name, cfg, domain.StatusActive, inst.LevelEnabled, map[string]string{mailPasswordKey: req.Password, mailSMTPPasswordKey: req.SMTPPassword}); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, map[string]string{"instance_id": inst.ID})
}

type connectedMailAccount struct {
	conn    connector.Instance
	account mailbox.Account
}

func (h *Handlers) mailClient(r *http.Request, expected ...string) (mailbox.Client, error) {
	connected, err := h.mailAccount(r, expected...)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(connectedMailServices(connected.conn.Config), linkedservices.Mail) {
		return nil, mailbox.ErrAuth
	}
	return h.mailClientFor(r.Context(), connected.conn, connected.account), nil
}

func (h *Handlers) mailAccount(r *http.Request, expected ...string) (*connectedMailAccount, error) {
	id := chi.URLParam(r, "instance")
	if _, err := uuid.Parse(id); err != nil {
		return nil, httpx.ErrBadRequest
	}
	actor, _ := authn.FromContext(r.Context())
	if err := h.store.RequireExternalChatActor(r.Context(), id, actor.Subject); err != nil {
		return nil, toAPIError(err)
	}
	inst, err := h.store.GetInstance(r.Context(), id)
	if err != nil {
		return nil, toAPIError(err)
	}
	if !externalMailTemplate(inst.TemplateKey) || inst.Status != domain.StatusActive {
		return nil, httpx.ErrNotFound
	}
	if err := h.integracionEncendida(r.Context(), externalMailProvider(inst.TemplateKey)); err != nil {
		return nil, err
	}
	var conn connector.Instance
	if externalMailOAuth(inst.TemplateKey) {
		conn, _, err = h.oauthConnection(r.Context(), id, externalMailOAuth)
		if err != nil && httpx.AsError(err).Status == http.StatusConflict {
			return nil, httpx.ErrConflict.WithCode("external_reconnect_required").WithMessage("Refresh or reconnect this mailbox before trying again.")
		}
	} else {
		conn, err = h.store.LoadInstanceWithSecrets(r.Context(), id)
	}
	if err != nil {
		return nil, toAPIError(err)
	}
	cfg, err := mailConfig(conn.Config)
	if err != nil {
		return nil, err
	}
	expectedAddress := r.URL.Query().Get("expected_address")
	if len(expected) > 0 {
		expectedAddress = expected[0]
	}
	if expectedAddress != "" && !strings.EqualFold(expectedAddress, cfg.Address) {
		return nil, httpx.ErrConflict.WithCode("external_reconnect_required").WithMessage("The connected account changed. Refresh the mailbox before trying again.")
	}
	account := mailbox.Account{Provider: externalMailProvider(inst.TemplateKey), Address: cfg.Address, Incoming: cfg.Incoming, Outgoing: cfg.Outgoing, Password: conn.Secrets[mailPasswordKey], SMTPPassword: conn.Secrets[mailSMTPPasswordKey], AccessToken: conn.Secrets[store.SecretKeyBearer]}
	return &connectedMailAccount{conn: conn, account: account}, nil
}

func (h *Handlers) mailClientFor(ctx context.Context, conn connector.Instance, account mailbox.Account) mailbox.Client {
	if account.Provider == "gmail" || account.Provider == "microsoft" {
		return mailbox.NewCloudClient(account, h.egressRuntime(conn, log.From(ctx)).HTTP)
	}
	return mailbox.NewProtocolClient(account)
}

// applyMailConsentMetadata verifies the provider account and both protocol
// logins (Yahoo) before redeemAndStore marks it connected. Sender address comes
// from the provider profile, never from a client-supplied From field.
func (h *Handlers) applyMailConsentMetadata(ctx context.Context, conn connector.Instance, cfg map[string]string) error {
	provider := externalMailProvider(conn.TemplateKey)
	account := mailbox.Account{Provider: provider, AccessToken: conn.Secrets[store.SecretKeyBearer]}
	if !slices.Contains(connectedMailServices(cfg), linkedservices.Mail) {
		profile, err := mailbox.OAuthProfile(ctx, provider, h.egressRuntime(conn, log.From(ctx)).HTTP)
		if err != nil {
			return err
		}
		encoded, _ := json.Marshal(mailConfiguration{Address: profile.Address})
		cfg["_mail_config"] = string(encoded)
		return nil
	}
	if provider == "yahoo" {
		var profile struct {
			Email string `json:"email"`
		}
		rt := h.egressRuntime(conn, log.From(ctx))
		if err := roomsProviderJSON(ctx, rt, http.MethodGet, "https://api.login.yahoo.com/openid/v1/userinfo", nil, &profile); err != nil {
			return mailbox.ErrAuth
		}
		account.Address = profile.Email
		account.Incoming = mailbox.Server{Host: "imap.mail.yahoo.com", Port: 993, Username: profile.Email, Security: "tls"}
		account.Outgoing = mailbox.Server{Host: "smtp.mail.yahoo.com", Port: 465, Username: profile.Email, Security: "tls"}
	}
	profile, err := h.mailClientFor(ctx, conn, account).Test(ctx)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(mailConfiguration{Address: profile.Address, Incoming: account.Incoming, Outgoing: account.Outgoing})
	cfg["_mail_config"] = string(encoded)
	return nil
}

func (h *Handlers) ExternalMailFolders(w http.ResponseWriter, r *http.Request) {
	client, err := h.mailClient(r)
	if err != nil {
		failExternalMail(w, r, err)
		return
	}
	items, err := client.Folders(r.Context())
	if err != nil {
		failExternalMail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) ExternalMailMessages(w http.ResponseWriter, r *http.Request) {
	client, err := h.mailClient(r)
	if err != nil {
		failExternalMail(w, r, err)
		return
	}
	page, err := client.Messages(r.Context(), r.URL.Query().Get("folder"), r.URL.Query().Get("cursor"))
	if err != nil {
		failExternalMail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, page)
}

func (h *Handlers) ExternalMailMessage(w http.ResponseWriter, r *http.Request) {
	client, err := h.mailClient(r)
	if err != nil {
		failExternalMail(w, r, err)
		return
	}
	message, err := client.Message(r.Context(), r.URL.Query().Get("folder"), r.URL.Query().Get("id"))
	if err != nil {
		failExternalMail(w, r, err)
		return
	}
	message.HTML = "" // Readers use verified plain text; do not duplicate untrusted HTML in the API response.
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, message)
}

func (h *Handlers) ExternalMailSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		mailbox.Outgoing
		ExpectedAddress string `json:"expected_address"`
	}
	if err := httpx.DecodeJSONLimit(w, r, &req, mailbox.MaxMessageBytes*2); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.ExpectedAddress == "" {
		failExternalMail(w, r, mailbox.ErrInvalid)
		return
	}
	client, err := h.mailClient(r, req.ExpectedAddress)
	if err != nil {
		failExternalMail(w, r, err)
		return
	}
	result, err := client.Send(r.Context(), req.Outgoing)
	if err != nil {
		failExternalMail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

type mailActionBody mailbox.Action

func (h *Handlers) ExternalMailAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		mailActionBody
		ExpectedAddress string `json:"expected_address"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.ExpectedAddress == "" {
		failExternalMail(w, r, mailbox.ErrInvalid)
		return
	}
	client, err := h.mailClient(r, req.ExpectedAddress)
	if err != nil {
		failExternalMail(w, r, err)
		return
	}
	err = client.Action(r.Context(), mailbox.Action(req.mailActionBody))
	if errors.Is(err, mailbox.ErrUnsupported) && req.Action == "trash" {
		folders, folderErr := client.Folders(r.Context())
		if folderErr != nil {
			failExternalMail(w, r, folderErr)
			return
		}
		for _, folder := range folders {
			if folder.Role == "trash" && folder.ID != req.FolderID {
				req.Action = "move"
				req.Destination = folder.ID
				err = client.Action(r.Context(), mailbox.Action(req.mailActionBody))
				break
			}
		}
	}
	if err != nil {
		failExternalMail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ExternalMailDisconnect(w http.ResponseWriter, r *http.Request) {
	// No credential refresh is needed to remove a connection. Authorization uses
	// the same actor and tenant checks even if consent was revoked externally.
	id := chi.URLParam(r, "instance")
	actor, _ := authn.FromContext(r.Context())
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if err := h.store.RequireExternalChatActor(r.Context(), id, actor.Subject); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	inst, m, cfg, err := h.loadInstanceManifest(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !externalMailTemplate(inst.TemplateKey) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if externalMailOAuth(inst.TemplateKey) {
		h.revokeAtProvider(r.Context(), id, inst.TemplateKey, m, cfg)
	}
	if err := h.store.DeleteInstance(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func failExternalMail(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusBadGateway, "mail_provider_unavailable", "The mail server could not be reached. Try again later."
	switch {
	case errors.Is(err, mailbox.ErrAuth):
		status, code, message = 422, "mail_auth", "Sign in again or check the account password and server username."
	case errors.Is(err, mailbox.ErrTLS):
		status, code, message = 422, "mail_tls", "The server certificate could not be verified. Check the server name and its TLS configuration."
	case errors.Is(err, mailbox.ErrBlocked):
		status, code, message = 422, "mail_blocked", "Use a publicly reachable mail server. Local and private addresses are not allowed."
	case errors.Is(err, mailbox.ErrInvalid):
		status, code, message = 400, "mail_invalid", "Check the email address, server names, ports and recipients."
	case errors.Is(err, mailbox.ErrTooLarge):
		status, code, message = 413, "mail_too_large", "The message exceeds the mail size limit."
	case errors.Is(err, mailbox.ErrUnsupported):
		status, code, message = 422, "mail_unsupported", "This account does not support that operation."
	case errors.Is(err, mailbox.ErrNotFound):
		status, code, message = 404, "mail_not_found", "The message is no longer available."
	case errors.Is(err, mailbox.ErrSendUnknown):
		status, code, message = 502, "mail_send_unknown", "Check the Sent folder before sending again: the server did not confirm the result."
	default:
		var apiError *httpx.Error
		if errors.As(err, &apiError) {
			httpx.Fail(w, r, apiError)
			return
		}
	}
	httpx.Fail(w, r, httpx.NewError(status, code, message))
}

// Every generic gateway route must preserve the privacy of Mail-owned instances.
func (h *Handlers) mailInstanceAccess(ctx context.Context, inst *domain.AppInstance) error {
	if !externalMailTemplate(inst.TemplateKey) {
		return nil
	}
	actor, _ := authn.FromContext(ctx)
	if err := h.store.RequireExternalChatActor(ctx, inst.ID, actor.Subject); err != nil {
		return toAPIError(err)
	}
	return nil
}
