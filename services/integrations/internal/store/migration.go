// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// MigrationRequestedType es el tipo de evento que dispara (o reanuda) un trabajo
// de migración de contenido. Viaja por el MISMO canal que el sync SCIM: un solo
// Dispatcher, que despacha por Event.Type.
const MigrationRequestedType = "integrations.migration.requested"

// MigrationJobRef es la carga útil del evento: qué trabajo ejecutar. La org viaja
// en Event.Tenant (para reconstruir el contexto por-tenant y aplicar RLS).
type MigrationJobRef struct {
	JobID string `json:"job_id"`
}

// NewMigrationJob son los datos de alta de un trabajo. TargetUser es el usuario de
// Sytue (correo o uuid) EN CUYA CUENTA aterriza el contenido: la pasarela canjea un
// token acotado a él y escribe con ese token, no con el de quien lanza el trabajo.
type NewMigrationJob struct {
	InstanceID string
	TargetUser string
	SourceUser string
	Source     domain.MigrationSource
	Domain     domain.MigrationDomain
	// Direction: vacío = 'in' (traer contenido a Sytue).
	Direction domain.MigrationDirection
}

// MaxMigracionesVivas es cuántos trabajos de migración puede tener una
// organización en cola o en marcha a la vez: de sobra para una organización de
// cientos de personas (personas × tipos de contenido), y un techo para la cola
// común.
const MaxMigracionesVivas = 2000

// ErrDemasiadasMigraciones es pasar de MaxMigracionesVivas.
var ErrDemasiadasMigraciones = errors.New("integrations: demasiadas migraciones en marcha")

// CreateMigrationJob da de alta el trabajo y publica su evento ATÓMICAMENTE en la
// misma transacción: si la instancia no existe en la org, no se encola nada.
func (s *Store) CreateMigrationJob(ctx context.Context, in NewMigrationJob) (domain.MigrationJob, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return domain.MigrationJob{}, err
	}

	var job domain.MigrationJob
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_instances WHERE org_id = current_setting('app.tenant', true) AND id = $1)`, in.InstanceID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return domain.ErrNotFound
		}
		// Tope por organización (NEG-20): la cola es de todas, y sin él una sola
		// encolaba trabajos sin fin —basta con repetir «Migrar»— y las demás
		// esperaban detrás. El mismo trabajo vivo dos veces no se encola: es el
		// mismo contenido copiándose a la vez.
		var vivos int
		var repetido bool
		if err := tx.QueryRow(ctx, `
			SELECT count(*),
			       coalesce(bool_or(instance_id = $1 AND target_user = $2 AND source_user = $3
			                        AND domain = $4 AND direction = $5), false)
			  FROM migration_jobs WHERE org_id = $6 AND status IN ('queued', 'running')`,
			in.InstanceID, in.TargetUser, in.SourceUser, string(in.Domain), string(in.Direction.OrIn()), t.Slug).Scan(&vivos, &repetido); err != nil {
			return err
		}
		if repetido {
			return domain.ErrConflict
		}
		if vivos >= MaxMigracionesVivas {
			return ErrDemasiadasMigraciones
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO migration_jobs
			    (org_id, instance_id, target_user, source_user, source, domain, direction)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, created_at`,
			t.Slug, in.InstanceID, in.TargetUser, in.SourceUser, string(in.Source), string(in.Domain),
			string(in.Direction.OrIn()),
		).Scan(&job.ID, &job.CreatedAt); err != nil {
			return err
		}
		ev, err := events.NewEvent(SyncChannel, MigrationRequestedType, t.Slug, MigrationJobRef{JobID: job.ID})
		if err != nil {
			return fmt.Errorf("construir evento de migración: %w", err)
		}
		return events.Publish(ctx, tx, ev)
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrConflict) || errors.Is(err, ErrDemasiadasMigraciones) {
			return domain.MigrationJob{}, err
		}
		return domain.MigrationJob{}, fmt.Errorf("store: crear trabajo de migración: %w", err)
	}
	job.OrgID, job.InstanceID = t.Slug, in.InstanceID
	job.TargetUser, job.SourceUser = in.TargetUser, in.SourceUser
	job.Source, job.Domain = in.Source, in.Domain
	job.Status = domain.MigrationQueued
	return job, nil
}

