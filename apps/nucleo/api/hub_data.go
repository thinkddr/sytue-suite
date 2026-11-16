// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// HubData is the device-owned Hub notification surface. Missing capabilities
// must return an error; the private facade never falls back to legacy transport.
type HubData interface {
	PrivateStorageScope() storage.Scope
	Notifications(context.Context, int) ([]HubNotification, error)
	NotificationsAntes(context.Context, int, HubNotification) ([]HubNotification, error)
	AvisarAMi(context.Context, HubAvisoPropio) error
	Contadores(context.Context) (HubContadores, error)
	MarkRead(context.Context, string) error
	MarkAllRead(context.Context) error
	NotificationPrefs(context.Context) ([]HubNotificationPref, error)
	SetNotificationPref(context.Context, HubNotificationPref) (HubNotificationPref, error)
}

var ErrPrivateHubOperation = errors.New("hub: operation unavailable through private data")

// WithPrivateData keeps the typed API used by existing screens. The caller owns
// the repository and drains its work before closing the SDK or changing account.
func (h *Hub) WithPrivateData(data HubData) (*Hub, error) {
	if h == nil || h.c == nil || data == nil || h.private != nil {
		return nil, ErrPrivateHubOperation
	}
	scope := data.PrivateStorageScope()
	if !scope.Valid() || scope.Product != "hub" {
		return nil, ErrPrivateHubOperation
	}
	return &Hub{c: privateDataClient(h.c, ErrPrivateHubOperation), private: data}, nil
}
