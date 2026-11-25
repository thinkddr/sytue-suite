// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// ResumenFormulario es lo que la bandeja sabe de un formulario sin abrirlo: si está
// encendido, a quién avisa y cuántas respuestas tiene.
type ResumenFormulario struct {
	Enabled bool
	Notify  []domain.StoreNotifyPerson
	Total   int
	// SinLeer son las respuestas que nadie ha marcado como leídas (read_at NULL).
	SinLeer int
}

// Formularios devuelve, en una transacción, el IR en vivo del sitio (nil si no está
// publicado) y el resumen de cada formulario con ajustes o con respuestas. Uno con
// respuestas que ya no está en la versión en vivo sigue saliendo: sus respuestas no se
// pierden por despublicarlo.
func (s *Store) Formularios(ctx context.Context, tenant string, siteID uuid.UUID) ([]byte, map[string]ResumenFormulario, error) {
	var raw []byte
	res := map[string]ResumenFormulario{}
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		var err error
		if raw, err = irPublicado(ctx, tx, tenant, siteID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT coalesce(f.form_id, n.form_id), coalesce(f.enabled, true), coalesce(f.notify, '[]'::jsonb),
			       coalesce(n.total, 0), coalesce(n.sin_leer, 0)
			  FROM (SELECT form_id, enabled, notify FROM form_settings WHERE site_id = $1 AND tenant = $2) f
			  FULL JOIN (SELECT form_id, count(*) AS total, count(*) FILTER (WHERE read_at IS NULL) AS sin_leer
			               FROM form_submissions
			              WHERE site_id = $1 AND tenant = $2 GROUP BY form_id) n ON n.form_id = f.form_id`,
			siteID, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var r ResumenFormulario
			var notify []byte
			if err := rows.Scan(&id, &r.Enabled, &notify, &r.Total, &r.SinLeer); err != nil {
				return err
			}
			_ = json.Unmarshal(notify, &r.Notify)
			res[id] = r
		}
		return rows.Err()
	})
	return raw, res, err
}

// GuardarAvisosFormulario fija a quién avisa un formulario. Crea su fila si aún no la
// tenía (sitios publicados antes de WEB-07) y no toca si está encendido.
func (s *Store) GuardarAvisosFormulario(ctx context.Context, tenant string, siteID uuid.UUID, formID string, notify []domain.StoreNotifyPerson) error {
	if notify == nil {
		notify = []domain.StoreNotifyPerson{}
	}
	raw, err := json.Marshal(notify)
	if err != nil {
		return err
	}
	return s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO form_settings (site_id, tenant, form_id, notify) VALUES ($1, $2, $3, $4)
			ON CONFLICT (site_id, form_id) DO UPDATE SET notify = EXCLUDED.notify`,
			siteID, tenant, formID, raw)
		return err
	})
}

// ListarEnvios devuelve las respuestas de un formulario, de la más nueva a la más vieja.
func (s *Store) ListarEnvios(ctx context.Context, tenant string, siteID uuid.UUID, formID string, limite int) ([]domain.FormSubmission, error) {
	out := []domain.FormSubmission{}
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, data, language, consent_text, consent_at, read_at, created_at
			  FROM form_submissions
			 WHERE site_id = $1 AND tenant = $2 AND form_id = $3
			 ORDER BY created_at DESC
			 LIMIT $4`, siteID, tenant, formID, limite)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			sub := domain.FormSubmission{Tenant: tenant, SiteID: siteID, FormID: formID}
			var data []byte
			if err := rows.Scan(&sub.ID, &data, &sub.Language, &sub.ConsentText, &sub.ConsentAt, &sub.ReadAt, &sub.CreatedAt); err != nil {
				return err
			}
			_ = json.Unmarshal(data, &sub.Data)
			out = append(out, sub)
		}
		return rows.Err()
	})
	return out, err
}

// MarcarEnviosLeidos marca como leídas (o vuelve a dejar sin leer) respuestas de un
// formulario y devuelve cuántas CAMBIARON. Con `ids` vacío y `leidas`, marca todas
// las que quedaban sin leer: es el «marcar todo como leído» de la bandeja.
//
// Marcar no reescribe la hora de una ya leída (coalesce): «leída el martes» sigue
// siendo verdad aunque se vuelva a pulsar el jueves. Y solo toca las filas que de
// verdad cambian, para que el recuento devuelto sea el que la pantalla resta.
func (s *Store) MarcarEnviosLeidos(ctx context.Context, tenant string, siteID uuid.UUID, formID string, ids []uuid.UUID, leidas bool) (int64, error) {
	var n int64
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE form_submissions
			   SET read_at = CASE WHEN $5 THEN coalesce(read_at, now()) ELSE NULL END
			 WHERE site_id = $1 AND tenant = $2 AND form_id = $3
			   AND (cardinality($4::uuid[]) = 0 OR id = ANY($4::uuid[]))
			   AND (read_at IS NULL) = $5`,
			siteID, tenant, formID, ids, leidas)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		return nil
	})
	return n, err
}

// BorrarEnvio borra UNA respuesta de un formulario: lo que se hace cuando quien la
// escribió pide que se borren sus datos (RGPD, art. 17). No hay papelera a propósito:
// una respuesta «borrada» que sigue guardada no está borrada. Devuelve
// ErrFormularioNoExiste si no había nada que borrar con ese id en ese formulario.
func (s *Store) BorrarEnvio(ctx context.Context, tenant string, siteID uuid.UUID, formID string, id uuid.UUID) error {
	return s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM form_submissions
			 WHERE site_id = $1 AND tenant = $2 AND form_id = $3 AND id = $4`,
			siteID, tenant, formID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrFormularioNoExiste
		}
		return nil
	})
}
