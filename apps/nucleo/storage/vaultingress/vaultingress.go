// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package vaultingress da a la bóveda post-cuántica directa, con su capa de
// ficheros (storage/vaultfiles), el contrato de dispositivo que el correo usa para
// recibir (ADR 0087): el `Isolated` de mail/application en modo de bóveda, en los
// clientes nativos. Es lo que en la web sirve el anfitrión al módulo de correo
// sobre `PqVault` (`VaultIngressSecretsPort`, `VaultIngressControlPort`):
//
//   - los secretos de entrada viven en la propia bóveda (pqvault.ReadIngressSecret),
//     no en el almacén de secretos de Matrix; el formato de los bytes no cambia;
//   - las acciones de entrada las firma la clave de la bóveda
//     (pqvault.SignIngressAction) y el servidor las verifica contra Sytue ID;
//   - una publicación autorizada (la de una entrega reclamada) comprueba su
//     política justo antes de cada envío a la bóveda (pqvault.WithPublicationPolicy).
//
// Lo que guarda una bóveda lo abre la web y al revés: mismos registros, misma
// transacción, misma raíz.
package vaultingress

import (
	"context"
	"errors"
	"io"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/keyring"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/vaultfiles"
)

// ErrAuthority: la autoridad del correo que da Rooms no es la de esta bóveda
// (es de Matrix, de otra clave o no está activa). El correo no abre aquí.
var ErrAuthority = errors.New("vaultingress: la autoridad del correo no es la de esta bóveda")

// Device es la bóveda del correo como dispositivo de entrada. Los registros, el
// catálogo y los ficheros son los de vaultfiles.Store.
type Device struct {
	*vaultfiles.Store
	now func() time.Time
}

// New ata el contrato de entrada a una bóveda abierta del correo. No toma
// posesión de ella: quien la abrió la cierra.
func New(store *vaultfiles.Store) (*Device, error) {
	if store == nil || store.Vault == nil || !contract.Product(store.Scope().Product) {
		return nil, pqvault.ErrIdentity
	}
	return &Device{Store: store, now: time.Now}, nil
}

// RequireStorageAuthority comprueba que la autoridad que da Rooms es la de esta
// bóveda: su identidad, su raíz (la mitad Ed25519 de la clave de firma) y activa.
// Si es la de Matrix, este no es el camino.
func (d *Device) RequireStorageAuthority(authority matrixproof.Authority) error {
	scope := d.Scope()
	if !vaultproof.IsAuthority(authority) || authority.RequireActive(scope, authority.Generation) != nil || authority.UserID != d.IngressUserID() {
		return ErrAuthority
	}
	root, err := d.IngressRootKey()
	if err != nil {
		return err
	}
	if authority.RootKey != root {
		return ErrAuthority
	}
	return nil
}

// recoverable comprueba ámbito y destino: una bóveda solo publica en sí misma.
func (d *Device) recoverable(ctx context.Context, scope storage.Scope, target storage.PublicationTarget) error {
	if scope != d.Scope() {
		return pqvault.ErrIdentity
	}
	if _, err := d.PrepareStorage(ctx, scope); err != nil {
		return err
	}
	if err := d.CheckStoragePublicationTarget(target); err != nil {
		return err
	}
	return ctx.Err()
}

// RequireRecoverableIngress: la bóveda es siempre recuperable (sus claves vuelven
// de Sytue ID); basta con que sea esta y esté abierta.
func (d *Device) RequireRecoverableIngress(ctx context.Context, scope storage.Scope, target storage.PublicationTarget) (context.Context, error) {
	if err := d.recoverable(ctx, scope, target); err != nil {
		return nil, err
	}
	return ctx, nil
}

// RequireAuthorizedStoragePublication ata la política de una entrega reclamada a
// cada envío hecho con el contexto devuelto.
func (d *Device) RequireAuthorizedStoragePublication(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, policy storage.PublicationPolicy) (context.Context, error) {
	if err := d.recoverable(ctx, scope, target); err != nil {
		return nil, err
	}
	return d.WithPublicationPolicy(ctx, target, policy)
}

