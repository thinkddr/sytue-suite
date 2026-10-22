// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

type oauthTemplateStatus struct {
	domain.AppTemplate
	OAuthAvailable bool `json:"oauth_available"`
}

type oauthInstanceStatus struct {
	domain.AppInstance
	OAuthAvailable bool `json:"oauth_available"`
}

// oauthAvailable reports an actionable consent path, rather than the mere
// presence of a manifest. Only the boolean crosses the public API boundary.
func (h *Handlers) oauthAvailable(ctx context.Context, key string, raw json.RawMessage, cfg map[string]string, hasOwnSecret bool) bool {
	if h.oauthCipher == nil || !h.oauthCipher.Enabled() {
		return false
	}
	m, err := manifest.Parse(raw)
	if err != nil || m.OAuth == nil {
		return false
	}
	if cfg != nil {
		if _, err := m.RenderOAuth(cfg); err != nil {
			return false
		}
	}
	client := h.clienteDePlataforma(ctx, key, m.OAuth.TokenURL)
	if client.ClientID != "" {
		return client.ClientSecret != ""
	}
	return cfg[configKeyClientID] != "" && hasOwnSecret
}

func (h *Handlers) instanceOAuthAvailable(ctx context.Context, inst domain.AppInstance) bool {
	return inst.ConsentTemplateEnabled && h.oauthAvailable(ctx, inst.TemplateKey, inst.ConsentManifest, inst.ConfigMap(), inst.HasOAuthClientSecret)
}
