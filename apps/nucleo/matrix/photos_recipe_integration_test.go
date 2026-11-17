// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var errRecipeInterrupted = errors.New("synthetic interrupted private recipe boundary")

type recipeBoundaryFault struct {
	*Client
	mu          sync.Mutex
	phase       string
	dropped     bool
	puts        int
	accepted    storage.ObjectVersion
	cancel      context.CancelFunc
	beforeIndex func()
}

func (f *recipeBoundaryFault) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, txn string, record storage.Record) (storage.Reference, error) {
	ref, err := f.Client.PutStorageRecordAt(ctx, target, txn, record)
	var link storage.FileLink
	if err != nil || json.Unmarshal(record.Payload, &link) != nil || link.Kind != storage.FileLinkKind {
		return ref, err
	}
	f.mu.Lock()
	drop := !f.dropped && (f.phase == "link" || f.phase == "cancel")
	if drop {
		f.dropped = true
	}
	phase, cancel := f.phase, f.cancel
	f.mu.Unlock()
	if drop && phase == "link" {
		return ref, errRecipeInterrupted
	}
	if drop && phase == "cancel" {
		cancel()
	}
	return ref, nil
}

func (f *recipeBoundaryFault) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	var header struct {
		Entity string `json:"entity"`
	}
	_ = json.Unmarshal(commit.Metadata, &header)
	if header.Entity == "edit" && f.beforeIndex != nil {
		f.beforeIndex()
	}
	version, err := f.Client.PutStorageObjectAt(ctx, target, txn, object, revision, commit)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.puts++
	if err == nil && header.Entity == "edit" {
		f.accepted = version
	}
	if err == nil && ((f.phase == "index" && header.Entity == "edit") || (f.phase == "old WAL" && header.Entity == "asset")) && !f.dropped {
		f.dropped = true
		f.accepted = version
		return version, errRecipeInterrupted
	}
	return version, err
}

func (f *recipeBoundaryFault) AcknowledgeStorageFileLink(ctx context.Context, journal *offline.Store, job string, link storage.Reference) error {
	f.mu.Lock()
	drop := f.phase == "ack" && !f.dropped
	if drop {
		f.dropped = true
	}
	f.mu.Unlock()
	if drop {
		return errRecipeInterrupted
	}
	return f.Client.AcknowledgeStorageFileLink(ctx, journal, job, link)
}

type photoRecipeBoundaryFixture struct {
	lifetime   *testing.T
	device     storageSynapseDevice
	peer       *Client
	base       string
	httpClient *http.Client
	asset      string
	scope      storage.Scope
	source     *photosRetainedSourceFixture
	open       func(privatePhotos.Crypto, *offline.Store) *privatePhotos.Repository
	journal    func() *offline.Store
}

func (f *photoRecipeBoundaryFixture) powerOff(ctx context.Context, t *testing.T) {
	t.Helper()
	for _, client := range []*Client{f.device.Client, f.peer} {
		if err := client.Backup(ctx); err != nil {
			t.Fatal("backup before producer power-off", err)
		}
		if err := client.Close(); err != nil {
			t.Fatal("actual SDK power-off", err)
		}
	}
}

func (f *photoRecipeBoundaryFixture) restartProducer(ctx context.Context, t *testing.T) *Client {
	t.Helper()
	client, err := Open(ctx, f.device.Config)
	if err != nil {
		t.Fatal("producer SDK restart", err)
	}
	f.lifetime.Cleanup(func() { _ = client.Close() })
	go func() { _ = client.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal("producer SDK ready", err)
	}
	f.device.Client = client
	return client
}

func (f *photoRecipeBoundaryFixture) newPeer(ctx context.Context, t *testing.T) *privatePhotos.Repository {
	t.Helper()
	f.peer = freshNotesStorageDevice(ctx, f.lifetime, f.base, f.httpClient, f.device)
	repository := f.open(f.peer, f.journal())
	t.Cleanup(repository.Close)
	before := f.source.opensCount()
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal("empty-journal recovered peer could not open catalog", err)
	}
	if f.source.opensCount() != before {
		t.Fatal("empty-journal peer reimported plaintext")
	}
	return repository
}