func (d *Device) secrets(ctx context.Context, scope storage.Scope) (keyring.Secrets, error) {
	target, err := d.StoragePublicationTarget()
	if err != nil {
		return nil, err
	}
	if err := d.recoverable(ctx, scope, target); err != nil {
		return nil, err
	}
	return secrets{vault: d.Vault}, nil
}

// CreateIngressKey guarda y vuelve a leer en la bóveda el secreto de un plan de
// entrada nuevo antes de dar su parte pública.
func (d *Device) CreateIngressKey(ctx context.Context, scope storage.Scope, epoch string, generation int64) (contract.KeyPlan, error) {
	store, err := d.secrets(ctx, scope)
	if err != nil {
		return contract.KeyPlan{}, err
	}
	return keyring.Create(ctx, store, scope, epoch, generation)
}

// RecoverIngressEnrollment autentica el plan contra su secreto en la bóveda. Si
// no está, keyring.ErrRecoveryRequired: el correo en modo de bóveda propone
// entonces el plan siguiente (ingress.EnrollmentConfig.Rotate).
func (d *Device) RecoverIngressEnrollment(ctx context.Context, plan contract.KeyPlan) (contract.KeyPlan, error) {
	if plan.Validate(d.Scope(), plan.Epoch) != nil {
		return contract.KeyPlan{}, contract.ErrContract
	}
	store, err := d.secrets(ctx, plan.Scope)
	if err != nil {
		return contract.KeyPlan{}, err
	}
	return keyring.Recover(ctx, store, plan)
}

// OpenIngressBody abre un correo entrante con la identidad de su plan, que no
// sale de la bóveda.
func (d *Device) OpenIngressBody(ctx context.Context, task contract.Task, source io.ReaderAt) ([]byte, *io.SectionReader, error) {
	if source == nil || task.Validate(d.Scope(), task.Epoch) != nil {
		return nil, nil, contract.ErrContract
	}
	store, err := d.secrets(ctx, task.Scope)
	if err != nil {
		return nil, nil, err
	}
	return keyring.Open(ctx, store, task, source)
}

// SignIngressAction firma con la clave de la bóveda solo la acción de entrada que
// pidió el correo, con el arrendamiento de trabajo de la sesión retenido mientras
// firma. No da de alta ni rota raíces: eso lo ancla Sytue ID.
func (d *Device) SignIngressAction(ctx context.Context, lease offline.WorkLease, expected matrixproof.Intent, action matrixproof.Action) (matrixproof.SignedAction, error) {
	var proof matrixproof.SignedAction
	now := d.now()
	if !expected.Valid() || action.Intent() != expected || !action.Valid(now) || !vaultproof.Signable(action.Kind) {
		return proof, matrixproof.ErrUnverified
	}
	if lease == nil {
		return proof, offline.ErrWorkLease
	}
	release, err := lease(ctx)
	if err != nil {
		return proof, err
	}
	if release == nil {
		return proof, offline.ErrWorkLease
	}
	defer release()
	if err := d.recoverable(ctx, action.Scope, action.Target); err != nil {
		return proof, err
	}
	proof, err = d.Vault.SignIngressAction(expected, action, now)
	if err != nil {
		return matrixproof.SignedAction{}, err
	}
	return proof, ctx.Err()
}

// secrets son los secretos de entrada guardados en la bóveda (keyring.Secrets).
type secrets struct{ vault *pqvault.Vault }

func (s secrets) Read(ctx context.Context, name string) ([]byte, error) {
	value, found, err := s.vault.ReadIngressSecret(ctx, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, keyring.ErrRecoveryRequired
	}
	return value, nil
}

func (s secrets) Write(ctx context.Context, name string, value []byte) error {
	return s.vault.WriteIngressSecret(ctx, name, value)
}
