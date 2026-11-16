// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privatecontacts "git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type photoByIDBlockedCrypto struct {
	records.Crypto
	block   atomic.Bool
	once    sync.Once
	started chan struct{}
}

func (p *photoByIDBlockedCrypto) ReadStorageRecord(ctx context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	if p.block.Load() {
		p.once.Do(func() { close(p.started) })
		<-ctx.Done()
		return storage.Record{}, ctx.Err()
	}
	return p.Crypto.ReadStorageRecord(ctx, ref, object, role)
}

func TestPrivatePhotoByIDWithRealSDKAndNonReentrantLease(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PRIVATE_CONTACTS") != "1" {
		t.Skip("requires disposable remote Synapse and synthetic PostgreSQL export")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	raw, err := os.ReadFile(os.Getenv("SYTUE_CONTACTS_PRIVATE_VECTOR"))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	var source contactsPGSource
	if json.Unmarshal(raw, &source) != nil || len(source.Rows) != 9 || source.Snapshot.Scope.Product != "contacts" {
		t.Fatal("invalid synthetic retained source")
	}
	slices.SortFunc(source.Rows, func(a, b privatecontacts.Row) int {
		if n := strings.Compare(a.Kind, b.Kind); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	address := os.Getenv("SYTUE_CONTACTS_SYNAPSE_URL")
	if address == "" {
		address = "http://127.0.0.1:18601"
	}
	target, err := url.Parse(address)
	if err != nil || target.Hostname() != "127.0.0.1" {
		t.Fatal("fixture must stay on remote loopback")
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), source.Snapshot.Scope, storage.IsolatedIdentity)
	defer func() { _ = device.Client.Close() }()
	files, gateway, _, _ := fileFixture(t)
	gateway.product = "contacts"
	config := privatecontacts.Config{Context: ctx, Scope: source.Snapshot.Scope, Crypto: device.Client, Gateway: files.gateway, Journal: files.journal, Source: &source}
	initial, err := privatecontacts.New(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal("prepare private snapshot", err)
	}
	snapshot, err := initial.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	people, err := snapshot.Contacts()
	if err != nil || len(people) != 1 {
		t.Fatal("missing fixture contact", err)
	}
	if err := initial.Restore(ctx, people[0].ID); err != nil {
		t.Fatal("restore fixture contact", err)
	}
	const photoData = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg=="
	if err := initial.SetPhoto(ctx, people[0].UID, "data:image/png;base64,"+photoData); err != nil {
		t.Fatal("set fixture photo", err)
	}
	person, err := initial.Contact(ctx, people[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	initial.Close()
	want, err := base64.StdEncoding.DecodeString(photoData)
	if err != nil {
		t.Fatal(err)
	}
	var active, acquired atomic.Int32
	config.WorkLease = func(ctx context.Context) (func(), error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !active.CompareAndSwap(0, 1) {
			return nil, errors.New("non-reentrant lease acquired twice")
		}
		acquired.Add(1)
		return func() { active.Store(0) }, nil
	}
	blocked := &photoByIDBlockedCrypto{Crypto: device.Client, started: make(chan struct{})}
	config.Crypto = blocked
	repo, err := privatecontacts.New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal("open private snapshot", err)
	}
	transport := &contactsNoLegacyTransport{}
	facade, err := api.NewContacts("https://legacy.invalid", nil, &http.Client{Transport: transport}).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	before := acquired.Load()
	got, err := facade.PhotoByID(ctx, person.ID, person.ETag)
	if err != nil || !bytes.Equal(got, want) || acquired.Load() != before+1 || active.Load() != 0 || transport.calls.Load() != 0 {
		t.Fatal("ID photo did not complete under exactly one lease", err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	before = acquired.Load()
	if photo, err := facade.PhotoByID(canceled, person.ID, person.ETag); photo != nil || !errors.Is(err, context.Canceled) || acquired.Load() != before {
		t.Fatal("canceled photo acquired a lease or returned bytes", err)
	}
	blocked.block.Store(true)
	result := make(chan error, 1)
	go func() { _, err := facade.PhotoByID(ctx, person.ID, person.ETag); result <- err }()
	select {
	case <-blocked.started:
	case <-time.After(10 * time.Second):
		t.Fatal("SDK read did not start")
	}
	closed := make(chan struct{})
	go func() { repo.Close(); close(closed) }()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("closed photo read returned success")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("photo read did not observe close")
	}
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("close did not drain photo read")
	}
	if active.Load() != 0 || transport.calls.Load() != 0 {
		t.Fatal("photo close retained lease or called legacy HTTP")
	}
}
