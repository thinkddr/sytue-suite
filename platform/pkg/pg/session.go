// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrganizationOperationLockGroup serializes administrative organization operations
// in each database, while their retained intent remains bound to its own job ID.
const OrganizationOperationLockGroup = "sytue:organization-operations"

var ErrSessionLockBusy = errors.New("pg: session lock is owned")

// Session binds the lock and every operation to one physical direct connection.
// Closing or losing that connection cannot silently borrow a replacement. The
// server retains its session lock until in-flight work commits or rolls back.
type Session struct {
	conn      *pgx.Conn
	acquired  *pgxpool.Conn
	closeOnce sync.Once
	closeErr  error
	key       string
	group     string
}

func TrySessionLock(ctx context.Context, pool *Pool, key string, groups ...string) (*Session, error) {
	valid := func(value string) bool { return value != "" && len(value) <= 1024 && !strings.ContainsRune(value, 0) }
	if !valid(key) || len(groups) > 1 {
		return nil, fmt.Errorf("pg: invalid session lock key")
	}
	group := ""
	keys := []string{key}
	if len(groups) == 1 {
		group = groups[0]
		if !valid(group) || group == key {
			return nil, fmt.Errorf("pg: invalid session lock group")
		}
		keys = []string{group, key}
	}
	conn, err := pool.AcquireDirect(ctx)
	if err != nil {
		return nil, err
	}
	acquiredAny := false
	for _, lockKey := range keys {
		var acquired bool
		err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,17107030))`, lockKey).Scan(&acquired)
		if err != nil || !acquired {
			// A missing response is not proof that the server did not acquire a lock.
			// Never return an uncertain or partially locked backend to the pool.
			if err != nil || acquiredAny {
				_ = conn.Conn().Close(ctx)
			}
			conn.Release()
			if err != nil {
				return nil, err
			}
			return nil, ErrSessionLockBusy
		}
		acquiredAny = true
	}
	return &Session{conn: conn.Conn(), acquired: conn, key: key, group: group}, nil
}
func (s *Session) GroupKey() string { return s.group }
func (s *Session) LockKey() string  { return s.key }
func (s *Session) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return s.conn.Exec(ctx, sql, args...)
}

func (s *Session) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return s.conn.Query(ctx, sql, args...)
}

func (s *Session) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return s.conn.QueryRow(ctx, sql, args...)
}

func (s *Session) InTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return runInTx(ctx, s.conn, pgx.TxOptions{}, fn)
}

func (s *Session) InMergeTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return inModeTx(ctx, s, SetMerge, fn)
}

func (s *Session) InMaintenanceTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return inModeTx(ctx, s, SetMaintenance, fn)
}

func (s *Session) Healthz(ctx context.Context) error { _, err := s.Exec(ctx, `SELECT 1`); return err }

func (s *Session) Close(ctx context.Context) error {
	s.closeOnce.Do(func() { s.closeErr = s.conn.Close(ctx); s.acquired.Release() })
	return s.closeErr
}
