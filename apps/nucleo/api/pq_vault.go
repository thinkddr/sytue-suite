// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

// La bóveda post-cuántica directa (ADR 0086) en services/drive
// (`/api/v1/pq-vault/{product}/*`) y el registro de su clave de firma en Sytue ID
// (ADR 0087, `/api/v1/me/vault-keys/{product}`). Es el mismo servidor que usan las
// apps web por su BFF: aquí solo viajan envoltorios y sobres ya cifrados, nunca la
// clave de desbloqueo ni la identidad privada.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PQVaultState es el estado público de una bóveda: su identificador, la clave
// activa y los envoltorios guardados. VaultID vacío: todavía no hay bóveda.
type PQVaultState struct {
	VaultID     string                        `json:"vault_id"`
	ActiveKeyID string                        `json:"active_key_id"`
	Keys        []recoverywrap.PQVaultWrapper `json:"keys"`
	// Version sube con cada registro nuevo y con cada vaciado: vaciar exige la
	// que se vio (PQVaultExpect).
	Version int64 `json:"version"`
	// Copy es la marca «copia en curso» (nil si no hay ninguna).
	Copy *PQVaultCopy `json:"copy,omitempty"`
}

// PQVaultCopy es la marca «copia en curso» de una bóveda (ADR 0086, «pasa Matrix
// a ella»): la pone quien copia el Drive de Matrix en la bóveda. Mientras está
// activa, solo él publica o vacía; caduca si no se renueva.
type PQVaultCopy struct {
	ID        string    `json:"id"`
	Holder    string    `json:"holder"`
	Kind      string    `json:"kind"`
	Epoch     string    `json:"epoch"`
	StartedAt time.Time `json:"started_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Active    bool      `json:"active"`
}

// PQVaultCopyKind es la marca de la copia de Matrix a la bóveda.
const PQVaultCopyKind = "matrix-copy"

// PQVaultExpect es lo que se vio de la bóveda al decidir vaciarla: el servidor
// solo la vacía si sigue igual (ni otra bóveda ni registros nuevos, como el
// recibo de una importación que terminó entre medias).
type PQVaultExpect struct {
	VaultID string `json:"vault_id"`
	Version int64  `json:"version"`
}

// PQVaultCopyClaim pide o renueva la marca de copia.
type PQVaultCopyClaim struct {
	ID     string
	Holder string
	Epoch  string
	TTL    time.Duration
}

// Códigos con los que services/drive rechaza tocar la bóveda.
const (
	// CodePQVaultChanged: la bóveda cambió desde que se leyó; no se vació nada.
	CodePQVaultChanged = "pq_vault_changed"
	// CodePQVaultCopyInProgress: otro dispositivo copia en ella, o la marca con
	// la que se escribía ya no vale (se canceló o la tomó otro).
	CodePQVaultCopyInProgress = "pq_vault_copy_in_progress"
)

// IsPQVaultCode dice si `err` es un rechazo de services/drive con ese código.
func IsPQVaultCode(err error, code string) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict && apiErr.Code() == code
}

type pqVaultCopyKey struct{}

// WithPQVaultCopy hace que las escrituras de la bóveda hechas con `ctx` (publicar
// y vaciar) vayan con la marca de copia `id`: la de quien copia.
func WithPQVaultCopy(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, pqVaultCopyKey{}, id)
}

// PQVaultCopyFrom es la marca con la que escribe `ctx` ("" si ninguna).
func PQVaultCopyFrom(ctx context.Context) string {
	id, _ := ctx.Value(pqVaultCopyKey{}).(string)
	return id
}

// PQVaultRecord es un registro tal como lo guarda el servidor: solo el sobre
// `org.sytue.storage_pq`, que el servidor no puede abrir.
type PQVaultRecord struct {
	EventID string          `json:"event_id"`
	VaultID string          `json:"vault_id"`
	Seq     int64           `json:"seq"`
	Payload json.RawMessage `json:"payload"`
}

// PQVaultRecordPage es una página de registros, del más reciente al más antiguo.
type PQVaultRecordPage struct {
	Entries    []PQVaultRecord `json:"entries"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

func pqVaultRequest(scope storage.Scope, tail string) (string, http.Header, error) {
	value, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		return "", nil, storage.ErrRecord
	}
	return "/pq-vault/" + url.PathEscape(scope.Product) + tail, http.Header{storage.ScopeHeader: {value}}, nil
}

