// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

const (
	// MaxPQVaultRecordBytes acota un registro cifrado individual (incluyendo sobre PQ).
	MaxPQVaultRecordBytes = 96 << 10
	maxPQTransactionLen   = 255
)

var (
	pqVaultIDPattern = regexp.MustCompile(`^!pqv_[A-Z2-7]{26,64}$`)
	pqEventIDPattern = regexp.MustCompile(`^\$pqe_[A-Z2-7]{26,64}$`)
)

type pqVaultStore interface {
	GetPQVault(context.Context, string) (store.PQVaultState, error)
	PutPQVaultKey(context.Context, string, recoverywrap.PQVaultWrapper, bool, *store.PQVaultExpect) (store.PQVaultState, error)
	ClaimPQVaultCopy(context.Context, string, store.PQVaultCopyClaim) (store.PQVaultState, error)
	ReleasePQVaultCopy(context.Context, string, string) (store.PQVaultState, error)
	AppendPQVaultRecord(context.Context, string, string, string, json.RawMessage) (store.PQVaultRecord, error)
	GetPQVaultRecord(context.Context, string, string, string) (store.PQVaultRecord, error)
	ListPQVaultRecords(context.Context, string, string, int64, int) (store.PQVaultRecordPage, error)
	FindPQVaultRecord(context.Context, string, string, string) (store.PQVaultRecord, error)
	PQVaultReceipt(context.Context, string, string, string) (string, error)
}

// PQVaultState, PQVaultRecord y PQVaultRecordPage son alias para desacoplar la capa HTTP.
type (
	PQVaultState      = store.PQVaultState
	PQVaultRecord     = store.PQVaultRecord
	PQVaultRecordPage = store.PQVaultRecordPage
	PQVaultExpect     = store.PQVaultExpect
	PQVaultCopyClaim  = store.PQVaultCopyClaim
)

const (
	// PQVaultCopyKind es la única marca de copia que existe: la del Drive de
	// Matrix copiándose en la bóveda desde un cliente nativo.
	PQVaultCopyKind = "matrix-copy"
	// MinPQVaultCopyTTL y MaxPQVaultCopyTTL acotan cuánto vale una marca sin
	// renovar: quien copia la renueva mucho antes; si se para, caduca pronto.
	MinPQVaultCopyTTL = time.Minute
	MaxPQVaultCopyTTL = 30 * time.Minute
)

var pqVaultCopyIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22,64}$`)

// GetPQVault devuelve las claves públicas/envueltas de la bóveda PQ del usuario.
func (s *Service) GetPQVault(ctx context.Context, product string) (PQVaultState, error) {
	if s.pqVault == nil || store.OwnerFrom(ctx) == "" {
		return PQVaultState{}, domain.ErrPermission
	}
	if !storage.Product(product) {
		return PQVaultState{}, domain.ErrInvalidName
	}
	return s.pqVault.GetPQVault(ctx, product)
}

// PutPQVaultKey valida y guarda el envoltorio público de una clave post-cuántica.
// Con `reset` vacía la bóveda, solo si sigue como la vio el cliente.
func (s *Service) PutPQVaultKey(ctx context.Context, product string, wrapper recoverywrap.PQVaultWrapper, setActive bool, reset *PQVaultExpect) (PQVaultState, error) {
	if s.pqVault == nil || store.OwnerFrom(ctx) == "" {
		return PQVaultState{}, domain.ErrPermission
	}
	if !storage.Product(product) || wrapper.Validate() != nil {
		return PQVaultState{}, domain.ErrInvalidName
	}
	if reset != nil && !reset.Unchecked && (reset.Version < 0 || reset.VaultID != "" && !pqVaultIDPattern.MatchString(reset.VaultID)) {
		return PQVaultState{}, domain.ErrInvalidName
	}
	return s.pqVault.PutPQVaultKey(ctx, product, wrapper, setActive, reset)
}

// ClaimPQVaultCopy pone o renueva la marca de copia en curso de la bóveda.
func (s *Service) ClaimPQVaultCopy(ctx context.Context, product string, claim PQVaultCopyClaim) (PQVaultState, error) {
	if s.pqVault == nil || store.OwnerFrom(ctx) == "" {
		return PQVaultState{}, domain.ErrPermission
	}
	if !storage.Product(product) || !pqVaultCopyIDPattern.MatchString(claim.ID) ||
		claim.Holder == "" || len(claim.Holder) > 128 || claim.Kind != PQVaultCopyKind ||
		claim.Epoch == "" || len(claim.Epoch) > 255 ||
		claim.TTL < MinPQVaultCopyTTL || claim.TTL > MaxPQVaultCopyTTL {
		return PQVaultState{}, domain.ErrInvalidName
	}
	return s.pqVault.ClaimPQVaultCopy(ctx, product, claim)
}

// ReleasePQVaultCopy retira (o cancela) la marca de copia `id`. No borra nada.
func (s *Service) ReleasePQVaultCopy(ctx context.Context, product, id string) (PQVaultState, error) {
	if s.pqVault == nil || store.OwnerFrom(ctx) == "" {
		return PQVaultState{}, domain.ErrPermission
	}
	if !storage.Product(product) || !pqVaultCopyIDPattern.MatchString(id) {
		return PQVaultState{}, domain.ErrInvalidName
	}
	return s.pqVault.ReleasePQVaultCopy(ctx, product, id)
}

// AppendPQVaultRecord valida que el registro lleva sobre PQ (nunca en claro) y
// lo añade a la bóveda con idempotencia por `transactionID`.
func (s *Service) AppendPQVaultRecord(ctx context.Context, product, vaultID, transactionID string, payload json.RawMessage) (PQVaultRecord, error) {
	if s.pqVault == nil || store.OwnerFrom(ctx) == "" {
		return PQVaultRecord{}, domain.ErrPermission
	}
	if !storage.Product(product) || !pqVaultIDPattern.MatchString(vaultID) ||
		transactionID == "" || len(transactionID) > maxPQTransactionLen ||
		len(payload) == 0 || len(payload) > MaxPQVaultRecordBytes {
		return PQVaultRecord{}, domain.ErrInvalidName
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(payload, &obj); err != nil {
		return PQVaultRecord{}, domain.ErrInvalidName
	}
	// Solo se aceptan paquetes cifrados `org.sytue.storage_pq`: jamás datos en claro.
	if _, ok := obj["org.sytue.storage_pq"]; !ok {
		return PQVaultRecord{}, domain.ErrInvalidName
	}
	return s.pqVault.AppendPQVaultRecord(ctx, product, vaultID, transactionID, payload)
}

// GetPQVaultRecord lee un registro cifrado concreto por `eventID`.
func (s *Service) GetPQVaultRecord(ctx context.Context, product, vaultID, eventID string) (PQVaultRecord, error) {
	if s.pqVault == nil || store.OwnerFrom(ctx) == "" {
		return PQVaultRecord{}, domain.ErrPermission
	}
	if !storage.Product(product) || !pqVaultIDPattern.MatchString(vaultID) || !pqEventIDPattern.MatchString(eventID) {
		return PQVaultRecord{}, domain.ErrInvalidName
	}
	return s.pqVault.GetPQVaultRecord(ctx, product, vaultID, eventID)
}

// VerifyPQVaultReceipt comprueba el recibo de una publicación en la bóveda (ADR
// 0087): el registro existe en la bóveda actual de `scope` y el destino es el de su
// clave activa. Cualquier otra cosa es ErrNotFound: el ACK falla cerrado. El
// contexto lleva ya el tenant y el propietario del ámbito.
func (s *Service) VerifyPQVaultReceipt(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, receipt storage.Reference) error {
	if s.pqVault == nil || store.OwnerFrom(ctx) != scope.Subject || !scope.Valid() {
		return domain.ErrPermission
	}
	if !target.Valid() || !receipt.Valid() || receipt.RoomID != target.RoomID ||
		!pqVaultIDPattern.MatchString(target.RoomID) || !pqEventIDPattern.MatchString(receipt.EventID) {
		return domain.ErrInvalidName
	}
	activeKeyID, err := s.pqVault.PQVaultReceipt(ctx, scope.Product, target.RoomID, receipt.EventID)
	if errors.Is(err, domain.ErrConflict) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if storage.PQVaultTarget(target.RoomID, activeKeyID) != target {
		return domain.ErrNotFound
	}
	return nil
}

// FindPQVaultRecord lee el registro guardado con `transactionID`.
func (s *Service) FindPQVaultRecord(ctx context.Context, product, vaultID, transactionID string) (PQVaultRecord, error) {
	if s.pqVault == nil || store.OwnerFrom(ctx) == "" {
		return PQVaultRecord{}, domain.ErrPermission
	}
	if !storage.Product(product) || !pqVaultIDPattern.MatchString(vaultID) ||
		transactionID == "" || len(transactionID) > maxPQTransactionLen {
		return PQVaultRecord{}, domain.ErrInvalidName
	}
	return s.pqVault.FindPQVaultRecord(ctx, product, vaultID, transactionID)
}

// ListPQVaultRecords pagina el historial de registros cifrados en orden inverso.
func (s *Service) ListPQVaultRecords(ctx context.Context, product, vaultID string, beforeSeq int64, limit int) (PQVaultRecordPage, error) {
	if s.pqVault == nil || store.OwnerFrom(ctx) == "" {
		return PQVaultRecordPage{}, domain.ErrPermission
	}
	if !storage.Product(product) || !pqVaultIDPattern.MatchString(vaultID) || beforeSeq < 0 {
		return PQVaultRecordPage{}, domain.ErrInvalidName
	}
	return s.pqVault.ListPQVaultRecords(ctx, product, vaultID, beforeSeq, limit)
}
