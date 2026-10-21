// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// MaxPaginaScripts es el tope de filas de cualquier listado de scripts o de auditoría.
const MaxPaginaScripts = 200

const colsScript = `s.id, s.name, s.kind, s.scope, coalesce(s.document_id::text, ''), s.author_sub, s.created_at,
	coalesce((SELECT max(version) FROM doc_script_versions v WHERE v.tenant = s.tenant AND v.script_id = s.id), 0),
	coalesce((SELECT max(version) FROM doc_script_versions v WHERE v.tenant = s.tenant AND v.script_id = s.id AND v.status = 'approved'), 0)`

func scanScript(row pgx.Row, s *domain.Script) error {
	return row.Scan(&s.ID, &s.Name, &s.Kind, &s.Scope, &s.DocumentID, &s.AuthorSub, &s.CreatedAt, &s.LatestVersion, &s.ApprovedVersion)
}

const colsVersionScript = `script_id, version, code, code_hash, manifest, status, created_by, created_at, coalesce(reviewed_by, ''), reviewed_at`

func scanVersionScript(row pgx.Row, v *domain.VersionScript) error {
	var m []byte
	if err := row.Scan(&v.ScriptID, &v.Version, &v.Code, &v.CodeHash, &m, &v.Status, &v.CreatedBy, &v.CreatedAt, &v.ReviewedBy, &v.ReviewedAt); err != nil {
		return err
	}
	v.Manifest = m
	return nil
}

func errScript(msg string, err error) error {
	if isNoRows(err) {
		return domain.ErrNotFound
	}
	return fmt.Errorf("store: %s: %w", msg, err)
}

// CrearScript guarda un script con su versión 1 (en borrador). `manifiesto` ya es el canónico.
func (s *Store) CrearScript(ctx context.Context, sc domain.Script, codigo string, manifiesto []byte) (domain.Script, domain.VersionScript, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return sc, domain.VersionScript{}, err
	}
	var v domain.VersionScript
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var doc any
		if sc.DocumentID != "" {
			doc = sc.DocumentID
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO doc_scripts (tenant, name, kind, scope, document_id, author_sub)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`,
			t.Slug, sc.Name, sc.Kind, sc.Scope, doc, sc.AuthorSub).Scan(&sc.ID, &sc.CreatedAt); err != nil {
			return err
		}
		return scanVersionScript(tx.QueryRow(ctx, `
			INSERT INTO doc_script_versions (tenant, script_id, version, code, code_hash, manifest, created_by)
			VALUES ($1, $2, 1, $3, $4, $5, $6) RETURNING `+colsVersionScript,
			t.Slug, sc.ID, codigo, domain.HashCodigoScript(codigo), manifiesto, sc.AuthorSub), &v)
	})
	if err != nil {
		return sc, v, fmt.Errorf("store: crear script: %w", err)
	}
	sc.LatestVersion = 1
	return sc, v, nil
}

// ListarScripts devuelve los scripts que `sub` puede ver, el más reciente primero: los
// suyos y los de la organización con una versión aprobada; un administrador ve además
// todos los de la organización (para revisarlos). Los privados de otra persona no los
// ve nadie, tampoco un administrador.
func (s *Store) ListarScripts(ctx context.Context, sub string, admin bool, limite int) ([]domain.Script, error) {
	limite = min(max(limite, 1), MaxPaginaScripts)
	out := []domain.Script{}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+colsScript+` FROM doc_scripts s
			WHERE s.deleted_at IS NULL AND (
				s.author_sub = $1
				OR (s.scope = 'org' AND ($2 OR EXISTS (
					SELECT 1 FROM doc_script_versions v
					WHERE v.tenant = s.tenant AND v.script_id = s.id AND v.status = 'approved'))))
			ORDER BY s.created_at DESC, s.id LIMIT $3`, sub, admin, limite)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sc domain.Script
			if err := scanScript(rows, &sc); err != nil {
				return err
			}
			out = append(out, sc)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar scripts: %w", err)
	}
	return out, nil
}

// ObtenerScript lee un script vivo. SIN filtrar por visibilidad: quien llama decide.
func (s *Store) ObtenerScript(ctx context.Context, id string) (domain.Script, error) {
	var sc domain.Script
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return scanScript(tx.QueryRow(ctx, `SELECT `+colsScript+` FROM doc_scripts s WHERE s.id = $1 AND s.deleted_at IS NULL`, id), &sc)
	})
	if err != nil {
		return sc, errScript("obtener script", err)
	}
	return sc, nil
}

// VersionesScript lista las versiones (la más reciente primero), con código.
func (s *Store) VersionesScript(ctx context.Context, id string, limite int) ([]domain.VersionScript, error) {
	limite = min(max(limite, 1), MaxPaginaScripts)
	out := []domain.VersionScript{}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+colsVersionScript+` FROM doc_script_versions WHERE script_id = $1 ORDER BY version DESC LIMIT $2`, id, limite)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.VersionScript
			if err := scanVersionScript(rows, &v); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: versiones de script: %w", err)
	}
	return out, nil
}

// ObtenerVersionScript lee una versión concreta.
func (s *Store) ObtenerVersionScript(ctx context.Context, id string, version int) (domain.VersionScript, error) {
	var v domain.VersionScript
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return scanVersionScript(tx.QueryRow(ctx, `SELECT `+colsVersionScript+` FROM doc_script_versions WHERE script_id = $1 AND version = $2`, id, version), &v)
	})
	if err != nil {
		return v, errScript("obtener versión", err)
	}
	return v, nil
}

// NuevaVersionScript añade la siguiente versión, SIEMPRE en borrador: editar no hereda
// la aprobación de la anterior. Un script borrado no admite versiones.
func (s *Store) NuevaVersionScript(ctx context.Context, id, codigo string, manifiesto []byte, sub string) (domain.VersionScript, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return domain.VersionScript{}, err
	}
	var v domain.VersionScript
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		// Dos guardados a la vez del mismo script no pueden repetir número.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('doc_script:' || $1))`, id); err != nil {
			return err
		}
		var vivo int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM doc_scripts WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&vivo); err != nil {
			return err
		}
		return scanVersionScript(tx.QueryRow(ctx, `
			INSERT INTO doc_script_versions (tenant, script_id, version, code, code_hash, manifest, created_by)
			VALUES ($1, $2, (SELECT coalesce(max(version), 0) + 1 FROM doc_script_versions WHERE script_id = $2), $3, $4, $5, $6)
			RETURNING `+colsVersionScript, t.Slug, id, codigo, domain.HashCodigoScript(codigo), manifiesto, sub), &v)
	})
	if err != nil {
		return v, errScript("nueva versión de script", err)
	}
	return v, nil
}

