// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// The actual transaction helpers execute after the prefilter, not a copy of
// their SQL. Forget commits and a new owner's notification appears between
// those two statements, without timing sleeps or blocking a database lock. The
// connection default is REPEATABLE READ to verify the explicit transaction level.
func TestNotificacionVuelveAComprobarOlvidoEnLaSentencia(t *testing.T) {
	for _, bypass := range []bool{false, true} {
		t.Run(fmt.Sprintf("bypass_%t", bypass), func(t *testing.T) {
			h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
			dsn := h.AppDSN
			if bypass {
				dsn = h.DSN
			}
			cfg, err := pgxpool.ParseConfig(dsn)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ConnConfig.RuntimeParams == nil {
				cfg.ConnConfig.RuntimeParams = make(map[string]string)
			}
			cfg.ConnConfig.RuntimeParams["default_transaction_isolation"] = "repeatable read"
			rawPool, err := pgxpool.NewWithConfig(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(rawPool.Close)
			pool := &pg.Pool{Pool: rawPool}
			st := New(pool)
			for _, cross := range []bool{false, true} {
				for _, op := range []string{"list", "count", "read", "read_all"} {
					t.Run(fmt.Sprintf("cross_%t_%s", cross, op), func(t *testing.T) {
						ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
						defer cancel()
						sub := uuid.NewString()
						email := uuid.NewString() + "@example.test"
						alias := hubnotify.PorCorreo(email)
						claims := authn.Claims{Subject: sub, Tenant: "acme", Emitido: time.Now().Add(-time.Minute)}
						if cross {
							claims.Mailboxes = []string{email}
						} else {
							claims.Email = email
						}
						acme := tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
						personal := domain.BuzonesPersonales(sub, append([]string{claims.Mailbox}, claims.Mailboxes...)...)
						request := domain.ConBuzonesPersonales(authn.Into(acme, claims), sub, personal, claims.Emitido)
						subjects := domain.Destinatarios(sub, claims.Email, claims.Mailbox)
						stable, err := st.CreateNotification(acme, domain.NewNotification{Subject: sub, Source: domain.SourceDrive, Type: "test", Level: domain.LevelInfo, Title: "stable"})
						if err != nil {
							t.Fatal(err)
						}
						ready, ids, result := make(chan error, 1), make(chan uuid.UUID, 1), make(chan error, 1)
						go func() {
							result <- st.enLecturaDeAvisos(request, func(tx pgx.Tx) error {
								var defaultLevel, actualLevel string
								if err := tx.QueryRow(request, "SHOW default_transaction_isolation").Scan(&defaultLevel); err != nil {
									ready <- err
									return err
								}
								if err := tx.QueryRow(request, "SHOW transaction_isolation").Scan(&actualLevel); err != nil {
									ready <- err
									return err
								}
								if defaultLevel != "repeatable read" || actualLevel != "read committed" {
									err := fmt.Errorf("isolation: default=%q transaction=%q", defaultLevel, actualLevel)
									ready <- err
									return err
								}
								d, err := conDestinatarios(request, tx, subjects)
								if err == nil && !(slices.Contains(d.enLaOrganizacion, alias) || slices.Contains(d.deLaPersona, alias)) {
									err = errors.New("prefilter unexpectedly removed alias before forget")
								}
								ready <- err
								if err != nil {
									return err
								}
								var id uuid.UUID
								select {
								case id = <-ids:
								case <-request.Done():
									return request.Err()
								}
								switch op {
								case "list":
									rows, err := listarNotificacionesTx(request, tx, d, false, nil, 100, uuid.Nil)
									if err != nil {
										return err
									}
									if len(rows) != 1 || rows[0].ID != stable.ID {
										return fmt.Errorf("list returned %d rows or missed stable subject", len(rows))
									}
								case "count":
									n, err := contarNotificacionesTx(request, tx, d)
									if err != nil {
										return err
									}
									if n != 1 {
										return fmt.Errorf("count=%d", n)
									}
								case "read":
									if err := marcarNotificacionTx(request, tx, id, d); !errors.Is(err, domain.ErrNotFound) {
										return fmt.Errorf("read retired alias: %v", err)
									}
								case "read_all":
									n, err := marcarTodasNotificacionesTx(request, tx, d, "")
									if err != nil {
										return err
									}
									if n != 1 {
										return fmt.Errorf("marked=%d", n)
									}
								}
								return nil
							})
						}()
						select {
						case err := <-ready:
							if err != nil {
								t.Fatal(err)
							}
						case err := <-result:
							t.Fatalf("transaction ended before prefilter: %v", err)
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						}
						if _, err := st.OlvidarBuzon(ctx, alias); err != nil {
							t.Fatal(err)
						}
						writer := acme
						if cross {
							writer = tenancy.Into(ctx, tenancy.Tenant{Slug: "other"})
						}
						n, err := st.CreateNotification(writer, domain.NewNotification{Subject: alias, Source: domain.SourceDrive, Type: "test", Level: domain.LevelInfo, Title: "new owner"})
						if err != nil {
							t.Fatal(err)
						}
						ids <- n.ID
						select {
						case err := <-result:
							if err != nil {
								t.Fatal(err)
							}
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						}
						var read bool
						if err := h.PoolSinRLS(t).QueryRow(ctx, `SELECT read FROM notifications WHERE id=$1`, n.ID).Scan(&read); err != nil || read {
							t.Fatalf("new owner's row changed: read=%t err=%v", read, err)
						}
					})
				}
			}
		})
	}
}

// A read update that began before forget may wait for its DELETE. The test
// holds an explicit DELETE+tombstone transaction at the same point as forget.
// PostgreSQL must not report the deleted row as a successful mark after commit.
// This does not cover a different concurrent row update or later publication.
func TestMarcarLeeRevalidaTrasEsperaPorBorrado(t *testing.T) {
	for _, bypass := range []bool{false, true} {
		t.Run(fmt.Sprintf("bypass_%t", bypass), func(t *testing.T) {
			probarMarcarTrasEsperaPorBorrado(t, bypass)
		})
	}
}

func probarMarcarTrasEsperaPorBorrado(t *testing.T, bypass bool) {
	t.Helper()
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	admin := h.PoolSinRLS(t)
	st := New(h.Pool)
	if bypass {
		st = New(admin)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	sub := uuid.NewString()
	email := uuid.NewString() + "@example.test"
	alias := hubnotify.PorCorreo(email)
	acme := tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	n, err := st.CreateNotification(acme, domain.NewNotification{Subject: alias, Source: domain.SourceDrive, Type: "test", Level: domain.LevelInfo, Title: "old recipient"})
	if err != nil {
		t.Fatal(err)
	}
	forget, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = forget.Rollback(ctx) }()
	var forgetPID int32
	if err := forget.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&forgetPID); err != nil {
		t.Fatal(err)
	}
	if _, err := forget.Exec(ctx, `DELETE FROM notifications WHERE subject=$1`, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := forget.Exec(ctx, `INSERT INTO buzones_olvidados (buzon) VALUES ($1)`, alias); err != nil {
		t.Fatal(err)
	}
	claims := authn.Claims{Subject: sub, Tenant: "acme", Email: email, Emitido: time.Now().Add(-time.Minute)}
	personal := domain.BuzonesPersonales(sub)
	request := domain.ConBuzonesPersonales(authn.Into(acme, claims), sub, personal, claims.Emitido)
	subjects := domain.Destinatarios(sub, claims.Email, "")
	ready, result := make(chan int32, 1), make(chan error, 1)
	go func() {
		result <- st.enLecturaDeAvisos(request, func(tx pgx.Tx) error {
			d, err := conDestinatarios(request, tx, subjects)
			if err != nil {
				return err
			}
			var pid int32
			if err := tx.QueryRow(request, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			ready <- pid
			return marcarNotificacionTx(request, tx, n.ID, d)
		})
	}()
	var pid int32
	select {
	case pid = <-ready:
	case err := <-result:
		t.Fatalf("transaction ended before update: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := admin.QueryRow(ctx, `SELECT $2::int = ANY(pg_blocking_pids($1))`, pid, forgetPID).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("update did not wait for delete: %v", err)
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if err := forget.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("deleted notification was marked: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
