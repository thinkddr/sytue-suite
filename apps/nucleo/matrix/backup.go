// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"maunium.net/go/mautrix"
	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/backup"
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var ErrRecoveryRequired = errors.New("matrix: unlock recovery before backing up encryption keys")

type backupSecret struct {
	Version id.KeyBackupVersion `json:"version"`
	Key     []byte              `json:"key"`
}

func (c *Client) backupSecretKey() string { return "rooms-matrix-backup-" + c.account }

// recoveryStorageKey rejects unverifiable metadata rather than silently accepting
// a recovery key whose identity could not be checked.
func (c *Client) recoveryStorageKey(ctx context.Context, recovery string) (*ssss.Key, error) {
	keyID, metadata, err := c.crypto.Machine().SSSS.GetDefaultKeyData(ctx)
	if err != nil {
		return nil, err
	}
	return metadata.VerifyRecoveryKey(keyID, recovery)
}

// prepareBackup shares the standard SSSS backup secret with other verified clients.
// It never replaces an existing backup when its secret is absent or inconsistent.
func (c *Client) prepareBackup(ctx context.Context, recovery string) error {
	machine := c.crypto.Machine()
	storageKey, err := c.recoveryStorageKey(ctx, recovery)
	if err != nil {
		return err
	}
	privateKey, err := machine.SSSS.GetDecryptedAccountData(ctx, event.AccountDataMegolmBackupKey, storageKey)
	if errors.Is(err, mautrix.MNotFound) {
		if _, latestErr := c.cli.GetKeyBackupLatestVersion(ctx); !errors.Is(latestErr, mautrix.MNotFound) {
			if latestErr != nil {
				return latestErr
			}
			return ErrRecoveryExists
		}
		generated, generateErr := backup.NewMegolmBackupKey()
		if generateErr != nil {
			return generateErr
		}
		privateKey = generated.Bytes()
		// Save the encrypted recovery material before creating the remote version,
		// so an interrupted setup can be resumed without losing a new private key.
		if err = machine.SSSS.SetEncryptedAccountData(ctx, event.AccountDataMegolmBackupKey, privateKey, storageKey); err != nil {
			clear(privateKey)
			return err
		}
	} else if err != nil {
		return err
	}
	defer clear(privateKey)
	key, err := backup.MegolmBackupKeyFromBytes(privateKey)
	if err != nil {
		return err
	}
	versionInfo, err := machine.GetAndVerifyLatestKeyBackupVersion(ctx, key)
	if errors.Is(err, mautrix.MNotFound) {
		if machine.CrossSigningKeys == nil {
			return ErrUnverified
		}
		authData := backup.MegolmAuthData{PublicKey: id.Ed25519(base64.RawStdEncoding.EncodeToString(key.PublicKey().Bytes()))}
		signature, signErr := machine.CrossSigningKeys.MasterKey.SignJSON(authData)
		if signErr != nil {
			return signErr
		}
		authData.Signatures = signatures.Signatures{c.cli.UserID: {
			id.NewKeyID(id.KeyAlgorithmEd25519, machine.CrossSigningKeys.MasterKey.PublicKey().String()): signature,
		}}
		created, createErr := c.cli.CreateKeyBackupVersion(ctx, &mautrix.ReqRoomKeysVersionCreate[backup.MegolmAuthData]{Algorithm: id.KeyBackupAlgorithmMegolmBackupV1, AuthData: authData})
		if createErr != nil {
			return createErr
		}
		if created == nil || created.Version == "" {
			return errors.New("matrix: missing backup version receipt")
		}
		versionInfo, err = machine.GetAndVerifyLatestKeyBackupVersion(ctx, key)
	}
	if err != nil {
		return err
	}
	if versionInfo == nil || versionInfo.Version == "" || versionInfo.AuthData.PublicKey != id.Ed25519(base64.RawStdEncoding.EncodeToString(key.PublicKey().Bytes())) {
		return errors.New("matrix: backup version is unavailable")
	}
	return c.saveBackupSecret(ctx, versionInfo.Version, privateKey)
}

func (c *Client) saveBackupSecret(ctx context.Context, version id.KeyBackupVersion, privateKey []byte) error {
	secret, err := json.Marshal(backupSecret{Version: version, Key: privateKey})
	if err != nil {
		return err
	}
	defer clear(secret)
	if err = c.cfg.Secrets.Write(c.backupSecretKey(), secret); err != nil {
		return ErrSecureStore
	}
	return c.crypto.Machine().SetKeyBackupVersion(ctx, version)
}

