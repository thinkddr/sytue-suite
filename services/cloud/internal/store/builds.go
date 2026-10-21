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

const buildCols = `id, tenant, project_id, commit_sha, git_ref, status, framework,
	image_ref, logs_key, triggered_by, started_at, finished_at, error_msg, kaniko_job_name, created_at`

func scanBuild(row pgx.Row, t string) (domain.Build, error) {
	var b domain.Build
	if err := row.Scan(&b.ID, &b.Tenant, &b.ProjectID, &b.CommitSHA, &b.GitRef, &b.Status,
		&b.Framework, &b.ImageRef, &b.LogsKey, &b.TriggeredBy, &b.StartedAt, &b.FinishedAt,
		&b.ErrorMsg, &b.KanikoJobName, &b.CreatedAt); err != nil {
		return domain.Build{}, err
	}
	b.Tenant = t
	return b, nil
}

// NewBuildRecord son los datos para encolar una build.
type NewBuildRecord struct {
	ProjectID   uuid.UUID
	CommitSHA   string
	GitRef      string
	Framework   domain.Framework
	TriggeredBy string
}

// EnqueueBuild crea una build en estado 'queued' (la toma cloud-builder).
func (s *Store) EnqueueBuild(ctx context.Context, rec NewBuildRecord) (domain.Build, error) {
	t := tenancy.MustFrom(ctx)
	var b domain.Build
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		b, e = enqueueBuildTx(ctx, tx, t.Slug, rec)
		return e
	})
	return b, err
}

// enqueueBuildTx es el cuerpo transaccional de EnqueueBuild (ver createProjectTx:
// lo comparte el alta atómica desde plantilla).
func enqueueBuildTx(ctx context.Context, tx pgx.Tx, tenant string, rec NewBuildRecord) (domain.Build, error) {
	if rec.Framework == "" {
		rec.Framework = domain.FrameworkDetect
	}
	var ok bool
	if err := tx.QueryRow(ctx,
		`SELECT true FROM projects WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`,
		tenant, rec.ProjectID).Scan(&ok); err != nil {
		return domain.Build{}, normalizeNoRows(err, domain.ErrNotFound)
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO builds (tenant, project_id, commit_sha, git_ref, framework, triggered_by, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'queued')
		RETURNING `+buildCols,
		tenant, rec.ProjectID, rec.CommitSHA, rec.GitRef, string(rec.Framework), rec.TriggeredBy)
	return scanBuild(row, tenant)
}

// GetBuild devuelve una build por ID (en el tenant activo).
func (s *Store) GetBuild(ctx context.Context, id uuid.UUID) (domain.Build, error) {
	t := tenancy.MustFrom(ctx)
	var b domain.Build
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT `+buildCols+` FROM builds WHERE tenant=$1 AND id=$2`, t.Slug, id)
		var e error
		b, e = scanBuild(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Build{}, domain.ErrNotFound
	}
	return b, err
}

// BuildPage es una p?gina de builds con el cursor para pedir la siguiente.
type BuildPage struct {
	Builds []domain.Build
	// Next es el cursor de continuaci?n; nil si no hay m?s resultados.
	Next *PageCursor
}

// ListBuilds lista las builds de un proyecto (m?s recientes primero) con
// paginaci?n keyset por (created_at, id) DESC. Devuelve hasta `limit` builds y,
// si hay m?s, el cursor de continuaci?n. Evita OFFSET (coste O(n)): el
// predicado (created_at, id) < ($ts, $id) recorre el ?ndice compuesto
// builds_project_created_id_idx.
func (s *Store) ListBuilds(ctx context.Context, projectID uuid.UUID, after PageCursor, limit int) (BuildPage, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var page BuildPage
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Pedimos limit+1 para saber si existe p?gina siguiente sin COUNT extra.
		var rows pgx.Rows
		var err error
		if after.IsZero() {
			rows, err = tx.Query(ctx,
				`SELECT `+buildCols+` FROM builds WHERE tenant=$1 AND project_id=$2
				 ORDER BY created_at DESC, id DESC LIMIT $3`,
				t.Slug, projectID, limit+1)
		} else {
			rows, err = tx.Query(ctx,
				`SELECT `+buildCols+` FROM builds WHERE tenant=$1 AND project_id=$2
				   AND (created_at, id) < ($3, $4)
				 ORDER BY created_at DESC, id DESC LIMIT $5`,
				t.Slug, projectID, after.CreatedAt, after.ID, limit+1)
		}
		if err != nil {
			return fmtErr("listar builds", err)
		}
		defer rows.Close()
		for rows.Next() {
			b, err := scanBuild(rows, t.Slug)
			if err != nil {
				return err
			}
			page.Builds = append(page.Builds, b)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(page.Builds) > limit {
			last := page.Builds[limit-1]
			page.Builds = page.Builds[:limit]
			page.Next = &PageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		}
		return nil
	})
	if err != nil {
		return BuildPage{}, err
	}
	return page, nil
}

// ClaimedBuild es una build reclamada por el builder, con la configuraci?n del
// proyecto necesaria para clonar y construir.
type ClaimedBuild struct {
	Build        domain.Build
	Project      domain.Project
	RepoCredsEnc []byte // deploy key cifrada (puede ser nil)
	// InstallationID es el id de la GitHub App instalada para el repo (0 = sin
	// App: el builder usa la deploy-key SSH o el clone público).
	InstallationID int64
}

