// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

const deployCols = `id, tenant, project_id, build_id, target, status, commit_sha, commit_message,
	git_ref, pr_number, url, replicas, created_by, created_at, updated_at, ready_at, error_msg, image_ref`

// deployDests son los destinos de `deployCols`, EN SU ORDEN. Mismo motivo que
// projectDests: tenerlos escritos a mano en cada consulta es lo que dejó fuera
// `commit_message` en la de los deployments pendientes.
func deployDests(d *domain.Deployment) []any {
	return []any{
		&d.ID, &d.Tenant, &d.ProjectID, &d.BuildID, &d.Target, &d.Status,
		&d.CommitSHA, &d.CommitMessage, &d.GitRef, &d.PRNumber, &d.URL, &d.Replicas,
		&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.ReadyAt, &d.ErrorMsg, &d.ImageRef,
	}
}

func scanDeployment(row pgx.Row, t string) (domain.Deployment, error) {
	var d domain.Deployment
	if err := row.Scan(deployDests(&d)...); err != nil {
		return domain.Deployment{}, err
	}
	d.Tenant = t
	return d, nil
}

// NewDeploymentRecord son los datos para crear un deployment.
type NewDeploymentRecord struct {
	ProjectID     uuid.UUID
	BuildID       *uuid.UUID
	Target        domain.DeployTarget
	CommitSHA     string
	CommitMessage string // ya saneado por domain.SanitizeCommitMessage
	GitRef        string
	PRNumber      *int
	URL           string
	Replicas      int
	CreatedBy     string
	ImageRef      string // imagen externa BYO (sin build)
}

// CreateDeployment inserta un deployment 'queued'. Para previews, retira el
// preview activo previo del mismo PR (un activo por PR).
func (s *Store) CreateDeployment(ctx context.Context, rec NewDeploymentRecord) (domain.Deployment, error) {
	t := tenancy.MustFrom(ctx)
	var d domain.Deployment
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		d, e = createDeploymentTx(ctx, tx, t.Slug, rec)
		return e
	})
	return d, err
}

// createDeploymentTx es el cuerpo transaccional de CreateDeployment (ver
// createProjectTx: lo comparte el alta atómica desde plantilla).
func createDeploymentTx(
	ctx context.Context, tx pgx.Tx, tenant string, rec NewDeploymentRecord,
) (domain.Deployment, error) {
	if rec.Replicas <= 0 {
		rec.Replicas = 1
	}
	if rec.Target == domain.TargetPreview && rec.PRNumber != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE deployments SET status='retired', updated_at=now()
			 WHERE tenant=$1 AND project_id=$2 AND target='preview' AND pr_number=$3
			   AND status NOT IN ('retired','canceled','error')`,
			tenant, rec.ProjectID, *rec.PRNumber); err != nil {
			return domain.Deployment{}, fmtErr("retirar preview previo", err)
		}
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO deployments (tenant, project_id, build_id, target, commit_sha, commit_message,
			git_ref, pr_number, url, replicas, created_by, status, image_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'queued', $12)
		RETURNING `+deployCols,
		tenant, rec.ProjectID, rec.BuildID, string(rec.Target), rec.CommitSHA, rec.CommitMessage,
		rec.GitRef, rec.PRNumber, rec.URL, rec.Replicas, rec.CreatedBy, rec.ImageRef)
	d, err := scanDeployment(row, tenant)
	if checkViolation(err) {
		return domain.Deployment{}, domain.ErrInvalidInput
	}
	return d, err
}

