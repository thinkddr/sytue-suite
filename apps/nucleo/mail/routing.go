// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// RoutingMetadata is public delivery configuration from the original Mail API.
// It cannot read or write a message body. The configured client retains its
// captured session/owner and exact scope across native and browser transports.
type RoutingMetadata interface {
	RoutingMetadata(context.Context, storage.Scope) (api.MailRoutingSnapshot, error)
	CreateRoutingAlias(context.Context, storage.Scope, string) (api.MailAlias, error)
	DeleteRoutingAlias(context.Context, storage.Scope, string) error
	RoutingQuota(context.Context, storage.Scope) (api.MailQuota, error)
}

type routingProjection struct {
	Version int             `json:"v"`
	Scope   storage.Scope   `json:"scope"`
	Primary string          `json:"primary"`
	Aliases []api.MailAlias `json:"aliases"`
}

func (p routingProjection) validate(scope storage.Scope) error {
	if p.Version != 1 || p.Scope != scope || !identityAddress(p.Primary) {
		return ErrRecord
	}
	addresses := map[string]bool{strings.ToLower(p.Primary): true}
	ids := map[string]bool{}
	for _, alias := range p.Aliases {
		address := strings.ToLower(alias.Address)
		if !storage.MigrationEpoch(alias.ID) || !identityAddress(alias.Address) || ids[alias.ID] || addresses[address] {
			return ErrRecord
		}
		ids[alias.ID] = true
		addresses[address] = true
	}
	return nil
}

func (r *Repository) routingProjection(ctx context.Context, s storage.AtomicCatalogSnapshot) (routingProjection, bool, error) {
	var value routingProjection
	raw, err := r.document(ctx, s, "routing", "aliases")
	defer clear(raw)
	if errors.Is(err, ErrNotFound) {
		return value, false, nil
	}
	if err != nil {
		return value, false, err
	}
	if json.Unmarshal(raw, &value) != nil || value.validate(r.cfg.Scope) != nil {
		return value, false, ErrRecord
	}
	return value, true, nil
}

func (r *Repository) readRoutingMetadata(ctx context.Context) (routingProjection, error) {
	if r.cfg.Metadata == nil {
		return routingProjection{}, api.ErrPrivateMailOperation
	}
	snapshot, err := r.cfg.Metadata.RoutingMetadata(ctx, r.cfg.Scope)
	if err != nil {
		return routingProjection{}, err
	}
	if err := ctx.Err(); err != nil {
		return routingProjection{}, err
	}
	value := routingProjection{Version: 1, Scope: r.cfg.Scope, Primary: snapshot.Primary, Aliases: slices.Clone(snapshot.Aliases)}
	if err := value.validate(r.cfg.Scope); err != nil {
		return routingProjection{}, err
	}
	slices.SortFunc(value.Aliases, func(a, b api.MailAlias) int { return strings.Compare(a.ID, b.ID) })
	return value, nil
}

func (r *Repository) saveRoutingProjection(ctx context.Context, s storage.AtomicCatalogSnapshot, value routingProjection) error {
	if err := value.validate(r.cfg.Scope); err != nil {
		return err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	defer clear(payload)
	// The original writer reconciles retained exact ACKs even when the visible
	// payload already matches; a separate no-op shortcut would leave WAL pending.
	return r.writeDocument(ctx, s, "routing", "aliases", payload, "live")
}

// RefreshRouting updates an encrypted client projection only after receiving a
// fresh authenticated metadata response. Ordinary draft work can subsequently
// use this exact projection offline; failed refreshes never become empty aliases.
func (r *Repository) RefreshRouting(parent context.Context) (api.MailRoutingSnapshot, error) {
	var out api.MailRoutingSnapshot
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		value, err := r.readRoutingMetadata(ctx)
		if err != nil {
			return err
		}
		if err := r.saveRoutingProjection(ctx, s, value); err != nil {
			return err
		}
		out = api.MailRoutingSnapshot{Primary: value.Primary, Aliases: slices.Clone(value.Aliases)}
		return nil
	})
	return out, err
}

func (r *Repository) CreateRoutingAlias(parent context.Context, address string) (api.MailAlias, error) {
	var out api.MailAlias
	if !identityAddress(address) {
		return out, ErrRecord
	}
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		current, err := r.readRoutingMetadata(ctx)
		if err != nil {
			return err
		}
		if strings.EqualFold(current.Primary, address) {
			return ErrConflict
		}
		for _, alias := range current.Aliases {
			if strings.EqualFold(alias.Address, address) {
				out = alias
				return r.saveRoutingProjection(ctx, s, current)
			}
		}
		created, err := r.cfg.Metadata.CreateRoutingAlias(ctx, r.cfg.Scope, address)
		if err != nil {
			return err
		}
		if !storage.MigrationEpoch(created.ID) || !strings.EqualFold(created.Address, address) {
			return ErrRecord
		}
		confirmed, err := r.readRoutingMetadata(ctx)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(confirmed.Aliases, func(value api.MailAlias) bool { return value == created }) {
			return ErrConflict
		}
		out = created
		return r.saveRoutingProjection(ctx, s, confirmed)
	})
	return out, err
}

func (r *Repository) DeleteRoutingAlias(parent context.Context, id string) error {
	if !storage.MigrationEpoch(id) {
		return ErrRecord
	}
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		current, err := r.readRoutingMetadata(ctx)
		if err != nil {
			return err
		}
		exists := slices.ContainsFunc(current.Aliases, func(value api.MailAlias) bool { return value.ID == id })
		if !exists {
			_, known, err := r.aliases(ctx, s)
			if err != nil {
				return err
			}
			if !slices.ContainsFunc(known, func(value api.MailAlias) bool { return value.ID == id }) {
				return ErrNotFound
			}
		} else if err := r.cfg.Metadata.DeleteRoutingAlias(ctx, r.cfg.Scope, id); err != nil {
			return err
		}
		confirmed, err := r.readRoutingMetadata(ctx)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(confirmed.Aliases, func(value api.MailAlias) bool { return value.ID == id }) {
			return ErrConflict
		}
		return r.saveRoutingProjection(ctx, s, confirmed)
	})
}