// LaunchSelfOutboundJob deja listo el trabajo de salida de UNA persona hacia una
// conexión y un contenido: reutiliza el último que ya tenga —con su mapa, así que
// volver a lanzarlo continúa donde se quedó y trae lo nuevo sin duplicar— o lo crea.
//
// Sin evento en el outbox, a diferencia de CreateMigrationJob: este trabajo no lo
// ejecuta el worker con la delegación de la organización, sino quien lo lanza, con
// la entrega de Sytue abierta con su propio token (migrate.RunEntregas). Se
// reconoce por dirección 'out' y source_user vacío.
func (s *Store) LaunchSelfOutboundJob(ctx context.Context, in NewMigrationJob) (domain.MigrationJob, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return domain.MigrationJob{}, err
	}
	var job domain.MigrationJob
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		err := scanMigrationJob(tx.QueryRow(ctx, `
			SELECT `+migrationJobCols+` FROM migration_jobs
			 WHERE org_id = current_setting('app.tenant', true) AND instance_id = $1 AND target_user = $2 AND domain = $3
			   AND direction = 'out' AND source_user = ''
			 ORDER BY created_at DESC LIMIT 1 FOR UPDATE`,
			in.InstanceID, in.TargetUser, string(in.Domain)), &job)
		switch {
		case err == nil:
			if job.Status == domain.MigrationRunning {
				return domain.ErrConflict
			}
			job.Status = domain.MigrationQueued
			_, err = tx.Exec(ctx, `UPDATE migration_jobs SET status = 'queued', last_error = '', finished_at = NULL WHERE org_id = current_setting('app.tenant', true) AND id = $1`, job.ID)
			return err
		case !isNoRows(err):
			return err
		}
		job = domain.MigrationJob{
			OrgID: t.Slug, InstanceID: in.InstanceID, TargetUser: in.TargetUser,
			Source: in.Source, Domain: in.Domain, Direction: domain.DirectionOut, Status: domain.MigrationQueued,
		}
		return tx.QueryRow(ctx, `
			INSERT INTO migration_jobs (org_id, instance_id, target_user, source_user, source, domain, direction)
			SELECT $1, id, $3, '', $4, $5, 'out' FROM app_instances WHERE org_id = $1 AND id = $2
			RETURNING id, created_at`,
			t.Slug, in.InstanceID, in.TargetUser, string(in.Source), string(in.Domain),
		).Scan(&job.ID, &job.CreatedAt)
	})
	switch {
	case isNoRows(err):
		return domain.MigrationJob{}, domain.ErrNotFound
	case errors.Is(err, domain.ErrConflict):
		return domain.MigrationJob{}, err
	case err != nil:
		return domain.MigrationJob{}, fmt.Errorf("store: lanzar salida propia: %w", err)
	}
	return job, nil
}

// migrationJobCols es la proyección compartida por las lecturas de trabajos.
const migrationJobCols = `id, org_id, instance_id, target_user, source_user, source, domain, direction, status,
	                      cursor, migrated_count, skipped_count, error_count, bytes_copied,
	                      last_error, created_at, started_at, finished_at`

func scanMigrationJob(row pgx.Row, job *domain.MigrationJob) error {
	var src, dom, dir, st string
	if err := row.Scan(&job.ID, &job.OrgID, &job.InstanceID, &job.TargetUser, &job.SourceUser,
		&src, &dom, &dir, &st, &job.Cursor, &job.Counts.Migrated, &job.Counts.Skipped,
		&job.Counts.Errors, &job.Counts.Bytes, &job.LastError,
		&job.CreatedAt, &job.StartedAt, &job.FinishedAt); err != nil {
		return err
	}
	job.Source, job.Domain, job.Status = domain.MigrationSource(src), domain.MigrationDomain(dom), domain.MigrationStatus(st)
	job.Direction = domain.MigrationDirection(dir).OrIn()
	return nil
}

