// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	retainedVideo   = "5411a7e3-b529-4654-9c37-15699d917489"
	retainedComment = "70e4686d-e51c-48c6-9eb6-3eb4873bd05f"
	retainedJob     = "fd944c56-790d-41a1-8715-2642816fe0ed"
)

func castScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "cast"}
}

func retainedRow(t *testing.T, kind, raw string) Row {
	t.Helper()
	row, err := RetainedRow(kind, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func castRows(t *testing.T, visibility string) []Row {
	t.Helper()
	return []Row{
		retainedRow(t, "video", `{"id":"`+retainedVideo+`","tenant":"acme","owner_sub":"alice","kind":"upload","status":"ready","visibility":"`+visibility+`","title":"retained title","original_key":"old/original.mov","hls_key":"old/hls/master.m3u8","webm_key":"old/webm/720p.webm","poster_key":"old/poster.jpg","qualities":[1080,720,360],"deleted_at":"2026-10-01T12:00:00Z","future_integer":900719925474099312345}`),
		retainedRow(t, "comment", `{"id":"`+retainedComment+`","tenant":"acme","video_id":"`+retainedVideo+`","author_sub":"alice","body":"retained exact comment","deleted_at":"2026-10-01T12:00:00Z"}`),
		retainedRow(t, "job", `{"id":"`+retainedJob+`","tenant":"acme","video_id":"`+retainedVideo+`","kind":"qualities","status":"done","payload":{"future_integer":900719925474099312345}}`),
	}
}

func TestCastSnapshotPreservesExactTrashMediaRoutingAndUnknownNumbers(t *testing.T) {
	rows := castRows(t, "org")
	want := bytes.Clone(rows[0].JSON)
	snapshot, err := NewSnapshot(castScope(), rows)
	if err != nil {
		t.Fatal(err)
	}
	rows[0].JSON[0] = '!'
	found := false
	for _, row := range snapshot.Rows() {
		if row.Kind == "video" {
			found = true
			if !bytes.Equal(row.JSON, want) {
				t.Fatal("retained video was projected or re-encoded")
			}
			row.JSON[0] = '!'
		}
	}
	if !found {
		t.Fatal("trashed video disappeared")
	}
	for _, row := range snapshot.Rows() {
		if row.Kind == "video" && !bytes.Equal(row.JSON, want) {
			t.Fatal("view aliases original rows")
		}
	}
}

func TestLegacyCastVisibilitiesNeverBecomeOwnerOnly(t *testing.T) {
	for _, visibility := range []string{"public", "unlisted", "org", "people"} {
		t.Run(visibility, func(t *testing.T) {
			snapshot, err := NewSnapshot(castScope(), castRows(t, visibility))
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(snapshot.PersonalCompatible(), ErrShared) {
				t.Fatal("legacy audience silently dropped, including zero-recipient people")
			}
		})
	}
	if Visibility(PrivateVisibility).Valida() {
		t.Fatal("device policy changed the legacy public service")
	}
	snapshot, err := NewSnapshot(castScope(), castRows(t, PrivateVisibility))
	if err != nil || snapshot.PersonalCompatible() != nil {
		t.Fatal("explicit device-only policy rejected", err)
	}
}

func TestCastSnapshotRejectsBrokenGraphsAndActiveServerWork(t *testing.T) {
	for _, scenario := range []string{"missing video", "duplicate", "wrong tenant", "active job", "live key"} {
		t.Run(scenario, func(t *testing.T) {
			rows := castRows(t, PrivateVisibility)
			switch scenario {
			case "missing video":
				rows = rows[1:]
			case "duplicate":
				rows = append(rows, rows[0])
			case "wrong tenant":
				rows[0].JSON = bytes.Replace(rows[0].JSON, []byte(`"acme"`), []byte(`"other"`), 1)
			case "active job":
				rows[2].JSON = bytes.Replace(rows[2].JSON, []byte(`"done"`), []byte(`"pending"`), 1)
			case "live key":
				rows = append(rows, retainedRow(t, "stream_key", `{"tenant":"acme","owner_sub":"alice","key":"synthetic-unused-live-key","ingress_id":"synthetic"}`))
			}
			snapshot, err := NewSnapshot(castScope(), rows)
			if scenario == "active job" {
				if err != nil || !errors.Is(snapshot.PersonalCompatible(), ErrDeviceWork) {
					t.Fatal("server job accepted as private device work", err)
				}
				return
			}
			if scenario == "live key" {
				if err != nil || !errors.Is(snapshot.PersonalCompatible(), ErrLive) {
					t.Fatal("active server ingress accepted", err)
				}
				return
			}
			if !errors.Is(err, ErrSnapshot) {
				t.Fatal("invalid graph accepted", err)
			}
		})
	}
}

func TestCastExternalRelationshipsRetainOriginalTenantAndAuthor(t *testing.T) {
	row := retainedRow(t, "external_comment", `{"id":"`+retainedComment+`","tenant":"other","video_id":"`+retainedVideo+`","author_sub":"alice","body":"exact external contribution"}`)
	snapshot, err := NewSnapshot(castScope(), []Row{row})
	if err != nil || !errors.Is(snapshot.PersonalCompatible(), ErrShared) {
		t.Fatal("external relationship discarded or activated", err)
	}
	row.JSON = bytes.Replace(row.JSON, []byte(`"alice"`), []byte(`"bob"`), 1)
	if _, err := NewSnapshot(castScope(), []Row{row}); !errors.Is(err, ErrSnapshot) {
		t.Fatal("foreign author's private record accepted", err)
	}
}

func TestCastCompositeIdentityKeepsTupleBoundaries(t *testing.T) {
	one, err := compositeID("a:b", "c")
	if err != nil {
		t.Fatal(err)
	}
	two, err := compositeID("a", "b:c")
	if err != nil || one == two {
		t.Fatal("composite identities collided", err)
	}
}
