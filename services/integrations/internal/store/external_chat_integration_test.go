// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestPersonalExternalAccountCannotBeDelegatedToOrgAdmin(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "slack-chat")
	alice := authn.Into(tenantCtx("acme"), authn.Claims{Subject: "alice"})
	admin := authn.Into(tenantCtx("acme"), authn.Claims{Subject: "admin", Roles: []string{"owner"}})
	inst, err := st.CreateInstance(alice, "slack-chat", "Personal Slack", json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.BindExternalChatActor(alice, inst.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := st.RequireExternalChatActor(admin, inst.ID, "admin"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin bypassed personal binding: %v", err)
	}
	if err := st.BindExternalChatActor(admin, inst.ID, "admin"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin reassigned account: %v", err)
	}
	if err := st.RequireExternalChatActor(tenantCtx("other"), inst.ID, "alice"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("cross-tenant account: %v", err)
	}
	if err := st.RequireExternalChatActor(alice, inst.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListInstances(admin)
	if err != nil || len(list) != 0 {
		t.Fatalf("private metadata exposed: %+v %v", list, err)
	}
	own, err := st.ListExternalChatInstances(alice, "alice")
	if err != nil || len(own) != 1 || own[0].ID != inst.ID {
		t.Fatalf("own accounts=%+v err=%v", own, err)
	}
}