// GetMigrationJob devuelve un trabajo por id (RLS por org). Sin el token.
func (s *Store) GetMigrationJob(ctx context.Context, id string) (domain.MigrationJob, error) {
	var job domain.MigrationJob
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return scanMigrationJob(tx.QueryRow(ctx, `SELECT `+migrationJobCols+` FROM migration_jobs WHERE org_id = current_setting('app.tenant', true) AND id = $1`, id), &job)
	})
	if isNoRows(err) {
		return domain.MigrationJob{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.MigrationJob{}, fmt.Errorf("store: leer trabajo de migración: %w", err)
	}
	return job, nil
}

// --- Client delegable de la organización (canje RFC 8693) ---------------------

// Delegate son las credenciales del client DELEGABLE de una org: con ellas la
// pasarela canjea, en Sytue ID, un token acotado a CADA usuario objetivo. Secret
// solo existe en memoria; en reposo va sellado con la KEK.
type Delegate struct {
	ClientID string
	Secret   string
}

// SaveDelegatedClient guarda (o rota) las credenciales del client delegable de la
// org activa. La escribe SOLO la operación de plataforma PUT /operator/delegated-client:
// un admin de organización no puede concederse capacidad de impersonación, igual
// que en Sytue ID `may_impersonate` no lo activa ninguna API.
// ErrNoCipher si no hay KEK: guardar este secreto en claro nunca es aceptable.
func (s *Store) SaveDelegatedClient(ctx context.Context, d Delegate) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	if !s.cipher.Enabled() {
		return ErrNoCipher
	}
	enc, err := s.cipher.Encrypt(d.Secret)
	if err != nil {
		return fmt.Errorf("store: sellar el secreto del client delegable: %w", err)
	}
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO migration_delegates (org_id, client_id, client_secret_enc)
			VALUES ($1, $2, $3)
			ON CONFLICT (org_id) DO UPDATE
			   SET client_id = EXCLUDED.client_id,
			       client_secret_enc = EXCLUDED.client_secret_enc,
			       updated_at = now()`, t.Slug, d.ClientID, enc)
		return err
	})
	if err != nil {
		return fmt.Errorf("store: guardar client delegable: %w", err)
	}
	return nil
}

// DelegatedClient abre las credenciales del client delegable de la org activa —
// la única lectura que las descifra. Solo la usa el worker del outbox para minar
// tokens de impersonación; su resultado jamás se serializa. ErrNotFound si la
// organización aún no tiene delegación aprovisionada.
func (s *Store) DelegatedClient(ctx context.Context) (Delegate, error) {
	var d Delegate
	var enc string
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT client_id, client_secret_enc FROM migration_delegates WHERE org_id = current_setting('app.tenant', true)`).Scan(&d.ClientID, &enc)
	})
	if isNoRows(err) {
		return Delegate{}, domain.ErrNotFound
	}
	if err != nil {
		return Delegate{}, fmt.Errorf("store: leer client delegable: %w", err)
	}
	if d.Secret, err = s.cipher.Decrypt(enc); err != nil {
		return Delegate{}, fmt.Errorf("store: abrir el secreto del client delegable: %w", err)
	}
	return d, nil
}

