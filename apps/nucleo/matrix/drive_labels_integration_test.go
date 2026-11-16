// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration && cgo

package matrix

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privatedrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
)

func exercisePrivateDriveLabels(ctx context.Context, t *testing.T, repository *privatedrive.Repository, open func() *privatedrive.Repository, faults *lostDriveImporterACK, source *syntheticDriveSource, gateway *fileGatewayFixture) *privatedrive.Repository {
	t.Helper()
	before, err := repository.File(ctx, source.File)
	if err != nil {
		t.Fatal(err)
	}
	versions, _, err := repository.Versions(ctx, source.File, "")
	if err != nil {
		t.Fatal(err)
	}
	captured := api.EtiquetaDrive{ID: "private-label", Name: "Private synthetic captured policy", Rank: 40, BlockExternalShare: true}
	gateway.mu.Lock()
	gets, puts := gateway.gets, gateway.puts
	gateway.labels = []api.EtiquetaDrive{captured}
	gateway.mu.Unlock()
	labels, err := repository.SensitivityLabels(ctx)
	if err != nil || len(labels) != 1 || labels[0] != captured {
		t.Fatal("private taxonomy lost its scoped organization policy", labels, err)
	}
	writes := faults.writes
	if err := repository.SetLabel(ctx, source.File, "foreign-label"); !errors.Is(err, privatedrive.ErrInvalidLabel) || writes != faults.writes {
		t.Fatal("foreign classification published a private revision", err)
	}
	initialBinding := faults.cfg.Bootstrap
	defer func() {
		if err := faults.UpdateBindings(initialBinding); err != nil {
			t.Error(err)
		}
	}()
	for _, stage := range []string{"content", "catalog", "complete"} {
		faults.contentLost, faults.catalogLost = stage != "content", stage != "catalog"
		err := repository.SetLabel(ctx, source.File, captured.ID)
		if (err != nil) != (stage != "complete") {
			t.Fatal("classification lost its durable publication", stage, err)
		}
		if stage == "content" {
			// Policy can change or disappear while a saved intent is pending.
			gateway.mu.Lock()
			gateway.labels = []api.EtiquetaDrive{{ID: captured.ID, Name: "Private synthetic relaxed policy", Rank: 1}}
			gateway.policyStatus = http.StatusServiceUnavailable
			gateway.mu.Unlock()
			rotated := initialBinding
			rotated.StorageRoomID = initialBinding.StorageRoomIDs[0]
			if err := faults.UpdateBindings(rotated); err != nil {
				t.Fatal(err)
			}
		}
		repository.Close()
		repository = open()
		if err := repository.Initialize(ctx); err != nil {
			t.Fatal("classified repository did not reopen", stage, err)
		}
	}
	file, err := repository.File(ctx, source.File)
	if err != nil || file.Label == nil || !file.Label.BlockExternalShare || file.Label.Name != captured.Name || file.Version != before.Version || !file.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("retry weakened captured policy or changed content metadata", file, err)
	}
	gateway.mu.Lock()
	policyCalls := gateway.policyCalls
	gateway.mu.Unlock()
	if policyCalls != 3 {
		t.Fatal("saved classification refetched changed policy", policyCalls)
	}
	copied, err := repository.CopyFile(ctx, source.File)
	if err != nil || copied.Label == nil || !copied.Label.BlockExternalShare || copied.Label.Name != captured.Name {
		t.Fatal("private copy lost classification", copied, err)
	}
	gateway.mu.Lock()
	gateway.policyStatus = http.StatusOK
	gateway.mu.Unlock()
	if err := repository.SetLabel(ctx, source.File, captured.ID); err != nil {
		t.Fatal("explicit reclassification failed", err)
	}
	file, err = repository.File(ctx, source.File)
	if err != nil || file.Label == nil || file.Label.BlockExternalShare || file.Label.Name == captured.Name {
		t.Fatal("explicit classification did not resolve fresh policy", file, err)
	}
	gateway.mu.Lock()
	gateway.policyStatus = http.StatusServiceUnavailable
	policyCalls = gateway.policyCalls
	gateway.mu.Unlock()
	if err := repository.SetLabel(ctx, source.File, ""); err != nil {
		t.Fatal("clear depended on unavailable control-plane policy", err)
	}
	file, err = repository.File(ctx, source.File)
	if err != nil || file.Label != nil || file.Version != before.Version || !file.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("clear changed content metadata or kept a restriction", file, err)
	}
	currentVersions, _, err := repository.Versions(ctx, source.File, "")
	if err != nil || len(currentVersions) != len(versions) {
		t.Fatal("classification manufactured a content version", len(currentVersions), err)
	}
	if err := repository.Delete(ctx, api.DriveFile, source.File); err != nil {
		t.Fatal(err)
	}
	writes = faults.writes
	if err := repository.SetLabel(ctx, source.File, captured.ID); !errors.Is(err, privatedrive.ErrNotFound) || writes != faults.writes {
		t.Fatal("hidden file classification reached policy or publication", err)
	}
	gateway.mu.Lock()
	afterCalls, afterGets, afterPuts := gateway.policyCalls, gateway.gets, gateway.puts
	gateway.mu.Unlock()
	if policyCalls != afterCalls || gets != afterGets || puts != afterPuts {
		t.Fatal("classification or clear transferred file media or reopened policy", afterCalls, policyCalls)
	}
	if err := repository.Restore(ctx, api.DriveFile, source.File); err != nil {
		t.Fatal(err)
	}
	writes = faults.writes
	if err := repository.SetLabel(ctx, source.File, captured.ID); err == nil || writes != faults.writes {
		t.Fatal("policy lookup failure published a partial classification", err)
	}
	t.Log("private classification retains captured policy across both lost ACKs, room rotation and restart, resolves explicit changes, clears without policy, preserves content version/time and reuses binary media")
	return repository
}
