// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func binaryCatalogFixture(t *testing.T) (*Repository, *binaryReadProbe, Entry) {
	t.Helper()
	r, probe, version, _ := binaryReaderFixture(t)
	version.Reference = storage.Reference{RoomID: "!photos:test", EventID: "$index"}
	version.RevisionID = "index_revision"
	r.cfg.Context = t.Context()
	r.gate = offline.NewWorkGate(t.Context())
	r.heads = map[string][]storage.ObjectVersion{version.ObjectID: {cloneVersion(version)}}
	return r, probe, Entry{Kind: "original", ID: "asset", State: "live", Binary: true, Version: version}
}

func TestBinaryContentCatalogValidatesSmallStructuresOnlyWithinEngineOrReadyView(t *testing.T) {
	r, probe, entry := binaryCatalogFixture(t)
	defer r.Close()
	public := catalogView{r: r, requireReady: true}
	if _, err := public.ReadBinary(t.Context(), entry, io.Discard); !errors.Is(err, ErrNotReady) {
		t.Fatal("public binary view bypassed readiness", err)
	}
	var content bytes.Buffer
	internal := catalogView{r: r}
	if _, err := internal.ReadBinary(t.Context(), entry, &content); err != nil || !bytes.Equal(content.Bytes(), probe.body) {
		t.Fatal("engine could not validate imported structure", err)
	}
	r.ready = true
	if _, err := public.ReadBinary(t.Context(), entry, io.Discard); err != nil {
		t.Fatal(err)
	}
	probe.body[0] ^= 1
	if _, err := internal.ReadBinary(t.Context(), entry, io.Discard); !errors.Is(err, ErrSource) {
		t.Fatal("binary structure accepted before final SHA verification", err)
	}
}

func TestBinaryContentCatalogRejectsReboundOrAlteredEntries(t *testing.T) {
	for _, alteration := range []string{"id", "reference", "content", "state", "binary"} {
		t.Run(alteration, func(t *testing.T) {
			r, _, entry := binaryCatalogFixture(t)
			defer r.Close()
			switch alteration {
			case "id":
				entry.ID = "other"
			case "reference":
				entry.Version.Reference.EventID = "$other"
			case "content":
				entry.Version.Commit.Content.Reference.EventID = "$other"
			case "state":
				entry.State = "trash"
			case "binary":
				entry.Binary = false
			}
			if _, err := (catalogView{r: r}).ReadBinary(t.Context(), entry, io.Discard); !errors.Is(err, ErrSource) {
				t.Fatal("changed entry reached binary verifier", err)
			}
		})
	}
}

func TestBinaryContentCatalogKeepsSessionLeaseAndCloseGates(t *testing.T) {
	r, _, entry := binaryCatalogFixture(t)
	r.ready = true
	r.gate = offline.NewWorkGateWithLease(t.Context(), func(context.Context) (func(), error) { return nil, offline.ErrWorkLease })
	view := catalogView{r: r, requireReady: true}
	if _, err := view.ReadBinary(t.Context(), entry, io.Discard); !errors.Is(err, offline.ErrWorkLease) {
		t.Fatal("binary view skipped device lease", err)
	}
	r.Close()
	if _, err := view.ReadBinary(t.Context(), entry, io.Discard); !errors.Is(err, ErrClosed) {
		t.Fatal("closed owner read a binary structure", err)
	}
}