// ClaimNextBuild toma at?micamente la build 'queued' m?s antigua (cualquier
// tenant), la marca 'running' y devuelve la build + el proyecto. Usa FOR UPDATE
// SKIP LOCKED para permitir varios builders concurrentes sin colisiones.
// Cross-tenant: la cola se lee en una transacción de mantenimiento y después se
// fija app.tenant a la fila tomada.
// Devuelve ok=false si no hay builds pendientes.
//
// Mantenimiento y no una transacción normal (auditoría 2026-10-06, INF-01): la
// cola es de TODAS las organizaciones y la RLS forzada de `builds` no deja ver
// ninguna fila sin `app.tenant` ni `app.maintenance`. Mientras el servicio entró
// como superusuario no se notaba; con su propio rol, sin BYPASSRLS, la consulta
// devolvía cero filas y ningún build se reclamaba nunca, sin un solo error.
func (s *Store) ClaimNextBuild(ctx context.Context, worker string) (ClaimedBuild, bool, error) {
	var cb ClaimedBuild
	found := false
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var id uuid.UUID
		var tenant string
		err := tx.QueryRow(ctx, `
			SELECT id, tenant FROM builds
			WHERE status='queued'
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1`).Scan(&id, &tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmtErr("reclamar build", err)
		}
		// Fija el tenant de la fila reclamada para operar bajo RLS.
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		brow := tx.QueryRow(ctx, `
			UPDATE builds SET status='running', started_at=now(), triggered_by=COALESCE(NULLIF(triggered_by,''),$3)
			WHERE id=$1 AND tenant=$2
			RETURNING `+buildCols, id, tenant, worker)
		b, e := scanBuild(brow, tenant)
		if e != nil {
			return fmtErr("marcar build running", e)
		}
		prow := tx.QueryRow(ctx,
			`SELECT `+projectCols+` FROM projects WHERE tenant=$1 AND id=$2`, tenant, b.ProjectID)
		p, e := scanProject(prow, tenant)
		if e != nil {
			return fmtErr("cargar proyecto de build", e)
		}
		var credsEnc []byte
		var installationID int64
		if err := tx.QueryRow(ctx,
			`SELECT repo_creds_enc, github_installation_id FROM projects WHERE tenant=$1 AND id=$2`, tenant, b.ProjectID).
			Scan(&credsEnc, &installationID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmtErr("cargar credenciales de repo", err)
		}
		cb = ClaimedBuild{Build: b, Project: p, RepoCredsEnc: credsEnc, InstallationID: installationID}
		found = true
		return nil
	})
	return cb, found, err
}

// BuildResult son los datos de finalizaci?n de una build.
type BuildResult struct {
	Status    domain.BuildStatus
	ImageRef  string
	Framework domain.Framework
	LogsKey   string
	ErrorMsg  string
}

// FinishBuild marca una build como terminada (succeeded/failed/canceled). Es
// cross-tenant: el builder pasa el tenant de la build.
func (s *Store) FinishBuild(ctx context.Context, tenant string, id uuid.UUID, res BuildResult) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		fw := string(res.Framework)
		tag, err := tx.Exec(ctx, `
			UPDATE builds SET
				status      = $3,
				image_ref   = COALESCE(NULLIF($4,''), image_ref),
				framework   = COALESCE(NULLIF($5,'auto'), framework),
				logs_key    = COALESCE(NULLIF($6,''), logs_key),
				error_msg   = $7,
				finished_at = now()
			WHERE tenant=$1 AND id=$2`,
			tenant, id, string(res.Status), res.ImageRef, fw, res.LogsKey, res.ErrorMsg)
		if err != nil {
			return fmtErr("finalizar build", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// SetBuildJobName persiste el nombre del Job Kaniko (cross-tenant, builder).
func (s *Store) SetBuildJobName(ctx context.Context, tenant string, id uuid.UUID, jobName string) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`UPDATE builds SET kaniko_job_name=$3 WHERE tenant=$1 AND id=$2`,
			tenant, id, jobName)
		if err != nil {
			return fmtErr("guardar job de build", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// CancelBuild marca una build queued/running como canceled. Devuelve el nombre del
// Job Kaniko si estaba running (para que el API lo elimine).
func (s *Store) CancelBuild(ctx context.Context, id uuid.UUID) (jobName string, err error) {
	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE builds SET status='canceled', finished_at=now(), error_msg='cancelada por el usuario'
			WHERE tenant=$1 AND id=$2 AND status IN ('queued','running')
			RETURNING kaniko_job_name`, t.Slug, id)
		var jn string
		if err := row.Scan(&jn); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrInvalidState
			}
			return fmtErr("cancelar build", err)
		}
		jobName = jn
		return nil
	})
	return jobName, err
}

// BuildStatus devuelve el estado actual de una build (cross-tenant).
func (s *Store) BuildStatus(ctx context.Context, tenant string, id uuid.UUID) (domain.BuildStatus, error) {
	var st domain.BuildStatus
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT status FROM builds WHERE tenant=$1 AND id=$2`, tenant, id).Scan(&st)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return st, err
}

// ActiveWork cuenta, para los topes de cola (CLOUD-06), los builds en cola o en
// curso de la organización y los pull requests con preview viva del proyecto
// (sin contar `pr`, que se está redesplegando).
func (s *Store) ActiveWork(ctx context.Context, projectID uuid.UUID, pr *int) (builds, previews int, err error) {
	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM builds WHERE tenant=$1 AND status IN ('queued','running')),
			(SELECT count(DISTINCT pr_number) FROM deployments
			  WHERE tenant=$1 AND project_id=$2 AND target='preview'
			    AND status IN ('queued','building','deploying','ready')
			    AND pr_number IS DISTINCT FROM $3)`,
			t.Slug, projectID, pr).Scan(&builds, &previews)
	})
	return builds, previews, err
}
