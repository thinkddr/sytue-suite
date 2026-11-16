// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

var errOfficeStateMissing = errors.New("docs: owner state must be committed before external effect")

// WithLegacyOfficeObjectWrite holds the authoritative owner lock across the
// entire S3 effect, paired with the source capture's exclusive side.
func (s *Store) WithLegacyOfficeObjectWrite(ctx context.Context, key string, write func() error) error {
	parts := strings.SplitN(key, "/", 4)
	if len(parts) != 4 || parts[0] != "docs" || parts[2] != "snapshots" && parts[2] != "exports" || parts[3] == "" {
		return pg.ErrPrivateStorageScope
	}
	return s.WithLegacyDocumentEffect(ctx, parts[1], write)
}

// WithLegacyDocumentEffect also covers workflows involving Drive and SQL. A
// state created inside the outer transaction would be invisible to its SQL
// callback and wait on itself. Missing states are committed before taking SH;
// a concurrently frozen/reassigned document is resolved and checked again.
func (s *Store) WithLegacyDocumentEffect(ctx context.Context, document string, effect func() error) error {
	tenant, err := tenancy.From(ctx)
	id, parseErr := uuid.Parse(document)
	if err != nil || parseErr != nil || id == uuid.Nil || id.String() != document || s == nil || s.pool == nil || effect == nil {
		return pg.ErrPrivateStorageScope
	}
	resolve := func(tx pgx.Tx) (string, string, error) {
		var owner, product string
		if _, err := tx.Exec(ctx, `SELECT public.docs_private_document_lock($1,false)`, id); err != nil {
			return "", "", err
		}
		err := tx.QueryRow(ctx, `SELECT owner_sub,docs_private_product(kind::text) FROM documents WHERE tenant=$1 AND id=$2`, tenant.Slug, id).Scan(&owner, &product)
		if err != nil {
			return "", "", err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, owner); err != nil {
			return "", "", err
		}
		return owner, product, nil
	}
	attempt := func() error {
		return s.tenantTx(ctx, func(tx pgx.Tx) error {
			owner, product, err := resolve(tx)
			if err != nil {
				return err
			}
			if _, err := pg.SharePrivateStorageState(ctx, tx, product+"_private_storage_states", tenant.Slug, owner); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return errOfficeStateMissing
				}
				return err
			}
			// Share found a visible committed row: the generic SQL guard cannot create
			// one here, and nested domain SQL takes a compatible SH lock on that row.
			if _, err := tx.Exec(ctx, `SELECT public.sytue_private_storage_guard(to_regclass('public.'||$1||'_private_storage_states'),$2,$3)`, product, tenant.Slug, owner); err != nil {
				return err
			}
			return effect()
		})
	}
	err = attempt()
	if !errors.Is(err, errOfficeStateMissing) {
		return err
	}
	if err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		owner, product, err := resolve(tx)
		if err != nil {
			return err
		}
		return pg.EnsurePrivateStorageState(ctx, tx, product+"_private_storage_states", tenant.Slug, owner, nil)
	}); err != nil {
		return err
	}
	return attempt()
}
