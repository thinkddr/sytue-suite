// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// SetProjectInstallation fija (o limpia, con installationID=0) el id de la
// instalación de la GitHub App del repo del proyecto. Tenant-scoped (RLS): lo
// llama el handler de conectar/desconectar App bajo el tenant del usuario.
func (s *Store) SetProjectInstallation(ctx context.Context, projectID uuid.UUID, installationID int64) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE projects SET github_installation_id=$3, updated_at=now()
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
			t.Slug, projectID, installationID)
		if err != nil {
			return fmtErr("fijar instalación de GitHub App", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// GitHubPreview reúne los datos que el notificador necesita para publicar/editar
// el comentario de preview del PR: el destino/PR del deployment, la URL pública
// de la preview, el repo del proyecto, la instalación de la App y el id del
// comentario ya publicado para este PR (0 si aún ninguno).
type GitHubPreview struct {
	Target          domain.DeployTarget
	PRNumber        *int
	URL             string
	ProjectID       uuid.UUID
	RepoURL         string
	InstallationID  int64
	ExistingComment int64
}

// LoadGitHubPreview resuelve el contexto del comentario de preview de un
// deployment (cross-tenant: el worker pasa el tenant del evento y se fija
// app.tenant bajo RLS). Devuelve ok=false si el deployment ya no existe.
func (s *Store) LoadGitHubPreview(ctx context.Context, tenant string, deploymentID uuid.UUID) (GitHubPreview, bool, error) {
	var out GitHubPreview
	found := false
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `
			SELECT d.target, d.pr_number, d.url, p.id, p.repo_url,
			       p.github_installation_id, COALESCE(c.comment_id, 0)
			FROM deployments d
			JOIN projects p ON p.id = d.project_id AND p.tenant = d.tenant
			LEFT JOIN github_pr_comments c
			       ON c.project_id = p.id AND c.pr_number = d.pr_number
			WHERE d.tenant=$1 AND d.id=$2`, tenant, deploymentID).
			Scan(&out.Target, &out.PRNumber, &out.URL, &out.ProjectID, &out.RepoURL,
				&out.InstallationID, &out.ExistingComment)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmtErr("cargar contexto de preview de GitHub", err)
		}
		found = true
		return nil
	})
	return out, found, err
}

// SaveGitHubPRComment recuerda el id del comentario publicado para (proyecto,
// PR), de modo que un re-deploy del mismo PR lo edite en vez de duplicarlo.
// Upsert por la PK (project_id, pr_number). Cross-tenant (worker).
func (s *Store) SaveGitHubPRComment(ctx context.Context, tenant string, projectID uuid.UUID, pr int, commentID int64) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO github_pr_comments (tenant, project_id, pr_number, comment_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (project_id, pr_number)
			DO UPDATE SET comment_id = EXCLUDED.comment_id, updated_at = now()`,
			tenant, projectID, pr, commentID); err != nil {
			return fmtErr("guardar comentario de PR de GitHub", err)
		}
		return nil
	})
}