// CambiarAmbitoScript pasa un script de privado a de la organización o al revés.
func (s *Store) CambiarAmbitoScript(ctx context.Context, id, ambito string) error {
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE doc_scripts SET scope = $2 WHERE id = $1 AND deleted_at IS NULL`, id, ambito)
		if err == nil && tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return err
	})
}

// BorrarScript lo retira (borrado lógico; el registro de ejecuciones se conserva).
func (s *Store) BorrarScript(ctx context.Context, id string) error {
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE doc_scripts SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return err
	})
}

// ProponerVersionScript pasa una versión de borrador (o rechazada) a pendiente de revisión.
func (s *Store) ProponerVersionScript(ctx context.Context, id string, version int) error {
	return s.transicionVersion(ctx, id, version, []string{domain.EstadoBorrador, domain.EstadoRechazada}, domain.EstadoPendiente, "", "")
}

// RevisarVersionScript aprueba o rechaza una versión PENDIENTE. La aprobación lleva el
// hash del código que el administrador revisó: si no es el de la versión, no se aplica.
func (s *Store) RevisarVersionScript(ctx context.Context, id string, version int, hash string, aprobar bool, sub string) error {
	hacia := domain.EstadoRechazada
	if aprobar {
		hacia = domain.EstadoAprobada
	}
	return s.transicionVersion(ctx, id, version, []string{domain.EstadoPendiente}, hacia, sub, hash)
}

// transicionVersion cambia el estado de una versión si está en uno de `desde`. `hash`
// vacío = no se comprueba; revisor vacío = se borra la revisión.
func (s *Store) transicionVersion(ctx context.Context, id string, version int, desde []string, hacia, revisor, hash string) error {
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		var viva int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM doc_scripts WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&viva); err != nil {
			return errScript("script", err)
		}
		var h, estado string
		if err := tx.QueryRow(ctx, `SELECT code_hash, status FROM doc_script_versions WHERE script_id = $1 AND version = $2 FOR UPDATE`, id, version).Scan(&h, &estado); err != nil {
			return errScript("versión", err)
		}
		if hash != "" && hash != h {
			return domain.ErrHashNoCoincide
		}
		ok := false
		for _, d := range desde {
			ok = ok || d == estado
		}
		if !ok {
			return domain.ErrConflictoEstadoScript
		}
		var rev any
		if revisor != "" {
			rev = revisor
		}
		_, err := tx.Exec(ctx, `
			UPDATE doc_script_versions
			SET status = $3, reviewed_by = $4::text, reviewed_at = CASE WHEN $4::text IS NULL THEN NULL ELSE now() END
			WHERE script_id = $1 AND version = $2`, id, version, hacia, rev)
		return err
	})
}

// ReservarEjecucion comprueba las cuotas y abre la línea de auditoría en la MISMA
// transacción, serializada por organización: dos ejecuciones simultáneas no pueden
// colarse las dos por el último hueco. Devuelve el id de la ejecución.
func (s *Store) ReservarEjecucion(ctx context.Context, e domain.EjecucionScript, maxMs int) (int64, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('doc_script_runs:' || $1))`, t.Slug); err != nil {
			return err
		}
		uso, err := usoCuota(ctx, tx, e.Sub)
		if err != nil {
			return err
		}
		if err := domain.DecidirCuota(uso, maxMs); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO doc_script_runs (tenant, script_id, version, code_hash, document_id, sub, max_ms)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			t.Slug, e.ScriptID, e.Version, e.CodeHash, e.DocumentID, e.Sub, maxMs).Scan(&id)
	})
	if errors.Is(err, domain.ErrCuotaScripts) {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("store: reservar ejecución: %w", err)
	}
	return id, nil
}

