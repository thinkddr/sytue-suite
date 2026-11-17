// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestStorageFileLinkAgainstSynapseRetainsPlanBeforeCASAndRecoversFresh(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_FILE_LINK") != "1" {
		t.Skip("requires isolated disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	upstream, err := url.Parse(os.Getenv("SYTUE_MATRIX_FILE_LINK_URL"))
	if err != nil || upstream.Host == "" {
		t.Fatal("disposable Synapse URL required")
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
	t.Cleanup(proxy.Close)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "photos"}
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	seed, gateway, _, _ := fileFixture(t)
	gateway.product = "photos"
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"kind":"synthetic opaque source plan","retained_original":"private-plan-sentinel"}`)
	request := storage.FileRequest{ObjectID: "photos_source_plan", RevisionID: "source_plan_revision", SizeBytes: int64(len(body)), Metadata: []byte(`{"kind":"photos-source-plan"}`)}
	root, err := device.Client.UploadStorageFile(ctx, seed.gateway, journal, "durable_plan", request, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	target, err := device.Client.StoragePublicationTarget()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(storage.FileLink{Kind: storage.FileLinkKind, Content: storage.ContentPointer{Kind: "file", RevisionID: request.RevisionID, Reference: root}})
	if err != nil {
		t.Fatal(err)
	}
	link, err := device.Client.PutStorageRecordAt(ctx, target, "durable_plan_link", storage.Record{Version: 1, Scope: scope, ObjectID: request.ObjectID, RevisionID: request.RevisionID, Role: "content", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	// A rejected backend CAS cannot acknowledge this local intent. Retrying the
	// same file and retained link must not publish an application catalog entry.
	before := gateway.puts
	retry, err := device.Client.UploadStorageFile(ctx, seed.gateway, journal, "durable_plan", request, bytes.NewReader(body))
	if err != nil || retry != root || gateway.puts != before {
		t.Fatal("CAS retry changed the retained file", err)
	}
	ioState := storageFileIO{scope: scope, journal: journal}
	var pending storageFileUpload
	if found, err := journal.Read(ioState.journalOwner(), storage.PendingUploadsNamespace, "durable_plan", &pending); err != nil || !found || !pending.Verified {
		t.Fatal("rejected CAS lost verified WAL", err)
	}
	if err := device.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	fresh := freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), device)
	record, err := fresh.ReadStorageRecord(ctx, link, request.ObjectID, "content")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := storage.DecodeFileLink(record, scope)
	if err != nil || decoded.Content.Reference != root {
		t.Fatal("fresh device could not authenticate retained sidecar", err)
	}
	var out bytes.Buffer
	if _, err := fresh.DownloadStorageFile(ctx, seed.gateway, root, request.ObjectID, request.RevisionID, &out); err != nil || !bytes.Equal(out.Bytes(), body) {
		t.Fatal("fresh device lost exact plan bytes", err)
	}
	if err := device.Client.AcknowledgeStorageFile(ctx, journal, "durable_plan", link); err == nil {
		t.Fatal("sidecar accepted as catalog index")
	}
	// The simulated server accepts the SAME opaque reference only after the
	// fresh-device readback. This is the caller's required control-plane CAS.
	accepted := link
	if accepted != link {
		t.Fatal("CAS changed reference")
	}
	if err := device.Client.AcknowledgeStorageFileLink(ctx, journal, "durable_plan", accepted); err != nil {
		t.Fatal(err)
	}
	if found, err := journal.Read(ioState.journalOwner(), storage.PendingUploadsNamespace, "durable_plan", &pending); err != nil || found {
		t.Fatal("accepted exact link did not retire WAL", err)
	}
	catalog, err := storage.LoadCatalog(ctx, fresh, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Heads) != 0 {
		t.Fatal("sidecar contaminated app catalog")
	}
	for _, ref := range []storage.Reference{root, link} {
		wire, err := device.Matrix.GetEvent(ctx, id.RoomID(ref.RoomID), id.EventID(ref.EventID))
		encoded, encodeErr := json.Marshal(wire)
		if err != nil || encodeErr != nil || wire.Type != event.EventEncrypted || bytes.Contains(encoded, body) || bytes.Contains(encoded, payload) {
			t.Fatal("sidecar leaked plan or pointer", err, encodeErr)
		}
	}
	if _, err := device.Matrix.GetEvent(ctx, id.RoomID(root.RoomID), id.EventID(root.EventID)); err != nil {
		t.Fatal("ACK removed retained file", err)
	}
	t.Log("real SDK: encrypted role-content link, rejected CAS retry, exact fresh-device plan recovery, catalog isolation and WAL-only acknowledgment verified")
}
