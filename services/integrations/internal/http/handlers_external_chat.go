// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
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
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

const (
	tmplSlackChat           = "slack-chat"
	tmplTeamsChat           = "microsoft-teams-chat"
	tmplGoogleConversations = "google-chat-conversations"
)

func externalChatTemplate(key string) bool {
	return key == tmplSlackChat || key == tmplTeamsChat || key == tmplGoogleConversations
}

func externalChatProvider(key string) string {
	switch key {
	case tmplSlackChat:
		return "slack"
	case tmplTeamsChat:
		return "microsoft-teams"
	case tmplGoogleConversations:
		return "google-chat"
	default:
		return ""
	}
}

func externalChatKey(provider string) string {
	switch provider {
	case "slack":
		return tmplSlackChat
	case "microsoft-teams":
		return tmplTeamsChat
	case "google-chat":
		return tmplGoogleConversations
	default:
		return ""
	}
}

func (h *Handlers) authorizeOAuthFinish(ctx context.Context, id string) error {
	inst, err := h.store.GetInstance(ctx, id)
	if err != nil {
		return err
	}
	claims, _ := authn.FromContext(ctx)
	// Las conexiones PERSONALES (chats externos, nubes, reuniones) las termina quien
	// las empezó, tenga o no rol de admin; las de la organización, solo un admin.
	if h.store.RequireExternalChatActor(ctx, id, claims.Subject) == nil {
		return nil
	}
	if externalChatTemplate(inst.TemplateKey) {
		return httpx.ErrForbidden
	}
	if !esAdmin(claims) {
		return httpx.ErrForbidden
	}
	return nil
}

func (h *Handlers) externalChatConnection(r *http.Request) (connector.Instance, connector.Runtime, error) {
	id := chi.URLParam(r, "instance")
	if _, err := uuid.Parse(id); err != nil {
		return connector.Instance{}, connector.Runtime{}, httpx.ErrBadRequest
	}
	actor, _ := authn.FromContext(r.Context())
	if err := h.store.RequireExternalChatActor(r.Context(), id, actor.Subject); err != nil {
		return connector.Instance{}, connector.Runtime{}, toAPIError(err)
	}
	conn, _, err := h.oauthConnection(r.Context(), id, externalChatTemplate)
	if err != nil {
		return connector.Instance{}, connector.Runtime{}, err
	}
	if conn.TemplateKey == tmplTeamsChat && !teamsServiceEnabled(conn.Config, teamsChatService) {
		return connector.Instance{}, connector.Runtime{}, httpx.ErrConflict.WithCode("external_chat_not_granted")
	}
	if err := h.integracionEncendida(r.Context(), externalChatProvider(conn.TemplateKey)); err != nil {
		return connector.Instance{}, connector.Runtime{}, err
	}
	return conn, h.egressRuntime(conn, log.From(r.Context())), nil
}

func externalCapabilities(provider, remote string) map[string]bool {
	// Google Chat solo concede chat.messages.create: leer, editar y borrar piden
	// chat.messages, un permiso restringido que exige auditoría de seguridad externa.
	readable := provider != "google-chat"
	editable := readable && (provider != "microsoft-teams" || strings.HasPrefix(remote, "chat/"))
	return map[string]bool{"read": readable, "send": true, "threads": provider != "microsoft-teams" || !strings.HasPrefix(remote, "chat/"), "edit": editable, "delete": editable, "attachments": false, "e2ee": false}
}

