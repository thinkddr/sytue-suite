// SPDX-License-Identifier: Apache-2.0
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// MatrixRecoveryKey devuelve la clave de recuperación que ADR 0081 custodiaba con
// la KEK. Solo sirve a la transición de ADR 0082: el cliente la usa una vez, la
// envuelve en el dispositivo y, al guardar el envoltorio, la copia con KEK se borra.
func (s *Service) MatrixRecoveryKey(ctx context.Context, actor string) (string, error) {
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return "", err
	}
	return s.pstore.MatrixRecoveryKey(ctx, actor)
}

// StoreMatrixRecoveryKey ya no guarda nada: ninguna clave nueva entra en custodia
// legible por el operador (ADR 0082).
func (s *Service) StoreMatrixRecoveryKey(context.Context, string, string) error {
	return domain.ErrRecoveryEscrowRetired
}

// MatrixRecoveryAccount elige la cuenta de Matrix de un envoltorio. Sin producto
// es `@sytue_<sub>` (Rooms). Con producto, la identidad heredada es esa misma
// cuenta; la aislada es la de ese producto en la organización de la petición, la
// misma que entrega BootstrapMatrixStorageForIdentity.
type MatrixRecoveryAccount struct {
	Product         string
	IdentityVersion storage.IdentityVersion
}

func (a MatrixRecoveryAccount) resolve(ctx context.Context) (store.MatrixRecoveryAccount, error) {
	if a.Product == "" {
		if a.IdentityVersion != 0 {
			return store.MatrixRecoveryAccount{}, domain.ErrInvalidInput
		}
		return store.MatrixRecoveryAccount{}, nil
	}
	version := a.IdentityVersion.Normalized()
	if !domain.StorageProduct(a.Product) || !version.Valid() {
		return store.MatrixRecoveryAccount{}, domain.ErrInvalidInput
	}
	if version == storage.LegacyIdentity {
		return store.MatrixRecoveryAccount{}, nil
	}
	return store.MatrixRecoveryAccount{Tenant: tenancy.MustFrom(ctx).Slug, Product: a.Product}, nil
}

// MatrixRecoveryWrappers es la respuesta de GET /matrix/recovery-wrappers.
type MatrixRecoveryWrappers struct {
	Wrappers     []recoverywrap.Wrapper `json:"wrappers"`
	LegacyEscrow bool                   `json:"legacy_escrow"`
}

// MatrixRecoveryWrappers devuelve los envoltorios de esa cuenta de quien pregunta.
// La identidad sale del token OIDC y pasa la misma comprobación que el bootstrap
// de Matrix y el de almacenamiento: nadie lee los de otra persona ni los de una
// organización que no es suya. Aunque los leyera, no los podría abrir.
func (s *Service) MatrixRecoveryWrappers(ctx context.Context, actor string, account MatrixRecoveryAccount) (MatrixRecoveryWrappers, error) {
	scope, err := account.resolve(ctx)
	if err != nil {
		return MatrixRecoveryWrappers{}, err
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return MatrixRecoveryWrappers{}, err
	}
	rows, legacy, err := s.pstore.MatrixRecoveryWrappers(ctx, actor, scope)
	if err != nil {
		return MatrixRecoveryWrappers{}, err
	}
	out := MatrixRecoveryWrappers{Wrappers: make([]recoverywrap.Wrapper, 0, len(rows)), LegacyEscrow: legacy}
	for _, row := range rows {
		var w recoverywrap.Wrapper
		if json.Unmarshal(row.Wrapper, &w) != nil || w.Validate() != nil {
			continue
		}
		w.ID, w.CreatedAt = row.ID, row.CreatedAt.UTC().Format(time.RFC3339)
		out.Wrappers = append(out.Wrappers, w)
	}
	return out, nil
}

// PutMatrixRecoveryWrapper guarda el envoltorio que el dispositivo acaba de hacer.
// Solo comprueba su forma (parámetros mínimos de Argon2id incluidos): abrirlo no
// puede, porque la clave nunca sale del dispositivo.
func (s *Service) PutMatrixRecoveryWrapper(ctx context.Context, actor string, account MatrixRecoveryAccount, wrapper recoverywrap.Wrapper) (string, error) {
	return s.putMatrixRecoveryWrapper(ctx, actor, account, wrapper, false)
}

// PutMatrixRecoveryWrapperRetiringOlder guarda un envoltorio `sytue-id` que el
// dispositivo acaba de comprobar que abre y retira en la misma transacción los de la
// ADR 0082 de esa clave (ADR 0085). Solo para `sytue-id`: es el que abre sin pedir
// nada, y retirar los viejos con otro tipo dejaría la cuenta peor.
func (s *Service) PutMatrixRecoveryWrapperRetiringOlder(ctx context.Context, actor string, account MatrixRecoveryAccount, wrapper recoverywrap.Wrapper) (string, error) {
	return s.putMatrixRecoveryWrapper(ctx, actor, account, wrapper, true)
}

func (s *Service) putMatrixRecoveryWrapper(ctx context.Context, actor string, account MatrixRecoveryAccount, wrapper recoverywrap.Wrapper, retireOlder bool) (string, error) {
	wrapper.ID, wrapper.CreatedAt = "", ""
	if err := wrapper.Validate(); err != nil || (retireOlder && wrapper.Kind != recoverywrap.KindSytueID) {
		return "", domain.ErrInvalidInput
	}
	scope, err := account.resolve(ctx)
	if err != nil {
		return "", err
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return "", err
	}
	var raw bytes.Buffer
	if err := json.NewEncoder(&raw).Encode(wrapper); err != nil {
		return "", err
	}
	if retireOlder {
		return s.pstore.PutMatrixRecoveryWrapperRetiringOlder(ctx, actor, scope, wrapper.KeyID, bytes.TrimSpace(raw.Bytes()))
	}
	return s.pstore.PutMatrixRecoveryWrapper(ctx, actor, scope, wrapper.Kind, wrapper.CredentialID, wrapper.KeyID, bytes.TrimSpace(raw.Bytes()))
}