// PQVaultKeys lee el estado de la bóveda de `scope` (la persona y la organización
// las pone el token; la cabecera de ámbito las confirma).
func (d *Drive) PQVaultKeys(ctx context.Context, scope storage.Scope) (PQVaultState, error) {
	path, headers, err := pqVaultRequest(scope, "/keys")
	if err != nil {
		return PQVaultState{}, err
	}
	return JSON[PQVaultState](ctx, d.c, Request{Path: path, Header: headers})
}

// PutPQVaultKey guarda un envoltorio. Con `reset` vacía la bóveda (borra registros
// y claves) y la estrena con un identificador nuevo, pero solo si sigue como se
// vio (`reset`): si no, 409 CodePQVaultChanged y no se toca nada. Solo tras la
// confirmación de la persona o cuando la bóveda no tiene nada de nadie.
func (d *Drive) PutPQVaultKey(ctx context.Context, scope storage.Scope, wrapper recoverywrap.PQVaultWrapper, setActive bool, reset *PQVaultExpect) (PQVaultState, error) {
	if err := wrapper.Validate(); err != nil {
		return PQVaultState{}, err
	}
	path, headers, err := pqVaultRequest(scope, "/keys")
	if err != nil {
		return PQVaultState{}, err
	}
	req := map[string]any{"wrapper": wrapper, "set_active": setActive, "reset": reset != nil}
	if reset != nil {
		req["expect"] = reset
	}
	if id := PQVaultCopyFrom(ctx); id != "" {
		req["copy_id"] = id
	}
	body, err := json.Marshal(req)
	if err != nil {
		return PQVaultState{}, err
	}
	headers.Set("Content-Type", "application/json")
	return JSON[PQVaultState](ctx, d.c, Request{Method: http.MethodPut, Path: path, Header: headers, Body: body, SinAviso: true})
}

// AppendPQVaultRecord publica un sobre. Es idempotente por `transaction`: repetirla
// con el mismo sobre devuelve el mismo registro; con otro distinto, 409.
func (d *Drive) AppendPQVaultRecord(ctx context.Context, scope storage.Scope, vaultID, transaction string, payload json.RawMessage) (PQVaultRecord, error) {
	path, headers, err := pqVaultRequest(scope, "/records")
	if err != nil {
		return PQVaultRecord{}, err
	}
	req := map[string]any{"vault_id": vaultID, "transaction_id": transaction, "payload": payload}
	if id := PQVaultCopyFrom(ctx); id != "" {
		req["copy_id"] = id
	}
	body, err := json.Marshal(req)
	if err != nil {
		return PQVaultRecord{}, err
	}
	headers.Set("Content-Type", "application/json")
	return JSON[PQVaultRecord](ctx, d.c, Request{Method: http.MethodPost, Path: path, Header: headers, Body: body})
}

// ClaimPQVaultCopy pone o renueva la marca de copia de la bóveda. Otra marca
// activa es 409 CodePQVaultCopyInProgress; sin bóveda, 404.
func (d *Drive) ClaimPQVaultCopy(ctx context.Context, scope storage.Scope, claim PQVaultCopyClaim) (PQVaultState, error) {
	path, headers, err := pqVaultRequest(scope, "/copy")
	if err != nil {
		return PQVaultState{}, err
	}
	seconds := int64(claim.TTL / time.Second)
	if seconds <= 0 {
		return PQVaultState{}, storage.ErrRecord
	}
	body, err := json.Marshal(map[string]any{"id": claim.ID, "holder": claim.Holder, "kind": PQVaultCopyKind, "epoch": claim.Epoch, "ttl_seconds": seconds})
	if err != nil {
		return PQVaultState{}, err
	}
	headers.Set("Content-Type", "application/json")
	return JSON[PQVaultState](ctx, d.c, Request{Method: http.MethodPut, Path: path, Header: headers, Body: body, SinAviso: true})
}