// ListMigrationJobs devuelve los trabajos de la org, más reciente primero
// (índice keyset migration_jobs_org_created). limit acota la página.
func (s *Store) ListMigrationJobs(ctx context.Context, limit int) ([]domain.MigrationJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := make([]domain.MigrationJob, 0, limit)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+migrationJobCols+`
			  FROM migration_jobs WHERE org_id = current_setting('app.tenant', true) ORDER BY created_at DESC, id DESC LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var job domain.MigrationJob
			if err := scanMigrationJob(rows, &job); err != nil {
				return err
			}
			out = append(out, job)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar trabajos de migración: %w", err)
	}
	return out, nil
}

// ListMigrationJobsForTarget devuelve los trabajos cuyo contenido aterriza en la cuenta
// de UNA persona.
//
// Filtra por destino y no por quien los creó: cuando un administrador migra la
// organización entera, cada persona necesita ver la suya. La verja no está en este
// filtro sino en el llamante, que solo pasa el sub de quien pregunta.
func (s *Store) ListMigrationJobsForTarget(ctx context.Context, targetUser string, limit int) ([]domain.MigrationJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := make([]domain.MigrationJob, 0, limit)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+migrationJobCols+`
			  FROM migration_jobs WHERE org_id = current_setting('app.tenant', true) AND target_user = $1
			  ORDER BY created_at DESC, id DESC LIMIT $2`, targetUser, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var job domain.MigrationJob
			if err := scanMigrationJob(rows, &job); err != nil {
				return err
			}
			out = append(out, job)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar mis trabajos de migración: %w", err)
	}
	return out, nil
}

// StartMigrationJob marca el trabajo como 'running' SOLO si estaba encolado
// (queued/error): así una cancelación mientras esperaba en el outbox no se pisa.
// Devuelve ErrNotFound si no hay nada que arrancar.
func (s *Store) StartMigrationJob(ctx context.Context, id string) error {
	return s.execAffecting(ctx, "arrancar trabajo de migración", `
		UPDATE migration_jobs
		   SET status = 'running', started_at = coalesce(started_at, now()), last_error = ''
		 WHERE org_id = current_setting('app.tenant', true) AND id = $1 AND status IN ('queued', 'error')`, id)
}

// FinishMigrationJob cierra el trabajo con su estado final y el último error
// legible (vacío si terminó bien). No toca un trabajo ya cancelado.
func (s *Store) FinishMigrationJob(ctx context.Context, id string, st domain.MigrationStatus, lastErr string) error {
	return s.execAffecting(ctx, "cerrar trabajo de migración", `
		UPDATE migration_jobs
		   SET status = $2, last_error = $3, finished_at = now()
		 WHERE org_id = current_setting('app.tenant', true) AND id = $1 AND status <> 'canceled'`, id, string(st), truncateErr(lastErr))
}

// CancelMigrationJob cancela un trabajo aún vivo. El cursor se conserva: cancelar
// no borra lo ya migrado ni impide reanudar más tarde.
func (s *Store) CancelMigrationJob(ctx context.Context, id string) error {
	return s.execAffecting(ctx, "cancelar trabajo de migración", `
		UPDATE migration_jobs
		   SET status = 'canceled', finished_at = now()
		 WHERE org_id = current_setting('app.tenant', true) AND id = $1 AND status IN ('queued', 'running')`, id)
}

// ResumeMigrationJob reanuda un trabajo detenido (error/canceled) y vuelve a
// encolarlo. El cursor y el mapa intactos ⇒ continúa donde se quedó sin repetir
// nada. No necesita token: el de escritura se canjea al vuelo por usuario.
func (s *Store) ResumeMigrationJob(ctx context.Context, id string) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	ev, err := events.NewEvent(SyncChannel, MigrationRequestedType, t.Slug, MigrationJobRef{JobID: id})
	if err != nil {
		return fmt.Errorf("store: construir evento de migración: %w", err)
	}
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE migration_jobs
			   SET status = 'queued', last_error = '', finished_at = NULL
			 WHERE org_id = current_setting('app.tenant', true) AND id = $1 AND status IN ('error', 'canceled', 'done')`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return events.Publish(ctx, tx, ev)
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("store: reanudar trabajo de migración: %w", err)
	}
	return nil
}

// SaveMigrationProgress persiste el checkpoint de un lote (cursor + contadores) y
// devuelve el estado VIVO del trabajo en la misma ida y vuelta: así el worker se
// entera de una cancelación sin una consulta extra por lote. No pisa un trabajo
// que ya no esté corriendo.
func (s *Store) SaveMigrationProgress(ctx context.Context, id, cursor string, c domain.MigrationCounts) (domain.MigrationStatus, error) {
	var st string
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE migration_jobs
			   SET cursor = $2, migrated_count = $3, skipped_count = $4,
			       error_count = $5, bytes_copied = $6
			 WHERE org_id = current_setting('app.tenant', true) AND id = $1 AND status = 'running'`,
			id, cursor, c.Migrated, c.Skipped, c.Errors, c.Bytes); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT status FROM migration_jobs WHERE org_id = current_setting('app.tenant', true) AND id = $1`, id).Scan(&st)
	})
	if isNoRows(err) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: guardar progreso de migración: %w", err)
	}
	return domain.MigrationStatus(st), nil
}

