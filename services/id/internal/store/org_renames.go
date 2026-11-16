// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// OrgRename retains the identity before any service is moved. Neither a free
// target slug nor an absent source is proof that an earlier rename succeeded.
type OrgRename struct {
	ID     string
	OrgID  string
	Source string
	Target string
	Status string
}

const orgRenameColumns = `id,org_id,source_slug,target_slug,status`

func scanOrgRename(row pgx.Row) (OrgRename, error) {
	var r OrgRename
	err := row.Scan(&r.ID, &r.OrgID, &r.Source, &r.Target, &r.Status)
	return r, err
}

func (s *Store) requireOrganizationOperation(id string) error {
	session, ok := s.db.(*pg.Session)
	if !ok || session.LockKey() != "organization-operation:"+id || session.GroupKey() != pg.OrganizationOperationLockGroup {
		return domain.ErrConflict
	}
	return nil
}

func (s *Store) FindOrgRename(ctx context.Context, source, target string) (OrgRename, error) {
	r, err := scanOrgRename(s.DB().QueryRow(ctx, `SELECT `+orgRenameColumns+` FROM org_renames WHERE source_slug=$1 AND target_slug=$2`, source, target))
	return r, errf("FindOrgRename", err)
}

func (s *Store) PrepareOrgRename(ctx context.Context, id, source, target string) (OrgRename, error) {
	var r OrgRename
	if err := s.requireOrganizationOperation(id); err != nil {
		return r, err
	}
	err := s.InTx(ctx, func(q pg.Querier) error {
		var err error
		r, err = scanOrgRename(q.QueryRow(ctx, `SELECT `+orgRenameColumns+` FROM org_renames WHERE source_slug=$1 AND target_slug=$2 FOR UPDATE`, source, target))
		if err == nil {
			if r.ID != id {
				return domain.ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		r, err = scanOrgRename(q.QueryRow(ctx, `INSERT INTO org_renames(id,org_id,source_slug,target_slug) SELECT $1,id,$2,$3 FROM organizations WHERE slug=$2 RETURNING `+orgRenameColumns, id, source, target))
		return err
	})
	return r, errf("PrepareOrgRename", err)
}

// OrgRenameIdentityOutcome waits for a pending COMMIT on the retained identity.
// Call it through the owned session; connection loss must never reopen scopes.
func (s *Store) OrgRenameIdentityOutcome(ctx context.Context, id string) (committed bool, err error) {
	if err := s.requireOrganizationOperation(id); err != nil {
		return false, err
	}
	err = s.InTx(ctx, func(q pg.Querier) error {
		var source, target, status, actual string
		if err := q.QueryRow(ctx, `SELECT r.source_slug,r.target_slug,r.status,o.slug FROM org_renames r JOIN organizations o ON o.id=r.org_id WHERE r.id=$1 FOR SHARE OF o,r`, id).Scan(&source, &target, &status, &actual); err != nil {
			return err
		}
		if actual == source && status == "pending" {
			return nil
		}
		if actual == target && (status == "identity_committed" || status == "completed") {
			committed = true
			return nil
		}
		return domain.ErrConflict
	})
	return committed, errf("OrgRenameIdentityOutcome", err)
}

func (s *Store) CommitOrgRename(ctx context.Context, r OrgRename) error {
	if err := s.requireOrganizationOperation(r.ID); err != nil {
		return err
	}
	err := s.InTx(ctx, func(q pg.Querier) error {
		current, err := scanOrgRename(q.QueryRow(ctx, `SELECT `+orgRenameColumns+` FROM org_renames WHERE id=$1 FOR UPDATE`, r.ID))
		if err != nil {
			return err
		}
		if current.ID != r.ID || current.OrgID != r.OrgID || current.Source != r.Source || current.Target != r.Target {
			return domain.ErrConflict
		}
		if current.Status != "pending" {
			return domain.ErrConflict
		}
		tag, err := q.Exec(ctx, `UPDATE organizations SET slug=$3,updated_at=now() WHERE id=$1 AND slug=$2`, r.OrgID, r.Source, r.Target)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return domain.ErrConflict
		}
		tag, err = q.Exec(ctx, `UPDATE org_slug_claims SET retired=true WHERE slug=$1 AND org_id=$2 AND NOT retired`, r.Source, r.OrgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return domain.ErrConflict
		}
		_, err = q.Exec(ctx, `UPDATE org_renames SET status='identity_committed' WHERE id=$1`, r.ID)
		return err
	})
	return errf("CommitOrgRename", err)
}

func (s *Store) CompleteOrgRename(ctx context.Context, id string) error {
	if err := s.requireOrganizationOperation(id); err != nil {
		return err
	}
	err := s.InTx(ctx, func(q pg.Querier) error {
		r, err := scanOrgRename(q.QueryRow(ctx, `SELECT `+orgRenameColumns+` FROM org_renames WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if r.Status == "completed" {
			return nil
		}
		if r.Status != "identity_committed" {
			return domain.ErrConflict
		}
		_, err = q.Exec(ctx, `UPDATE org_renames SET status='completed' WHERE id=$1`, id)
		return err
	})
	return errf("CompleteOrgRename", err)
}