// ReleasePQVaultCopy retira la marca `id` (sin marca no hace nada). Nunca borra
// registros.
func (d *Drive) ReleasePQVaultCopy(ctx context.Context, scope storage.Scope, id string) (PQVaultState, error) {
	path, headers, err := pqVaultRequest(scope, "/copy")
	if err != nil {
		return PQVaultState{}, err
	}
	return JSON[PQVaultState](ctx, d.c, Request{Method: http.MethodDelete, Path: path, Header: headers, Query: url.Values{"id": {id}}, SinAviso: true})
}

// PQVaultRecord lee un registro concreto de la bóveda `vaultID`.
func (d *Drive) PQVaultRecord(ctx context.Context, scope storage.Scope, vaultID, eventID string) (PQVaultRecord, error) {
	path, headers, err := pqVaultRequest(scope, "/records/"+url.PathEscape(eventID))
	if err != nil {
		return PQVaultRecord{}, err
	}
	return JSON[PQVaultRecord](ctx, d.c, Request{Path: path, Header: headers, Query: url.Values{"vault_id": {vaultID}}})
}

// PQVaultRecords lee una página de registros antes de `cursor` (vacío: desde el
// último). `limit` va de 1 a 100.
func (d *Drive) PQVaultRecords(ctx context.Context, scope storage.Scope, vaultID, cursor string, limit int) (PQVaultRecordPage, error) {
	if limit < 1 || limit > 100 {
		return PQVaultRecordPage{}, storage.ErrRecord
	}
	path, headers, err := pqVaultRequest(scope, "/records")
	if err != nil {
		return PQVaultRecordPage{}, err
	}
	query := url.Values{"vault_id": {vaultID}, "limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	return JSON[PQVaultRecordPage](ctx, d.c, Request{Path: path, Header: headers, Query: query})
}

// VaultKey es una generación de la clave de firma de una bóveda en Sytue ID.
type VaultKey struct {
	Generation    int64           `json:"generation"`
	KeyID         string          `json:"key_id"`
	PreviousKeyID string          `json:"previous_key_id"`
	PublicKey     json.RawMessage `json:"public_key"`
	Reason        string          `json:"reason"`
	RetiredAt     *time.Time      `json:"retired_at,omitempty"`
}

// VaultKeyRegistration es el cuerpo de PUT /api/v1/me/vault-keys/{product}.
type VaultKeyRegistration struct {
	PublicKey     recoverywrap.PQVaultVerifyKey `json:"public_key"`
	PreviousKeyID string                        `json:"previous_key_id"`
	Proof         []byte                        `json:"proof"`
	Continuity    []byte                        `json:"continuity,omitempty"`
	// Handover es la marca de traspaso del correo de Matrix a la bóveda (ADR 0087):
	// Sytue ID la acepta solo en `mail` y siempre con identificación reciente.
	Handover bool `json:"handover,omitempty"`
}

// CodeVaultKeyReauth es el código del 403 de un vaciado sin identificación reciente.
const CodeVaultKeyReauth = "vault_key_reauth"

// VaultKeys lee el historial de la clave de firma de la bóveda `product`, la
// activa primero.
func (a *Account) VaultKeys(ctx context.Context, product string) ([]VaultKey, error) {
	if !storage.Product(product) {
		return nil, storage.ErrRecord
	}
	out, err := JSON[struct {
		Keys []VaultKey `json:"keys"`
	}](ctx, a.c, Request{Path: "/me/vault-keys/" + url.PathEscape(product)})
	return out.Keys, err
}

// PutVaultKey registra la generación siguiente. Un vaciado sin identificación
// reciente vuelve como *Error con Code() == CodeVaultKeyReauth.
func (a *Account) PutVaultKey(ctx context.Context, product string, registration VaultKeyRegistration) error {
	if !storage.Product(product) {
		return storage.ErrRecord
	}
	body, err := json.Marshal(registration)
	if err != nil {
		return err
	}
	_, err = JSON[json.RawMessage](ctx, a.c, Request{
		Method: http.MethodPut, Path: "/me/vault-keys/" + url.PathEscape(product), Body: body,
		Header: http.Header{"Content-Type": {"application/json"}}, SinAviso: true,
	})
	return err
}
