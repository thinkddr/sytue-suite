// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNativeRecoveryIsAccountScopedLoginBoundAndSeparateFromDeviceSecrets(t *testing.T) {
	drive, key := ownerCacheFixture(t)
	shared := &memorySecrets{values: map[string][]byte{}}
	revoked := false
	authorize := func(ctx context.Context) error {
		if revoked {
			return ErrIdentity
		}
		return ctx.Err()
	}
	drive.cfg.NativeRecovery = &NativeRecoveryConfig{Binding: strings.Repeat("a", 32), Secrets: shared, Authorize: authorize}
	cfg := drive.cfg
	cfg.NativeConsumer = "pdf"
	cfg.Secrets = &memorySecrets{values: map[string][]byte{}}
	cfg.StoreDir = t.TempDir()
	consumer, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if consumer.account == drive.account || consumer.nativeRecoveryKey() != drive.nativeRecoveryKey() {
		t.Fatal("recipient reused device account or lost scoped recovery")
	}
	if err = consumer.authorizeNativeRecovery(t.Context()); err != nil {
		t.Fatal(err)
	}
	secret, err := drive.encodeStorageAuthKey(key)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(secret)
	encoded, err := json.Marshal(nativeRecoveryEnvelope{Binding: drive.cfg.NativeRecovery.Binding, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	defer clear(encoded)
	got, err := consumer.decodeNativeRecovery(encoded)
	if err != nil || !bytes.Equal(got.Key, key.Key) {
		t.Fatal("consumer could not validate scoped SSSS material", err)
	}
	clear(got.Key)
	consumer.cfg.NativeRecovery.Binding = strings.Repeat("b", 32)
	if consumer.nativeRecoveryKey() == drive.nativeRecoveryKey() {
		t.Fatal("new login selected previous recovery entry")
	}
	if got, err = consumer.decodeNativeRecovery(encoded); got != nil || !errors.Is(err, ErrSecureStore) {
		t.Fatal("recovery crossed login incarnation", err)
	}
	consumer.cfg.NativeRecovery.Binding = drive.cfg.NativeRecovery.Binding
	consumer.cfg.Subject = "foreign"
	if got, err = consumer.decodeNativeRecovery(encoded); got != nil || !errors.Is(err, ErrSecureStore) {
		t.Fatal("recovery crossed storage owner", err)
	}
	consumer.cfg.Subject = drive.cfg.Subject
	revoked = true
	if err = consumer.authorizeNativeRecovery(t.Context()); !errors.Is(err, ErrIdentity) {
		t.Fatal("revoked session retained recovery authority", err)
	}
	for _, binding := range []string{"", strings.Repeat("A", 32), strings.Repeat("a", 31), strings.Repeat("a", 33)} {
		consumer.cfg.NativeRecovery.Binding = binding
		if err = consumer.authorizeNativeRecovery(t.Context()); !errors.Is(err, ErrIdentity) {
			t.Fatal("malformed recovery binding accepted", err)
		}
	}
}
