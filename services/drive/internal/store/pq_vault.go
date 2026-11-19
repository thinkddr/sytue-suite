// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// PQVaultState es el estado público de la bóveda post-cuántica de una persona
// para un producto: identificador lógico de bóveda, clave activa y envoltorios.
type PQVaultState struct {
	VaultID     string                        `json:"vault_id"`
	ActiveKeyID string                        `json:"active_key_id"`
	Keys        []recoverywrap.PQVaultWrapper `json:"keys"`
}

// PQVaultRecord es un registro cifrado (solo texto cifrado PQ) de una bóveda.
type PQVaultRecord struct {
	EventID   string          `json:"event_id"`
	VaultID   string          `json:"vault_id"`
	Seq       int64           `json:"seq"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// PQVaultRecordPage es una página cronológica inversa de registros cifrados.
type PQVaultRecordPage struct {
	Entries    []PQVaultRecord `json:"entries"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

// GetPQVault devuelve el estado de la bóveda PQ para `(tenant, owner_sub, product)`.
// Si todavía no se ha creado ninguna clave, devuelve `VaultID == ""` y lista vacía.
func (s *Store) GetPQVault(ctx context.Context, product string) (PQVaultState, error) {
	out := PQVaultState{Keys: []recoverywrap.PQVaultWrapper{}}
	owner := OwnerFrom(ctx)
	if !storage.Product(product) || owner == "" {
		return out, domain.ErrPermission
	}
	t := tenancy.MustFrom(ctx)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT vault_id, active_key_id FROM pq_vaults WHERE tenant=$1 AND owner_sub=$2 AND product=$3`,
			t.Slug, owner, product,
		).Scan(&out.VaultID, &out.ActiveKeyID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT wrapper_json FROM pq_vault_keys WHERE tenant=$1 AND owner_sub=$2 AND product=$3 ORDER BY updated_at DESC, key_id ASC`,
			t.Slug, owner, product,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			var w recoverywrap.PQVaultWrapper
			if err := json.Unmarshal(raw, &w); err != nil {
				return err
			}
			out.Keys = append(out.Keys, w)
		}
		return rows.Err()
	})
	return out, err
}

// PutPQVaultKey registra o reenvuelve una clave pública/envuelta en la bóveda PQ.
// Crea la bóveda con un identificador `!pqv_<base32>` en el primer alta. Si
// `reset` es true, sustituye las claves previas y crea un nuevo `vault_id` limpio.
func (s *Store) PutPQVaultKey(ctx context.Context, product string, wrapper recoverywrap.PQVaultWrapper, setActive, reset bool) (PQVaultState, error) {
	out := PQVaultState{Keys: []recoverywrap.PQVaultWrapper{}}
	owner := OwnerFrom(ctx)
	if !storage.Product(product) || owner == "" {
		return out, domain.ErrPermission
	}
	if err := wrapper.Validate(); err != nil {
		return out, domain.ErrInvalidName
	}
	rawWrapper, err := json.Marshal(wrapper)
	if err != nil {
		return out, err
	}
	t := tenancy.MustFrom(ctx)
	newVaultID := "!pqv_" + rand.Text()
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if reset {
			if _, err := tx.Exec(ctx,
				`DELETE FROM pq_vault_keys WHERE tenant=$1 AND owner_sub=$2 AND product=$3`,
				t.Slug, owner, product,
			); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx,
				`INSERT INTO pq_vaults(tenant,owner_sub,product,vault_id,active_key_id)
				 VALUES($1,$2,$3,$4,$5)
				 ON CONFLICT (tenant,owner_sub,product) DO UPDATE
				 SET vault_id=EXCLUDED.vault_id, active_key_id=EXCLUDED.active_key_id, updated_at=now()
				 RETURNING vault_id, active_key_id`,
				t.Slug, owner, product, newVaultID, wrapper.KeyID,
			).Scan(&out.VaultID, &out.ActiveKeyID); err != nil {
				return err
			}
		} else {
			if err := tx.QueryRow(ctx,
				`INSERT INTO pq_vaults(tenant,owner_sub,product,vault_id,active_key_id)
				 VALUES($1,$2,$3,$4,$5)
				 ON CONFLICT (tenant,owner_sub,product) DO UPDATE
				 SET active_key_id=CASE WHEN $6 THEN EXCLUDED.active_key_id ELSE pq_vaults.active_key_id END,
				     updated_at=now()
				 RETURNING vault_id, active_key_id`,
				t.Slug, owner, product, newVaultID, wrapper.KeyID, setActive,
			).Scan(&out.VaultID, &out.ActiveKeyID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO pq_vault_keys(tenant,owner_sub,product,key_id,profile,recipient,wrapper_json)
			 VALUES($1,$2,$3,$4,$5,$6,$7)
			 ON CONFLICT (tenant,owner_sub,product,key_id) DO UPDATE
			 SET wrapper_json=EXCLUDED.wrapper_json, updated_at=now()`,
			t.Slug, owner, product, wrapper.KeyID, wrapper.Profile, wrapper.Recipient, rawWrapper,
		); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT wrapper_json FROM pq_vault_keys WHERE tenant=$1 AND owner_sub=$2 AND product=$3 ORDER BY updated_at DESC, key_id ASC`,
			t.Slug, owner, product,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			var w recoverywrap.PQVaultWrapper
			if err := json.Unmarshal(raw, &w); err != nil {
				return err
			}
			out.Keys = append(out.Keys, w)
		}
		return rows.Err()
	})
	return out, err
}

