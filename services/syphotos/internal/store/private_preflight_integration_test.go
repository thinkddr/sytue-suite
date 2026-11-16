// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivatePhotosPagedPreflightRetainsMoreThan32MiBAggregate(t *testing.T) {
	source, st, objects, _, scope := sourceFixture(t)
	ctx := ctxDe(scope.Tenant)
	objects.bodies = map[string][]byte{}
	// Large retained EXIF strings stand in for a library with vectors/unknown
	// metadata. Each entity is below the existing32MiB entity limit; the whole
	// library exceeds it. A descriptor page never transfers these JSON bodies.
	for i := range 8 {
		input := foto("large.png", uuid.NewString(), nil)
		input.Camara = strings.Repeat("retained EXIF / ", 350000)
		if _, err := st.Ingerir(ctx, input); err != nil {
			t.Fatal(i, err)
		}
	}
	epoch := uuid.NewString()
	info, err := source.StartPreflight(ctx, scope, epoch)
	if err != nil || info.Validate(scope, epoch) != nil || info.Rows != 9 || info.Originals != 9 || info.Blobs != 0 {
		t.Fatal("large aggregate preflight failed", info, err)
	}
	cursor := storage.LegacySourceCursor{}
	var total, largest, count int64
	for {
		page, err := source.PreflightPage(ctx, scope, epoch, info.CAS, cursor, 3)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(page)
		if len(encoded) > 4096 || len(page.Rows) > 3 {
			t.Fatal("metadata bodies leaked into descriptor page", len(encoded))
		}
		for _, row := range page.Rows {
			descriptor, reader, err := source.OpenPreflightJSON(ctx, scope, epoch, info.CAS, row.Kind, row.ID)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.New()
			n, err := io.Copy(sum, reader)
			closeErr := reader.Close()
			if err != nil || closeErr != nil || descriptor != row || n != row.Size || hex.EncodeToString(sum.Sum(nil)) != row.SHA256 {
				t.Fatal("one retained body changed", err, closeErr)
			}
			total += n
			count++
			if n > largest {
				largest = n
			}
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	if total <= 32<<20 || largest >= 8<<20 || count != info.Rows {
		t.Fatal("large-library regression did not exercise entity-bounded IO", total, largest, count)
	}
	link := storage.Reference{RoomID: "!photos:test", EventID: "$large_paged_plan"}
	if _, err := source.AcceptPlan(ctx, scope, epoch, info.CAS, link); err != nil {
		t.Fatal("large accepted plan recapture failed", err)
	}
	state, err := source.Begin(ctx, scope, uuid.MustParse(epoch))
	if err != nil || state.Rows != info.Rows || state.Blobs != 0 {
		t.Fatal("large library could not freeze atomically", state, err)
	}
	if _, err := st.Ingerir(ctx, foto("old writer.png", uuid.NewString(), nil)); !privatePhotoBlocked(err) {
		t.Fatal("large source escaped writer fence", err)
	}
	t.Log("PHOTOS_LARGE_PREFLIGHT_PASS", total, "aggregate bytes; largest entity", largest, "descriptor page <4096 bytes; only per-entity body IO")
}

func TestPrivatePhotosPagedPreflightNeverSubstitutesChangedOrForeignJSON(t *testing.T) {
	source, st, _, asset, scope := sourceFixture(t)
	ctx := ctxDe(scope.Tenant)
	epoch := uuid.NewString()
	info, err := source.StartPreflight(ctx, scope, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.PreflightPage(ctx, scope, epoch, uuid.NewString(), storage.LegacySourceCursor{}, 2); err == nil {
		t.Fatal("foreign CAS opened inventory")
	}
	if _, err := source.PreflightPage(ctx, scope, epoch, info.CAS, storage.LegacySourceCursor{Kind: "asset", ID: uuid.NewString()}, 2); err == nil {
		t.Fatal("unknown cursor skipped source rows")
	}
	foreign := scope
	foreign.Subject = "other"
	if _, _, err := source.OpenPreflightJSON(ctx, foreign, epoch, info.CAS, "asset", asset.ID.String()); err == nil {
		t.Fatal("foreign owner opened source JSON")
	}
	if err := st.GuardarDescripcion(ctx, asset.ID, scope.Subject, "changed after descriptor capture", nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := source.OpenPreflightJSON(ctx, scope, epoch, info.CAS, "asset", asset.ID.String()); err == nil {
		t.Fatal("changed source body was substituted into old plan")
	}
	state, err := source.State(ctx, scope)
	if err != nil || state.Mode != "legacy" {
		t.Fatal("rejected descriptor read froze source", err)
	}
}

func TestPrivatePhotosPagedPreflightReaderClearsOnClose(t *testing.T) {
	source, _, _, asset, scope := sourceFixture(t)
	ctx := ctxDe(scope.Tenant)
	epoch := uuid.NewString()
	info, err := source.StartPreflight(ctx, scope, epoch)
	if err != nil {
		t.Fatal(err)
	}
	_, reader, err := source.OpenPreflightJSON(ctx, scope, epoch, info.CAS, "asset", asset.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(&bytes.Buffer{}, reader); err == nil {
		t.Fatal("closed preflight reader retained readable plaintext")
	}
	if model.MaxRowBytes != 32<<20 {
		t.Fatal("test must preserve per-entity guard")
	}
}
