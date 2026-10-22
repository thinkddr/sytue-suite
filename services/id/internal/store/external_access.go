// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// ExternalAccessGuard revokes access held by an independent encrypted transport.
// It never receives message keys. Configure it once, before serving requests.
type ExternalAccessGuard struct {
	Apply func(context.Context, MatrixAccessChange) error
}

const MatrixAccessChannel events.Channel = "id_matrix_access"

const MatrixAccessActive = "id.matrix.access.active"

type MatrixAccessChange struct {
	Subject string `json:"subject"`
	Tenant  string `json:"tenant,omitempty"`
	Status  string `json:"status"`
	Version int64  `json:"version"`
}

func (s *Store) SetExternalAccessGuard(g ExternalAccessGuard) { s.externalAccess = g }

func (s *Store) needsExternalAccessTransaction(q pg.Querier) bool {
	if q == nil {
		return true
	}
	if s.externalAccess.Apply == nil && s.callIdentity == nil {
		return false
	}
	_, transactional := q.(pgx.Tx)
	return !transactional
}

// The row lock serializes account status changes until the caller's commit.
// Revocation precedes the local mutation; a transport failure leaves access
// unchanged and returns an error. Re-enabling is done by the transport only
// after a fresh check against ID, never before a local activation commits.
func (s *Store) suspendExternalAccount(ctx context.Context, q pg.Querier, subject string, status domain.Status) error {
	if status == domain.StatusActive || s.externalAccess.Apply == nil {
		return nil
	}
	var found string
	err := q.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, subject).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errf("suspendExternalAccount.lock", err)
	}
	return s.applyExternalRevocation(ctx, q, subject, "", "suspended")
}

func (s *Store) removeExternalOrgAccess(ctx context.Context, q pg.Querier, orgID, subject string) error {
	if s.externalAccess.Apply == nil {
		return nil
	}
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, orgID); err != nil {
		return err
	}
	var slug string
	err := q.QueryRow(ctx, `SELECT o.slug FROM organizations o
		JOIN memberships m ON m.org_id=o.id WHERE o.id=$1 AND m.user_id=$2
		FOR UPDATE OF m`, orgID, subject).Scan(&slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errf("removeExternalOrgAccess.lock", err)
	}
	return s.applyExternalRevocation(ctx, q, subject, slug, "removed")
}

func (s *Store) suspendExternalOrg(ctx context.Context, q pg.Querier, orgID string, status domain.Status) error {
	if s.externalAccess.Apply == nil {
		return nil
	}
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, orgID); err != nil {
		return err
	}
	if status == domain.StatusActive {
		return nil
	}
	var slug string
	err := q.QueryRow(ctx, `SELECT slug FROM organizations WHERE id=$1 FOR UPDATE`, orgID).Scan(&slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errf("suspendExternalOrg.lock", err)
	}
	rows, err := q.Query(ctx, `SELECT DISTINCT user_id FROM memberships WHERE org_id=$1`, orgID)
	if err != nil {
		return errf("suspendExternalOrg.members", err)
	}
	var subjects []string
	for rows.Next() {
		var subject string
		if err := rows.Scan(&subject); err != nil {
			rows.Close()
			return err
		}
		subjects = append(subjects, subject)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, subject := range subjects {
		if err := s.applyExternalRevocation(ctx, q, subject, slug, "removed"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyExternalRevocation(ctx context.Context, q pg.Querier, subject, tenant, status string) error {
	change := MatrixAccessChange{Subject: subject, Tenant: tenant, Status: status}
	if err := q.QueryRow(ctx, `SELECT nextval('matrix_access_version_seq')`).Scan(&change.Version); err != nil {
		return err
	}
	return s.externalAccess.Apply(ctx, change)
}

// Activations are delivered by the shared outbox only after commit. Matrix
// also checks ID live and rejects a stale version after a later revocation.
func (s *Store) queueExternalActivation(ctx context.Context, q pg.Querier, subject, orgID string) error {
	if s.externalAccess.Apply == nil {
		return nil
	}
	change := MatrixAccessChange{Subject: subject, Status: "active"}
	if orgID != "" {
		err := q.QueryRow(ctx, `SELECT o.slug FROM memberships m
			JOIN organizations o ON o.id=m.org_id JOIN users u ON u.id=m.user_id
			WHERE m.user_id=$1 AND m.org_id=$2 AND o.status='active' AND u.status='active'`,
			subject, orgID).Scan(&change.Tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	if err := q.QueryRow(ctx, `SELECT nextval('matrix_access_version_seq')`).Scan(&change.Version); err != nil {
		return err
	}
	ev, err := events.NewEvent(MatrixAccessChannel, MatrixAccessActive, change.Tenant, change)
	if err != nil {
		return err
	}
	return events.Publish(ctx, q, ev)
}

func (s *Store) queueExternalOrgActivation(ctx context.Context, q pg.Querier, orgID string) error {
	if s.externalAccess.Apply == nil {
		return nil
	}
	rows, err := q.Query(ctx, `SELECT user_id FROM memberships WHERE org_id=$1`, orgID)
	if err != nil {
		return err
	}
	var subjects []string
	for rows.Next() {
		var subject string
		if err := rows.Scan(&subject); err != nil {
			rows.Close()
			return err
		}
		subjects = append(subjects, subject)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, subject := range subjects {
		if err := s.queueExternalActivation(ctx, q, subject, orgID); err != nil {
			return err
		}
	}
	return nil
}