// AppendPQVaultRecord inserta un registro cifrado de forma idempotente por
// `transaction_id`. Repetir el mismo `transaction_id` con distinto cuerpo es
// `domain.ErrConflict`.
func (s *Store) AppendPQVaultRecord(ctx context.Context, product, vaultID, transactionID string, payload json.RawMessage) (PQVaultRecord, error) {
	var out PQVaultRecord
	owner := OwnerFrom(ctx)
	if !storage.Product(product) || owner == "" {
		return out, domain.ErrPermission
	}
	t := tenancy.MustFrom(ctx)
	eventID := "$pqe_" + rand.Text()
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var currentVaultID string
		err := tx.QueryRow(ctx,
			`SELECT vault_id FROM pq_vaults WHERE tenant=$1 AND owner_sub=$2 AND product=$3`,
			t.Slug, owner, product,
		).Scan(&currentVaultID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if currentVaultID != vaultID {
			return domain.ErrConflict
		}
		err = tx.QueryRow(ctx,
			`INSERT INTO pq_vault_records(tenant,owner_sub,product,vault_id,event_id,transaction_id,payload_json)
			 VALUES($1,$2,$3,$4,$5,$6,$7)
			 ON CONFLICT (tenant,owner_sub,product,transaction_id) DO NOTHING
			 RETURNING event_id, vault_id, seq, payload_json, created_at`,
			t.Slug, owner, product, vaultID, eventID, transactionID, []byte(payload),
		).Scan(&out.EventID, &out.VaultID, &out.Seq, &out.Payload, &out.CreatedAt)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// Ya existía `transaction_id`: admitir solo si bóveda y cuerpo son idénticos.
		var existingPayload []byte
		if err := tx.QueryRow(ctx,
			`SELECT event_id, vault_id, seq, payload_json, created_at
			 FROM pq_vault_records
			 WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND transaction_id=$4`,
			t.Slug, owner, product, transactionID,
		).Scan(&out.EventID, &out.VaultID, &out.Seq, &existingPayload, &out.CreatedAt); err != nil {
			return err
		}
		if out.VaultID != vaultID || !jsonEqual(existingPayload, payload) {
			return domain.ErrConflict
		}
		out.Payload = existingPayload
		return nil
	})
	return out, err
}

// GetPQVaultRecord devuelve un registro cifrado concreto por `event_id`.
func (s *Store) GetPQVaultRecord(ctx context.Context, product, vaultID, eventID string) (PQVaultRecord, error) {
	var out PQVaultRecord
	owner := OwnerFrom(ctx)
	if !storage.Product(product) || owner == "" {
		return out, domain.ErrPermission
	}
	t := tenancy.MustFrom(ctx)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT event_id, vault_id, seq, payload_json, created_at
			 FROM pq_vault_records
			 WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND vault_id=$4 AND event_id=$5`,
			t.Slug, owner, product, vaultID, eventID,
		).Scan(&out.EventID, &out.VaultID, &out.Seq, &out.Payload, &out.CreatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrNotFound
	}
	return out, err
}

// ListPQVaultRecords lista registros en orden cronológico inverso (`seq DESC`),
// con paginación por cursor `beforeSeq` (0 = desde el más reciente).
func (s *Store) ListPQVaultRecords(ctx context.Context, product, vaultID string, beforeSeq int64, limit int) (PQVaultRecordPage, error) {
	page := PQVaultRecordPage{Entries: []PQVaultRecord{}}
	owner := OwnerFrom(ctx)
	if !storage.Product(product) || owner == "" {
		return page, domain.ErrPermission
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	t := tenancy.MustFrom(ctx)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var (
			rows pgx.Rows
			err  error
		)
		if beforeSeq > 0 {
			rows, err = tx.Query(ctx,
				`SELECT event_id, vault_id, seq, payload_json, created_at
				 FROM pq_vault_records
				 WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND vault_id=$4 AND seq < $5
				 ORDER BY seq DESC
				 LIMIT $6`,
				t.Slug, owner, product, vaultID, beforeSeq, limit+1,
			)
		} else {
			rows, err = tx.Query(ctx,
				`SELECT event_id, vault_id, seq, payload_json, created_at
				 FROM pq_vault_records
				 WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND vault_id=$4
				 ORDER BY seq DESC
				 LIMIT $5`,
				t.Slug, owner, product, vaultID, limit+1,
			)
		}
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var rec PQVaultRecord
			if err := rows.Scan(&rec.EventID, &rec.VaultID, &rec.Seq, &rec.Payload, &rec.CreatedAt); err != nil {
				return err
			}
			page.Entries = append(page.Entries, rec)
		}
		return rows.Err()
	})
	if err != nil {
		return page, err
	}
	if len(page.Entries) > limit {
		page.Entries = page.Entries[:limit]
		page.NextCursor = strconv.FormatInt(page.Entries[len(page.Entries)-1].Seq, 10)
	}
	return page, nil
}

func jsonEqual(a, b []byte) bool {
	decode := func(raw []byte) (any, bool) {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, false
		}
		return v, true
	}
	va, okA := decode(a)
	vb, okB := decode(b)
	if !okA || !okB {
		return false
	}
	return reflect.DeepEqual(va, vb)
}
