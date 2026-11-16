// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func revisionMIME(t *testing.T, subject string) []byte {
	t.Helper()
	raw, err := eml.Componer(eml.Salida{De: "writing@example.test", Asunto: subject, Texto: "PRIVATE-IMMUTABLE-REVISION", Cco: []string{"hidden@example.test"}, Borrador: true}, time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPrivateDraftRevisionKeepsOriginalAcrossLostACKRestartAndLaterEdits(t *testing.T) {
	f := setupDrafts(t)
	facade, err := api.NewMail("https://closed.invalid", nil, nil).WithPrivateData(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	port, private := facade.PrivateDraftRevisions()
	if !private || port == nil {
		t.Fatal("private editor lost its admitted revision port")
	}
	raw := revisionMIME(t, "First immutable revision")
	original := bytes.Clone(raw)
	defer clear(original)
	input := api.MailDraftRevision{Editor: uuid.NewString(), Revision: uuid.NewString(), From: "writing@example.test", MIME: raw}
	beforePuts, files := f.crypto.puts, len(f.crypto.files)
	attempt, err := port.PrepareDraftRevision(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	clear(raw)
	if f.crypto.puts != beforePuts || len(f.crypto.files) != files {
		t.Fatal("draft preparation published before its durable intent")
	}
	pending, err := f.repo.Pending(t.Context())
	if err != nil || pending.Count != 1 {
		t.Fatal("admitted revision escaped retained ownership", pending, err)
	}
	f.crypto.loseIndex = true
	if _, err := port.CommitDraftRevision(t.Context(), attempt); err == nil {
		t.Fatal("accepted MIME index ACK was not lost")
	}
	files = len(f.crypto.files)
	newer := input
	newer.Revision = uuid.NewString()
	newer.MIME = revisionMIME(t, "New edits after the uncertain revision")
	if _, err := port.PrepareDraftRevision(t.Context(), newer); !errors.Is(err, ErrDraftRevisionPending) {
		t.Fatal("a later edit replaced an unacknowledged original", err)
	}
	repo := f.reopen(t)
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	pendingAttempts, err := repo.PendingDraftRevisions(t.Context())
	if err != nil || len(pendingAttempts) != 1 || pendingAttempts[0] != attempt {
		t.Fatal("restart lost the exact revision handle", pendingAttempts, err)
	}
	saved, err := repo.CommitDraftRevision(t.Context(), attempt)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.crypto.files) != files {
		t.Fatal("replay recreated MIME ciphertext")
	}
	body, err := repo.Raw(t.Context(), saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(body)
	closeErr := body.Close()
	defer clear(actual)
	if err != nil || closeErr != nil || !bytes.Equal(actual, original) {
		t.Fatal("replay used replacement editor bytes", err, closeErr)
	}
	if pending, err := repo.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("completed editor revision retained unsent work", pending, err)
	}
	if ids, err := repo.cfg.Journal.RecordIDs(repo.draftRevisionOwner(), storage.MailDraftRevisionStagingNamespace); err != nil || len(ids) != 0 {
		t.Fatal("completed revision retained its staged original", ids, err)
	}
	if again, err := repo.CommitDraftRevision(t.Context(), attempt); err != nil || again.ID != saved.ID || len(f.crypto.files) != files {
		t.Fatal("lost UI callback could not reconcile authenticated history", again, err)
	}
	newer.ID = saved.ID
	next, err := repo.PrepareDraftRevision(t.Context(), newer)
	if err != nil {
		t.Fatal(err)
	}
	concurrent := revisionMIME(t, "Concurrent accepted revision")
	if _, err := repo.SaveDraft(t.Context(), saved.ID, "writing@example.test", concurrent); err != nil {
		t.Fatal(err)
	}
	files = len(f.crypto.files)
	if _, err := repo.CommitDraftRevision(t.Context(), next); !errors.Is(err, ErrConflict) {
		t.Fatal("replay recaptured newer message parents", err)
	}
	if len(f.crypto.files) != files {
		t.Fatal("conflicting revision uploaded before validating its original parents")
	}
	wrong := next
	wrong.Scope.Subject = "other"
	if _, err := repo.CommitDraftRevision(t.Context(), wrong); !errors.Is(err, ErrRecord) {
		t.Fatal("a revision handle crossed its identity", err)
	}
}
