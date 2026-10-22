// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// LA RETENCIÓN LEGAL ES LA CONTRAPARTIDA DE LA PAPELERA QUE CADUCA.
//
// Desde la migración 0005 Sytue Notepads destruye lo borrado pasado su plazo, que es
// lo que toca cuando nadie tiene obligación de conservar. El día que llega un
// litigio o una inspección hace falta poder decir «esto no se borra» SIN apagar
// la purga de toda la instalación, que era lo único que se podía hacer.
//
// Lo que NO hay aquí, dicho para que no se busque: no hay eDiscovery. Drive y
// Mail sí lo tienen —buscar y exportar contenido ajeno—, y en Notes es otra
// decisión, más grande, que empieza por decidir si un admin puede leer las notas
// de alguien. Preservar no es leer: esto solo impide que el recolector destruya.

// CreateHold abre una retención legal y devuelve el hold creado.
//
// `subject` vacío = TODA la organización. Es el caso de una inspección que no
// apunta a nadie en concreto, y el que más se usa al principio de un caso: se
// preserva todo y luego se acota, nunca al revés.
//
// No se comprueba si ya existe otro hold que cubra lo mismo, y es a propósito:
// dos casos distintos pueden preservar a la misma persona, y cerrar uno no puede
// levantar la preservación del otro. Se solapan y no pasa nada — la purga
// pregunta «¿hay ALGÚN hold activo?», no cuántos.
func (s *Store) CreateHold(ctx context.Context, subject, reason, caseRef, actor string) (domain.LegalHold, error) {
	t := tenancy.MustFrom(ctx)
	var h domain.LegalHold
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// `NULLIF(...,'')`: el vacío del API es NULL en la base, que es como la
		// consulta de la purga expresa «cubre a toda la organización».
		row := tx.QueryRow(ctx, `
			INSERT INTO legal_holds (tenant, subject, reason, case_ref, created_by)
			VALUES ($1, NULLIF($2, ''), $3, $4, $5)
			RETURNING id, tenant, COALESCE(subject, ''), reason, case_ref,
			          created_by, created_at, released_at, released_by`,
			t.Slug, subject, reason, caseRef, actor)
		if err := escanearHold(row, &h); err != nil {
			return err
		}
		return writeAuditTx(ctx, tx, t.Slug, actor, domain.AuditLegalHoldCreate, h.ID.String(),
			map[string]any{"sujeto": subject, "caso": caseRef, "motivo": reason})
	})
	return h, err
}

// ListHolds devuelve los holds de la organización, del más reciente al más
// antiguo. Incluye los LIBERADOS a propósito: la pregunta que se le hace a esta
// lista casi siempre es «¿qué se preservó y hasta cuándo?», y un hold que
// desaparece al liberarlo deja esa pregunta sin respuesta.
func (s *Store) ListHolds(ctx context.Context, limit int) ([]domain.LegalHold, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	holds := []domain.LegalHold{}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, COALESCE(subject, ''), reason, case_ref,
			       created_by, created_at, released_at, released_by
			  FROM legal_holds
			 WHERE tenant = $1
			 ORDER BY created_at DESC
			 LIMIT $2`, t.Slug, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		holds = holds[:0]
		for rows.Next() {
			var h domain.LegalHold
			if err := escanearHold(rows, &h); err != nil {
				return err
			}
			holds = append(holds, h)
		}
		return rows.Err()
	})
	return holds, err
}

// ReleaseHold lo levanta. A partir de aquí el contenido que cubría vuelve al
// ciclo normal y el recolector puede destruirlo en la siguiente pasada.
//
// LIBERAR NO BORRA LA FILA. El hold queda con `released_at` puesto, porque la
// pregunta de una auditoría no es «¿qué está preservado ahora?» sino «¿qué
// estuvo preservado, por orden de quién y entre qué fechas?». Borrar la fila
// respondería la primera y haría imposible la segunda.
//
// Liberar dos veces no es un error, pero tampoco reescribe nada: la condición
// `released_at IS NULL` deja la primera liberación —su fecha y su autor— tal
// como estaba, que es la que cuenta.
func (s *Store) ReleaseHold(ctx context.Context, id uuid.UUID, actor string) (domain.LegalHold, error) {
	t := tenancy.MustFrom(ctx)
	var h domain.LegalHold
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE legal_holds
			   SET released_at = now(), released_by = $2
			 WHERE id = $1 AND released_at IS NULL
			RETURNING id, tenant, COALESCE(subject, ''), reason, case_ref,
			          created_by, created_at, released_at, released_by`, id, actor)
		if err := escanearHold(row, &h); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// No se distingue «no existe» de «ya estaba liberado», y es
				// deliberado: quien pregunta es un admin de ESTA organización, la RLS
				// ya le impide ver las de otra, y afinar el mensaje solo serviría
				// para saber si un id existe en otro sitio.
				return domain.ErrNotFound
			}
			return err
		}
		return writeAuditTx(ctx, tx, t.Slug, actor, domain.AuditLegalHoldRelease, h.ID.String(),
			map[string]any{"sujeto": h.Subject, "caso": h.CaseRef})
	})
	return h, err
}

// escanearHold lee una fila de legal_holds en el mismo orden en las tres
// consultas de arriba. Está aquí para que ese orden se escriba una vez: cuando
// se copia, se cambia una columna en un sitio y las otras dos leen torcido.
func escanearHold(row pgx.Row, h *domain.LegalHold) error {
	return row.Scan(&h.ID, &h.Tenant, &h.Subject, &h.Reason, &h.CaseRef,
		&h.CreatedBy, &h.CreatedAt, &h.ReleasedAt, &h.ReleasedBy)
}
