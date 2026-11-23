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
// para un producto: identificador lógico de bóveda, clave activa, envoltorios,
// versión y, si la hay, la marca de copia en curso.
type PQVaultState struct {
	VaultID     string                        `json:"vault_id"`
	ActiveKeyID string                        `json:"active_key_id"`
	Keys        []recoverywrap.PQVaultWrapper `json:"keys"`
	// Version sube con cada registro nuevo y con cada vaciado. Vaciar exige la que
	// vio el cliente (PQVaultExpect).
	Version int64        `json:"version"`
	Copy    *PQVaultCopy `json:"copy,omitempty"`
}

// PQVaultCopy es la marca «copia en curso» de una bóveda (ADR 0086, «pasa Matrix
// a ella»): la pone un cliente nativo antes de copiar en ella el Drive de Matrix.
// Mientras está activa, solo ese cliente publica en la bóveda o la vacía. Caduca
// si no se renueva, y se puede cancelar.
type PQVaultCopy struct {
	ID        string    `json:"id"`
	Holder    string    `json:"holder"`
	Kind      string    `json:"kind"`
	Epoch     string    `json:"epoch"`
	StartedAt time.Time `json:"started_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Active    bool      `json:"active"`
}

// PQVaultExpect es lo que el cliente vio de la bóveda al decidir vaciarla. El
// vaciado solo se hace si sigue igual (compare-and-swap).
type PQVaultExpect struct {
	VaultID string `json:"vault_id"`
	Version int64  `json:"version"`
	// Unchecked: el cliente no dijo qué vio (nativos v3.22 y anteriores). Se vacía
	// sin comparar, como antes; la marca de copia activa de otro sigue mandando.
	Unchecked bool `json:"-"`
}

// PQVaultCopyClaim pide (o renueva) la marca de copia en curso.
type PQVaultCopyClaim struct {
	ID     string
	Holder string
	Kind   string
	Epoch  string
	TTL    time.Duration
}

type pqVaultCopyKey struct{}

// WithPQVaultCopy dice en `ctx` con qué marca de copia escribe quien llama: la
// que puso él. Sin ella, una marca activa de otro rechaza sus escrituras.
func WithPQVaultCopy(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, pqVaultCopyKey{}, id)
}

// PQVaultCopyFrom es la marca de copia que trae `ctx` (vacía si no trae ninguna).
func PQVaultCopyFrom(ctx context.Context) string {
	id, _ := ctx.Value(pqVaultCopyKey{}).(string)
	return id
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
		var err error
		out, err = readPQVault(ctx, tx, t.Slug, owner, product, "")
		return err
	})
	return out, err
}

// readPQVault lee el estado entero de la bóveda (con `lock`, la fila bloqueada).
// Sin bóveda, devuelve el estado vacío.
func readPQVault(ctx context.Context, tx pgx.Tx, tenant, owner, product, lock string) (PQVaultState, error) {
	out := PQVaultState{Keys: []recoverywrap.PQVaultWrapper{}}
	var (
		copyID, holder, kind, epoch *string
		started, expires            *time.Time
		active                      *bool
	)
	err := tx.QueryRow(ctx,
		`SELECT vault_id, active_key_id, version, copy_id, copy_holder, copy_kind, copy_epoch,
		        copy_started_at, copy_expires_at, copy_expires_at > now()
		 FROM pq_vaults WHERE tenant=$1 AND owner_sub=$2 AND product=$3 `+lock,
		tenant, owner, product,
	).Scan(&out.VaultID, &out.ActiveKeyID, &out.Version, &copyID, &holder, &kind, &epoch, &started, &expires, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if copyID != nil && holder != nil && kind != nil && epoch != nil && started != nil && expires != nil && active != nil {
		out.Copy = &PQVaultCopy{ID: *copyID, Holder: *holder, Kind: *kind, Epoch: *epoch, StartedAt: *started, ExpiresAt: *expires, Active: *active}
	}
	out.Keys, err = pqVaultKeys(ctx, tx, tenant, owner, product)
	return out, err
}

// PutPQVaultKey registra o reenvuelve una clave pública/envuelta en la bóveda PQ.
// Crea la bóveda con un identificador `!pqv_<base32>` en el primer alta. Con
// `reset`, la vacía (borra registros y claves) y la estrena con un `vault_id`
// nuevo, pero solo si sigue siendo la que vio el cliente (mismo identificador y
// misma versión: nadie publicó nada después, ni un recibo) y si nadie más tiene
// una marca de copia activa. Si no, ErrPQVaultChanged o ErrPQVaultCopyInProgress
// y no se toca nada. La comprobación y el vaciado van con la fila bloqueada: una
// publicación concurrente espera o se ve.
func (s *Store) PutPQVaultKey(ctx context.Context, product string, wrapper recoverywrap.PQVaultWrapper, setActive bool, reset *PQVaultExpect) (PQVaultState, error) {
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
	mine := PQVaultCopyFrom(ctx)
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if reset != nil {
			current, err := readPQVault(ctx, tx, t.Slug, owner, product, "FOR UPDATE")
			if err != nil {
				return err
			}
			if !reset.Unchecked && (current.VaultID != reset.VaultID || current.Version != reset.Version) {
				return domain.ErrPQVaultChanged
			}
			if current.Copy != nil && current.Copy.Active && current.Copy.ID != mine {
				return domain.ErrPQVaultCopyInProgress
			}
			if err := vaciarPQVault(ctx, tx, t.Slug, owner, product); err != nil {
				return err
			}
		}
		// Con `reset` la bóveda estrena identificador, sube de versión y la clave
		// nueva queda activa; conserva la marca de copia solo si es de quien vacía.
		// Sin él conserva todo y solo cambia la activa si se pide.
		if _, err := tx.Exec(ctx,
			`INSERT INTO pq_vaults(tenant,owner_sub,product,vault_id,active_key_id)
			 VALUES($1,$2,$3,$4,$5)
			 ON CONFLICT (tenant,owner_sub,product) DO UPDATE
			 SET vault_id=CASE WHEN $7 THEN EXCLUDED.vault_id ELSE pq_vaults.vault_id END,
			     active_key_id=CASE WHEN $6 OR $7 THEN EXCLUDED.active_key_id ELSE pq_vaults.active_key_id END,
			     version=CASE WHEN $7 THEN pq_vaults.version+1 ELSE pq_vaults.version END,
			     copy_id=CASE WHEN $7 AND pq_vaults.copy_id IS DISTINCT FROM $8 THEN NULL ELSE pq_vaults.copy_id END,
			     copy_holder=CASE WHEN $7 AND pq_vaults.copy_id IS DISTINCT FROM $8 THEN NULL ELSE pq_vaults.copy_holder END,
			     copy_kind=CASE WHEN $7 AND pq_vaults.copy_id IS DISTINCT FROM $8 THEN NULL ELSE pq_vaults.copy_kind END,
			     copy_epoch=CASE WHEN $7 AND pq_vaults.copy_id IS DISTINCT FROM $8 THEN NULL ELSE pq_vaults.copy_epoch END,
			     copy_started_at=CASE WHEN $7 AND pq_vaults.copy_id IS DISTINCT FROM $8 THEN NULL ELSE pq_vaults.copy_started_at END,
			     copy_expires_at=CASE WHEN $7 AND pq_vaults.copy_id IS DISTINCT FROM $8 THEN NULL ELSE pq_vaults.copy_expires_at END,
			     updated_at=now()`,
			t.Slug, owner, product, newVaultID, wrapper.KeyID, setActive, reset != nil, mine,
		); err != nil {
			return err
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
		out, err = readPQVault(ctx, tx, t.Slug, owner, product, "")
		return err
	})
	return out, err
}

// ClaimPQVaultCopy pone o renueva la marca de copia en curso. La tiene quien la
// puso hasta que caduca (`TTL` desde la última renovación) o se retira: otra
// distinta y activa es ErrPQVaultCopyInProgress. Sin bóveda, ErrNotFound.
func (s *Store) ClaimPQVaultCopy(ctx context.Context, product string, claim PQVaultCopyClaim) (PQVaultState, error) {
	out := PQVaultState{Keys: []recoverywrap.PQVaultWrapper{}}
	owner := OwnerFrom(ctx)
	if !storage.Product(product) || owner == "" {
		return out, domain.ErrPermission
	}
	t := tenancy.MustFrom(ctx)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := readPQVault(ctx, tx, t.Slug, owner, product, "FOR NO KEY UPDATE")
		if err != nil {
			return err
		}
		if current.VaultID == "" {
			return domain.ErrNotFound
		}
		if current.Copy != nil && current.Copy.Active && current.Copy.ID != claim.ID {
			return domain.ErrPQVaultCopyInProgress
		}
		if _, err := tx.Exec(ctx,
			`UPDATE pq_vaults
			 SET copy_started_at=CASE WHEN copy_id=$4 THEN copy_started_at ELSE now() END,
			     copy_id=$4, copy_holder=$5, copy_kind=$6, copy_epoch=$7,
			     copy_expires_at=now() + make_interval(secs => $8), updated_at=now()
			 WHERE tenant=$1 AND owner_sub=$2 AND product=$3`,
			t.Slug, owner, product, claim.ID, claim.Holder, claim.Kind, claim.Epoch, claim.TTL.Seconds(),
		); err != nil {
			return err
		}
		out, err = readPQVault(ctx, tx, t.Slug, owner, product, "")
		return err
	})
	return out, err
}

// ReleasePQVaultCopy retira la marca de copia `id`: la retira quien la puso al
// terminar, o la cancela la persona desde otro cliente. Sin marca no hace nada;
// con otra distinta, ErrPQVaultChanged (la que se quería retirar ya no está).
// Nunca borra registros: la bóveda queda como esté.
func (s *Store) ReleasePQVaultCopy(ctx context.Context, product, id string) (PQVaultState, error) {
	out := PQVaultState{Keys: []recoverywrap.PQVaultWrapper{}}
	owner := OwnerFrom(ctx)
	if !storage.Product(product) || owner == "" {
		return out, domain.ErrPermission
	}
	t := tenancy.MustFrom(ctx)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := readPQVault(ctx, tx, t.Slug, owner, product, "FOR NO KEY UPDATE")
		if err != nil {
			return err
		}
		if current.Copy == nil {
			out = current
			return nil
		}
		if current.Copy.ID != id {
			return domain.ErrPQVaultChanged
		}
		if _, err := tx.Exec(ctx,
			`UPDATE pq_vaults
			 SET copy_id=NULL, copy_holder=NULL, copy_kind=NULL, copy_epoch=NULL,
			     copy_started_at=NULL, copy_expires_at=NULL, updated_at=now()
			 WHERE tenant=$1 AND owner_sub=$2 AND product=$3`,
			t.Slug, owner, product,
		); err != nil {
			return err
		}
		out, err = readPQVault(ctx, tx, t.Slug, owner, product, "")
		return err
	})
	return out, err
}

// vaciarPQVault borra los registros y las claves de la bóveda. Vaciar es vaciar:
// los registros de la bóveda anterior ya no los abre nadie. Y `transaction_id` es
// único por (tenant, owner, product), no por bóveda: si se quedaran, una
// reimportación con los mismos identificadores chocaría con ErrConflict contra
// registros que nadie puede leer.
func vaciarPQVault(ctx context.Context, tx pgx.Tx, tenant, owner, product string) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM pq_vault_records WHERE tenant=$1 AND owner_sub=$2 AND product=$3`,
		tenant, owner, product,
	); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`DELETE FROM pq_vault_keys WHERE tenant=$1 AND owner_sub=$2 AND product=$3`,
		tenant, owner, product,
	)
	return err
}

