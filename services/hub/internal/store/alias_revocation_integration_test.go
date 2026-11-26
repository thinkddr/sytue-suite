// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

func TestAliasRevocadosConClaimsYRolesDeBase(t *testing.T) {
	for _, bypass := range []bool{false, true} {
		t.Run(fmt.Sprintf("bypass_rls_%t", bypass), func(t *testing.T) {
			h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
			st := store.New(h.Pool)
			if bypass {
				st = store.New(h.PoolSinRLS(t))
			}
			retired := time.Now().Add(-time.Minute).Truncate(time.Second)
			cases := []struct {
				name      string
				issued    time.Time
				delegated bool
				loginOnly bool
				want      int
				forbid    bool
			}{
				{"old", retired.Add(-time.Second), false, false, 2, true},
				{"delegated_old", retired.Add(-time.Second), true, false, 1, true},
				{"login_only_old", retired.Add(-time.Second), false, true, 2, true},
				{"delegated_login_only_old", retired.Add(-time.Second), true, true, 1, true},
				{"missing_iat", time.Time{}, false, false, 2, true},
				{"zero_iat", time.Unix(0, 0), false, false, 2, true},
				{"negative_iat", time.Unix(-1, 0), false, false, 2, true},
				{"delegated_missing_iat", time.Time{}, true, false, 1, true},
				{"future_iat", time.Now().Add(time.Hour), false, false, 2, true},
				{"delegated_future_iat", time.Now().Add(time.Hour), true, false, 1, true},
				{"same_second", retired, false, false, 2, true},
				{"new", retired.Add(time.Second), false, false, 5, false},
				{"delegated_new", retired.Add(time.Second), true, false, 3, false},
				{"login_only_new", retired.Add(time.Second), false, true, 3, false},
			}
			for i, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					// Stable subjects do not depend on recipient-list ordering.
					sub := uuid.NewString()
					if tc.name == "old" {
						sub = "stable-non-uuid"
					}
					login := fmt.Sprintf("login-%d@example.test", i)
					mailbox := fmt.Sprintf("box-%d@example.test", i)
					other := fmt.Sprintf("other-%d@example.test", i)
					aliases := []string{hubnotify.PorCorreo(login), hubnotify.PorCorreo(mailbox), hubnotify.PorCorreo(other)}
					for _, address := range aliases {
						if _, err := st.OlvidarBuzon(context.Background(), address); err != nil {
							t.Fatal(err)
						}
						if _, err := h.PoolSinRLS(t).Exec(t.Context(), `UPDATE buzones_olvidados SET olvidado=$1 WHERE buzon=$2`, retired, address); err != nil {
							t.Fatal(err)
						}
					}
					acme, otherOrg := tenantCtx("acme"), tenantCtx("other")
					var loginID uuid.UUID
					for _, row := range []struct {
						ctx        context.Context
						sub, title string
					}{
						{acme, sub, "stable local"},
						{otherOrg, sub, "stable cross"},
						{acme, aliases[0], "login"},
						{acme, aliases[1], "mailbox"},
						{otherOrg, aliases[2], "other mailbox"},
						{acme, "unrelated", "unrelated"},
					} {
						n, err := st.CreateNotification(row.ctx, sampleNotification(row.sub, row.title))
						if err != nil {
							t.Fatal(err)
						}
						if row.title == "login" {
							loginID = n.ID
						}
					}
					claims := authn.Claims{Subject: sub, Tenant: "acme", Email: login, Mailbox: mailbox, Mailboxes: []string{other}, Delegado: tc.delegated, Emitido: tc.issued}
					if tc.loginOnly {
						claims.Mailbox, claims.Mailboxes = "", nil
					}
					var personal []string
					if !tc.delegated {
						personal = domain.BuzonesPersonales(sub, append([]string{claims.Mailbox}, claims.Mailboxes...)...)
					}
					ctx := domain.ConBuzonesPersonales(authn.Into(acme, claims), sub, personal, tc.issued)
					// The input order and unrelated entries cannot extend token authority.
					supplied := []string{"unrelated", aliases[0], sub, aliases[1]}
					rows, err := st.ListNotifications(ctx, supplied, false, nil, 100)
					if err != nil || len(rows) != tc.want {
						t.Fatalf("list=%d want=%d err=%v", len(rows), tc.want, err)
					}
					count, err := st.UnreadCount(ctx, supplied)
					if err != nil || count != tc.want {
						t.Fatalf("unread=%d want=%d err=%v", count, tc.want, err)
					}
					if tc.forbid {
						if err := st.MarkRead(ctx, loginID, supplied); !errors.Is(err, domain.ErrNotFound) {
							t.Fatalf("retired login marked: %v", err)
						}
					}
					changed, err := st.MarkAllRead(ctx, supplied, "")
					if err != nil || changed != int64(tc.want) {
						t.Fatalf("marked=%d want=%d err=%v", changed, tc.want, err)
					}
					wrong := domain.ConBuzonesPersonales(authn.Into(acme, claims), "other-sub", personal, tc.issued)
					if _, err := st.UnreadCount(wrong, supplied); !errors.Is(err, domain.ErrInvalidInput) {
						t.Fatalf("mismatched subject accepted: %v", err)
					}
					if _, err := st.UnreadCount(acme, supplied); !errors.Is(err, domain.ErrInvalidInput) {
						t.Fatalf("missing identity accepted: %v", err)
					}
					wrongIAT := domain.ConBuzonesPersonales(authn.Into(acme, claims), sub, personal, tc.issued.Add(-time.Second))
					if _, err := st.UnreadCount(wrongIAT, supplied); !errors.Is(err, domain.ErrInvalidInput) {
						t.Fatalf("mismatched issued-at accepted: %v", err)
					}
				})
			}
		})
	}
}
