// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"errors"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/ssss"
)

// RecoveryEscrow guarda la clave de recuperación ENVUELTA en el dispositivo (ADR
// 0082) antes de que nada la use en el servidor de Matrix. keyID es el id público
// de la clave de almacenamiento secreto: ata el envoltorio a esa clave. Si falla,
// la clave no se publica: una clave que nadie puede volver a abrir no se estrena.
type RecoveryEscrow func(ctx context.Context, keyID, recoveryKey string) error

// ErrNoRecoveryEscrow es configurar o restablecer el cifrado sin dónde guardar la
// clave: ya no se enseña a nadie, así que sin custodia no se crea.
var ErrNoRecoveryEscrow = errors.New("matrix: no recovery key escrow")

// AccountUserID es la cuenta de Matrix de este cliente: lo que ata un envoltorio
// a su cuenta (el AAD del formato de recoverywrap).
func (c *Client) AccountUserID() string { return c.cli.UserID.String() }

// BootstrapRecoveryWrapped es BootstrapRecovery con la clave guardada ANTES de
// publicarla. La configuración pendiente vive en el llavero del sistema, así que
// un reintento tras un fallo de custodia o de UIA usa la misma clave y la vuelve a
// custodiar (es idempotente). La clave no se devuelve: no hay nada que enseñar.
// Tras él, AcknowledgeRecoverySetup retira la configuración pendiente.
func (c *Client) BootstrapRecoveryWrapped(ctx context.Context, uia mautrix.UIACallback, escrow RecoveryEscrow) error {
	if escrow == nil {
		return ErrNoRecoveryEscrow
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed.Load() {
		return ErrClosed
	}
	machine := c.crypto.Machine()
	pending, err := c.prepareRecoverySetup(ctx)
	if err != nil {
		return err
	}
	defer pending.clear()
	recovery := pending.storageKey().RecoveryKey()
	if err := escrow(ctx, pending.KeyID, recovery); err != nil {
		return err
	}
	if !pending.Complete {
		if err := c.publishRecoverySetup(ctx, pending, uia); err != nil {
			return err
		}
	} else if err := machine.ImportCrossSigningKeys(cloneRecoverySeeds(pending.Seeds)); err != nil {
		return err
	}
	if err := machine.SignOwnDevice(ctx, machine.OwnIdentity()); err != nil {
		return err
	}
	if err := machine.SignOwnMasterKey(ctx); err != nil {
		return err
	}
	if err := machine.FlushStore(ctx); err != nil {
		return err
	}
	if err := c.prepareBackup(ctx, recovery); err != nil {
		return err
	}
	if err := c.saveStorageAuthKey(ctx, pending.storageKey()); err != nil {
		return err
	}
	pending.Complete = true
	return c.saveRecoverySetup(ctx, pending)
}

// ResetRecoveryWrapped es ResetRecovery con la clave nueva guardada antes de
// publicar la identidad nueva. Si la custodia falla, la identidad vieja sigue en
// pie (solo queda subida una clave de almacenamiento que no es la de por defecto).
func (c *Client) ResetRecoveryWrapped(ctx context.Context, uia mautrix.UIACallback, escrow RecoveryEscrow) error {
	if escrow == nil {
		return ErrNoRecoveryEscrow
	}
	_, err := c.resetRecovery(ctx, uia, escrow)
	return err
}

// DefaultRecoveryKeyID es el id de la clave de almacenamiento secreto por defecto
// de la cuenta: elige, entre los envoltorios, el de la clave que de verdad abre.
// "" sin error es una cuenta sin almacenamiento secreto.
func (c *Client) DefaultRecoveryKeyID(ctx context.Context) (string, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.closed.Load() {
		return "", ErrClosed
	}
	keyID, err := c.crypto.Machine().SSSS.GetDefaultKeyID(ctx)
	if errors.Is(err, ssss.ErrNoDefaultKeyAccountDataEvent) {
		return "", nil
	}
	return keyID, err
}

// LocalRecoveryKey devuelve la clave de recuperación que este dispositivo ya
// guarda en el llavero del sistema: solo la tienen las cuentas de almacenamiento,
// que la conservan para autenticar su historial. Sirve para envolverla en un
// dispositivo ya verificado de una cuenta que el servicio no conocía. No va a
// ningún sitio sin envolver.
func (c *Client) LocalRecoveryKey(ctx context.Context) (keyID, recoveryKey string, err error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.cfg.Bootstrap.StorageProduct == "" {
		return "", "", ErrRecoveryRequired
	}
	key, err := c.storageAuthKey(ctx, "")
	if err != nil {
		return "", "", err
	}
	defer clear(key.Key)
	return key.ID, key.RecoveryKey(), nil
}
