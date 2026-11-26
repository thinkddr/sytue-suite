// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	cuentaschat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

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
		if err := roomsGraph(rt).Pedir(ctx, http.MethodGet, "/me?$select=id", nil, &who); err != nil {
			return roomsProviderError(err)
		}
		if _, err := uuid.Parse(who.ID); err != nil {
			return httpx.ErrUnavailable.WithCode("external_provider_identity")
		}
		cfg["_external_user_id"] = who.ID
	case tmplGoogleConversations:
		who, err := cuentaschat.Usuario(ctx, rt.HTTP, nil, "https://openidconnect.googleapis.com/v1/userinfo")
		if errors.Is(err, cuentaschat.ErrIdentidad) {
			return httpx.ErrUnavailable.WithCode("external_provider_identity")
		}
		if err != nil {
			return roomsProviderError(err)
		}
		cfg["_external_user_id"] = who.Recurso()
	}
	return nil
}