// LoadMigrationMap carga EN BLOQUE el mapa de un trabajo (id de origen → id en
// Sytue + motivo de omisión). El runner lo indexa en memoria para decidir sin N+1.
func (s *Store) LoadMigrationMap(ctx context.Context, jobID string) ([]domain.MigrationItem, error) {
	out := make([]domain.MigrationItem, 0, 256)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT source_id, kind, sytue_id, item_hash, path, skip_reason FROM migration_map WHERE org_id = current_setting('app.tenant', true) AND job_id = $1`, jobID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				it   domain.MigrationItem
				kind string
			)
			if err := rows.Scan(&it.SourceID, &kind, &it.SytueID, &it.Hash, &it.Path, &it.SkipReason); err != nil {
				return err
			}
			it.Kind = domain.MigrationKind(kind)
			out = append(out, it)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: cargar mapa de migración: %w", err)
	}
	return out, nil
}

// RecordMigrationItems inserta/actualiza un LOTE del mapa en UNA sola sentencia
// (unnest, sin N+1). Idempotente por (job_id, source_id).
func (s *Store) RecordMigrationItems(ctx context.Context, jobID string, items []domain.MigrationItem) error {
	if len(items) == 0 {
		return nil
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	n := len(items)
	srcIDs, kinds := make([]string, n), make([]string, n)
	sytueIDs, hashes := make([]string, n), make([]string, n)
	paths, reasons := make([]string, n), make([]string, n)
	for i, it := range items {
		srcIDs[i], kinds[i] = it.SourceID, string(it.Kind)
		sytueIDs[i], hashes[i] = it.SytueID, it.Hash
		paths[i], reasons[i] = it.Path, it.SkipReason
	}
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO migration_map (job_id, org_id, source_id, kind, sytue_id, item_hash, path, skip_reason)
			SELECT $1, $2, u.source_id, u.kind, u.sytue_id, u.item_hash, u.path, u.skip_reason
			  FROM unnest($3::text[], $4::text[], $5::text[], $6::text[], $7::text[], $8::text[])
			       AS u(source_id, kind, sytue_id, item_hash, path, skip_reason)
			ON CONFLICT (job_id, source_id) DO UPDATE
			   SET kind = EXCLUDED.kind, sytue_id = EXCLUDED.sytue_id,
			       item_hash = EXCLUDED.item_hash, path = EXCLUDED.path,
			       skip_reason = EXCLUDED.skip_reason, updated_at = now()`,
			jobID, t.Slug, srcIDs, kinds, sytueIDs, hashes, paths, reasons)
		return err
	})
	if err != nil {
		return fmt.Errorf("store: registrar entradas del mapa de migración: %w", err)
	}
	return nil
}

// ListMigrationSkipped devuelve las entidades OMITIDAS de un trabajo con su
// motivo — el informe de honestidad de GET /migrations/{id}.
func (s *Store) ListMigrationSkipped(ctx context.Context, jobID string, limit int) ([]domain.MigrationItem, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := make([]domain.MigrationItem, 0, limit)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT source_id, kind, path, skip_reason
			  FROM migration_map
			 WHERE org_id = current_setting('app.tenant', true) AND job_id = $1 AND skip_reason <> ''
			 ORDER BY source_id
			 LIMIT $2`, jobID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				it   domain.MigrationItem
				kind string
			)
			if err := rows.Scan(&it.SourceID, &kind, &it.Path, &it.SkipReason); err != nil {
				return err
			}
			it.Kind = domain.MigrationKind(kind)
			out = append(out, it)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar omitidos de la migración: %w", err)
	}
	return out, nil
}

// truncateErr acota el mensaje de error persistido (last_error es para leerlo, no
// para volcar un stack).
func truncateErr(s string) string {
	const maxLen = 500
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
