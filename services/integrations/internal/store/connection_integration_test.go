// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func connectionFixture(t *testing.T) (*store.Store, *pgtest.Harness, context.Context, string) {
	t.Helper()
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "atomic-mail-fixture")
	ctx := authn.Into(tenantCtx("acme"), authn.Claims{Subject: "alice"})
	i, err := st.CreateInstance(ctx, "atomic-mail-fixture", "mail", json.RawMessage(`{"server":"old"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.BindExternalChatActor(ctx, i.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := st.PutSecrets(ctx, i.ID, map[string]string{"mail_password": "old", store.SecretKeyBearer: "bearer-old", "oauth_refresh": "refresh-old", "unrelated": "keep"}); err != nil {
		t.Fatal(err)
	}
	return st, h, ctx, i.ID
}

func TestConnectionCommitRollsBackCredentialAndServerTogether(t *testing.T) {
	st, h, ctx, id := connectionFixture(t)
	// Invalid status fails the metadata UPDATE after vault INSERTs. Neither the
	// password nor an added vault key may survive the transaction rollback.
	_, err := st.CommitInstanceConnection(ctx, id, "new", json.RawMessage(`{"server":"new"}`), domain.Status("invalid-status"), nil, map[string]string{"mail_password": "new", "added": "must-rollback"})
	if err == nil {
		t.Fatal("invalid metadata unexpectedly committed")
	}
	conn, err := st.LoadInstanceWithSecrets(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Config["server"] != "old" || conn.Secrets["mail_password"] != "old" || conn.Secrets["added"] != "" {
		t.Fatal("failed commit mixed credentials and server")
	}
	updated, err := st.CommitInstanceConnection(ctx, id, "new", json.RawMessage(`{"server":"new"}`), domain.StatusActive, []string{"read"}, map[string]string{"mail_password": "new"})
	if err != nil || updated.Name != "new" || !updated.HasSecrets {
		t.Fatalf("successful commit: %v", err)
	}
	conn, err = st.LoadInstanceWithSecrets(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Config["server"] != "new" || conn.Secrets["mail_password"] != "new" || conn.Secrets["unrelated"] != "keep" {
		t.Fatal("successful commit lost connection metadata or unrelated secret")
	}
	if _, err := store.New(h.Pool, nil).CommitInstanceConnection(ctx, id, "unsafe", nil, domain.StatusActive, nil, map[string]string{"mail_password": "plaintext"}); !errors.Is(err, store.ErrNoCipher) {
		t.Fatalf("missing encryption key accepted: %v", err)
	}
}

func TestConnectionRefreshCannotOverwriteLaterReconnect(t *testing.T) {
	st, _, ctx, id := connectionFixture(t)
	if _, err := st.CommitInstanceConnection(ctx, id, "new account", json.RawMessage(`{"server":"new-account","_mail_config":"new-address"}`), domain.StatusActive, nil, map[string]string{store.SecretKeyBearer: "bearer-new", "oauth_refresh": "refresh-new"}); err != nil {
		t.Fatal(err)
	}
	applied, err := st.RefreshInstanceConnection(ctx, id, "bearer-old", map[string]string{"_oauth_expires": "old-expiry"}, map[string]string{store.SecretKeyBearer: "stale-refresh", "oauth_refresh": "stale-rotation"})
	if err != nil || applied {
		t.Fatalf("stale refresh applied: %v", err)
	}
	conn, err := st.LoadInstanceWithSecrets(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Config["_mail_config"] != "new-address" || conn.Secrets[store.SecretKeyBearer] != "bearer-new" || conn.Secrets["oauth_refresh"] != "refresh-new" {
		t.Fatal("late refresh reverted reconnected identity")
	}
	applied, err = st.RefreshInstanceConnection(ctx, id, "bearer-new", map[string]string{"_oauth_expires": "fresh-expiry"}, map[string]string{store.SecretKeyBearer: "fresh-token"})
	if err != nil || !applied {
		t.Fatalf("current refresh rejected: %v", err)
	}
	conn, err = st.LoadInstanceWithSecrets(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Config["_mail_config"] != "new-address" || conn.Config["_oauth_expires"] != "fresh-expiry" || conn.Secrets[store.SecretKeyBearer] != "fresh-token" || conn.Secrets["oauth_refresh"] != "refresh-new" {
		t.Fatal("refresh patch replaced account metadata or refresh credential")
	}
}

func TestConnectionAtomicMethodsKeepPersonalAndTenantOwnership(t *testing.T) {
	st, _, ctx, id := connectionFixture(t)
	admin := authn.Into(tenantCtx("acme"), authn.Claims{Subject: "admin", Roles: []string{"owner"}})
	for _, other := range []context.Context{admin, authn.Into(tenantCtx("other"), authn.Claims{Subject: "alice"})} {
		if _, err := st.CommitInstanceConnection(other, id, "stolen", nil, domain.StatusActive, nil, map[string]string{"mail_password": "replacement"}); err == nil {
			t.Fatal("foreign account commit accepted")
		}
		if _, err := st.RefreshInstanceConnection(other, id, "bearer-old", nil, map[string]string{store.SecretKeyBearer: "stolen"}); err == nil {
			t.Fatal("foreign account refresh accepted")
		}
	}
	conn, err := st.LoadInstanceWithSecrets(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Config["server"] != "old" || conn.Secrets[store.SecretKeyBearer] != "bearer-old" {
		t.Fatal("foreign update modified personal connection")
	}
}

func TestConnectionReaderNeverMixesConcurrentCredentialVersions(t *testing.T) {
	st, _, ctx, id := connectionFixture(t)
	var wg sync.WaitGroup
	errorsFound := make(chan error, 2)
	wg.Go(func() {
		for n := range 60 {
			version := fmt.Sprintf("version-%d", n)
			config, _ := json.Marshal(map[string]string{"server": version})
			if _, err := st.CommitInstanceConnection(ctx, id, "mail", config, domain.StatusActive, nil, map[string]string{"mail_password": version}); err != nil {
				errorsFound <- err
				return
			}
		}
	})
	wg.Go(func() {
		for range 160 {
			conn, err := st.LoadInstanceWithSecrets(ctx, id)
			if err != nil {
				errorsFound <- err
				return
			}
			if conn.Config["server"] != conn.Secrets["mail_password"] {
				errorsFound <- errors.New("reader mixed credential and server versions")
				return
			}
		}
	})
	wg.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
}
