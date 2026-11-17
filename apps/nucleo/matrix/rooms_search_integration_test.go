//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	roomsearch "git.sytue.eu/sytue/one/apps/nucleo/rooms/search"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

type searchMetadataFixture struct {
	mu                      sync.Mutex
	rows                    []roomsearch.Metadata
	subject, tenant         string
	revoked                 bool
	visibleCalls, scanCalls int
	revokeOnVisible         int
	onScan                  func()
}

func (f *searchMetadataFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Query().Has("q") {
		http.Error(w, "search text reached metadata", 400)
		return
	}
	switch r.URL.Path {
	case "/api/v1/messages/metadata":
		f.scanCalls++
		if f.onScan != nil {
			fn := f.onScan
			f.onScan = nil
			fn()
		}
		before, err := roomsearch.DecodeCursor(r.URL.Query().Get("cursor"), f.tenant, f.subject)
		if err != nil {
			http.Error(w, "bad cursor", 400)
			return
		}
		page := roomsearch.Page{Messages: []roomsearch.Metadata{}}
		if !f.revoked {
			for _, row := range f.rows {
				if before != nil && roomsearch.Compare(row, roomsearch.Metadata{ID: before.ID, Tenant: before.Tenant, CreatedAt: before.CreatedAt}) <= 0 {
					continue
				}
				page.Messages = append(page.Messages, row)
				if len(page.Messages) == 101 {
					page.Messages = page.Messages[:100]
					page.Next = roomsearch.EncodeCursor(page.Messages[99], f.tenant, f.subject)
					break
				}
			}
		}
		_ = json.NewEncoder(w).Encode(page)
	case "/api/v1/messages/metadata/visible":
		f.visibleCalls++
		if f.revokeOnVisible > 0 && f.visibleCalls == f.revokeOnVisible {
			f.revoked = true
		}
		var input struct {
			Messages []roomsearch.Identity `json:"messages"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			http.Error(w, "query or invalid body reached metadata", 400)
			return
		}
		selected := map[roomsearch.Identity]bool{}
		for _, ref := range input.Messages {
			selected[ref] = true
		}
		rows := []roomsearch.Metadata{}
		if !f.revoked {
			for _, row := range f.rows {
				if selected[row.Identity()] {
					rows = append(rows, row)
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"messages": rows})
	default:
		http.Error(w, "unexpected metadata endpoint", 400)
	}
}

func searchSDKDevice(ctx context.Context, t *testing.T, base string, httpClient *http.Client, actor, tenant, channel string) (*Client, string, string, func(string) (*mautrix.RespLogin, error)) {
	return publicationSDKDeviceFixture(ctx, t, base, httpClient, actor, tenant, channel, "")
}

func publicationSDKDeviceFixture(ctx context.Context, t *testing.T, base string, httpClient *http.Client, actor, tenant, channel, product string) (*Client, string, string, func(string) (*mautrix.RespLogin, error)) {
	t.Helper()
	raw, err := mautrix.NewClient(base, "", "")
	if err != nil {
		t.Fatal(err)
	}
	raw.Client = httpClient
	password := uuid.NewString()
	username := "sytue_" + actor
	if product != "" {
		if !storage.Product(product) {
			t.Fatal("invalid fixture product")
		}
		username = strings.TrimSuffix(strings.TrimPrefix(storage.IsolatedMatrixUserPrefix(storage.Scope{Subject: actor, Tenant: tenant, Product: product}), "@"), ":")
	}
	registered := matrixprooftest.Register(ctx, t, raw, username, password, "SEARCH_TEST")
	room := newPrivateStorageTestRoom(ctx, t, raw)
	_, host, _ := strings.Cut(registered.UserID.String(), ":")
	cfg := Config{Bootstrap: Bootstrap{Actor: actor, Tenant: tenant, HomeserverURL: base, MatrixUserID: registered.UserID.String(), ControllerUserID: "@rooms_controller:" + host, Encryption: string(id.AlgorithmMegolmV1), Channels: []RoomBinding{{ChannelID: channel, RoomID: room.String()}}}, Subject: actor, Tenant: tenant, StoreDir: t.TempDir(), Secrets: &memorySecrets{values: map[string][]byte{}}, HTTPClient: httpClient}
	if product != "" {
		cfg.Bootstrap.Channels = nil
		cfg.Bootstrap.StorageProduct = product
		cfg.Bootstrap.StorageIdentityVersion = storage.IsolatedIdentity
		cfg.Bootstrap.StorageRoomID = room.String()
		cfg.Bootstrap.StorageRoomIDs = []string{room.String()}
	}
	c := storageSynapseSession(ctx, t, cfg, credentials{Homeserver: base, UserID: registered.UserID, DeviceID: registered.DeviceID, AccessToken: registered.AccessToken, Subject: actor, Tenant: tenant})
	recovery, err := c.BootstrapRecovery(ctx, storageFixtureUIA(registered.UserID, password))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := c.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	return c, room.String(), recovery, func(device string) (*mautrix.RespLogin, error) {
		return raw.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: registered.UserID.String()}, Password: password, DeviceID: id.DeviceID(device)})
	}
}

func TestPrivateSearchAgainstSynapseScansHistoryOperatorsAndRevokes(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_ROOMS_SEARCH") != "1" {
		t.Skip("isolated remote Synapse proof")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	upstream, err := url.Parse(os.Getenv("SYTUE_MATRIX_ROOMS_SEARCH_URL"))
	if err != nil || upstream.Host == "" {
		t.Fatal("isolated Synapse URL required")
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
	t.Cleanup(proxy.Close)
	actor, channel := uuid.NewString(), uuid.NewString()
	const tenant = "search-tenant"
	c, room, _, _ := searchSDKDevice(ctx, t, proxy.URL, proxy.Client(), actor, tenant, channel)
	fixture, target, fileMessage := searchCorpus(ctx, t, c, room, actor, tenant, channel)
	server := httptest.NewServer(fixture)
	defer server.Close()
	service := NewRoomsService(api.NewRooms(server.URL, nil, server.Client()), func(context.Context) (*Client, error) { return c, nil })
	for _, tc := range []struct{ query, want string }{{"REUNION viernes from:ana in:#general on:2024-03-05 -borrador", target}, {"is:saved", target}, {"is:pinned has:file has:link", fileMessage}} {
		got, err := service.Search(ctx, tc.query, 1)
		if err != nil || len(got) != 1 || got[0].ID != tc.want {
			t.Fatal("private query did not search authenticated complete history", tc.query, len(got), err)
		}
	}
	fixture.mu.Lock()
	scans := fixture.scanCalls
	fixture.mu.Unlock()
	if scans < 3 {
		t.Fatal("deep reply did not require multiple metadata pages")
	}
	// Revoke only at the FINAL authorization read, after ciphertext has already
	// decrypted locally; the late body must never enter results or view caches.
	service.Forget()
	fixture.mu.Lock()
	fixture.visibleCalls = 0
	fixture.revokeOnVisible = 2
	fixture.mu.Unlock()
	got, err := service.Search(ctx, "REUNION viernes", 1)
	if err != nil || len(got) != 0 {
		t.Fatal("revocation after decryption exposed a search result", len(got), err)
	}
	service.mu.Lock()
	cached := len(service.messages)
	service.mu.Unlock()
	if cached != 0 {
		t.Fatal("revoked result populated decrypted view cache")
	}
	fixture.mu.Lock()
	fixture.revoked = false
	fixture.revokeOnVisible = 0
	fixture.onScan = func() { service.ForgetChannel(channel) }
	fixture.mu.Unlock()
	if got, err := service.Search(ctx, "REUNION", 1); !errors.Is(err, ErrIdentity) || len(got) != 0 {
		t.Fatal("in-flight scan survived account/channel retirement", len(got), err)
	}
	t.Log("real SDK: deep encrypted reply, shared operators and encrypted file/link flags; final revocation and epoch retirement expose zero bytes and retain no search index")
}

func searchCorpus(ctx context.Context, t *testing.T, c *Client, room, actor, tenant, channel string) (*searchMetadataFixture, string, string) {
	t.Helper()
	at := time.Date(2024, 3, 5, 12, 0, 0, 0, time.UTC)
	fixture := &searchMetadataFixture{subject: actor, tenant: tenant}
	file, err := c.UploadAttachment(ctx, room, Attachment{Reader: bytes.NewBufferString("private"), Name: "private.txt", MIME: "text/plain", Size: 7})
	if err != nil {
		t.Fatal(err)
	}
	var root, target, fileMessage string
	var rootReference EventRef
	for i := 0; i < 207; i++ {
		body := fmt.Sprintf("neutral history %d", i)
		content := MessageContent{Version: 1, Body: body}
		if i == 1 {
			content.Body = "el viernes hay reunión de equipo"
		}
		if i == 5 {
			content.Body = "adjunto https://sytue.eu"
			content.Attachments = []MessageAttachment{{FileID: string(file.URL), Name: "private.txt", ContentType: "text/plain", SizeBytes: 7, MatrixFile: file}}
		}
		relation := Relation{}
		if i == 1 {
			relation.Thread = &rootReference
		}
		ref, err := c.SendContent(ctx, room, content, relation)
		if err != nil {
			t.Fatal("publish encrypted search corpus", i, err)
		}
		row := roomsearch.Metadata{ID: uuid.NewString(), Tenant: tenant, ChannelID: channel, AuthorSub: actor, AuthorName: "Ána", CreatedAt: at.Add(time.Duration(i) * time.Millisecond), MatrixEvent: &ref, ChannelSlug: "general", ChannelName: "General", Saved: i == 1, Pinned: i == 5}
		if i == 0 {
			root = row.ID
			rootReference = ref
		}
		if i == 1 {
			row.ParentID = &root
			target = row.ID
		}
		if i == 5 {
			fileMessage = row.ID
		}
		fixture.rows = append(fixture.rows, row)
	}
	slices.SortFunc(fixture.rows, roomsearch.Compare)

	return fixture, target, fileMessage
}