// ReservarDenegada deja constancia de un intento que el servidor rechazó. No cuenta para
// la cuota (un rechazo no ejecuta nada) y no lleva más que el código del motivo.
// ponytail: sin tope propio; lo acota el limitador de peticiones de la plataforma.
func (s *Store) ReservarDenegada(ctx context.Context, e domain.EjecucionScript, codigo string) (int64, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO doc_script_runs (tenant, script_id, version, code_hash, document_id, sub, max_ms, outcome, error_code, duration_ms)
			VALUES ($1, $2, $3, $4, $5, $6, 1, 'denied', $7, 0) RETURNING id`,
			t.Slug, e.ScriptID, e.Version, e.CodeHash, e.DocumentID, e.Sub, codigo).Scan(&id)
	})
	return id, err
}

func usoCuota(ctx context.Context, tx pgx.Tx, sub string) (domain.UsoCuota, error) {
	var uso domain.UsoCuota
	err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE sub = $1), count(*),
		       coalesce(sum(coalesce(duration_ms, max_ms)) FILTER (WHERE sub = $1), 0)::bigint
		FROM doc_script_runs WHERE started_at > now() - interval '1 hour' AND outcome <> 'denied'`, sub).
		Scan(&uso.EjecucionesUsuario, &uso.EjecucionesOrg, &uso.MsUsuario)
	return uso, err
}

// UsoCuotaScripts devuelve lo consumido en la última hora (para mostrarlo).
func (s *Store) UsoCuotaScripts(ctx context.Context, sub string) (domain.UsoCuota, error) {
	var uso domain.UsoCuota
	err := s.tenantTx(ctx, func(tx pgx.Tx) (err error) {
		uso, err = usoCuota(ctx, tx, sub)
		return err
	})
	return uso, err
}

// CerrarEjecucion anota el resultado, UNA sola vez y solo por quien la abrió.
func (s *Store) CerrarEjecucion(ctx context.Context, id int64, sub, resultado, codigoError string, ms, ops int) error {
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE doc_script_runs SET outcome = $3, error_code = left($4, 64), duration_ms = $5, ops = $6
			WHERE id = $1 AND sub = $2 AND outcome = 'running'`, id, sub, resultado, codigoError, ms, ops)
		if err == nil && tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return err
	})
}

// EjecucionesScript pagina el registro, de la más reciente a la más antigua (id < antesDe;
// 0 = desde el final). Con docID, solo las de ese documento.
func (s *Store) EjecucionesScript(ctx context.Context, docID string, antesDe int64, limite int) ([]domain.EjecucionScript, error) {
	limite = min(max(limite, 1), MaxPaginaScripts)
	out := []domain.EjecucionScript{}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, script_id, version, code_hash, document_id, sub, outcome, error_code, duration_ms, ops, started_at
			FROM doc_script_runs
			WHERE ($1 = '' OR document_id::text = $1) AND ($2::bigint = 0 OR id < $2)
			ORDER BY id DESC LIMIT $3`, docID, antesDe, limite)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e domain.EjecucionScript
			if err := rows.Scan(&e.ID, &e.ScriptID, &e.Version, &e.CodeHash, &e.DocumentID, &e.Sub, &e.Outcome, &e.ErrorCode, &e.DurationMs, &e.Ops, &e.StartedAt); err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: ejecuciones de scripts: %w", err)
	}
	return out, nil
}
