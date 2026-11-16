// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	mxcrypto "maunium.net/go/mautrix/crypto"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type recoveryFaultSecrets struct {
	Secrets
	writeErr  bool
	dropWrite bool
}

func (s *recoveryFaultSecrets) Write(key string, raw []byte) error {
	if s.writeErr {
		return errors.New("artificial locked keychain")
	}
	if s.dropWrite {
		return nil
	}
	return s.Secrets.Write(key, raw)
}

func setupJournalFixture(t *testing.T) (*Client, *recoverySetup) {
	t.Helper()
	c, key := ownerCacheFixture(t)
	p := &recoverySetup{Version: 1, Account: c.account, KeyID: key.ID, Key: bytes.Clone(key.Key), Metadata: key.Metadata, Seeds: mxcrypto.CrossSigningSeeds{MasterKey: bytes.Repeat([]byte{1}, 32), SelfSigningKey: bytes.Repeat([]byte{2}, 32), UserSigningKey: bytes.Repeat([]byte{3}, 32)}}
	t.Cleanup(p.clear)
	return c, p
}

func TestRecoverySetupJournalIsDurableAndBoundToTheExactAccount(t *testing.T) {
	c, p := setupJournalFixture(t)
	if err := c.saveRecoverySetup(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		reopened, err := newClient(c.cfg)
		if err != nil {
			t.Fatal(err)
		}
		got, err := reopened.readRecoverySetup()
		if err != nil || got == nil || got.Complete || got.storageKey().RecoveryKey() != p.storageKey().RecoveryKey() || !bytes.Equal(got.Seeds.MasterKey, p.Seeds.MasterKey) {
			t.Fatal("journal did not survive reopening", err)
		}
		got.clear()
		_ = reopened.Close()
	}
	for _, change := range []func(*Config){
		func(v *Config) { v.Tenant = "other"; v.Bootstrap.Tenant = "other" },
		func(v *Config) {
			v.Bootstrap.StorageProduct = "mail"
			v.Bootstrap.StorageRoomID = "!mail:matrix.test"
			v.Bootstrap.StorageRoomIDs = []string{v.Bootstrap.StorageRoomID}
		},
		func(v *Config) {
			v.Bootstrap.StorageIdentityVersion = storage.IsolatedIdentity
			v.Bootstrap.MatrixUserID = storage.IsolatedMatrixUserPrefix(storage.Scope{Subject: v.Subject, Tenant: v.Tenant, Product: v.Bootstrap.StorageProduct}) + "matrix.test"
		},
	} {
		cfg := c.cfg
		change(&cfg)
		other, err := newClient(cfg)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := c.cfg.Secrets.Read(c.recoverySetupKey())
		if err != nil {
			t.Fatal(err)
		}
		if err = other.cfg.Secrets.Write(other.recoverySetupKey(), raw); err != nil {
			t.Fatal(err)
		}
		clear(raw)
		if value, err := other.readRecoverySetup(); value != nil || !errors.Is(err, ErrSecureStore) {
			t.Fatal("journal copied into a different product or identity was accepted", err)
		}
		_ = other.Close()
	}
}

func TestIncompleteRecoverySetupCannotBeAcknowledged(t *testing.T) {
	c, p := setupJournalFixture(t)
	if err := c.saveRecoverySetup(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	before, err := c.cfg.Secrets.Read(c.recoverySetupKey())
	if err != nil {
		t.Fatal(err)
	}
	defer clear(before)
	if err = c.AcknowledgeRecoverySetup(t.Context()); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("incomplete setup discarded its only recovery material", err)
	}
	after, err := c.cfg.Secrets.Read(c.recoverySetupKey())
	defer clear(after)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected acknowledgment changed private custody", err)
	}
}

func TestRecoverySetupJournalRejectsCorruptionAndFailedPersistence(t *testing.T) {
	c, p := setupJournalFixture(t)
	for _, mutate := range []func(*recoverySetup){
		func(v *recoverySetup) { v.Version = 2 },
		func(v *recoverySetup) { v.Account = "other" },
		func(v *recoverySetup) { v.Key[0] ^= 1 },
		func(v *recoverySetup) { v.Seeds.MasterKey = nil },
	} {
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var changed recoverySetup
		if err = json.Unmarshal(raw, &changed); err != nil {
			t.Fatal(err)
		}
		clear(raw)
		mutate(&changed)
		raw, err = json.Marshal(changed)
		changed.clear()
		if err != nil {
			t.Fatal(err)
		}
		if err = c.cfg.Secrets.Write(c.recoverySetupKey(), raw); err != nil {
			t.Fatal(err)
		}
		clear(raw)
		if value, err := c.readRecoverySetup(); value != nil || !errors.Is(err, ErrSecureStore) {
			t.Fatal("corrupt journal accepted", err)
		}
	}
	if err := c.cfg.Secrets.Delete(c.recoverySetupKey()); err != nil {
		t.Fatal(err)
	}
	base := c.cfg.Secrets
	for _, fault := range []*recoveryFaultSecrets{{Secrets: base, writeErr: true}, {Secrets: base, dropWrite: true}} {
		c.cfg.Secrets = fault
		if err := c.saveRecoverySetup(t.Context(), p); !errors.Is(err, ErrSecureStore) {
			t.Fatal("setup proceeded without durable key custody", err)
		}
	}
	c.cfg.Secrets = base
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.saveRecoverySetup(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled setup persisted new material", err)
	}
	if value, err := c.readRecoverySetup(); value != nil || !errors.Is(err, errNoRecoverySetup) {
		t.Fatal("failed setup left a different key", err)
	}
}