// pqVaultKeys lee los envoltorios de la bóveda, el más reciente primero.
func pqVaultKeys(ctx context.Context, tx pgx.Tx, tenant, owner, product string) ([]recoverywrap.PQVaultWrapper, error) {
	keys := []recoverywrap.PQVaultWrapper{}
	rows, err := tx.Query(ctx,
		`SELECT wrapper_json FROM pq_vault_keys WHERE tenant=$1 AND owner_sub=$2 AND product=$3 ORDER BY updated_at DESC, key_id ASC`,
		tenant, owner, product,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var w recoverywrap.PQVaultWrapper
		if err := json.Unmarshal(raw, &w); err != nil {
			return nil, err
		}
		keys = append(keys, w)
	}
	return keys, rows.Err()
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
	mine := PQVaultCopyFrom(ctx)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// La fila de la bóveda queda bloqueada hasta el final: un vaciado
		// concurrente espera a esta publicación y ve su versión.
		var (
			currentVaultID string
			copyID         *string
			copyActive     *bool
		)
		err := tx.QueryRow(ctx,
			`SELECT vault_id, copy_id, copy_expires_at > now() FROM pq_vaults
			 WHERE tenant=$1 AND owner_sub=$2 AND product=$3 FOR NO KEY UPDATE`,
			t.Slug, owner, product,
		).Scan(&currentVaultID, &copyID, &copyActive)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if currentVaultID != vaultID {
			return domain.ErrConflict
		}
		if err := pqVaultCopyAllows(copyID, copyActive, mine); err != nil {
			return err
		}
		err = tx.QueryRow(ctx,
			`INSERT INTO pq_vault_records(tenant,owner_sub,product,vault_id,event_id,transaction_id,payload_json)
			 VALUES($1,$2,$3,$4,$5,$6,$7)
			 ON CONFLICT (tenant,owner_sub,product,transaction_id) DO NOTHING
			 RETURNING event_id, vault_id, seq, payload_json, created_at`,
			t.Slug, owner, product, vaultID, eventID, transactionID, []byte(payload),
		).Scan(&out.EventID, &out.VaultID, &out.Seq, &out.Payload, &out.CreatedAt)
		if err == nil {
			_, err = tx.Exec(ctx,
				`UPDATE pq_vaults SET version=version+1 WHERE tenant=$1 AND owner_sub=$2 AND product=$3`,
				t.Slug, owner, product,
			)
			return err
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

// pqVaultCopyAllows: con una marca de copia activa solo escribe quien la tiene; y
// quien escribe con una marca solo sigue si esa marca sigue puesta (no se canceló
// ni la tomó otro tras caducar).
func pqVaultCopyAllows(copyID *string, active *bool, mine string) error {
	switch {
	case copyID == nil:
		if mine != "" {
			return domain.ErrPQVaultCopyInProgress
		}
	case *copyID == mine:
	case mine != "", active != nil && *active:
		return domain.ErrPQVaultCopyInProgress
	}
	return nil
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

// PQVaultReceipt comprueba que `eventID` es un registro de la bóveda ACTUAL del
// propietario y que esa bóveda es `vaultID`, y devuelve su clave activa (de la que
// sale su destino de publicación). Es lo que mira Rooms en el recibo de un ACK del
// correo (ADR 0087): ErrConflict si la bóveda ya no es esa, ErrNotFound si el
// registro no está.
func (s *Store) PQVaultReceipt(ctx context.Context, product, vaultID, eventID string) (string, error) {
	owner := OwnerFrom(ctx)
	if !storage.Product(product) || owner == "" {
		return "", domain.ErrPermission
	}
	t := tenancy.MustFrom(ctx)
	var activeKeyID string
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var current string
		if err := tx.QueryRow(ctx,
			`SELECT vault_id, active_key_id FROM pq_vaults WHERE tenant=$1 AND owner_sub=$2 AND product=$3`,
			t.Slug, owner, product,
		).Scan(&current, &activeKeyID); err != nil {
			return err
		}
		if current != vaultID {
			return domain.ErrConflict
		}
		var found bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pq_vault_records
			 WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND vault_id=$4 AND event_id=$5)`,
			t.Slug, owner, product, vaultID, eventID,
		).Scan(&found); err != nil {
			return err
		}
		if !found {
			return domain.ErrNotFound
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return activeKeyID, err
}

// FindPQVaultRecord devuelve el registro que se guardó con `transactionID` en
// esa bóveda (ADR 0087: los secretos de entrada del correo se guardan una sola
// vez con un identificador fijo y se leen por él).
func (s *Store) FindPQVaultRecord(ctx context.Context, product, vaultID, transactionID string) (PQVaultRecord, error) {
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
			 WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND vault_id=$4 AND transaction_id=$5`,
			t.Slug, owner, product, vaultID, transactionID,
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
