// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package offline

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
)

func TestEncryptedJournalLeaseExcludesOtherStoresAndSurvivesClear(t *testing.T) {
	dir := t.TempDir()
	secrets := &authtest.Secrets{}
	one, err := OpenWithSecrets(dir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	two, err := OpenWithSecrets(dir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if !one.Encrypted() || (&Store{}).Encrypted() {
		t.Fatal("encrypted capability misreported")
	}
	if err := one.Exclusive(t.Context(), "owner", "jobs", "id", func() error {
		if err := one.Write("owner", "jobs", "id", map[string]string{"status": "pending"}); err != nil {
			return err
		}
		// Clear removes the record, not its lease inode. A waiter must not acquire
		// a fresh lock while the previous operation still owns the original inode.
		if err := two.Clear(); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
		defer cancel()
		ran := false
		err := two.Exclusive(ctx, "owner", "jobs", "id", func() error { ran = true; return nil })
		if !errors.Is(err, context.DeadlineExceeded) || ran {
			return errors.New("concurrent journal acquired a replaced lease")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := two.Exclusive(t.Context(), "owner", "jobs", "id", func() error { return nil }); err != nil {
		t.Fatal("released lease remained locked", err)
	}
	if err := (&Store{}).Exclusive(t.Context(), "owner", "jobs", "id", func() error { return nil }); !errors.Is(err, ErrEncryptedStore) {
		t.Fatal("plaintext backend accepted private journal", err)
	}
}
