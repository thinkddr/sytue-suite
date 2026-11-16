// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type draftFinalACK struct {
	*cryptoFixture
	lose bool
}

func (c *draftFinalACK) AcknowledgeStorageFile(ctx context.Context, journal *offline.Store, job string, index storage.Reference) error {
	if err := c.cryptoFixture.AcknowledgeStorageFile(ctx, journal, job, index); err != nil {
		return err
	}
	if c.lose {
		c.lose = false
		return errors.New("synthetic lost final draft ACK")
	}
	return nil
}

func setupDrafts(t *testing.T, roles ...string) *importFixture {
	t.Helper()
	f := setupImport(t)
	set := func(kind, id string, raw []byte) {
		key := storage.LegacySourceCursor{Kind: kind, ID: id}
		_, found := f.source.bodies[key]
		f.source.bodies[key] = raw
		row := storage.StreamSourceRow{Kind: kind, ID: id, Size: int64(len(raw)), SHA256: digest(raw)}
		if found {
			for i := range f.source.rows {
				if f.source.rows[i].Kind == kind && f.source.rows[i].ID == id {
					f.source.rows[i] = row
				}
			}
		} else {
			f.source.rows = append(f.source.rows, row)
			f.source.state.Rows++
		}
	}
	set("account", "account", []byte(`{"id":"account","address":"alice@example.test","signature":"Private signature"}`))
	set("alias", "alias", []byte(`{"id":"alias","address":"writing@example.test"}`))
	set("mailbox", "drafts", []byte(`{"id":"drafts","name":"Drafts","role":"drafts"}`))
	for _, role := range roles {
		raw, err := json.Marshal(api.Mailbox{ID: role, Name: role, Role: role})
		if err != nil {
			t.Fatal(err)
		}
		set("mailbox", role, raw)
	}
	slices.SortFunc(f.source.rows, func(a, b storage.StreamSourceRow) int {
		if n := strings.Compare(a.Kind, b.Kind); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	f.repo.cfg.MigrationState = f.source.state
	f.cfg.MigrationState = f.source.state
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestPrivateMailDraftExactMIMERecoveryAndLostACK(t *testing.T) {
	f := setupDrafts(t)
	facade, err := api.NewMail("https://closed.invalid", nil, nil).WithPrivateData(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	primary, aliases, err := facade.Aliases(t.Context())
	if err != nil || primary != "alice@example.test" || len(aliases) != 1 || aliases[0].Address != "writing@example.test" {
		t.Fatal("private sender inventory was not recovered", primary, aliases, err)
	}
	raw, err := eml.Componer(eml.Salida{De: aliases[0].Address, Para: []string{"to@example.test"}, Cco: []string{"private@example.test"}, Asunto: "Private draft", EnRespuestaA: "<original@example.test>", Texto: "Retained original", Borrador: true, Adjuntos: []eml.Adjunto{{Nombre: "proof.txt", Tipo: "text/plain", Datos: []byte("private attachment")}}}, time.Date(2026, 10, 30, 10, 20, 30, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	f.crypto.loseIndex = true
	if _, err := facade.SaveDraft(t.Context(), "", aliases[0].Address, raw); err == nil {
		t.Fatal("lost accepted draft ACK was not exercised")
	}
	files := len(f.crypto.files)
	draft, err := facade.SaveDraft(t.Context(), "", aliases[0].Address, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.crypto.files) != files || draft.MailboxID != "drafts" || draft.ThreadID != "thread" || !draft.Has(`\Draft`) || !draft.Has(api.MailFlagSeen) || len(draft.Bcc) != 1 || draft.Bcc[0] != "private@example.test" || !draft.HasAttachment {
		t.Fatal("draft did not resume exact original without duplicating ciphertext", draft, len(f.crypto.files), files)
	}
	if again, err := facade.SaveDraft(t.Context(), "", aliases[0].Address, raw); err != nil || again.ID != draft.ID || len(f.crypto.files) != files {
		t.Fatal("completed draft retry duplicated its object", again, err)
	}
	// A fresh repository has no editor state and must recover MIME from E2E history.
	repo := f.reopen(t)
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	body, err := repo.Raw(t.Context(), draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	actual, readErr := io.ReadAll(body)
	closeErr := body.Close()
	defer clear(actual)
	if readErr != nil || closeErr != nil || !bytes.Equal(actual, raw) {
		t.Fatal("reopened draft changed exact MIME", readErr, closeErr)
	}
	snapshot, err := storage.LoadAtomicCatalog(t.Context(), f.crypto, testScope)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := repo.document(t.Context(), snapshot, "message", draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	var doc MessageDocument
	if json.Unmarshal(payload, &doc) != nil {
		t.Fatal("bad draft document")
	}
	clear(payload)
	doc.Envelope, err = patchField(doc.Envelope, "future", json.RawMessage(`900719925474099512345`))
	if err != nil {
		t.Fatal(err)
	}
	versions := heads(snapshot, objectID("message", draft.ID))
	f.crypto.seed(t, "message", draft.ID, doc, versions[0].Reference)
	clear(doc.Envelope)
	changed, err := eml.Componer(eml.Salida{De: aliases[0].Address, Asunto: "", Texto: "Edited", Borrador: true}, time.Date(2026, 10, 30, 10, 21, 30, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	replaceTestCrypto(t, repo, &draftFinalACK{cryptoFixture: f.crypto, lose: true})
	if _, err := repo.SaveDraft(t.Context(), draft.ID, aliases[0].Address, changed); err == nil {
		t.Fatal("final accepted draft ACK was not lost")
	}
	files = len(f.crypto.files)
	edited, err := repo.SaveDraft(t.Context(), draft.ID, aliases[0].Address, changed)
	if err != nil || edited.ID != draft.ID || len(edited.Bcc) != 0 || len(edited.To) != 0 || edited.Subject != "" || edited.HasAttachment {
		t.Fatal("draft edit retained stale indexed fields", edited, err)
	}
	if len(f.crypto.files) != files {
		t.Fatal("completed envelope retry duplicated MIME ciphertext")
	}
	if count, err := repo.cfg.Journal.CountOwner(testScope.Tenant+"\x00"+testScope.Subject+"\x00mail", ingestNamespace); err != nil || count != 0 {
		t.Fatal("accepted draft retry retained unfinished WAL", count, err)
	}
	snapshot, err = storage.LoadAtomicCatalog(t.Context(), f.crypto, testScope)
	if err != nil {
		t.Fatal(err)
	}
	payload, err = repo.document(t.Context(), snapshot, "message", draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(payload)
	if json.Unmarshal(payload, &doc) != nil || !bytes.Contains(doc.Envelope, []byte(`"future":900719925474099512345`)) {
		t.Fatal("draft edit rounded or removed unknown extension")
	}
	clear(doc.Envelope)
	if _, err := repo.SaveDraft(t.Context(), draft.ID, aliases[0].Address, raw); err != nil {
		t.Fatal("returning to earlier draft bytes reused an old transaction", err)
	}
	if _, err := repo.SaveDraft(t.Context(), "live", aliases[0].Address, raw); !errors.Is(err, api.ErrPrivateMailOperation) {
		t.Fatal("ordinary received message became a draft", err)
	}
	if _, err := repo.SaveDraft(t.Context(), "", "forged@example.test", raw); !errors.Is(err, api.ErrPrivateMailOperation) {
		t.Fatal("unowned sender was inferred from MIME", err)
	}
}