func exercisePrivateRecipeBoundaries(ctx context.Context, t *testing.T, f photoRecipeBoundaryFixture) {
	t.Helper()
	f.lifetime = t
	exercisePrivateRecipeOldWAL(ctx, t, &f)
	for _, phase := range []string{"link", "index", "ack", "cancel"} {
		t.Run(phase, func(t *testing.T) {
			journal := f.journal()
			fault := &recipeBoundaryFault{Client: f.device.Client, phase: phase}
			writer := f.open(fault, journal)
			t.Cleanup(writer.Close)
			if err := writer.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			facade, err := api.NewPhotos("https://legacy.invalid", nil, nil).WithPrivateData(writer)
			if err != nil {
				t.Fatal(err)
			}
			recipe, err := facade.LeerReceta(ctx, f.asset)
			if err != nil {
				t.Fatal(err)
			}
			base := recipe.Huella()
			recipe.Pasos = append(recipe.Pasos, api.PasoReceta{Tipo: api.PasoGiro, Params: map[string]float64{"grados": 90}})
			wanted := recipe.Huella()
			expectedInput := base
			if phase == "index" {
				expectedInput = ""
			}
			life, cancel := context.WithCancel(ctx)
			defer cancel()
			fault.cancel = cancel
			_, err = facade.GuardarRecetaDesde(life, f.asset, recipe, expectedInput)
			if err == nil || !fault.dropped {
				t.Fatal("actual SDK boundary was not interrupted", phase, err)
			}
			if phase == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancel did not reach the producer", err)
			}
			pending, err := writer.Pending(ctx)
			if err != nil || pending.Count == 0 {
				t.Fatal("unacknowledged producer intent disappeared", err)
			}
			writer.Close()
			f.powerOff(ctx, t)
			reader := f.newPeer(ctx, t)
			visible, err := reader.Recipe(ctx, f.asset)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := visible.Huella()
			if err != nil {
				t.Fatal(err)
			}
			expected := wanted
			if phase == "link" || phase == "cancel" {
				expected = base
			}
			if actual != expected {
				t.Fatal("fresh peer saw a partial recipe publication", phase, actual, expected)
			}
			if phase == "index" || phase == "ack" {
				assertPrivateRecipeRenditions(ctx, t, reader, f.asset, wanted)
			}
			restarted := f.restartProducer(ctx, t)
			retryFault := &recipeBoundaryFault{Client: restarted}
			resumed := f.open(retryFault, journal)
			if err := resumed.Initialize(ctx); err != nil {
				t.Fatal("producer repository reopen", err)
			}
			resumedFacade, err := api.NewPhotos("https://legacy.invalid", nil, nil).WithPrivateData(resumed)
			if err != nil {
				t.Fatal(err)
			}
			intent, err := resumedFacade.EdicionPendiente(ctx, f.asset)
			if err != nil || intent == nil || intent.Base != expectedInput || intent.Receta.Huella() != wanted || retryFault.puts != 0 {
				t.Fatal("reopened API lost or published its exact captured input", phase, err)
			}
			result, err := resumedFacade.GuardarRecetaDesde(ctx, f.asset, intent.Receta, intent.Base)
			if err != nil || result != wanted {
				t.Fatal("captured producer intent did not resume", phase, err)
			}
			if phase == "ack" && retryFault.puts != 0 {
				t.Fatal("accepted publication repeated after failed content ACK", retryFault.puts)
			}
			if phase == "index" {
				snapshot, err := storage.LoadCatalog(ctx, restarted, f.scope)
				if err != nil {
					t.Fatal(err)
				}
				matches := 0
				for _, head := range snapshot.Heads {
					if head.Reference == fault.accepted.Reference {
						matches++
					}
				}
				if matches != 1 {
					t.Fatal("lost index ACK duplicated the accepted revision", matches)
				}
			}
			pending, err = resumed.Pending(ctx)
			if err != nil || pending.Count != 0 {
				t.Fatal("completed recipe kept its producer WAL", phase, pending.Count, err)
			}
			if intent, err := resumedFacade.EdicionPendiente(ctx, f.asset); intent != nil || err != nil {
				t.Fatal("completed API attempt remained pending", err)
			}
			if err := reader.RefreshPrivateData(ctx); err != nil {
				t.Fatal("peer refresh after resumed recipe", err)
			}
			assertPrivateRecipeRenditions(ctx, t, reader, f.asset, wanted)
			resumed.Close()
			reader.Close()
		})
	}
	exercisePrivateRecipeConcurrentCAS(ctx, t, &f)
}

func assertPrivateRecipeRenditions(ctx context.Context, t *testing.T, repository *privatePhotos.Repository, asset, fingerprint string) {
	t.Helper()
	for _, variant := range model.Variantes() {
		body, err := repository.VariantDe(ctx, asset, string(variant), fingerprint)
		if err != nil {
			t.Fatal("fresh recipe variant open", variant, err)
		}
		var output bytes.Buffer
		_, err = output.ReadFrom(body)
		_ = body.Close()
		if err != nil || output.Len() == 0 {
			t.Fatal("fresh recipe variant absent", variant, err)
		}
	}
}

