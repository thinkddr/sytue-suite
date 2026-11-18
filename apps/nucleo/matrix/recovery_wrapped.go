// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
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
	return c.bootstrapRecoveryWrapped(ctx, uia, escrow, false)
}

// RestartRecoveryWrapped es BootstrapRecoveryWrapped en una cuenta que AbandonedRecovery
// da por abandonada, comprobado otra vez aquí: su clave de almacenamiento por defecto
// no protegió nunca nada, así que se configura como una cuenta nueva con otra clave
// (envuelta antes de publicarla), y lo que este dispositivo guardaba de la vieja (su
// clave de propietario en el llavero) se olvida. En el servidor no se borra nada.
func (c *Client) RestartRecoveryWrapped(ctx context.Context, uia mautrix.UIACallback, escrow RecoveryEscrow) error {
	return c.bootstrapRecoveryWrapped(ctx, uia, escrow, true)
}

func (c *Client) bootstrapRecoveryWrapped(ctx context.Context, uia mautrix.UIACallback, escrow RecoveryEscrow, abandoned bool) error {
	if escrow == nil {
		return ErrNoRecoveryEscrow
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed.Load() {
		return ErrClosed
	}
	machine := c.crypto.Machine()
	if err := c.requireAbandoned(ctx, abandoned); err != nil {
		return err
	}
	pending, err := c.prepareRecoverySetupFor(ctx, abandoned)
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
	// Sin copia en el servidor (comprobado), un secreto de copia que dejara el intento
	// abandonado va cerrado con la clave vieja: se hace una nueva.
	if err := c.setupBackup(ctx, recovery, abandoned); err != nil {
		return err
	}
	if abandoned {
		if err := c.forgetAbandonedOwnerKey(ctx, pending.KeyID); err != nil {
			return err
		}
	}
	if err := c.saveStorageAuthKey(ctx, pending.storageKey()); err != nil {
		return err
	}
	pending.Complete = true
	return c.saveRecoverySetup(ctx, pending)
}

// requireAbandoned comprueba otra vez, ya con el cerrojo, que la cuenta sigue a medias.
func (c *Client) requireAbandoned(ctx context.Context, abandoned bool) error {
	if !abandoned {
		return nil
	}
	ok, err := c.abandonedRecovery(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrRecoveryExists
	}
	return nil
}

// forgetAbandonedOwnerKey: la clave de propietario de la clave vieja no firmó nada (sin
// identidad nunca hubo un dispositivo verificado que escribiera); la sustituye la nueva.
func (c *Client) forgetAbandonedOwnerKey(ctx context.Context, keyID string) error {
	if c.cfg.Bootstrap.StorageProduct == "" {
		return nil
	}
	current, err := c.storageAuthKey(ctx, "")
	if err != nil {
		return nil //nolint:nilerr // sin clave de propietario guardada no hay nada que olvidar.
	}
	stale := current.ID != keyID
	clear(current.Key)
	if stale && c.cfg.Secrets.Delete(c.storageAuthSecretKey()) != nil {
		return ErrSecureStore
	}
	return nil
}

// AbandonedRecovery dice si la cuenta se quedó a medias (ADR 0084, 0085): tiene una
// clave de almacenamiento secreto por defecto, pero el homeserver no tiene identidad de
// firmas cruzadas ni copia de seguridad de claves, y el baúl no tiene secretos. Ningún
// dispositivo llegó a estar verificado, así que esa clave no protegió nada: la cuenta
// se configura como nueva. Todo sale del servidor; una consulta fallida es «no».
func (c *Client) AbandonedRecovery(ctx context.Context) (bool, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.abandonedRecovery(ctx)
}

func (c *Client) abandonedRecovery(ctx context.Context) (bool, error) {
	if c.closed.Load() {
		return false, ErrClosed
	}
	if _, err := c.crypto.Machine().SSSS.GetDefaultKeyID(ctx); errors.Is(err, ssss.ErrNoDefaultKeyAccountDataEvent) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	current, err := c.cli.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: mautrix.DeviceKeysRequest{c.cli.UserID: {}}})
	if err != nil {
		return false, err
	}
	if len(current.Failures) != 0 {
		return false, nil
	}
	_, master := current.MasterKeys[c.cli.UserID]
	_, self := current.SelfSigningKeys[c.cli.UserID]
	_, user := current.UserSigningKeys[c.cli.UserID]
	if master || self || user {
		return false, nil
	}
	if _, err := c.cli.GetKeyBackupLatestVersion(ctx); err == nil {
		return false, nil
	} else if !errors.Is(err, mautrix.MNotFound) {
		return false, err
	}
	var vault json.RawMessage
	err = c.cli.GetAccountData(ctx, ssssVaultEventPrefix+storageSessionsSecretID(c.vaultScope()), &vault)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, mautrix.MNotFound) {
		return false, err
	}
	return true, nil
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
