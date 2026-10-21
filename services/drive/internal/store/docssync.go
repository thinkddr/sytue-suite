// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// Estados de un fichero tal como los entrega la cola hacia Sytue Docs (mig 0027).
const (
	EstadoVivo      = "live"
	EstadoPapelera  = "trashed"
	EstadoPurgado   = "gone"
	pendientesLimit = 500
)

// PendienteDocs es un fichero .odt/.ods/.odp que entró o salió de la papelera, o se
// purgó, con su estado LEÍDO AHORA (no el del momento del cambio).
type PendienteDocs struct {
	Tenant   string
	FileID   uuid.UUID
	OwnerSub string
	Name     string
	Estado   string
	// Encolado es la marca de la fila; solo se borra si sigue siendo esa, así un
	// cambio que llega mientras se entregaba no se pierde.
	Encolado time.Time
}

// PendientesDocs lee hasta `limit` ficheros de la cola, de todas las organizaciones,
// con su estado actual. No los reclama: dos réplicas pueden entregar el mismo, y
// Docs lo aplica igual dos veces (idempotente). Así ninguna transacción queda
// abierta mientras se habla con Docs, que es lo que haría esperar a quien borra.
func (s *Store) PendientesDocs(ctx context.Context, limit int) ([]PendienteDocs, error) {
	if limit <= 0 || limit > pendientesLimit {
		limit = pendientesLimit
	}
	var out []PendienteDocs
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.tenant, p.file_id, coalesce(f.owner_sub, p.owner_sub), coalesce(f.name, p.name),
			       CASE WHEN f.id IS NULL THEN $2::text
			            WHEN f.deleted_at IS NULL THEN $3::text
			            ELSE $4::text END,
			       p.enqueued_at
			  FROM docs_sync_pending p
			  LEFT JOIN files f ON f.tenant = p.tenant AND f.id = p.file_id
			 WHERE p.enqueued_at <= clock_timestamp()
			 ORDER BY p.enqueued_at
			 LIMIT $1`, limit, EstadoPurgado, EstadoVivo, EstadoPapelera)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p PendienteDocs
			if err := rows.Scan(&p.Tenant, &p.FileID, &p.OwnerSub, &p.Name, &p.Estado, &p.Encolado); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: leer la cola hacia Docs: %w", err)
	}
	return out, nil
}

// QuitarPendientesDocs borra de la cola lo ya entregado, solo si la fila no ha
// vuelto a cambiar desde que se leyó.
func (s *Store) QuitarPendientesDocs(ctx context.Context, entregados []PendienteDocs) error {
	if len(entregados) == 0 {
		return nil
	}
	tenants := make([]string, len(entregados))
	ids := make([]uuid.UUID, len(entregados))
	marcas := make([]time.Time, len(entregados))
	for i, p := range entregados {
		tenants[i], ids[i], marcas[i] = p.Tenant, p.FileID, p.Encolado
	}
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			DELETE FROM docs_sync_pending p
			 USING unnest($1::text[], $2::uuid[], $3::timestamptz[]) AS e(tenant, file_id, enqueued_at)
			 WHERE p.tenant = e.tenant AND p.file_id = e.file_id AND p.enqueued_at = e.enqueued_at`,
			tenants, ids, marcas)
		if err != nil {
			return fmt.Errorf("store: vaciar la cola hacia Docs: %w", err)
		}
		return nil
	})
}

// MaxIntentosDocs es cuántas entregas fallidas seguidas aguanta una fila antes de
// darse por perdida (con el aplazamiento, más de un día y medio).
const MaxIntentosDocs = 48

// AplazarPendientesDocs aplaza lo que Docs no aceptó: cada fallo dobla la espera
// (5 s, 10 s, … hasta una hora), y así una organización que falla no se queda en
// cabeza de la cola tapando a las demás. Lo que llega a MaxIntentosDocs se quita y
// se devuelve, para que quien llama lo registre.
func (s *Store) AplazarPendientesDocs(ctx context.Context, fallidos []PendienteDocs) ([]PendienteDocs, error) {
	if len(fallidos) == 0 {
		return nil, nil
	}
	tenants := make([]string, len(fallidos))
	ids := make([]uuid.UUID, len(fallidos))
	marcas := make([]time.Time, len(fallidos))
	for i, p := range fallidos {
		tenants[i], ids[i], marcas[i] = p.Tenant, p.FileID, p.Encolado
	}
	var perdidos []PendienteDocs
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE docs_sync_pending p
			   SET attempts = p.attempts + 1,
			       enqueued_at = clock_timestamp() + make_interval(secs => least(5 * 2 ^ p.attempts, 3600))
			  FROM unnest($1::text[], $2::uuid[], $3::timestamptz[]) AS e(tenant, file_id, enqueued_at)
			 WHERE p.tenant = e.tenant AND p.file_id = e.file_id AND p.enqueued_at = e.enqueued_at`,
			tenants, ids, marcas); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `DELETE FROM docs_sync_pending WHERE attempts >= $1
			RETURNING tenant, file_id, owner_sub, name`, MaxIntentosDocs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p PendienteDocs
			if err := rows.Scan(&p.Tenant, &p.FileID, &p.OwnerSub, &p.Name); err != nil {
				return err
			}
			perdidos = append(perdidos, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: aplazar la cola hacia Docs: %w", err)
	}
	return perdidos, nil
}

// FicheroEnCualquierEstado devuelve un fichero de «Mi unidad» esté vivo o en la
// papelera (DeletedAt lo dice). No ve los de las unidades de equipo (su RLS pide
// `app.team_drive`) ni los adjuntos de canal, que es lo que se quiere: son los mismos
// que la mig 0027 deja fuera de la cola. GetFile solo ve los vivos.
func (s *Store) FicheroEnCualquierEstado(ctx context.Context, id uuid.UUID) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.File
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		f, e = scanFile(tx.QueryRow(ctx,
			`SELECT `+fileCols+` FROM files
			 WHERE tenant=$1 AND id=$2 AND channel_id IS NULL AND team_drive_id IS NULL`,
			t.Slug, id), t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}
