// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	privatedrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
)

func exerciseConditionalPrivatePDFRecovery(ctx context.Context, t *testing.T, repository *privatedrive.Repository, open func(privatedrive.Crypto) *privatedrive.Repository, faults *lostDriveImporterACK, gateway *fileGatewayFixture, proof *privatePDFStorageProof) *privatedrive.Repository {
	t.Helper()
	repository.Close()
	repository = open(faults)
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	before := proof.survivor
	before.Checksum = strings.ToUpper(before.Checksum)
	faults.catalogLost = false
	if _, err := repository.ReplaceFile(ctx, before, "application/pdf", bytes.NewReader(proof.survivorBytes), int64(len(proof.survivorBytes))); err == nil || !faults.catalogLost {
		t.Fatal("conditional write did not lose its accepted index ACK", err)
	}
	writes := faults.writes
	changed := bytes.Clone(proof.survivorBytes)
	changed[0] ^= 1
	if _, err := repository.ReplaceFile(ctx, before, "application/pdf", bytes.NewReader(changed), int64(len(changed))); !errors.Is(err, privatedrive.ErrSource) || faults.writes != writes {
		t.Fatal("changed conditional retry published over its retained source", err)
	}
	clear(changed)
	gateway.mu.Lock()
	puts := gateway.puts
	gateway.mu.Unlock()
	repository.Close()
	repository = open(faults)
	if err := repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	resumed, err := repository.ResumeUploads(ctx)
	if err != nil || len(resumed) != 1 || resumed[0].ID != before.ID || resumed[0].Version != before.Version+1 {
		t.Fatal("conditional WAL did not recover its fixed original version", resumed, err)
	}
	proof.survivor = resumed[0]
	gateway.mu.Lock()
	after := gateway.puts
	gateway.mu.Unlock()
	if puts != after {
		t.Fatal("conditional recovery uploaded another ciphertext", puts, after)
	}
	writes = faults.writes
	if _, err := repository.ReplaceFile(ctx, before, "application/pdf", bytes.NewReader(proof.survivorBytes), int64(len(proof.survivorBytes))); !errors.Is(err, privatedrive.ErrConflict) || faults.writes != writes {
		t.Fatal("stale conditional write silently advanced the file", err)
	}
	wrong := proof.survivor
	wrong.ID = proof.original.ID
	if _, err := repository.ReplaceFile(ctx, wrong, "application/pdf", bytes.NewReader(proof.survivorBytes), int64(len(proof.survivorBytes))); !errors.Is(err, privatedrive.ErrConflict) || faults.writes != writes {
		t.Fatal("conditional write retargeted a different file with the same path", err)
	}
	checkPrivatePDFOriginal(ctx, t, privatePDFClient(t, repository), proof.survivor, proof.survivorBytes)
	t.Log("conditional Drive WAL recovered accepted ACK without another ciphertext; changed input and stale or different original rejected")
	return repository
}
