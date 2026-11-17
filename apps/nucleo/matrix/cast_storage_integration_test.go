// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateCast "git.sytue.eu/sytue/one/apps/nucleo/cast"
	"git.sytue.eu/sytue/one/apps/nucleo/cast/model"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

func TestPrivateCastAgainstSynapsePreservesCompleteMediaAndFreshDevice(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PRIVATE_CAST") != "1" {
		t.Skip("requires isolated disposable remote Synapse and real encoded media fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 7*time.Minute)
	defer cancel()
	upstream, err := url.Parse(os.Getenv("SYTUE_MATRIX_PRIVATE_CAST_URL"))
	if err != nil || upstream.Host == "" {
		t.Fatal("disposable Synapse URL required")
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
	t.Cleanup(proxy.Close)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "cast"}
	old, controller := controlledMigrationSource(ctx, t, proxy.URL, proxy.Client(), scope)
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	opaque := &fileGatewayFixture{product: "cast"}
	blobServer := httptest.NewServer(opaque)
	t.Cleanup(blobServer.Close)
	gateway := api.NewDrive(blobServer.URL, nil, blobServer.Client())
	videoID := uuid.NewString()
	prefix := "v/" + videoID + "/"
	root := os.Getenv("SYTUE_MATRIX_PRIVATE_CAST_MEDIA_DIR")
	if root == "" {
		t.Fatal("real encoded MP4/HLS/WebM/poster fixture required")
	}
	objects := map[string][]byte{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		objects[prefix+filepath.ToSlash(relative)] = body
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(objects) < 6 {
		t.Fatal("incomplete generated real-media fixture")
	}
	webm := objects[prefix+"video.webm"]
	if len(webm) == 0 {
		t.Fatal("real WebM fixture required")
	}
	objects[prefix+"720p.webm"] = webm
	objects[prefix+"360p.webm"] = webm
	video := []byte(`{"id":"` + videoID + `","tenant":"acme","owner_sub":"` + scope.Subject + `","kind":"upload","status":"ready","visibility":"private","title":"cast-private-title-sentinel","original_key":"` + prefix + `original.mp4","hls_key":"` + prefix + `hls/master.m3u8","webm_key":"` + prefix + `video.webm","webm_qualities":[720,360],"poster_key":"` + prefix + `poster.png","created_at":"2026-10-30T12:00:00Z","future_integer":900719925474099312345}`)
	source := &photosRetainedSourceFixture{notesRetainedSourceFixture: notesRetainedSourceFixture{state: storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}, rows: map[string][]byte{"video/" + videoID: video}}, blobs: int64(len(objects))}
	mediaIDs := map[string]string{}
	for key, body := range objects {
		mediaID, err := model.MediaID(videoID, key)
		if err != nil {
			t.Fatal(err)
		}
		mediaIDs[key] = mediaID
		media := model.MediaRow{ID: mediaID, Tenant: scope.Tenant, Owner: scope.Subject, VideoID: videoID, Key: key, Size: int64(len(body)), SHA256: digestBytes(body), ContentType: "application/octet-stream"}
		source.rows["media/"+mediaID], _ = json.Marshal(media)
		source.rows[model.BinaryKind+"/"+mediaID] = body
	}
	httpSource := httptest.NewServer(source.handler(t))
	t.Cleanup(httpSource.Close)
	port, err := privateCast.NewHTTPSource(api.NewCast(httpSource.URL, nil, httpSource.Client()))
	if err != nil {
		t.Fatal(err)
	}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	config := func(client privateCast.Crypto) privateCast.Config {
		return privateCast.Config{
			Context: ctx, Scope: scope, Crypto: client, Gateway: gateway, Journal: journal, Source: port,
			WorkLease: func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() },
		}
	}
	historical, err := privateCast.NewHistoricalSource(config(old.Client))
	if err != nil {
		t.Fatal(err)
	}
	defer historical.Close()
	routing := &driveCutoverRouting{state: storage.IdentityCutoverState{Version: 1, Scope: scope, Phase: "legacy", SourceRooms: []string{old.Room.String()}}}
	repository, err := privateCast.New(config(device.Client))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	source.mu.Lock()
	source.state.Shared = true
	source.mu.Unlock()
	if err := historical.Prepare(ctx, routing.state); !errors.Is(err, privateCast.ErrShared) || source.beginsCount() != 0 {
		t.Fatal("shared capability froze before rejecting recipients", err)
	}
	source.mu.Lock()
	source.state.Shared = false
	source.mu.Unlock()
	if err := repository.Initialize(ctx); !errors.Is(err, privateCast.ErrNotReady) {
		t.Fatal("uninitialized private Cast exposed content", err)
	}
	if err := historical.Prepare(ctx, routing.state); err != nil {
		t.Fatal("prepare complete historical media", err)
	}
	readsBeforeTransfer := source.opensCount()
	if _, err := controller.SendStateEvent(ctx, old.Room, event.StatePowerLevels, "", migrationFixturePowers(true)); err != nil {
		t.Fatal(err)
	}
	routing.state.Phase = "frozen"
	routing.state.Epoch = uuid.NewString()
	cutover, err := repository.IdentityCutover(records.IdentityCutoverConfig{
		Scope: scope, Service: routing, Legacy: historical, Authorize: func(ctx context.Context) error { return ctx.Err() },
		CopyFile: func(ctx context.Context, target storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
			return old.Client.MigrateStorageFileContentAt(ctx, device.Client, gateway, journal, job, target, ref, object, revision)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := cutover.Prepare(ctx); err != nil {
		t.Fatal("frozen legacy to isolated Cast history", err)
	}
	if routing.state.Phase != "isolated" || source.opensCount() != readsBeforeTransfer {
		t.Fatal("cutover did not use the complete private source")
	}
	if pending, err := repository.Pending(ctx); err != nil || pending.Count != 0 {
		t.Fatal("confirmed transfer kept active work", pending, err)
	}
	archive, err := journal.RecordIDs(scope.Tenant+"\x00"+scope.Subject+"\x00"+scope.Product, storage.IdentityHistoryNamespace(scope.Product))
	if err != nil || len(archive) == 0 {
		t.Fatal("transfer evidence was lost", err)
	}
	if err := repository.Rename(ctx, videoID, "updated private Cast title"); err != nil {
		t.Fatal(err)
	}
	if err := device.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	fresh := freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), device)
	journal, err = offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := privateCast.New(config(fresh))
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	reads := source.opensCount()
	if err := recovered.Initialize(ctx); err != nil {
		t.Fatal("fresh private Cast catalog", err)
	}
	values, err := recovered.Videos(ctx, false)
	if err != nil || len(values) != 1 || values[0].Title != "updated private Cast title" {
		t.Fatal("private metadata not recovered", err)
	}
	raw, err := recovered.ReadRaw(ctx, "video", videoID)
	if err != nil || !bytes.Contains(raw, []byte(`900719925474099312345`)) || !bytes.Contains(raw, []byte(`"visibility":"private"`)) {
		t.Fatal("private metadata changed audience or unknown fields", err)
	}
	clear(raw)
	for key, body := range objects {
		var output bytes.Buffer
		if err := recovered.ReadObject(ctx, mediaIDs[key], &output); err != nil || !bytes.Equal(output.Bytes(), body) {
			t.Fatal("retained media bytes changed", filepath.Ext(key), err)
		}
	}
	facade, err := api.NewCast(httpSource.URL, nil, httpSource.Client()).WithPrivateData(recovered.Data())
	if err != nil {
		t.Fatal(err)
	}
	page, err := facade.Videos(ctx, "mine", "ready", "updated private", "", 1)
	if err != nil || len(page.Videos) != 1 || page.Videos[0].Owner.Sub != scope.Subject {
		t.Fatal("private API catalog", err)
	}
	play, err := facade.Play(ctx, videoID)
	if err != nil || play.WebMURL == "" || play.DownloadURL == "" || play.PosterURL == "" || play.HLSURL != "" {
		t.Fatal("private playback routing", err)
	}
	for _, entry := range []struct{ route, key string }{{play.WebMURLDe(api.CastCalidadPorDefecto), prefix + "720p.webm"}, {play.WebMURLDe(360), prefix + "360p.webm"}, {play.DownloadURL, prefix + "original.mp4"}, {play.PosterURL, prefix + "poster.png"}} {
		if facade.MediaURL(entry.route) != "" {
			t.Fatal("private playback leaked a provider URL")
		}
		media, err := facade.Media(ctx, entry.route)
		if err != nil {
			t.Fatal("open authenticated media", err)
		}
		if media.Size() != int64(len(objects[entry.key])) {
			t.Fatal("media size changed")
		}
		if _, err := media.Seek(-3, io.SeekEnd); err != nil {
			t.Fatal(err)
		}
		tail, err := io.ReadAll(media)
		if err != nil || !bytes.Equal(tail, objects[entry.key][len(objects[entry.key])-3:]) {
			t.Fatal("private seek changed media bytes", err)
		}
		clear(tail)
		if err := media.Close(); err != nil {
			t.Fatal(err)
		}
	}
	poster, err := facade.Poster(ctx, play.PosterURL)
	if err != nil || !bytes.Equal(poster, objects[prefix+"poster.png"]) {
		t.Fatal("private poster", err)
	}
	clear(poster)
	if _, err := facade.Media(ctx, httpSource.URL+"/media/plain.webm"); err == nil {
		t.Fatal("private facade accepted clear HTTP media")
	}
	if _, err := facade.Shared(ctx, ""); !errors.Is(err, api.ErrPrivateCastOperation) {
		t.Fatal("private unsupported method reached HTTP", err)
	}
	if source.opensCount() != reads {
		t.Fatal("fresh playback objects reimported clear source")
	}
	audit, err := recovered.AuditSource(ctx)
	if err != nil || audit.Blobs != int64(len(objects)) || audit.Private.Objects < int64(len(objects)) {
		t.Fatal("source and private binary audit", audit, err)
	}
	// Keep the source and provider intact for subsequent checks; the audit must
	// fail if either side changes, even though the encrypted receipt still exists.
	sourceKey := model.BinaryKind + "/" + mediaIDs[prefix+"original.mp4"]
	source.mu.Lock()
	original := source.rows[sourceKey]
	source.rows[sourceKey] = []byte("changed source")
	source.mu.Unlock()
	_, sourceErr := recovered.AuditSource(ctx)
	source.mu.Lock()
	source.rows[sourceKey] = original
	source.mu.Unlock()
	if sourceErr == nil {
		t.Fatal("source audit accepted changed retained bytes")
	}
	opaque.mu.Lock()
	opaque.tamper = true
	opaque.mu.Unlock()
	_, providerErr := recovered.AuditSource(ctx)
	opaque.mu.Lock()
	opaque.tamper = false
	opaque.mu.Unlock()
	if providerErr == nil {
		t.Fatal("source audit accepted corrupted private ciphertext")
	}
	snapshot, err := storage.LoadCatalog(ctx, fresh, scope)
	if err != nil {
		t.Fatal(err)
	}
	for ref := range snapshot.Entries {
		stored, err := device.Matrix.GetEvent(ctx, id.RoomID(ref.RoomID), id.EventID(ref.EventID))
		if err != nil || stored.Type != event.EventEncrypted {
			t.Fatal("Cast index stored in clear", err)
		}
	}
	for _, body := range opaque.data {
		if bytes.Contains(body, []byte("cast-private-title-sentinel")) || bytes.Contains(body, []byte("#EXTM3U")) {
			t.Fatal("opaque gateway received private content")
		}
	}
	t.Log("real SDK/Synapse identity cutover: " + strconv.Itoa(len(objects)) + " exact MP4/HLS/WebM/poster objects, full bounded playlist topology validation before receipt, private metadata mutation, typed API playback and seeking, fresh recovery without source reimport; legacy audiences/ACLs, publication, live and UI remain unactivated")
	if !strings.HasSuffix(values[0].HLSKey, "hls/master.m3u8") {
		t.Fatal("retained HLS root relationship lost")
	}
}