func (c *Client) readBackup(ctx context.Context) (*backup.MegolmBackupKey, id.KeyBackupVersion, error) {
	if c.closed.Load() {
		return nil, "", ErrClosed
	}
	data, err := c.cfg.Secrets.Read(c.backupSecretKey())
	if err != nil {
		return nil, "", ErrSecureStore
	}
	defer clear(data)
	if len(data) == 0 {
		return nil, "", ErrRecoveryRequired
	}
	var secret backupSecret
	if json.Unmarshal(data, &secret) != nil || secret.Version == "" || len(secret.Key) != 32 {
		return nil, "", ErrSecureStore
	}
	defer clear(secret.Key)
	key, err := backup.MegolmBackupKeyFromBytes(secret.Key)
	if err != nil {
		return nil, "", err
	}
	version, err := c.crypto.Machine().GetAndVerifyLatestKeyBackupVersion(ctx, key)
	if err != nil {
		return nil, "", err
	}
	if version == nil || version.Version != secret.Version || version.AuthData.PublicKey != id.Ed25519(base64.RawStdEncoding.EncodeToString(key.PublicKey().Bytes())) {
		return nil, "", ErrRecoveryRequired
	}
	return key, version.Version, nil
}

// Backup uploads batches of standard encrypted Megolm session backups. Private
// session material is encrypted by maintained mautrix backup code before HTTP.
// A failed upload never marks the local session as backed up; retry is safe.
func (c *Client) Backup(ctx context.Context) error {
	if c.closed.Load() {
		return ErrClosed
	}
	c.backupMu.Lock()
	defer c.backupMu.Unlock()
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	key, version, err := c.readBackup(ctx)
	if err != nil {
		return err
	}
	machine := c.crypto.Machine()
	for {
		var batch []*mxcrypto.InboundGroupSession
		err = machine.CryptoStore.GetGroupSessionsWithoutKeyBackupVersion(ctx, version).Iter(func(session *mxcrypto.InboundGroupSession) (bool, error) {
			if c.allowedRoom(session.RoomID.String()) {
				batch = append(batch, session)
			}
			return len(batch) < 100, nil
		})
		if err != nil || len(batch) == 0 {
			return err
		}
		request := &mautrix.ReqKeyBackup{Rooms: map[id.RoomID]mautrix.ReqRoomKeyBackup{}}
		for _, session := range batch {
			exported, exportErr := session.Internal.Export(session.Internal.FirstKnownIndex())
			if exportErr != nil {
				return exportErr
			}
			payload, encryptErr := backup.EncryptSessionData(key, backup.MegolmSessionData{Algorithm: id.AlgorithmMegolmV1, ForwardingKeyChain: session.ForwardingChains, SenderClaimedKeys: backup.SenderClaimedKeys{Ed25519: session.SigningKey}, SenderKey: session.SenderKey, SessionKey: string(exported), SharedHistory: session.SharedHistory})
			clear(exported)
			if encryptErr != nil {
				return encryptErr
			}
			encoded, marshalErr := json.Marshal(payload)
			if marshalErr != nil {
				return marshalErr
			}
			room := request.Rooms[session.RoomID]
			if room.Sessions == nil {
				room.Sessions = map[id.SessionID]mautrix.ReqKeyBackupData{}
			}
			room.Sessions[session.ID()] = mautrix.ReqKeyBackupData{FirstMessageIndex: int(session.Internal.FirstKnownIndex()), ForwardedCount: len(session.ForwardingChains), IsVerified: false, SessionData: encoded}
			request.Rooms[session.RoomID] = room
		}
		if _, err = c.cli.PutKeysInBackup(ctx, version, request); err != nil {
			return err
		}
		for _, session := range batch {
			session.KeyBackupVersion = version
			if err := machine.CryptoStore.PutGroupSession(ctx, session); err != nil {
				return err
			}
		}
	}
}

// RestoreBackup authenticates the standard backup and imports only this Rooms
// session's allowed rooms. Corrupt sessions surface an error instead of silently
// reporting complete recovery. Message plaintext is never part of the backup.
func (c *Client) RestoreBackup(ctx context.Context) error {
	if c.closed.Load() {
		return ErrClosed
	}
	c.backupMu.Lock()
	defer c.backupMu.Unlock()
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.restoreBackup(ctx)
}

func (c *Client) restoreBackup(ctx context.Context) error {
	key, version, err := c.readBackup(ctx)
	if err != nil {
		return err
	}
	stored, err := c.cli.GetKeyBackup(ctx, version)
	if err != nil {
		return err
	}
	for roomID, room := range stored.Rooms {
		if !c.allowedRoom(roomID.String()) {
			continue
		}
		for sessionID, encrypted := range room.Sessions {
			data, decryptErr := encrypted.SessionData.Decrypt(key)
			if decryptErr != nil {
				return fmt.Errorf("matrix: recovery contains an invalid session: %w", decryptErr)
			}
			if _, err = c.crypto.Machine().ImportRoomKeyFromBackup(ctx, version, roomID, sessionID, data); err != nil {
				return err
			}
		}
	}
	return c.crypto.Machine().FlushStore(ctx)
}

// Coalescing prevents a large sync response from starting unbounded backup jobs.
func (c *Client) scheduleBackup() {
	select {
	case c.backupSignal <- struct{}{}:
	default:
	}
}

func (c *Client) backupLoop(ctx context.Context) {
	for {
		select {
		case <-c.backupSignal:
			err := c.Backup(ctx)
			if err != nil && !errors.Is(err, ErrRecoveryRequired) && ctx.Err() == nil {
				c.deliver(DecryptedEvent{}, err)
			}
		case <-ctx.Done():
			return
		}
	}
}
