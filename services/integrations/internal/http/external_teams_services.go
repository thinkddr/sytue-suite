// SPDX-License-Identifier: Apache-2.0
package integrationshttp

import (
	"encoding/json"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
)

const (
	teamsMeetingSection = "external-chat-meeting"
	teamsServicesKey    = "_teams_services"
	teamsScopesKey      = "_teams_scopes"
	teamsChatService    = "chat"
	teamsMeetService    = "meeting"
	teamsMeetScope      = "OnlineMeetings.ReadWrite"
)

var teamsChatScopes = []string{"Team.ReadBasic.All", "Channel.ReadBasic.All", "ChannelMessage.Read.All", "ChannelMessage.Send", "Chat.ReadWrite"}

func validExternalChatRequest(key, purpose, returnTo string) bool {
	return key != "" && (purpose == "" || key == tmplTeamsChat && (purpose == teamsChatService || purpose == teamsMeetService)) &&
		(returnTo == "" || returnTo == "rooms" || returnTo == "native-rooms")
}

func teamsServices(cfg map[string]string) []string {
	if _, present := cfg[teamsServicesKey]; !present {
		return []string{teamsChatService} // conexiones de chat anteriores a los grants por finalidad
	}
	var services []string
	_ = json.Unmarshal([]byte(cfg[teamsServicesKey]), &services)
	return services
}

func teamsServiceEnabled(cfg map[string]string, service string) bool {
	return slices.Contains(teamsServices(cfg), service)
}

// Teams usa el cliente gestionado y el mismo vault. Un alta sólo-reunión nunca
// pide leer chats; ampliar una cuenta conserva las finalidades ya consentidas.
func teamsConsentScopes(purpose string, cfg map[string]string, connected bool) ([]string, error) {
	if purpose == "" {
		purpose = teamsChatService
	}
	if purpose != teamsChatService && purpose != teamsMeetService {
		return nil, httpx.ErrBadRequest
	}
	services := []string{purpose}
	if connected {
		services = append(services, teamsServices(cfg)...)
	}
	scopes := []string{"offline_access", "User.Read"}
	if slices.Contains(services, teamsChatService) {
		scopes = append(scopes, teamsChatScopes...)
	}
	if slices.Contains(services, teamsMeetService) {
		scopes = append(scopes, teamsMeetScope)
	}
	return scopes, nil
}

// Graph accepts qualified and short permission names, including the lowercase
// form returned by some Microsoft token responses. Other resource origins do not match.
func teamsHasScope(scopes []string, wanted string) bool {
	for _, scope := range scopes {
		normalized := strings.TrimPrefix(strings.ToLower(scope), strings.TrimSuffix(roomsGraphAPI, "v1.0"))
		if strings.EqualFold(normalized, wanted) {
			return true
		}
	}
	return false
}

func teamsGranted(requested, granted []string) []string {
	if granted == nil {
		granted = requested
	}
	contains := func(required, scopes []string) bool {
		for _, scope := range required {
			if !teamsHasScope(scopes, scope) {
				return false
			}
		}
		return true
	}
	services := []string{}
	if contains(teamsChatScopes, requested) && contains(teamsChatScopes, granted) {
		services = append(services, teamsChatService)
	}
	if teamsHasScope(requested, teamsMeetScope) && teamsHasScope(granted, teamsMeetScope) {
		services = append(services, teamsMeetService)
	}
	return services
}

func applyTeamsGrant(cfg map[string]string, raw json.RawMessage, requested, legacy []string) error {
	granted, err := consent.ParseScopes(raw)
	if err != nil {
		return httpx.ErrForbidden.WithCode("external_scope_denied")
	}
	if len(requested) == 0 {
		requested = legacy
	}
	services := teamsGranted(requested, granted)
	if len(services) == 0 || !slices.Equal(services, teamsGranted(requested, nil)) {
		return httpx.ErrForbidden.WithCode("external_scope_denied")
	}
	encoded, _ := json.Marshal(services)
	cfg[teamsServicesKey] = string(encoded)
	cfg[teamsScopesKey] = strings.Join(requested, " ")
	return nil
}

func narrowedTeamsServices(cfg map[string]string, raw json.RawMessage) (map[string]string, error) {
	requested := strings.Fields(cfg[teamsScopesKey])
	if len(requested) == 0 {
		requested = append([]string{"offline_access", "User.Read"}, teamsChatScopes...)
	}
	granted, err := consent.ParseScopes(raw)
	if err != nil {
		return nil, httpx.ErrConflict.WithCode("external_reconnect_required")
	}
	services := teamsGranted(requested, granted)
	encoded, _ := json.Marshal(services)
	return map[string]string{teamsServicesKey: string(encoded), teamsScopesKey: strings.Join(requested, " ")}, nil
}
