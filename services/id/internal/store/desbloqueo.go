// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Los sobres de la clave de desbloqueo (migración 0122, ADR 0084). El almacén no
// sabe abrirlos ni lo intenta: guarda el JSON que el navegador ya cerró, y lo único
// que comprueba es que todos los de una persona lleven la misma huella.

// MaxUnlockEnvelopes es el tope de sobres por persona: uno de contraseña y uno por
// passkey. Pasado el tope, guardar uno nuevo es ErrConflict.
const MaxUnlockEnvelopes = 16

// UnlockEnvelopeRow es un sobre tal como se guardó.
type UnlockEnvelopeRow struct {
	Kind         string
	CredentialID string
	Hint         string
	Envelope     []byte
	UpdatedAt    time.Time
}

// UnlockEnvelopes devuelve los sobres de la persona.
func (s *Store) UnlockEnvelopes(ctx context.Context, userID string) ([]UnlockEnvelopeRow, error) {
	rows, err := s.DB().Query(ctx, `SELECT kind, credential_id, hint, envelope, updated_at
		FROM e2e_unlock_envelopes WHERE user_id = $1 ORDER BY kind, credential_id`, userID)
	if err != nil {
		return nil, errf("UnlockEnvelopes", err)
	}
	defer rows.Close()
	var out []UnlockEnvelopeRow
	for rows.Next() {
		var r UnlockEnvelopeRow
		if err := rows.Scan(&r.Kind, &r.CredentialID, &r.Hint, &r.Envelope, &r.UpdatedAt); err != nil {
			return nil, errf("UnlockEnvelopes", err)
		}
		out = append(out, r)
	}
	return out, errf("UnlockEnvelopes", rows.Err())
}

// PutUnlockEnvelope guarda (o sustituye) el sobre de un factor. Un sobre con otra
// huella que los que ya hay es ErrConflict: sería otra clave de desbloqueo, y los
// envoltorios de Rooms cerrados con la de antes dejarían de abrirse en silencio.
// Solo `reset` (empezar de cero, que la persona confirma en la página) retira los
// demás en la misma transacción y deja este como el único.
func (s *Store) PutUnlockEnvelope(ctx context.Context, userID, kind, credentialID, hint string, envelope []byte, reset bool) error {
	if userID == "" || hint == "" || len(envelope) == 0 || (kind != "password" && kind != "passkey") ||
		(kind == "password") != (credentialID == "") {
		return domain.ErrInvalidInput
	}
	return s.InTx(ctx, func(q pg.Querier) error {
		// Serializa a la persona: dos dispositivos que crean su clave a la vez no
		// pueden acabar con dos huellas.
		if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('e2e_unlock_envelopes:'||$1, 0))`, userID); err != nil {
			return errf("PutUnlockEnvelope", err)
		}
		if reset {
			if _, err := q.Exec(ctx, `DELETE FROM e2e_unlock_envelopes WHERE user_id = $1`, userID); err != nil {
				return errf("PutUnlockEnvelope", err)
			}
		} else {
			var otras int
			if err := q.QueryRow(ctx, `SELECT count(*) FROM e2e_unlock_envelopes WHERE user_id = $1 AND hint <> $2`, userID, hint).Scan(&otras); err != nil {
				return errf("PutUnlockEnvelope", err)
			}
			if otras > 0 {
				return domain.ErrConflict
			}
		}
		if _, err := q.Exec(ctx, `INSERT INTO e2e_unlock_envelopes (user_id, kind, credential_id, hint, envelope, updated_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, now())
			ON CONFLICT (user_id, kind, credential_id) DO UPDATE
			SET hint = excluded.hint, envelope = excluded.envelope, updated_at = excluded.updated_at`,
			userID, kind, credentialID, hint, string(envelope)); err != nil {
			return errf("PutUnlockEnvelope", err)
		}
		var total int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM e2e_unlock_envelopes WHERE user_id = $1`, userID).Scan(&total); err != nil {
			return errf("PutUnlockEnvelope", err)
		}
		if total > MaxUnlockEnvelopes {
			return domain.ErrConflict
		}
		return nil
	})
}