func (h *Handlers) ExternalChatAccounts(w http.ResponseWriter, r *http.Request) {
	claims, _ := authn.FromContext(r.Context())
	instances, err := h.store.ListExternalChatInstances(r.Context(), claims.Subject)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := externalChatAccountSummaries(instances)
	off, err := h.store.DisabledIntegrations(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	providers := []map[string]any{}
	for _, key := range []string{tmplSlackChat, tmplTeamsChat, tmplGoogleConversations} {
		template, err := h.store.GetTemplate(r.Context(), key)
		available := false
		if err == nil && template.Enabled {
			if m, err := manifest.Parse(template.Manifest); err == nil && m.OAuth != nil {
				client := h.clienteDePlataforma(r.Context(), key, m.OAuth.TokenURL)
				available = client.ClientID != "" && client.ClientSecret != "" && h.oauthCipher.Enabled()
			}
		}
		providers = append(providers, map[string]any{"provider": externalChatProvider(key), "available": available && !off[externalChatProvider(key)], "disabled_by_org": off[externalChatProvider(key)], "capabilities": externalCapabilities(externalChatProvider(key), "")})
	}
	meetings := h.externalMeetingAccounts(r.Context(), instances, off)
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items, "providers": providers, "meetings": meetings})
}

func externalChatAccountSummaries(instances []domain.AppInstance) []map[string]any {
	items := []map[string]any{}
	for _, inst := range instances {
		if externalChatTemplate(inst.TemplateKey) {
			var cfg map[string]string
			_ = json.Unmarshal(inst.Config, &cfg)
			if inst.TemplateKey == tmplTeamsChat && !teamsServiceEnabled(cfg, teamsChatService) {
				continue
			}
			items = append(items, map[string]any{"instance_id": inst.ID, "provider": externalChatProvider(inst.TemplateKey), "name": inst.Name, "connected": inst.Status == domain.StatusActive && inst.HasSecrets, "mode": "external-chat", "workspace_name": cfg[roomsTeamName], "account_id": cfg["_external_user_id"]})
		}
	}
	return items
}

func (h *Handlers) externalMeetingAccounts(ctx context.Context, instances []domain.AppInstance, off map[string]bool) []map[string]any {
	_, _, flowErr := h.externalChatFlow(ctx, tmplGoogleMeet)
	meetAvailable := flowErr == nil && h.oauthCipher.Enabled() && !off["google-meet"]
	meetings := []map[string]any{}
	for _, inst := range instances {
		if inst.TemplateKey == tmplGoogleMeet {
			meetings = append(meetings, map[string]any{
				"provider": "google-meet", "disabled_by_org": off["google-meet"], "available": meetAvailable,
				"instance_id": inst.ID, "name": inst.Name, "connected": inst.Status == domain.StatusActive && inst.HasSecrets,
			})
		}
	}
	if len(meetings) == 0 {
		meetings = append(meetings, map[string]any{"provider": "google-meet", "disabled_by_org": off["google-meet"], "available": meetAvailable})
	}
	_, _, teamsFlowErr := h.externalChatFlow(ctx, tmplTeamsChat)
	teamsAvailable := teamsFlowErr == nil && h.oauthCipher.Enabled() && !off["microsoft-teams"]
	meetings = append(meetings, teamsMeetingAccounts(instances, teamsAvailable, off["microsoft-teams"])...)
	return meetings
}

func teamsMeetingAccounts(instances []domain.AppInstance, available, disabled bool) []map[string]any {
	meetings := []map[string]any{}
	for _, inst := range instances {
		if inst.TemplateKey != tmplTeamsChat {
			continue
		}
		var cfg map[string]string
		_ = json.Unmarshal(inst.Config, &cfg)
		meetings = append(meetings, map[string]any{
			"provider": "microsoft-teams", "disabled_by_org": disabled, "available": available,
			"instance_id": inst.ID, "name": inst.Name, "account_id": cfg["_external_user_id"], "e2ee_required": true,
			"connected": inst.Status == domain.StatusActive && inst.HasSecrets && teamsServiceEnabled(cfg, teamsMeetService),
		})
	}
	if len(meetings) == 0 {
		meetings = append(meetings, map[string]any{
			"provider": "microsoft-teams", "available": available,
			"disabled_by_org": disabled, "e2ee_required": true,
		})
	}
	return meetings
}

