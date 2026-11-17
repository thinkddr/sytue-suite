// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
)

// Office only supplies its domain routing and outbox callback. Durable holds,
// SDK evidence, immutable reservations and logical CAS use the common relay.
type (
	ResourceRelay         = resourcepg.Relay
	ResourceRelayVerifier = resourcepg.Verifier
)

func NewResourceRelay(store *Store, verifier ResourceRelayVerifier, subject string) (*ResourceRelay, error) {
	if store == nil {
		return nil, resource.ErrInvalid
	}
	return resourcepg.New(officeResourceRelayDependencies(store, subject), verifier, subject)
}

func officeResourceRoute(_ context.Context, value resource.Resource) error {
	switch value.Product {
	case "docs":
		if value.Kind == "document" {
			return nil
		}
	case "sheets":
		if value.Kind == "spreadsheet" {
			return nil
		}
	case "slides":
		if value.Kind == "presentation" {
			return nil
		}
	}
	return resource.ErrInvalid
}

// The pre-epoch view reuses the same ORIGINAL source lock and product routing,
// without inventing a live epoch verifier before one can exist.
func NewResourceBootstrapCaptureStore(store *Store, owner string) (*resourcepg.BootstrapCaptureStore, error) {
	if store == nil {
		return nil, resource.ErrInvalid
	}
	return resourcepg.NewBootstrapCaptureStore(officeResourceRelayDependencies(store, owner), owner)
}

func officeResourceRelayDependencies(store *Store, subject string) resourcepg.Dependencies {
	return resourcepg.Dependencies{TenantTx: store.tenantTx, ValidateResource: officeResourceRoute, RequirePrivateRules: true, CheckBootstrapSource: func(ctx context.Context, tx pgx.Tx, snapshot resource.BootstrapSourceSnapshot) error {
		if snapshot.OwnerScope.Subject != subject {
			return resource.ErrRevoked
		}
		source, err := NewResourceBootstrapSource(store, snapshot.OwnerScope)
		if err != nil {
			return err
		}
		return source.CheckBootstrapSourceTx(ctx, tx, snapshot)
	}, Notify: func(ctx context.Context, tx pgx.Tx, receipt resource.Receipt) error {
		e, err := events.NewEvent(EventChannel, "docs.resource.updated", receipt.Resource.Tenant, map[string]any{"resource": receipt.Resource, "base_seq": receipt.Base.Seq, "seq": receipt.Head.Seq})
		if err != nil {
			return err
		}
		return events.Publish(ctx, tx, e)
	}}
}
