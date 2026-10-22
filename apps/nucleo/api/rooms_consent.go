// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"net/url"
)

// RoomsIntegrationDestination is a provider channel available to the connected account.
type RoomsIntegrationDestination struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (r *Rooms) IntegrationDestinations(ctx context.Context, instance string) ([]RoomsIntegrationDestination, error) {
	out, err := JSON[struct {
		Items []RoomsIntegrationDestination `json:"items"`
	}](ctx, r.c, Request{Path: "/integrations/destinations", Query: url.Values{"instance": {instance}}})
	return out.Items, err
}

// ConnectOAuthBridge checks channel administration and sends a test before saving.
func (r *Rooms) ConnectOAuthBridge(ctx context.Context, channel, platform, instance, remote string) (RoomsBridge, error) {
	out, err := PostJSON[struct {
		Bridge RoomsBridge `json:"bridge"`
	}](ctx, r.c, ruta("/channels/", channel, "/bridges"), struct {
		Platform   string `json:"platform"`
		InstanceID string `json:"instance_id"`
		RemoteID   string `json:"remote_id"`
	}{platform, instance, remote})
	return out.Bridge, err
}

type RoomsWhatsAppConsent struct {
	TeamID        string `json:"team_id"`
	InstanceID    string `json:"instance_id"`
	PhoneNumberID string `json:"phone_number_id"`
	PIN           string `json:"pin,omitempty"`
}

func (r *Rooms) ConnectWhatsAppConsent(ctx context.Context, input RoomsWhatsAppConsent) (RoomsWhatsApp, error) {
	out, err := PostJSON[struct {
		Account RoomsWhatsApp `json:"account"`
	}](ctx, r.c, "/whatsapp/consent", input)
	return out.Account, err
}
