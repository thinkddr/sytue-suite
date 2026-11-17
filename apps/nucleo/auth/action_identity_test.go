// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"context"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestActionIdentityIsNonblockingDuringRenewalAndStableAcrossRefresh(t *testing.T) {
	aparte(t)
	started, release := make(chan struct{}), make(chan struct{})
	tokens := Tokens{Issuer: "https://id.example", Cliente: ClientID, Subject: "ana", Organizacion: "acme"}
	session := NewSession(tokens, func(context.Context, Tokens) (Tokens, error) {
		close(started)
		<-release
		tokens.Access = "synthetic-refreshed"
		tokens.Refresh = "synthetic-rotated"
		tokens.Expiry = time.Now().Add(time.Hour)
		return tokens, nil
	}, nil)
	before := session.SnapshotActionIdentity()
	done := make(chan error, 1)
	go func() { _, err := session.AccessToken(t.Context()); done <- err }()
	<-started
	got := make(chan ActionIdentity, 1)
	go func() { got <- session.SnapshotActionIdentity() }()
	select {
	case snapshot := <-got:
		if snapshot != before {
			close(release)
			t.Fatal("renewal mixed action identity", snapshot)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("UI action waited for network renewal")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if after := session.SnapshotActionIdentity(); after != before {
		t.Fatal("ordinary refresh invalidated action", after, before)
	}
	session.Sincronizar(tokens)
	if after := session.SnapshotActionIdentity(); after != before {
		t.Fatal("shared refresh invalidated action", after, before)
	}
	if err := session.AdoptarCon(t.Context(), tokens); err != nil {
		t.Fatal(err)
	}
	after := session.SnapshotActionIdentity()
	if after.Identity != before.Identity || after.Incarnation == before.Incarnation {
		t.Fatal("interactive adoption reused an action", after, before)
	}
}

func TestGenericSessionIdentityAllowsIndividualWithoutEnablingDriveOrStorageTenant(t *testing.T) {
	individual := IdentitySnapshot{Issuer: "https://id.example/", Client: ClientID, Subject: "ana"}
	if (storage.Scope{Subject: "ana", Product: "notes"}).Valid() {
		t.Fatal("session marker invented a storage tenant")
	}
	hash := individual.IdentidadSesion()
	if len(hash) != 64 || individual.IdentidadDrive() != "" {
		t.Fatal("individual login depends on Drive organisation")
	}
	organisation := individual
	organisation.Organization = "acme"
	if organisation.IdentidadSesion() == hash || organisation.IdentidadDrive() != organisation.IdentidadSesion() {
		t.Fatal("personal and organisation identities collided")
	}
}
