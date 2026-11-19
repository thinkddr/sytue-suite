// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"regexp"

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
	PutPQVaultKey(context.Context, string, recoverywrap.PQVaultWrapper, bool, bool) (store.PQVaultState, error)
	AppendPQVaultRecord(context.Context, string, string, string, json.RawMessage) (store.PQVaultRecord, error)
	GetPQVaultRecord(context.Context, string, string, string) (store.PQVaultRecord, error)
	ListPQVaultRecords(context.Context, string, string, int64, int) (store.PQVaultRecordPage, error)
}

// PQVaultState, PQVaultRecord y PQVaultRecordPage son alias para desacoplar la capa HTTP.
type (
	PQVaultState      = store.PQVaultState
	PQVaultRecord     = store.PQVaultRecord
	PQVaultRecordPage = store.PQVaultRecordPage
)

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
func (s *Service) PutPQVaultKey(ctx context.Context, product string, wrapper recoverywrap.PQVaultWrapper, setActive, reset bool) (PQVaultState, error) {
	if s.pqVault == nil || store.OwnerFrom(ctx) == "" {
		return PQVaultState{}, domain.ErrPermission
	}
	if !storage.Product(product) || wrapper.Validate() != nil {
		return PQVaultState{}, domain.ErrInvalidName
	}
	return s.pqVault.PutPQVaultKey(ctx, product, wrapper, setActive, reset)
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