// GetDeployment devuelve un deployment por ID.
func (s *Store) GetDeployment(ctx context.Context, id uuid.UUID) (domain.Deployment, error) {
	t := tenancy.MustFrom(ctx)
	var d domain.Deployment
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT `+deployCols+` FROM deployments WHERE tenant=$1 AND id=$2`, t.Slug, id)
		var e error
		d, e = scanDeployment(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Deployment{}, domain.ErrNotFound
	}
	return d, err
}

// DeploymentPage es una página de deployments con el cursor para la siguiente.
type DeploymentPage struct {
	Deployments []domain.Deployment
	// Next es el cursor de continuación; nil si no hay más resultados.
	Next *PageCursor
}

// ListDeployments lista los deployments de un proyecto (más recientes primero)
// con paginación keyset por (created_at, id) DESC. Devuelve hasta `limit`
// deployments y, si hay más, el cursor de continuación. Evita OFFSET: el
// predicado (created_at, id) < ($ts, $id) recorre el índice compuesto
// deployments_project_created_id_idx.
func (s *Store) ListDeployments(ctx context.Context, projectID uuid.UUID, after PageCursor, limit int) (DeploymentPage, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var page DeploymentPage
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Pedimos limit+1 para saber si existe página siguiente sin COUNT extra.
		var rows pgx.Rows
		var err error
		if after.IsZero() {
			rows, err = tx.Query(ctx,
				`SELECT `+deployCols+` FROM deployments WHERE tenant=$1 AND project_id=$2
				 ORDER BY created_at DESC, id DESC LIMIT $3`,
				t.Slug, projectID, limit+1)
		} else {
			rows, err = tx.Query(ctx,
				`SELECT `+deployCols+` FROM deployments WHERE tenant=$1 AND project_id=$2
				   AND (created_at, id) < ($3, $4)
				 ORDER BY created_at DESC, id DESC LIMIT $5`,
				t.Slug, projectID, after.CreatedAt, after.ID, limit+1)
		}
		if err != nil {
			return fmtErr("listar deployments", err)
		}
		defer rows.Close()
		for rows.Next() {
			d, err := scanDeployment(rows, t.Slug)
			if err != nil {
				return err
			}
			page.Deployments = append(page.Deployments, d)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(page.Deployments) > limit {
			last := page.Deployments[limit-1]
			page.Deployments = page.Deployments[:limit]
			page.Next = &PageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		}
		return nil
	})
	if err != nil {
		return DeploymentPage{}, err
	}
	return page, nil
}

// GetActivePreview devuelve el deployment de preview vivo (no retirado ni
// cancelado) de un PR del proyecto. Como mucho hay uno no terminal por
// (proyecto, PR) — lo garantiza deployments_active_preview_idx.
func (s *Store) GetActivePreview(ctx context.Context, projectID uuid.UUID, pr int) (domain.Deployment, error) {
	t := tenancy.MustFrom(ctx)
	var d domain.Deployment
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+deployCols+` FROM deployments
			 WHERE tenant=$1 AND project_id=$2 AND target='preview' AND pr_number=$3
			   AND status NOT IN ('retired','canceled')
			 ORDER BY created_at DESC LIMIT 1`, t.Slug, projectID, pr)
		var e error
		d, e = scanDeployment(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Deployment{}, domain.ErrNotFound
	}
	return d, err
}

// GetActiveProduction devuelve el deployment de producción activo del proyecto.
func (s *Store) GetActiveProduction(ctx context.Context, projectID uuid.UUID) (domain.Deployment, error) {
	t := tenancy.MustFrom(ctx)
	var d domain.Deployment
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+deployCols+` FROM deployments
			 WHERE tenant=$1 AND project_id=$2 AND target='production' AND status='ready'
			 ORDER BY created_at DESC LIMIT 1`, t.Slug, projectID)
		var e error
		d, e = scanDeployment(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Deployment{}, domain.ErrNotFound
	}
	return d, err
}

// Promote marca un deployment de producción como activo (ready) y retira las
// producciones anteriores del mismo proyecto. Sirve de rollback: promociona un
// deployment 'ready' anterior. Devuelve el deployment promovido.
func (s *Store) Promote(ctx context.Context, id uuid.UUID) (domain.Deployment, error) {
	t := tenancy.MustFrom(ctx)
	var d domain.Deployment
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Carga el deployment objetivo y comprueba que es producción.
		row := tx.QueryRow(ctx, `SELECT `+deployCols+` FROM deployments WHERE tenant=$1 AND id=$2`, t.Slug, id)
		target, e := scanDeployment(row, t.Slug)
		if e != nil {
			return normalizeNoRows(e, domain.ErrNotFound)
		}
		if target.Target != domain.TargetProduction {
			return domain.ErrInvalidState
		}
		// Retira las demás producciones ready del proyecto.
		if _, err := tx.Exec(ctx,
			`UPDATE deployments SET status='retired', updated_at=now()
			 WHERE tenant=$1 AND project_id=$2 AND target='production' AND status='ready' AND id<>$3`,
			t.Slug, target.ProjectID, id); err != nil {
			return fmtErr("retirar producciones", err)
		}
		prow := tx.QueryRow(ctx, `
			UPDATE deployments SET status='ready', ready_at=COALESCE(ready_at, now()), updated_at=now()
			WHERE tenant=$1 AND id=$2
			RETURNING `+deployCols, t.Slug, id)
		d, e = scanDeployment(prow, t.Slug)
		if e != nil {
			return e
		}
		// Producción viva (deploy inicial ready o rollback): notifica ready por el
		// bus, en la MISMA tx (outbox transaccional). El dedupe (deployment_id,
		// status) del worker evita reenviar si se re-promociona el mismo deployment.
		return publishDeployEvent(ctx, tx, t.Slug, id, EventDeployReady)
	})
	return d, err
}

// CancelDeployment cancela un deployment vivo (lo retira el controller).
func (s *Store) CancelDeployment(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE deployments SET status='canceled', updated_at=now()
			 WHERE tenant=$1 AND id=$2 AND status NOT IN ('retired','canceled')`, t.Slug, id)
		if err != nil {
			return fmtErr("cancelar deployment", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// PendingDeployment es un deployment a reconciliar con su proyecto.
type PendingDeployment struct {
	Deployment domain.Deployment
	Project    domain.Project
}

// ListPendingDeployments toma hasta `limit` deployments no terminales (cualquier
// tenant) para que el controller los reconcilie. No los bloquea de forma
// exclusiva entre ciclos; la idempotencia del reconciliador lo permite. Devuelve
// cada deployment junto a su proyecto. Cross-tenant: corre bajo
// pg.InMaintenanceTx (habilita las políticas PERMISSIVE de deployments y projects
// que 0002/0003 usan para el reconciliador); con FORCE RLS sería deny-by-default.
func (s *Store) ListPendingDeployments(ctx context.Context, limit int) ([]PendingDeployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []PendingDeployment
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+prefixCols("d", deployCols)+`,
			       `+prefixCols("p", projectCols)+`
			FROM deployments d
			JOIN projects p ON p.id = d.project_id AND p.tenant = d.tenant
			WHERE d.status IN ('queued','building','deploying') AND p.deleted_at IS NULL
			ORDER BY d.updated_at
			LIMIT $1`, limit)
		if err != nil {
			return fmtErr("listar deployments pendientes", err)
		}
		defer rows.Close()
		for rows.Next() {
			pd, err := scanPendingDeployment(rows)
			if err != nil {
				return err
			}
			out = append(out, pd)
		}
		return rows.Err()
	})
	return out, err
}