// ExternalChatMeeting — POST /api/v1/external-conversations/meeting — crea una reunión
// de Google Meet con la cuenta PERSONAL de quien la pide, sin pasar por Admin. Devuelve
// el enlace; Rooms no lo mete en ninguna conversación por su cuenta.
func (h *Handlers) ExternalChatMeeting(w http.ResponseWriter, r *http.Request) {
	var req externalMeetingRequest
	// Older clients sent no account. They remain valid when exactly one personal
	// account is connected; selecting the first of several would use the wrong one.
	if r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if req.Provider == "microsoft-teams" {
		h.externalTeamsMeeting(w, r, req)
		return
	}
	if req.Provider != "" && req.Provider != "google-meet" || r.Method != http.MethodPost || req.ID != "" || req.E2EE != nil && *req.E2EE {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if req.Instance != "" {
		if _, err := uuid.Parse(req.Instance); err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
	}
	if err := h.integracionEncendida(r.Context(), "google-meet"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := authn.FromContext(r.Context())
	instances, err := h.store.ListExternalChatInstances(r.Context(), actor.Subject)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	id, err := personalMeetAccount(instances, req.Instance)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	conn, _, err := h.oauthConnection(r.Context(), id, func(k string) bool { return k == tmplGoogleMeet })
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	u, err := createRoomsMeeting(r.Context(), h.egressRuntime(conn, log.From(r.Context())))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]string{"url": u})
}

