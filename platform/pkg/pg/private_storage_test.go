// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"bytes"
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestPrivateStorageMigrationExpansion(t *testing.T) {
	original := []byte("SELECT 1;\n")
	input := fstest.MapFS{"migrations/0001_old.up.sql": &fstest.MapFile{Data: original}, "migrations/0002_fence.up.sql": &fstest.MapFile{Data: []byte(privateStorageMarkerV1 + "\nSELECT 2;")}}
	wrapped := WithPrivateStorageDDL(input)
	unchanged, err := fs.ReadFile(wrapped, "migrations/0001_old.up.sql")
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatalf("old migration changed %s %v", unchanged, err)
	}
	expanded, err := fs.ReadFile(wrapped, "migrations/0002_fence.up.sql")
	if err != nil || !bytes.Contains(expanded, privateStorageV1) || bytes.Contains(expanded, []byte(privateStorageMarkerV1)) {
		t.Fatalf("shared SQL not expanded %v", err)
	}
	file, err := wrapped.Open("migrations/0002_fence.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || info.Size() != int64(len(expanded)) {
		t.Fatal("wrong expanded file size", err)
	}
	for _, name := range []string{"migrations/0003_bad.down.sql", "migrations/0004_twice.up.sql"} {
		input[name] = &fstest.MapFile{Data: []byte(privateStorageMarkerV1)}
		if strings.Contains(name, "twice") {
			input[name].Data = append(input[name].Data, []byte(privateStorageMarkerV1)...)
		}
		if _, err := fs.ReadFile(wrapped, name); err == nil {
			t.Fatal("invalid migration marker accepted", name)
		}
	}
	entries, err := fs.ReadDir(wrapped, "migrations")
	if err != nil || len(entries) != 4 {
		t.Fatal("wrapped FS lost directory traversal", err)
	}
}

func TestPrivateStorageIdentifiersAndTenantAreBound(t *testing.T) {
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	if got, err := privateStorageTable(ctx, "mail_private_storage_states", "acme", "ada"); err != nil || got != `"public"."mail_private_storage_states"` {
		t.Fatal(got, err)
	}
	for _, test := range []struct{ table, tenant, owner string }{
		{"mail_private_storage_states;DROP TABLE accounts", "acme", "ada"}, {"public.mail_private_storage_states", "acme", "ada"}, {"accounts", "acme", "ada"}, {"_private_storage_states", "acme", "ada"}, {"mail_private_storage_states", "other", "ada"}, {"mail_private_storage_states", "acme", ""}, {"mail_private_storage_states", "acme", "a\x00b"},
	} {
		if _, err := privateStorageTable(ctx, test.table, test.tenant, test.owner); err == nil {
			t.Fatalf("accepted %+v", test)
		}
	}
}

func TestPrivateMigrationFragmentsExpandTogetherWithoutChangingExistingBytes(t *testing.T) {
	const marker = "-- sytue:private-fixture-v1"
	second := []byte("SELECT 'second';")
	original := []byte("-- preserved / 漢字\nSELECT 1;\n")
	input := fstest.MapFS{
		"migrations/0001_absent.up.sql": &fstest.MapFile{Data: original},
		"migrations/0002_mixed.up.sql":  &fstest.MapFile{Data: []byte(privateStorageMarkerV1 + "\n" + marker + "\nSELECT 2;")},
	}
	wrapped := withMigrationDDL(input, migrationDDLFragment{marker: privateStorageMarkerV1, sql: privateStorageV1}, migrationDDLFragment{marker: marker, sql: second})
	unchanged, err := fs.ReadFile(wrapped, "migrations/0001_absent.up.sql")
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatal("unmarked migration changed", err)
	}
	expanded, err := fs.ReadFile(wrapped, "migrations/0002_mixed.up.sql")
	if err != nil || !bytes.Contains(expanded, privateStorageV1) || !bytes.Contains(expanded, second) || bytes.Contains(expanded, []byte("-- sytue:private-")) {
		t.Fatal("mixed migration was not fully expanded", err)
	}
	if _, err := fs.ReadFile(WithPrivateStorageDDL(input), "migrations/0002_mixed.up.sql"); err == nil {
		t.Fatal("unregistered marker escaped to SQL")
	}
	for _, name := range []string{"migrations/0003_duplicate.up.sql", "migrations/0004_wrong.down.sql", "migrations/0005_unknown.up.sql"} {
		raw := marker
		if strings.Contains(name, "duplicate") {
			raw += "\n" + marker
		}
		if strings.Contains(name, "unknown") {
			raw = "-- sytue:private-unknown-v1"
		}
		input[name] = &fstest.MapFile{Data: []byte(raw)}
		if _, err := fs.ReadFile(wrapped, name); err == nil {
			t.Fatal("invalid fragment reached SQL", name)
		}
	}
}
