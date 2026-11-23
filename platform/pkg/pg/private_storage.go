// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

//go:embed private_storage_v1.sql
var privateStorageV1 []byte

//go:embed private_storage_release_v1.sql
var privateStorageReleaseV1 []byte

const (
	privateStorageMarkerV1        = "-- sytue:private-storage-fence-v1"
	privateStorageReleaseMarkerV1 = "-- sytue:private-storage-release-v1"
)

// privateStorageFragments are the shared private storage versions every
// service wrapper expands. The release version must follow the fence it
// replaces in the same migration history (ADR 0086, «soltar la fuente»).
func privateStorageFragments() []migrationDDLFragment {
	return []migrationDDLFragment{
		{marker: privateStorageMarkerV1, sql: privateStorageV1},
		{marker: privateStorageReleaseMarkerV1, sql: privateStorageReleaseV1},
	}
}

// WithPrivateStorageDDL expands an explicit versioned migration marker from one
// shared SQL implementation. Services expose this wrapped FS as db.Migrations;
// runtime, pgtest and SQL policy checks consequently read identical migrations.
// Unmarked existing migrations are returned unchanged, preserving old history.
func WithPrivateStorageDDL(source fs.FS) fs.FS {
	return withMigrationDDL(source, privateStorageFragments()...)
}

type migrationDDLFragment struct {
	marker string
	sql    []byte
	suffix string // Empty preserves the original up-only behavior.
}

func withMigrationDDL(source fs.FS, fragments ...migrationDDLFragment) fs.FS {
	return privateStorageFS{source: source, fragments: fragments}
}

type privateStorageFS struct {
	source    fs.FS
	fragments []migrationDDLFragment
}

func (f privateStorageFS) Open(name string) (fs.File, error) {
	file, err := f.source.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.IsDir() || !strings.HasSuffix(name, ".sql") {
		return file, nil
	}
	raw, err := io.ReadAll(file)
	closeErr := file.Close()
	if joined := errors.Join(err, closeErr); joined != nil {
		return nil, joined
	}
	for _, fragment := range f.fragments {
		count := bytes.Count(raw, []byte(fragment.marker))
		if count > 0 {
			suffix := fragment.suffix
			if suffix == "" {
				suffix = ".up.sql"
			}
			if fragment.marker == "" || len(fragment.sql) == 0 || count != 1 || (suffix != ".up.sql" && suffix != ".down.sql") || !strings.HasSuffix(name, suffix) {
				return nil, fmt.Errorf("pg: invalid private storage migration marker in %s", name)
			}
			raw = bytes.Replace(raw, []byte(fragment.marker), fragment.sql, 1)
		}
	}
	if bytes.Contains(raw, []byte("-- sytue:private-")) {
		return nil, fmt.Errorf("pg: unexpanded private migration marker in %s", name)
	}
	return &privateStorageFile{Reader: bytes.NewReader(raw), info: privateStorageInfo{FileInfo: info, size: int64(len(raw))}}, nil
}

type privateStorageFile struct {
	*bytes.Reader
	info fs.FileInfo
}

func (f *privateStorageFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *privateStorageFile) Close() error               { return nil }

type privateStorageInfo struct {
	fs.FileInfo
	size int64
}

func (i privateStorageInfo) Size() int64 { return i.size }

type PrivateStorageState struct {
	Mode        string
	Epoch       string
	StartedAt   *time.Time
	Rows, Blobs int64
}

var ErrPrivateStorageScope = errors.New("pg: invalid private storage state scope")

func privateStorageTable(ctx context.Context, table, tenant, owner string) (string, error) {
	current, err := tenancy.From(ctx)
	if err != nil || current.Slug != tenant || !tenancy.ValidSlug(tenant) || owner == "" || len(owner) > 1024 || !utf8.ValidString(owner) || strings.ContainsRune(owner, 0) || !strings.HasSuffix(table, "_private_storage_states") || len(table) > 63 {
		return "", ErrPrivateStorageScope
	}
	if !migrationTableName.MatchString(table) || strings.HasPrefix(table, "_") {
		return "", ErrPrivateStorageScope
	}
	return pgx.Identifier{"public", table}.Sanitize(), nil
}

// EnsurePrivateStorageState is called in the same tenant transaction as Lock.
// A nil account is used by owner-keyed domains; Mail additionally pins account ID.
func EnsurePrivateStorageState(ctx context.Context, tx pgx.Tx, table, tenant, owner string, account *uuid.UUID) error {
	name, err := privateStorageTable(ctx, table, tenant, owner)
	if err != nil {
		return err
	}
	if account == nil {
		_, err = tx.Exec(ctx, `INSERT INTO `+name+`(tenant,owner_sub) VALUES($1,$2) ON CONFLICT DO NOTHING`, tenant, owner)
	} else {
		if *account == uuid.Nil {
			return ErrPrivateStorageScope
		}
		_, err = tx.Exec(ctx, `INSERT INTO `+name+`(tenant,owner_sub,account_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, tenant, owner, *account)
	}
	return err
}

type privateStateLock uint8

const (
	privateStateRead privateStateLock = iota
	privateStateUpdate
	privateStateShare
)

func readPrivateStorageState(ctx context.Context, tx pgx.Tx, table, tenant, owner string, lock privateStateLock) (PrivateStorageState, error) {
	name, err := privateStorageTable(ctx, table, tenant, owner)
	if err != nil {
		return PrivateStorageState{}, err
	}
	query := `SELECT mode,coalesce(epoch::text,''),started_at,source_rows,source_blobs FROM ` + name + ` WHERE tenant=$1 AND owner_sub=$2`
	switch lock {
	case privateStateRead:
	case privateStateUpdate:
		query += ` FOR UPDATE`
	case privateStateShare:
		query += ` FOR SHARE`
	default:
		return PrivateStorageState{}, ErrPrivateStorageScope
	}
	result := PrivateStorageState{}
	err = tx.QueryRow(ctx, query, tenant, owner).Scan(&result.Mode, &result.Epoch, &result.StartedAt, &result.Rows, &result.Blobs)
	if lock == privateStateRead && errors.Is(err, pgx.ErrNoRows) {
		return PrivateStorageState{Mode: "legacy"}, nil
	}
	return result, err
}

func ReadPrivateStorageState(ctx context.Context, tx pgx.Tx, table, tenant, owner string) (PrivateStorageState, error) {
	return readPrivateStorageState(ctx, tx, table, tenant, owner, privateStateRead)
}

// LockPrivateStorageState takes the exclusive row lock paired with the shared
// SQL writer guard. Call Ensure first; missing/hidden rows are never legacy here.
func LockPrivateStorageState(ctx context.Context, tx pgx.Tx, table, tenant, owner string) (PrivateStorageState, error) {
	return readPrivateStorageState(ctx, tx, table, tenant, owner, privateStateUpdate)
}

// SharePrivateStorageState holds the read side of the source freeze fence.
// Unlike an unlocked read, missing/hidden rows are errors. Ensure and COMMIT a
// missing state before using a callback that performs SQL in another transaction.
func SharePrivateStorageState(ctx context.Context, tx pgx.Tx, table, tenant, owner string) (PrivateStorageState, error) {
	return readPrivateStorageState(ctx, tx, table, tenant, owner, privateStateShare)
}