// scanPendingDeployment escanea una fila (deployment + su proyecto) de las
// consultas cross-tenant del reconciliador. Conserva el tenant real de cada
// columna (no lo sobreescribe: el listado mezcla varios tenants).
func scanPendingDeployment(rows pgx.Rows) (PendingDeployment, error) {
	var d domain.Deployment
	var p domain.Project
	// La consulta selecciona deployCols seguidas de projectCols, así que los
	// destinos son exactamente esas dos listas concatenadas. Escribirlos a mano
	// aquí es lo que las descuadró en su día.
	if err := rows.Scan(append(deployDests(&d), projectDests(&p)...)...); err != nil {
		return PendingDeployment{}, err
	}
	return PendingDeployment{Deployment: d, Project: p}, nil
}

// RetirableDeployment identifica un deployment terminado cuyos objetos en el
// clúster deben eliminarse. El namespace se deriva del tenant (tenant-<slug>).
type RetirableDeployment struct {
	ID     uuid.UUID
	Tenant string
	Target domain.DeployTarget
}

// ListRetirableDeployments devuelve los deployments terminados (canceled por
// cierre de PR o borrado de proyecto; retired por promoción/rollback) cuyos
// objetos (Deployment/Service/Ingress/HPA/Secret) aún no se han desmontado
// (torn_down_at IS NULL), para que el controller los retire y no queden
// huérfanos. Cross-tenant vía pg.InMaintenanceTx (deployments tiene su política
// de mantenimiento en 0002); con FORCE RLS sería deny-by-default (0 filas). El
// controller fija torn_down_at tras RetireApp, así que cada deployment sale del
// conjunto una vez desmontado: el LIMIT pagina el backlog (FIFO por updated_at)
// sin dejar huérfanos aunque se terminen >LIMIT deployments dentro de una ventana.
func (s *Store) ListRetirableDeployments(ctx context.Context, limit int) ([]RetirableDeployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []RetirableDeployment
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, target FROM deployments
			WHERE status IN ('canceled','retired') AND torn_down_at IS NULL
			ORDER BY updated_at LIMIT $1`, limit)
		if err != nil {
			return fmtErr("listar deployments retirables", err)
		}
		defer rows.Close()
		for rows.Next() {
			var r RetirableDeployment
			if err := rows.Scan(&r.ID, &r.Tenant, &r.Target); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// ListReadyProductions devuelve las producciones 'ready' modificadas dentro de la
// ventana `since`, junto a su proyecto vivo, ordenadas por updated_at DESC (las
// conmutaciones más recientes primero). El controller deduplica por proyecto: la
// primera fila de cada proyecto (la más reciente) es el estado deseado a
// (re)aplicar y las siguientes, producciones a retirar. Cross-tenant vía
// pg.InMaintenanceTx (habilita las políticas de mantenimiento de deployments y
// projects); con FORCE RLS sería deny-by-default. La ventana evita reaplicar en
// cada ciclo las producciones estables (cuello a escala); deploy y Promote/
// rollback bumpean updated_at, así que las conmutaciones caen dentro y el LIMIT
// acota las más recientes (el conjunto accionable), no un subconjunto
// determinista por project_id que dejaría rollbacks sin reaplicar jamás.
func (s *Store) ListReadyProductions(ctx context.Context, since time.Time, limit int) ([]PendingDeployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []PendingDeployment
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+prefixCols("d", deployCols)+`,
			       `+prefixCols("p", projectCols)+`
			FROM deployments d
			JOIN projects p ON p.id = d.project_id AND p.tenant = d.tenant
			WHERE d.target='production' AND d.status='ready'
			  AND d.updated_at >= $1 AND p.deleted_at IS NULL
			ORDER BY d.updated_at DESC
			LIMIT $2`, since, limit)
		if err != nil {
			return fmtErr("listar producciones ready", err)
		}
		defer rows.Close()
		for rows.Next() {
			pd, err := scanPendingDeployment(rows)
			if err != nil {
				return err
			}
			out = append(out, pd)
		}
		return rows.Err()
	})
	return out, err
}

// SetDeploymentStatus actualiza el estado de un deployment (cross-tenant, lo usa
// el controller pasando el tenant). url/error son opcionales.
func (s *Store) SetDeploymentStatus(ctx context.Context, tenant string, id uuid.UUID, status domain.DeployStatus, url, errMsg string) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		ready := status == domain.DeployReady
		tag, err := tx.Exec(ctx, `
			UPDATE deployments SET
				status    = $3,
				url       = COALESCE(NULLIF($4,''), url),
				error_msg = $5,
				ready_at  = CASE WHEN $6 THEN COALESCE(ready_at, now()) ELSE ready_at END,
				updated_at = now()
			WHERE tenant=$1 AND id=$2`, tenant, id, string(status), url, errMsg, ready)
		if err != nil {
			return fmtErr("actualizar estado deployment", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		// Notifica la transición terminal (ready de preview / error) por el bus,
		// en la MISMA tx (outbox transaccional: el aviso solo viaja si el cambio
		// hace commit). El ready de PRODUCCIÓN pasa por Promote (no por aquí), que
		// emite su propio aviso; building/deploying/retired/canceled no notifican.
		switch status {
		case domain.DeployReady:
			return publishDeployEvent(ctx, tx, tenant, id, EventDeployReady)
		case domain.DeployError:
			return publishDeployEvent(ctx, tx, tenant, id, EventDeployError)
		default:
			// queued/building/deploying/retired/canceled: no notifican.
			return nil
		}
	})
}

// MarkDeploymentTornDown fija torn_down_at tras desmontar los objetos del
// deployment (cross-tenant, lo llama el controller pasando el tenant). Así el
// deployment sale del conjunto de ListRetirableDeployments y no se vuelve a
// intentar retirar. Idempotente: si la fila ya no existe o ya estaba marcada, es
// un no-op (no exige RowsAffected).
func (s *Store) MarkDeploymentTornDown(ctx context.Context, tenant string, id uuid.UUID) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE deployments SET torn_down_at=now() WHERE tenant=$1 AND id=$2`, tenant, id); err != nil {
			return fmtErr("marcar deployment desmontado", err)
		}
		return nil
	})
}

// prefixCols antepone "<alias>." a cada columna de una lista separada por comas.
func prefixCols(alias, cols string) string {
	var b []byte
	field := make([]byte, 0, 32)
	flush := func() {
		f := trimWS(string(field))
		if f != "" {
			if len(b) > 0 {
				b = append(b, ',', ' ')
			}
			b = append(b, alias...)
			b = append(b, '.')
			b = append(b, f...)
		}
		field = field[:0]
	}
	for i := 0; i < len(cols); i++ {
		if cols[i] == ',' {
			flush()
			continue
		}
		field = append(field, cols[i])
	}
	flush()
	return string(b)
}

func trimWS(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\n' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\n' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