func exercisePrivateRecipeOldWAL(ctx context.Context, t *testing.T, f *photoRecipeBoundaryFixture) {
	t.Helper()
	journal := f.journal()
	fault := &recipeBoundaryFault{Client: f.device.Client, phase: "old WAL"}
	writer := f.open(fault, journal)
	if err := writer.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	description := "legacy mutation WAL replay sentinel"
	if err := writer.SetDescription(ctx, f.asset, description, []string{"old WAL"}); err == nil || !fault.dropped {
		t.Fatal("old mutation ACK was not lost", err)
	}
	writer.Close()
	f.powerOff(ctx, t)
	reader := f.newPeer(ctx, t)
	photo, err := reader.Photo(ctx, f.asset)
	if err != nil || photo.Description != description {
		t.Fatal("accepted old mutation was not visible to fresh recovery", err)
	}
	restarted := f.restartProducer(ctx, t)
	resumed := f.open(restarted, journal)
	if err := resumed.Initialize(ctx); err != nil {
		t.Fatal("old mutation journal reopen", err)
	}
	if err := resumed.SetDescription(ctx, f.asset, description, []string{"old WAL"}); err != nil {
		t.Fatal("old mutation WAL retry changed semantics", err)
	}
	snapshot, err := storage.LoadCatalog(ctx, restarted, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, head := range snapshot.Heads {
		if head.Reference == fault.accepted.Reference {
			found++
		}
	}
	if found != 1 {
		t.Fatal("old mutation replay duplicated the accepted revision", found)
	}
	pending, err := resumed.Pending(ctx)
	if err != nil || pending.Count != 0 {
		t.Fatal("old mutation replay kept pending WAL", pending.Count, err)
	}
	resumed.Close()
	reader.Close()
}

func exercisePrivateRecipeConcurrentCAS(ctx context.Context, t *testing.T, f *photoRecipeBoundaryFixture) {
	t.Helper()
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	var released sync.Once
	unblock := func() { released.Do(func() { close(release) }) }
	defer unblock()
	wait := func() { arrived <- struct{}{}; <-release }
	firstFault := &recipeBoundaryFault{Client: f.device.Client, beforeIndex: wait}
	secondFault := &recipeBoundaryFault{Client: f.peer, beforeIndex: wait}
	first, second := f.open(firstFault, f.journal()), f.open(secondFault, f.journal())
	defer first.Close()
	defer second.Close()
	if err := first.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := first.Recipe(ctx, f.asset)
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.Recipe(ctx, f.asset)
	if err != nil {
		t.Fatal(err)
	}
	base, err := a.Huella()
	if err != nil {
		t.Fatal(err)
	}
	a.Pasos = append(a.Pasos, model.Paso{Tipo: model.PasoExposicion, Params: map[string]float64{"valor": .25}})
	b.Pasos = append(b.Pasos, model.Paso{Tipo: model.PasoContraste, Params: map[string]float64{"valor": .25}})
	results := make(chan error, 2)
	go func() { _, err := first.SaveRecipe(ctx, f.asset, a, base); results <- err }()
	go func() { _, err := second.SaveRecipe(ctx, f.asset, b, base); results <- err }()
	for range 2 {
		select {
		case <-arrived:
		case err := <-results:
			t.Fatal("concurrent recipe stopped before its index boundary", err)
		case <-time.After(30 * time.Second):
			t.Fatal("concurrent recipe did not reach its publication boundary")
		}
	}
	unblock()
	for range 2 {
		select {
		case err := <-results:
			if err != nil && !errors.Is(err, privatePhotos.ErrConflict) {
				t.Fatal("unexpected concurrent recipe failure", err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	snapshot, err := storage.LoadCatalog(ctx, f.device.Client, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	branches := map[storage.Reference]bool{}
	for _, head := range snapshot.Heads {
		var header struct {
			Entity string `json:"entity"`
		}
		if json.Unmarshal(head.Commit.Metadata, &header) == nil && header.Entity == "edit" {
			branches[head.Reference] = true
		}
	}
	if len(branches) != 2 || !branches[firstFault.accepted.Reference] || !branches[secondFault.accepted.Reference] {
		t.Fatal("concurrent recipe silently chose one branch", len(branches))
	}
	first.Close()
	second.Close()
	f.powerOff(ctx, t)
	f.peer = freshNotesStorageDevice(ctx, f.lifetime, f.base, f.httpClient, f.device)
	recovered := f.open(f.peer, f.journal())
	defer recovered.Close()
	before := f.source.opensCount()
	if err := recovered.Initialize(ctx); !errors.Is(err, privatePhotos.ErrConflict) {
		t.Fatal("fresh recovery silently resolved concurrent recipe heads", err)
	}
	if f.source.opensCount() != before {
		t.Fatal("conflict reimported plaintext source")
	}
}