func (h *Handlers) externalTeamsMeeting(w http.ResponseWriter, r *http.Request, req externalMeetingRequest) {
	if req.Instance == "" || (r.Method == http.MethodPatch) != (req.ID != "") {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if _, err := uuid.Parse(req.Instance); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if err := h.integracionEncendida(r.Context(), "microsoft-teams"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := authn.FromContext(r.Context())
	if err := h.store.RequireExternalChatActor(r.Context(), req.Instance, actor.Subject); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	conn, _, err := h.oauthConnection(r.Context(), req.Instance, func(k string) bool { return k == tmplTeamsChat })
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !teamsServiceEnabled(conn.Config, teamsMeetService) || conn.Config["_external_user_id"] != req.Account {
		httpx.Fail(w, r, httpx.ErrConflict.WithCode("external_meeting_not_granted"))
		return
	}
	result, err := protectedTeamsMeeting(r.Context(), h.egressRuntime(conn, log.From(r.Context())), req, time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	status := http.StatusCreated
	if r.Method == http.MethodPatch {
		status = http.StatusOK
	}
	httpx.JSON(w, r, status, result)
}

// instances has already been filtered by tenant and the authenticated owner.
func personalMeetAccount(instances []domain.AppInstance, requested string) (string, error) {
	id := ""
	for _, inst := range instances {
		if inst.TemplateKey != tmplGoogleMeet || (requested != "" && inst.ID != requested) {
			continue
		}
		if inst.Status != domain.StatusActive || !inst.HasSecrets {
			if requested != "" {
				return "", httpx.ErrConflict.WithCode("meeting_not_connected").WithMessage("Connect your Google account first.")
			}
			continue
		}
		if id != "" {
			return "", httpx.ErrConflict.WithCode("meeting_account_required").WithMessage("Choose the Google account for this meeting.")
		}
		id = inst.ID
	}
	if id != "" {
		return id, nil
	}
	if requested != "" {
		return "", httpx.ErrForbidden
	}
	return "", httpx.ErrConflict.WithCode("meeting_not_connected").WithMessage("Connect your Google account first.")
}

func (h *Handlers) ExternalChatConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Instance string `json:"instance_id,omitempty"`
		ReturnTo string `json:"return_to,omitempty"`
		Purpose  string `json:"purpose,omitempty"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	key := externalChatKey(req.Provider)
	if req.Provider == "google-meet" {
		// Las reuniones de Meet también las conecta cada persona: ver ExternalChatMeeting.
		key = tmplGoogleMeet
	}
	if !validExternalChatRequest(key, req.Purpose, req.ReturnTo) {
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
	if key == tmplTeamsChat {
		var cfg map[string]string
		_ = json.Unmarshal(inst.Config, &cfg)
		flow.Scopes, err = teamsConsentScopes(req.Purpose, cfg, inst.Status == domain.StatusActive && inst.HasSecrets)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	section := req.ReturnTo
	if section == "" {
		section = "rooms"
	}
	if section == "rooms" {
		section = "external-chat"
		if key == tmplTeamsChat && req.Purpose == teamsMeetService {
			section = teamsMeetingSection
		}
	}
	tenant := tenancy.MustFrom(r.Context())
	start, err := consent.Begin(cipher, flow, tenant.Slug, actor.Subject, inst.ID, h.oauthRedirectURI(), section, time.Now())
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

func (h *Handlers) externalChatFlow(ctx context.Context, key string) (string, consent.Flow, error) {
	template, err := h.store.GetTemplate(ctx, key)
	if err != nil || !template.Enabled {
		return "", consent.Flow{}, httpx.ErrNotFound
	}
	catalog, err := manifest.Parse(template.Manifest)
	if err != nil || catalog.OAuth == nil {
		return "", consent.Flow{}, httpx.ErrInternal
	}
	client := h.clienteDePlataforma(ctx, key, catalog.OAuth.TokenURL)
	if client.ClientID == "" || client.ClientSecret == "" {
		return "", consent.Flow{}, httpx.ErrUnavailable.WithCode("external_provider_unavailable").WithMessage("This provider is not configured yet.")
	}
	flow, err := h.resolveFlow(ctx, catalog, key, map[string]string{})
	return catalog.Name, flow, err
}

func (h *Handlers) externalChatInstance(ctx context.Context, key, name, id, actor string) (*domain.AppInstance, error) {
	var inst *domain.AppInstance
	var err error
	if id == "" {
		inst, err = h.store.CreateInstance(ctx, key, name, json.RawMessage(`{}`), nil)
	} else {
		if _, err := uuid.Parse(id); err != nil {
			return nil, domain.ErrInvalidInput
		}
		inst, err = h.store.GetInstance(ctx, id)
		if err == nil && inst.TemplateKey != key {
			return nil, domain.ErrForbidden
		}
	}
	if err != nil {
		return nil, err
	}
	if err := h.store.BindExternalChatActor(ctx, inst.ID, actor); err != nil {
		return nil, err
	}
	return inst, nil
}

func (h *Handlers) ExternalChatChannels(w http.ResponseWriter, r *http.Request) {
	conn, rt, err := h.externalChatConnection(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := externalListChannels(r.Context(), conn, rt, r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) ExternalChatMessages(w http.ResponseWriter, r *http.Request) {
	conn, rt, err := h.externalChatConnection(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := externalListMessages(r.Context(), conn, rt, r.URL.Query().Get("remote_id"), r.URL.Query().Get("cursor"), r.URL.Query().Get("thread_id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) ExternalChatSend(w http.ResponseWriter, r *http.Request) {
	var req externalWrite
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	conn, rt, err := h.externalChatConnection(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := externalSend(r.Context(), conn, rt, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, result)
}

func (h *Handlers) ExternalChatAction(w http.ResponseWriter, r *http.Request) {
	var req externalAction
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	conn, rt, err := h.externalChatConnection(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := externalApplyAction(r.Context(), conn, rt, req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ExternalChatDisconnect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "instance")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	actor, _ := authn.FromContext(r.Context())
	if err := h.store.RequireExternalChatActor(r.Context(), id, actor.Subject); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	inst, m, cfg, err := h.loadInstanceManifest(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !externalChatTemplate(inst.TemplateKey) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	h.revokeAtProvider(r.Context(), id, inst.TemplateKey, m, cfg)
	if err := h.store.DeleteInstance(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
