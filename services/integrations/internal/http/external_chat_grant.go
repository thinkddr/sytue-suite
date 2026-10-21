// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"net/http"
	"regexp"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

var externalGoogleSubject = regexp.MustCompile(`^\d{1,40}$`)

func (h *Handlers) applyPersonalChatConsent(ctx context.Context, conn connector.Instance, tok tokenResponse, cfg map[string]string, requested, legacy []string) error {
	if conn.TemplateKey == tmplTeamsChat {
		if err := applyTeamsGrant(cfg, tok.Scope, requested, legacy); err != nil {
			return err
		}
	}
	return h.applyExternalConsentMetadata(ctx, conn, tok, cfg)
}

// These identity fields are obtained with the freshly consented user token. No
// ID token is decoded without verification and no guessed /users/me API is used.
func (h *Handlers) applyExternalConsentMetadata(ctx context.Context, conn connector.Instance, tok tokenResponse, cfg map[string]string) error {
	rt := h.egressRuntime(conn, log.From(ctx))
	switch conn.TemplateKey {
	case tmplSlackChat:
		if tok.AuthedUser == nil {
			return httpx.ErrForbidden
		}
		cfg["_external_user_id"] = tok.AuthedUser.ID
		if tok.Team != nil {
			cfg[roomsTeamID], cfg[roomsTeamName] = tok.Team.ID, tok.Team.Name
		}
	case tmplTeamsChat:
		var who struct {
			ID string `json:"id"`
		}
		if err := roomsProviderJSON(ctx, rt, http.MethodGet, roomsGraphAPI+"/me?$select=id", nil, &who); err != nil {
			return err
		}
		if _, err := uuid.Parse(who.ID); err != nil {
			return httpx.ErrUnavailable.WithCode("external_provider_identity")
		}
		cfg["_external_user_id"] = who.ID
	case tmplGoogleConversations:
		var who struct {
			Subject string `json:"sub"`
		}
		if err := roomsProviderJSON(ctx, rt, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil, &who); err != nil {
			return err
		}
		if !externalGoogleSubject.MatchString(who.Subject) {
			return httpx.ErrUnavailable.WithCode("external_provider_identity")
		}
		cfg["_external_user_id"] = "users/" + who.Subject
	}
	return nil
}
