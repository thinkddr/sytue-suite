// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// directMailSource es el servidor de correo visto por la bóveda: empieza en
// `legacy` y Begin lo pasa a la época retenida, como el de verdad.
type directMailSource struct {
	*retainedMailSource
	retained storage.LegacyMigrationState
	begins   []string
}

func (s *directMailSource) SnapshotEpoch(context.Context, storage.Scope) (string, error) {
	return testEpoch, nil
}

func (s *directMailSource) BeginPrivateStorageMigration(_ context.Context, _ storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
	s.begins = append(s.begins, epoch)
	if epoch != s.retained.Epoch {
		return storage.LegacyMigrationState{}, ErrRecord
	}
	s.state = s.retained
	return s.state, nil
}

func setupDirect(t *testing.T) (*importFixture, *directMailSource, Config) {
	t.Helper()
	f := setupImport(t)
	f.repo.Close()
	source := &directMailSource{retainedMailSource: f.source, retained: f.source.state}
	source.state = storage.LegacyMigrationState{Scope: testScope, Mode: "legacy"}
	cfg := f.cfg
	cfg.WorkLease = mailTransferLease
	cfg.MigrationState = storage.LegacyMigrationState{}
	return f, source, cfg
}

func TestDirectSourceImportsServerMailStraightIntoTheVault(t *testing.T) {
	_, source, cfg := setupDirect(t)
	state, err := CaptureDirectSource(t.Context(), cfg, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.begins) != 1 || !state.SameRetainedSource(source.retained) {
		t.Fatalf("Begin = %v, estado = %+v", source.begins, state)
	}
	cfg.MigrationState = state
	repo, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if !repo.MigrationState().SameRetainedSource(state) {
		t.Fatal("el repositorio no conserva su época")
	}
	if err := repo.ImportDirectSource(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	boxes, err := repo.Mailboxes(t.Context())
	if err != nil || len(boxes) != 1 || boxes[0].TotalEmails != 1 {
		t.Fatal("el buzón importado no está", boxes, err)
	}
	owner := testScope.Tenant + "\x00" + testScope.Subject + "\x00mail"
	var intent struct{}
	if found, err := cfg.Journal.Read(owner, importNamespace, "snapshot", &intent); err != nil || found {
		t.Fatal("la intención de la época sigue en el diario", found, err)
	}
	// Reabrir no vuelve a pedir nada al servidor: el recibo ya está.
	pages := source.pages
	source.forbidPages = true
	again, err := CaptureDirectSource(t.Context(), cfg, source)
	if err != nil || len(source.begins) != 1 || !again.SameRetainedSource(state) {
		t.Fatal("una segunda apertura empezó otra época", source.begins, err)
	}
	if err := repo.ImportDirectSource(t.Context(), source); err != nil || source.pages != pages {
		t.Fatal("una segunda apertura reimportó", err, source.pages, pages)
	}
}

func TestDirectSourceNeverMixesAnImportWithExistingData(t *testing.T) {
	f := setupImport(t)
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	f.repo.Close()
	source := &directMailSource{retainedMailSource: f.source, retained: f.source.state}
	source.state = storage.LegacyMigrationState{Scope: testScope, Mode: "legacy"}
	cfg := f.cfg
	cfg.WorkLease = mailTransferLease
	if _, err := CaptureDirectSource(t.Context(), cfg, source); err == nil || len(source.begins) != 0 {
		t.Fatal("una bóveda con datos empezó la importación", err, source.begins)
	}
}

func TestDirectSourceIsOnlyForTheIsolatedTarget(t *testing.T) {
	f, source, cfg := setupDirect(t)
	f.crypto.identity = storage.LegacyIdentity
	if _, err := CaptureDirectSource(t.Context(), cfg, source); !errors.Is(err, ErrRecord) || len(source.begins) != 0 {
		t.Fatal("la identidad 1 de Matrix recibió la importación directa", err)
	}
	f.crypto.identity = storage.IsolatedIdentity
	cfg.WorkLease = nil
	if _, err := CaptureDirectSource(t.Context(), cfg, source); !errors.Is(err, ErrRecord) {
		t.Fatal("sin concesión de trabajo", err)
	}
	cfg.WorkLease = mailTransferLease
	if _, err := CaptureDirectSource(t.Context(), cfg, nil); !errors.Is(err, ErrRecord) {
		t.Fatal("sin origen", err)
	}
	var repo *Repository
	if err := repo.ImportDirectSource(t.Context(), source); !errors.Is(err, ErrRecord) {
		t.Fatal("sin repositorio", err)
	}
}

func TestDirectSourceKeepsAnAlreadyRetainedEpoch(t *testing.T) {
	_, source, cfg := setupDirect(t)
	source.state = source.retained
	state, err := CaptureDirectSource(t.Context(), cfg, source)
	if err != nil || len(source.begins) != 0 || !state.SameRetainedSource(source.retained) {
		t.Fatal("una época ya retenida se volvió a empezar", err, source.begins)
	}
}
