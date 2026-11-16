// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// The embedded interface intentionally exposes only logical Get, not GetRaw.
type logicalOnlyBlobs struct{ objectstore.BlobStore }

type trackedRawBlobs struct {
	*fakeBlobs
	key   string
	calls int
}

func (b *trackedRawBlobs) GetRaw(_ context.Context, _ tenancy.Tenant, key string) (io.ReadCloser, error) {
	b.key = key
	b.calls++
	return io.NopCloser(strings.NewReader("raw")), nil
}

func TestCipherRawFailsClosedWithoutProviderCapability(t *testing.T) {
	prior := committedCipherForRetry()
	legacy := &fakeBlobs{getBody: "abc"}
	s := newSvc(&fakeStore{}, logicalOnlyBlobs{legacy}, nil)
	s.cipher = &memoryCipherBlobs{blob: prior}
	ctx := store.WithOwner(tenantCtx(t), "alice")
	_, body, err := s.OpenCipherBlob(ctx, "mail", prior.ID)
	if body != nil || !errors.Is(err, objectstore.ErrRawUnsupported) {
		t.Fatal("download fell back to logical Get", err)
	}
	_, err = s.PutCipherBlob(ctx, "mail", prior.ID, 3, strings.NewReader("abc"))
	if !errors.Is(err, ErrCipherBlobProvider) {
		t.Fatal("retry fell back to logical Get", err)
	}
}

func TestRawRouterPreservesHistoricalLocation(t *testing.T) {
	location := uuid.New()
	base := &trackedRawBlobs{fakeBlobs: &fakeBlobs{}}
	external := &trackedRawBlobs{fakeBlobs: &fakeBlobs{}}
	r := newBlobRouter(base, &fakeExternal{}, "", nil)
	r.cache["acme/"+location.String()] = entradaExterna{locationID: location, blobs: external, expires: time.Now().Add(time.Minute)}
	tenant := tenancy.Tenant{Slug: "acme"}
	key := store.ExternalPrefix + "v1/" + location.String() + "/opaque"
	body, err := r.GetRaw(t.Context(), tenant, key)
	if err != nil {
		t.Fatal(err)
	}
	_ = body.Close()
	if external.calls != 1 || external.key != key || base.calls != 0 {
		t.Fatal("raw read changed provider location")
	}
	r.cache["acme/"+location.String()] = entradaExterna{locationID: location, blobs: logicalOnlyBlobs{external}, expires: time.Now().Add(time.Minute)}
	body, err = r.GetRaw(t.Context(), tenant, key)
	if body != nil || !errors.Is(err, objectstore.ErrRawUnsupported) || base.calls != 0 {
		t.Fatal("unsupported raw provider fell back", err)
	}
}
