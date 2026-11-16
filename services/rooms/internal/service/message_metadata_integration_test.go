//go:build integration

// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	roomsearch "git.sytue.eu/sytue/one/apps/nucleo/rooms/search"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestMessageMetadataRealPostgresPreservesACLRepliesTiesAndRevocation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	svc := New(config.Config{MatrixRequired: true}, st, nil, nil)
	own := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})
	team, err := st.CreateTeam(own, store.NewTeamRecord{Slug: "search", Name: "Search", CreatedBy: "alice", CreatorName: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"bob", "carol"} {
		if err := st.AddTeamMember(own, team.ID, "alice", sub, sub, domain.RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	public, err := st.CreateChannel(own, store.NewChannelRecord{TeamID: team.ID, Slug: "history", Name: "History", Kind: domain.ChannelPublic, CreatedBy: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	private, err := st.CreateChannel(own, store.NewChannelRecord{TeamID: team.ID, Slug: "private", Name: "Private", Kind: domain.ChannelPrivate, CreatedBy: "carol"})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 2, 12, 0, 0, 123456000, time.UTC)
	ids := make([]uuid.UUID, 307)
	for i := range ids {
		ids[i] = uuid.New()
	}
	if err := h.Pool.InTenantTx(own, func(tx pgx.Tx) error {
		for i, id := range ids {
			var parent any
			if i > 0 {
				parent = ids[0]
			}
			body, room, event := "", "!opaque:fixture", "$opaque_"+id.String()
			if i == len(ids)-1 {
				body, room, event = "legacy sentinel NEVER IN METADATA", "", ""
			}
			_, err := tx.Exec(own, `INSERT INTO messages(tenant,id,channel_id,parent_id,author_sub,author_name,body,created_at,matrix_room_id,matrix_event_id) VALUES($1,$2,$3,$4,'alice','Álice',$5,$6,$7,$8)`, "acme", id, public.ID, parent, body, at, room, event)
			if err != nil {
				return err
			}
		}
		_, err := tx.Exec(own, `INSERT INTO messages(tenant,id,channel_id,author_sub,body,matrix_room_id,matrix_event_id) VALUES('acme',$1,$2,'carol','','!private:fixture','$private')`, uuid.New(), private.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMessage(own, ids[3], "bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PinMessage(own, ids[4], "alice"); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	cursor := ""
	pages, replies := 0, 0
	saved, pinned := false, false
	for {
		page, err := svc.MessageMetadata(own, "bob", cursor, 17)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		encoded, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "sentinel") || strings.Contains(string(encoded), `"body"`) {
			t.Fatal("opaque scan disclosed content")
		}
		for _, row := range page.Messages {
			if row.ChannelID != public.ID.String() || seen[row.ID] {
				t.Fatal("ACL leak or duplicate across equal timestamps", row.ID)
			}
			seen[row.ID] = true
			if row.ParentID != nil {
				replies++
			}
			saved = saved || row.Saved
			pinned = pinned || row.Pinned
		}
		if page.Next == "" {
			break
		}
		if page.Next == cursor {
			t.Fatal("cursor did not advance")
		}
		cursor = page.Next
	}
	if len(seen) != 307 || replies != 306 || pages < 18 || !saved || !pinned {
		t.Fatal("scan truncated history or lost metadata", len(seen), replies, pages, saved, pinned)
	}
	// The public channel is visible to a team member who never joined it.
	if channels, err := st.ListChannels(own, team.ID, "bob", false); err != nil {
		t.Fatal(err)
	} else {
		for _, ch := range channels {
			if ch.ID == public.ID {
				t.Fatal("fixture unexpectedly joined public history")
			}
		}
	}
	if err := st.RemoveTeamMember(own, team.ID, "alice", "bob"); err != nil {
		t.Fatal(err)
	}
	visible, err := svc.MessageMetadataVisible(own, "bob", []roomsearch.Identity{{ID: ids[3].String(), Tenant: "acme"}})
	if err != nil || len(visible) != 0 {
		t.Fatal("revoked team access survived revalidation", visible, err)
	}

	// A stale channel-member row must not bypass a removed team membership.
	if err := h.Pool.InTenantTx(own, func(tx pgx.Tx) error {
		_, err := tx.Exec(own, `INSERT INTO channel_members(tenant,channel_id,subject,role) VALUES('acme',$1,'bob','member') ON CONFLICT DO NOTHING`, private.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	pageAfterRemoval, err := svc.MessageMetadata(own, "bob", "", 17)
	if err != nil || len(pageAfterRemoval.Messages) != 0 {
		t.Fatal("orphan private membership bypassed team ACL", pageAfterRemoval, err)
	}
	legacyAfterRemoval, err := st.SearchAll(own, "bob", "", 100)
	if err != nil || len(legacyAfterRemoval) != 0 {
		t.Fatal("compatibility search diverged from metadata ACL", legacyAfterRemoval, err)
	}
	// Tombstones protect a still-valid JWT even when membership cleanup lags.
	if err := h.Pool.InTenantTx(own, func(tx pgx.Tx) error {
		_, err := tx.Exec(own, `INSERT INTO matrix_tenant_access(tenant,subject,status,version,applied) VALUES('acme','alice','removed',1,true)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	removedOrg, err := svc.MessageMetadata(own, "alice", "", 17)
	if err != nil || len(removedOrg.Messages) != 0 {
		t.Fatal("removed org identity retained metadata", removedOrg, err)
	}
	if err := h.Pool.InMaintenanceTx(t.Context(), func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO matrix_identity_access(subject,status,version,applied) VALUES('carol','suspended',1,true)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MessageMetadata(own, "carol", "", 17); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("suspended identity retained metadata", err)
	}
	if _, err := st.SearchAll(own, "carol", "", 100); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("suspended identity retained compatibility search", err)
	}

	// An external guest gets exactly the invited channel, never the host's other
	// public/private history, even with an administrative token in its own tenant.
	other := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "other"})
	dm, err := st.OpenGroupDirect(own, team.ID, "alice", []string{"bea"}, map[string]string{"bea": "Bea"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AceptarInvitacion(context.Background(), "bea", "other", dm.ID); err != nil {
		t.Fatal(err)
	}
	external, err := st.CreateMessage(own, store.NewMessageRecord{ChannelID: dm.ID, AuthorSub: "alice", Body: "external sentinel"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(own, func(tx pgx.Tx) error {
		_, err := tx.Exec(own, `UPDATE messages SET body='',matrix_room_id='!external:fixture',matrix_event_id='$external' WHERE tenant='acme' AND id=$1`, external.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	page, err := svc.MessageMetadata(other, "bea", "", 17)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != external.ID.String() {
		t.Fatal("external scan broadened host access", page, err)
	}
	if err := st.HideDirect(own, dm.ID, "bea"); err != nil {
		t.Fatal(err)
	}
	page, err = svc.MessageMetadata(other, "bea", "", 17)
	if err != nil || len(page.Messages) != 0 {
		t.Fatal("hidden direct history remained searchable", page, err)
	}
	fresh, err := st.CreateMessage(own, store.NewMessageRecord{ChannelID: dm.ID, AuthorSub: "alice", Body: "fresh external sentinel"})
	if err != nil {
		t.Fatal(err)
	}
	page, err = svc.MessageMetadata(other, "bea", "", 17)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != fresh.ID.String() {
		t.Fatal("cleared direct history returned after reactivation", page, err)
	}
	// Simulate account/guest offboarding at its durable membership boundary;
	// two-person DMs intentionally forbid the ordinary member-management route.
	if err := h.Pool.InTenantTx(own, func(tx pgx.Tx) error {
		_, err := tx.Exec(own, `DELETE FROM channel_members WHERE tenant='acme' AND channel_id=$1 AND subject='bea'`, dm.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	page, err = svc.MessageMetadata(other, "bea", "", 17)
	if err != nil || len(page.Messages) != 0 {
		t.Fatal("revoked external channel survived scan", page, err)
	}
	t.Log(fmt.Sprintf("opaque real PG scan preserved %d same-time rows and %d replies; public team, private ACL, save/pin, guest, hide/cut and revocation verified", len(seen), replies))
}
