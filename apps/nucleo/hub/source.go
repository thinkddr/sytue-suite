// SPDX-License-Identifier: Apache-2.0
package hub

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

func SourceKind(kind string) bool {
	switch kind {
	case "notification", "preference", noticeKind:
		return true
	default:
		return false
	}
}

func recordsConfiguration(cfg Config) (Config, error) {
	scope := cfg.Scope
	if !scope.Valid() || scope.Product != "hub" {
		return Config{}, ErrNotice
	}
	cfg.Product = "hub"
	cfg.EntityKind = "org.sytue.hub.entity.v1"
	cfg.ReceiptKind = "org.sytue.hub.snapshot.v1"
	cfg.BinaryKind, cfg.ValidateBinary = nil, nil
	cfg.ValidateRow = func(_ context.Context, kind, id string, raw []byte) error {
		if kind == "notification" || kind == "preference" {
			if !storage.Identifier(id) || !json.Valid(raw) {
				return ErrNotice
			}
			return nil
		}
		_, err := decode(scope, kind, id, raw)
		return err
	}
	cfg.ValidateCatalog = func(ctx context.Context, catalog records.Catalog) error {
		entries, err := catalog.Entries()
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Kind != "notification" && entry.Kind != "preference" {
				continue
			}
			raw, err := catalog.ReadJSON(ctx, entry)
			if err != nil {
				return err
			}
			valid := json.Valid(raw)
			clear(raw)
			if !valid {
				return ErrNotice
			}
		}
		return nil
	}
	return cfg, nil
}

// NewSource preserves the original authenticated API solely for the immutable
// retained source. The shared adapter owns delegation and typed-nil validation.
func NewSource(client *api.Hub) (records.Source, error) {
	return records.NewAppSource(client)
}

// NewHistoricalSource uses Hub's validation while exposing only the common
// read/prepare lifecycle. Historical CRUD is never mounted.
func NewHistoricalSource(cfg Config) (*records.HistoricalSource, error) {
	configuration, err := recordsConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	return records.NewHistoricalSource(configuration)
}

func (r *Repository) IdentityCutover(cfg records.IdentityCutoverConfig) (*records.IdentityCutover, error) {
	if r == nil || r.core == nil || cfg.Scope != r.scope || cfg.Isolated != nil {
		return nil, ErrNotice
	}
	cfg.Isolated = r.core
	return records.NewIdentityCutover(cfg)
}

func toAPINotification(n Notice) api.HubNotification {
	return api.HubNotification{
		ID:               n.ID,
		Source:           n.Source,
		Type:             n.Type,
		Title:            n.Title,
		Body:             n.Body,
		URL:              n.URL,
		Params:           n.Params,
		Read:             n.Read,
		CreatedAt:        n.CreatedAt,
		Organization:     n.Organization,
		OrganizationName: n.OrganizationName,
		Historic:         n.Historic,
	}
}

func (r *Repository) Notifications(ctx context.Context, limite int) ([]api.HubNotification, error) {
	notices, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	if limite > 0 && len(notices) > limite {
		notices = notices[:limite]
	}
	out := make([]api.HubNotification, 0, len(notices))
	for _, n := range notices {
		out = append(out, toAPINotification(n))
	}
	return out, nil
}

func (r *Repository) NotificationsAntes(ctx context.Context, limite int, antes api.HubNotification) ([]api.HubNotification, error) {
	notices, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.HubNotification, 0, len(notices))
	for _, n := range notices {
		cmp := n.CreatedAt.Compare(antes.CreatedAt)
		if cmp > 0 || (cmp == 0 && strings.Compare(n.ID, antes.ID) >= 0) {
			continue
		}
		out = append(out, toAPINotification(n))
		if limite > 0 && len(out) >= limite {
			break
		}
	}
	return out, nil
}

func (r *Repository) AvisarAMi(ctx context.Context, n api.HubAvisoPropio) error {
	notice := Notice{
		Version:   1,
		Scope:     r.scope,
		ID:        uuid.NewString(),
		Source:    n.Source,
		Type:      n.Type,
		Level:     "info",
		Title:     n.Title,
		URL:       n.URL,
		Params:    n.Params,
		CreatedAt: time.Now().UTC(),
	}
	return r.Put(ctx, notice)
}

func (r *Repository) Contadores(ctx context.Context) (api.HubContadores, error) {
	notices, err := r.List(ctx)
	if err != nil {
		return api.HubContadores{}, err
	}
	unread := 0
	for _, n := range notices {
		if !n.Read {
			unread++
		}
	}
	return api.HubContadores{Unread: unread}, nil
}

func (r *Repository) MarkAllRead(ctx context.Context) error {
	notices, err := r.List(ctx)
	if err != nil {
		return err
	}
	for _, n := range notices {
		if !n.Read {
			if err := r.MarkRead(ctx, n.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) NotificationPrefs(ctx context.Context) ([]api.HubNotificationPref, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []api.HubNotificationPref{}, nil
}

func (r *Repository) SetNotificationPref(ctx context.Context, p api.HubNotificationPref) (api.HubNotificationPref, error) {
	if err := ctx.Err(); err != nil {
		return api.HubNotificationPref{}, err
	}
	return p, nil
}
