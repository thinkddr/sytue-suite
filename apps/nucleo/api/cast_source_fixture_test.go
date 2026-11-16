// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/cast/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type castSourceSession string

func (s castSourceSession) AccessToken(context.Context) (string, error) { return string(s), nil }
func (castSourceSession) Invalidate(string)                             {}

func TestCastSourceRemoteFixture(t *testing.T) {
	endpoint := os.Getenv("SYTUE_CAST_SOURCE_URL")
	if endpoint == "" {
		t.Skip("real PostgreSQL/HTTP Cast fixture required")
	}
	ctx := t.Context()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "cast"}
	original := bytes.Repeat([]byte{0, 1, 2, 3, 0xfe, 0xff}, 128<<10)
	client := NewCast(endpoint, castSourceSession("source-alice"), http.DefaultClient)
	state, err := client.PrivateStorageState(ctx, scope)
	if err != nil || state.Mode != "legacy" || state.Shared {
		t.Fatal("native source state unavailable", err)
	}
	epoch := uuid.NewString()
	state, err = client.BeginPrivateStorageMigration(ctx, scope, epoch)
	if err != nil || state.Mode != "frozen" || state.Rows != 3 || state.Blobs != 1 {
		t.Fatal("native source capture incomplete", err)
	}
	page, err := client.PrivateStorageSourcePage(ctx, scope, epoch, storage.LegacySourceCursor{})
	if err != nil || len(page.Rows) != 3 {
		t.Fatal("native source page missing", err)
	}
	wire, err := json.Marshal(page)
	if err != nil || bytes.Contains(wire, []byte("/original")) {
		t.Fatal("internal S3 route leaked in descriptors", err)
	}
	var media storage.StreamSourceRow
	for _, row := range page.Rows {
		reader, err := client.OpenPrivateStorageSource(ctx, scope, epoch, row)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			t.Fatal("exact source stream rejected", err, closeErr)
		}
		if row.Kind == model.BinaryKind {
			media = row
			if !bytes.Equal(raw, original) {
				t.Fatal("native reader changed original bytes")
			}
		}
	}
	if media.ID == "" {
		t.Fatal("native binary missing")
	}
	other := NewCast(endpoint, castSourceSession("source-bob"), http.DefaultClient)
	_, err = other.PrivateStorageSourcePage(ctx, scope, epoch, storage.LegacySourceCursor{})
	var status *Error
	if !errors.As(err, &status) || status.Status != http.StatusConflict {
		t.Fatal("foreign authenticated account did not hit identity guard", err)
	}
	unauthenticated := NewCast(endpoint, castSourceSession("unverified"), http.DefaultClient)
	_, err = unauthenticated.PrivateStorageState(ctx, scope)
	if !errors.As(err, &status) || status.Status != http.StatusUnauthorized {
		t.Fatal("unverified caller did not hit authentication guard", err)
	}
	// Provider corruption after freeze must never become a complete native body.
	response, err := http.Post(endpoint+"/fixture/corrupt", "application/octet-stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatal("synthetic provider corruption was not installed")
	}
	reader, err := client.OpenPrivateStorageSource(ctx, scope, epoch, media)
	if err == nil {
		_, err = io.ReadAll(reader)
		_ = reader.Close()
		if err == nil {
			t.Fatal("changed S3 bytes accepted as retained source")
		}
	}
	t.Log("CAST_NATIVE_REMOTE_FIXTURE_PASS")
}
