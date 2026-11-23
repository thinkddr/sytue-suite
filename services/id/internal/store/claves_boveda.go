// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// El registro de las claves de firma de las bóvedas (migración 0123, ADR 0087).
// El almacén no verifica firmas: guarda lo que el servicio ya decidió, y garantiza
// que la decisión se toma sobre la clave activa de ESE momento (bloqueo por bóveda).

// MaxVaultKeyGenerations acota el historial de una bóveda: pasado el tope, una
// clave nueva es ErrConflict. Una persona no vacía su correo cien veces.
const MaxVaultKeyGenerations = 100

// VaultKeyRow es una generación de la clave de firma de una bóveda.
type VaultKeyRow struct {
	Generation    int64
	KeyID         string
	PreviousKeyID string
	PublicKey     []byte // JSON de recoverywrap.PQVaultVerifyKey
	Reason        string
	CreatedAt     time.Time
	RetiredAt     *time.Time
}

// VaultKeyChange es lo que el servicio decide guardar como generación siguiente.
type VaultKeyChange struct {
	KeyID         string
	PreviousKeyID string
	PublicKey     []byte
	Reason        string
}

// ErrVaultKeyUnchanged lo devuelve la decisión cuando la clave pedida ya es la
// activa: no hay nada que guardar y la petición se contesta con la que hay.
var ErrVaultKeyUnchanged = errors.New("store: la clave ya es la activa")

// VaultKeys devuelve el historial de una bóveda, de la generación más reciente a
// la más antigua. Vacío si nunca se registró ninguna.
func (s *Store) VaultKeys(ctx context.Context, userID, tenant, product string) ([]VaultKeyRow, error) {
	return vaultKeys(ctx, s.DB(), userID, tenant, product)
}

func vaultKeys(ctx context.Context, q pg.Querier, userID, tenant, product string) ([]VaultKeyRow, error) {
	rows, err := q.Query(ctx, `SELECT generation, key_id, previous_key_id, public_key, reason, created_at, retired_at
		FROM e2e_vault_keys WHERE user_id = $1 AND tenant = $2 AND product = $3
		ORDER BY generation DESC`, userID, tenant, product)
	if err != nil {
		return nil, errf("VaultKeys", err)
	}
	defer rows.Close()
	var out []VaultKeyRow
	for rows.Next() {
		var r VaultKeyRow
		if err := rows.Scan(&r.Generation, &r.KeyID, &r.PreviousKeyID, &r.PublicKey, &r.Reason, &r.CreatedAt, &r.RetiredAt); err != nil {
			return nil, errf("VaultKeys", err)
		}
		out = append(out, r)
	}
	return out, errf("VaultKeys", rows.Err())
}

// ActiveVaultKey es la clave activa de una bóveda; domain.ErrNotFound si no hay.
func (s *Store) ActiveVaultKey(ctx context.Context, userID, tenant, product string) (*VaultKeyRow, error) {
	return activeVaultKey(ctx, s.DB(), userID, tenant, product)
}

func activeVaultKey(ctx context.Context, q pg.Querier, userID, tenant, product string) (*VaultKeyRow, error) {
	var r VaultKeyRow
	err := q.QueryRow(ctx, `SELECT generation, key_id, previous_key_id, public_key, reason, created_at, retired_at
		FROM e2e_vault_keys WHERE user_id = $1 AND tenant = $2 AND product = $3 AND retired_at IS NULL`,
		userID, tenant, product).
		Scan(&r.Generation, &r.KeyID, &r.PreviousKeyID, &r.PublicKey, &r.Reason, &r.CreatedAt, &r.RetiredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, errf("ActiveVaultKey", err)
	}
	return &r, nil
}

// RegisterVaultKey guarda la generación siguiente de la clave de una bóveda.
// `decide` recibe la activa (nil si no hay) DENTRO de la transacción, con la
// bóveda bloqueada, y devuelve el cambio o un error: así dos dispositivos que
// registran a la vez no deciden los dos sobre la misma clave. Devuelve la fila
// activa al terminar; con ErrVaultKeyUnchanged, la que ya estaba, sin error.
func (s *Store) RegisterVaultKey(ctx context.Context, userID, tenant, product string,
	decide func(actual *VaultKeyRow) (VaultKeyChange, error),
) (VaultKeyRow, error) {
	var out VaultKeyRow
	if userID == "" || tenant == "" || product == "" {
		return out, domain.ErrInvalidInput
	}
	err := s.InTx(ctx, func(q pg.Querier) error {
		if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('e2e_vault_keys:'||$1||':'||$2||':'||$3, 0))`,
			userID, tenant, product); err != nil {
			return errf("RegisterVaultKey", err)
		}
		actual, err := activeVaultKey(ctx, q, userID, tenant, product)
		if errors.Is(err, domain.ErrNotFound) {
			actual, err = nil, nil
		}
		if err != nil {
			return err
		}
		cambio, err := decide(actual)
		if errors.Is(err, ErrVaultKeyUnchanged) && actual != nil {
			out = *actual
			return nil
		}
		if err != nil {
			return err
		}
		generacion := int64(1)
		if actual != nil {
			generacion = actual.Generation + 1
			if generacion > MaxVaultKeyGenerations {
				return domain.ErrConflict
			}
			if _, err := q.Exec(ctx, `UPDATE e2e_vault_keys SET retired_at = now()
				WHERE user_id = $1 AND tenant = $2 AND product = $3 AND generation = $4`,
				userID, tenant, product, actual.Generation); err != nil {
				return errf("RegisterVaultKey", err)
			}
		}
		return errf("RegisterVaultKey", q.QueryRow(ctx, `INSERT INTO e2e_vault_keys
			(user_id, tenant, product, generation, key_id, previous_key_id, public_key, reason)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
			RETURNING generation, key_id, previous_key_id, public_key, reason, created_at, retired_at`,
			userID, tenant, product, generacion, cambio.KeyID, cambio.PreviousKeyID, string(cambio.PublicKey), cambio.Reason).
			Scan(&out.Generation, &out.KeyID, &out.PreviousKeyID, &out.PublicKey, &out.Reason, &out.CreatedAt, &out.RetiredAt))
	})
	return out, err
}
