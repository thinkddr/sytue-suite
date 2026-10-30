// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// This uses real maintained Go Olm and SQLite, with a transport double that records
// every server-facing body. It exercises actual encryption, restart and decryption.
func TestEncryptedSendAndAttachmentSurviveRestart(t *testing.T) {
	var mu sync.Mutex
	var deviceKeys json.RawMessage
	var sent []byte
	var sendPaths []string
	var uploaded []byte
	var contentName string
	var beforeUploadReceipt func()
	var encryptionReads, eventReads int
	var readDelay time.Duration
	var unsupportedEncryption bool
	var cancelRead context.CancelFunc
	readEvents := map[string]json.RawMessage{}
	accountData := map[string]json.RawMessage{}
	var backupVersion json.RawMessage
	var keyBackup json.RawMessage
	var crossSigning map[string]json.RawMessage
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/account/whoami"):
			_, _ = io.WriteString(w, `{"user_id":"@sytue_person:matrix.test","device_id":"DEVICE"}`)
		case strings.Contains(r.URL.Path, "/account_data/"):
			key := strings.SplitN(r.URL.Path, "/account_data/", 2)[1]
			if r.Method == http.MethodPut {
				accountData[key], _ = io.ReadAll(r.Body)
				_, _ = io.WriteString(w, `{}`)
			} else if data, ok := accountData[key]; ok {
				_, _ = w.Write(data)
			} else {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errcode":"M_NOT_FOUND"}`)
			}
		case strings.HasSuffix(r.URL.Path, "/keys/device_signing/upload"):
			_ = json.NewDecoder(r.Body).Decode(&crossSigning)
			_, _ = io.WriteString(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/keys/signatures/upload"):
			_, _ = io.WriteString(w, `{"failures":{}}`)
		case strings.HasSuffix(r.URL.Path, "/room_keys/version"):
			switch {
			case r.Method == http.MethodPost:
				backupVersion, _ = io.ReadAll(r.Body)
				_, _ = io.WriteString(w, `{"version":"1"}`)
			case len(backupVersion) > 0:
				var version map[string]any
				_ = json.Unmarshal(backupVersion, &version)
				version["version"] = "1"
				version["count"] = 1
				_ = json.NewEncoder(w).Encode(version)
			default:
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errcode":"M_NOT_FOUND"}`)
			}
		case strings.HasSuffix(r.URL.Path, "/room_keys/keys"):
			if r.Method == http.MethodPut {
				keyBackup, _ = io.ReadAll(r.Body)
				_, _ = io.WriteString(w, `{"count":1,"etag":"one"}`)
			} else {
				_, _ = w.Write(keyBackup)
			}
		case strings.HasSuffix(r.URL.Path, "/keys/query"):
			if len(deviceKeys) == 0 {
				_, _ = io.WriteString(w, `{"device_keys":{}}`)
			} else {
				response := map[string]any{"device_keys": map[string]any{"@sytue_person:matrix.test": map[string]any{"DEVICE": deviceKeys}}}
				for name, keys := range crossSigning {
					response[name+"s"] = map[string]any{"@sytue_person:matrix.test": keys}
				}
				_ = json.NewEncoder(w).Encode(response)
			}
		case strings.HasSuffix(r.URL.Path, "/keys/upload"):
			var body struct {
				DeviceKeys json.RawMessage `json:"device_keys"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.DeviceKeys) > 0 {
				deviceKeys = body.DeviceKeys
			}
			_, _ = io.WriteString(w, `{"one_time_key_counts":{"signed_curve25519":50}}`)
		case strings.Contains(r.URL.Path, "/state/m.room.encryption"):
			encryptionReads++
			time.Sleep(readDelay)
			if unsupportedEncryption {
				_, _ = io.WriteString(w, `{"algorithm":"unsupported"}`)
			} else {
				_, _ = io.WriteString(w, `{"algorithm":"m.megolm.v1.aes-sha2"}`)
			}
		case strings.Contains(r.URL.Path, "/event/"):
			eventReads++
			time.Sleep(readDelay)
			if data, ok := readEvents[r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]]; ok {
				_, _ = w.Write(data)
			} else {
				http.Error(w, `{"errcode":"M_NOT_FOUND"}`, http.StatusNotFound)
			}
			if cancelRead != nil {
				cancelRead()
			}
		case strings.Contains(r.URL.Path, "/state/m.room.history_visibility"):
			_, _ = io.WriteString(w, `{"history_visibility":"joined"}`)
		case strings.Contains(r.URL.Path, "/sendToDevice/m.room.encrypted/"):
			_, _ = io.WriteString(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_ = json.NewEncoder(w).Encode(map[string]any{"chunk": []any{map[string]any{"event_id": "$draft", "room_id": "!room:matrix.test", "sender": "@sytue_person:matrix.test", "type": "m.room.encrypted", "content": json.RawMessage(sent)}}})
		case strings.Contains(r.URL.Path, "/send/m.room.encrypted/"):
			sendPaths = append(sendPaths, r.URL.Path)
			sent, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"event_id":"$event"}`)
		case strings.Contains(r.URL.Path, "/media/v3/upload"):
			uploaded, _ = io.ReadAll(r.Body)
			contentName = r.URL.Query().Get("filename")
			if beforeUploadReceipt != nil {
				beforeUploadReceipt()
			}
			_, _ = io.WriteString(w, `{"content_uri":"mxc://matrix.test/uploaded"}`)
		default:
			t.Errorf("unexpected Matrix request %s %s", r.Method, r.URL.Path)
			http.Error(w, `{"errcode":"M_NOT_FOUND"}`, http.StatusNotFound)
		}
	}))
	defer server.Close()
	cfg := configFor(t, server)
	cfg.Bootstrap.MatrixUserID = "@sytue_person:matrix.test"
	c, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.loadMaster(true); err != nil {
		t.Fatal(err)
	}
	c.session = credentials{Homeserver: cfg.Bootstrap.HomeserverURL, UserID: id.UserID(cfg.Bootstrap.MatrixUserID), DeviceID: "DEVICE", AccessToken: "test-session", Subject: cfg.Subject, Tenant: cfg.Tenant}
	saved, _ := json.Marshal(c.session)
	if err = cfg.Secrets.Write(c.sessionKey(), saved); err != nil {
		t.Fatal(err)
	}
	if err = c.initialize(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	c.cli.StateStoreSyncHandler(context.Background(), &event.Event{RoomID: "!room:matrix.test", Type: event.StateEncryption, StateKey: new(""), Content: event.Content{Parsed: &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}}})
	c.ready.Store(true)
	if _, err = c.SendText(context.Background(), "!room:matrix.test", "must be blocked"); !errors.Is(err, ErrUnverified) {
		t.Fatalf("unverified device was allowed: %v", err)
	}
	recovery, err := c.BootstrapRecovery(context.Background(), nil)
	if err != nil || recovery == "" {
		t.Fatalf("bootstrap recovery: %v", err)
	}
	if _, err = c.BootstrapRecovery(context.Background(), nil); !errors.Is(err, ErrRecoveryExists) {
		t.Fatalf("existing identity was replaced: %v", err)
	}
	secret := "Artificial Go → JS encrypted message · Áé🙂"
	ref, err := c.SendText(context.Background(), "!room:matrix.test", secret)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	ciphertext := bytes.Clone(sent)
	mu.Unlock()
	if ref.EventID != "$event" || bytes.Contains(ciphertext, []byte(secret)) {
		t.Fatal("server received plaintext or no encrypted event")
	}
	// The application record bound must include the maintained codec's expansion,
	// not merely the size of plaintext JSON. Reserve 8 KiB for the server PDU.
	recordScope := storage.Scope{Tenant: strings.Repeat("t", 63), Subject: strings.Repeat("s", 255), Product: "drive"}
	record := storage.Record{Version: 1, Scope: recordScope, ObjectID: strings.Repeat("o", 128), RevisionID: strings.Repeat("r", 128), Role: "content"}
	low, high := 0, storage.MaxRecordBytes
	for low < high {
		length := low + (high-low+1)/2
		record.Payload = []byte(`"` + strings.Repeat("x", length) + `"`)
		if record.Validate(recordScope) == nil {
			low = length
		} else {
			high = length - 1
		}
	}
	record.Payload = []byte(`"` + strings.Repeat("x", low) + `"`)
	maxContent := map[string]any{"msgtype": "m.text", "body": "", "org.sytue.storage": record}
	maxCipher, err := c.crypto.Encrypt(t.Context(), "!room:matrix.test", event.EventMessage, maxContent)
	if err != nil {
		t.Fatal(err)
	}
	maxWire, err := json.Marshal(maxCipher)
	if err != nil || len(maxWire) > 56<<10 || bytes.Contains(maxWire, []byte(record.ObjectID)) {
		t.Fatal("maximum private record exceeds the Matrix envelope budget", len(maxWire), err)
	}
	t.Logf("maximum storage record: encrypted event content=%d bytes", len(maxWire))
	// A lost receipt must reuse the same homeserver transaction while retaining
	// the encrypted wire contract; the SDK generates a fresh transaction otherwise.
	const meetingURL, meetingTxn = "https://meet.google.com/abc-defg-hij", "meeting-stable-transaction"
	for range 2 {
		if _, err = c.SendContentWithTxn(context.Background(), "!room:matrix.test", MessageContent{Body: meetingURL}, Relation{}, meetingTxn); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	paths, meetingCiphertext := append([]string(nil), sendPaths...), bytes.Clone(sent)
	mu.Unlock()
	if len(paths) != 3 || !strings.HasSuffix(paths[1], "/"+meetingTxn) || paths[1] != paths[2] || bytes.Contains(meetingCiphertext, []byte(meetingURL)) {
		t.Fatalf("retry lost its encrypted transaction: %v", paths)
	}
	// A real page of independent Megolm messages, not repeated decryption of a
	// single receipt. The controlled wire delay isolates redundant round trips.
	references := make([]EventRef, 50)
	for i := range references {
		reference := EventRef{RoomID: "!room:matrix.test", EventID: fmt.Sprintf("$page-%d", i)}
		payload, encryptErr := c.crypto.Encrypt(t.Context(), id.RoomID(reference.RoomID), event.EventMessage, event.MessageEventContent{MsgType: event.MsgText, Body: fmt.Sprintf("page message %d", i)})
		if encryptErr != nil {
			t.Fatal(encryptErr)
		}
		wire, marshalErr := json.Marshal(map[string]any{"event_id": reference.EventID, "room_id": reference.RoomID, "sender": "@sytue_person:matrix.test", "origin_server_ts": i + 1, "type": "m.room.encrypted", "content": payload})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		mu.Lock()
		readEvents[reference.EventID] = wire
		mu.Unlock()
		references[i] = reference
	}
	mu.Lock()
	encryptionReads, eventReads, readDelay = 0, 0, 5*time.Millisecond
	mu.Unlock()
	readStart := time.Now()
	for i, reference := range references {
		value, readErr := c.Read(t.Context(), reference)
		if readErr != nil {
			t.Fatal(readErr)
		}
		content, decodeErr := DecodeMessage(value)
		if decodeErr != nil || content.Body != fmt.Sprintf("page message %d", i) {
			t.Fatalf("page receipt %d: %v", i, decodeErr)
		}
	}
	mu.Lock()
	t.Logf("50 encrypted receipts: encryption state=%d event=%d elapsed=%s (5ms/request)", encryptionReads, eventReads, time.Since(readStart))
	if encryptionReads != 50 || eventReads != 50 {
		t.Fatalf("single receipt reference path changed: state=%d event=%d", encryptionReads, eventReads)
	}
	encryptionReads, eventReads = 0, 0
	mu.Unlock()
	readStart = time.Now()
	results, readErr := c.ReadMany(t.Context(), "!room:matrix.test", references)
	if readErr != nil || len(results) != len(references) {
		t.Fatalf("encrypted receipt page: %v", readErr)
	}
	for i, result := range results {
		content, decodeErr := DecodeMessage(result.Event)
		if result.Err != nil || decodeErr != nil || content.Body != fmt.Sprintf("page message %d", i) || result.Event.Trust < id.TrustStateCrossSignedVerified {
			t.Fatalf("page receipt %d lost verified content: read=%v decode=%v", i, result.Err, decodeErr)
		}
	}
	mu.Lock()
	t.Logf("50 batched encrypted receipts: encryption state=%d event=%d elapsed=%s (5ms/request)", encryptionReads, eventReads, time.Since(readStart))
	if encryptionReads != 1 || eventReads != 50 {
		t.Fatalf("page repeated state or skipped authenticating receipts: state=%d event=%d", encryptionReads, eventReads)
	}
	encryptionReads, eventReads = 0, 0
	mu.Unlock()
	service := NewRoomsService(nil, func(context.Context) (*Client, error) { return c, nil })
	metadata := make([]api.RoomsMessage, len(references))
	for i := range metadata {
		metadata[i] = api.RoomsMessage{ID: fmt.Sprintf("message-%d", i), ChannelID: "channel", AuthorSub: "person", MatrixEvent: &references[i]}
	}
	hydrated, hydrateErr := service.hydrateMany(t.Context(), metadata, service.revision())
	if hydrateErr != nil || len(hydrated) != len(references) {
		t.Fatalf("product page hydration: %v", hydrateErr)
	}
	for i, message := range hydrated {
		if message.Body != fmt.Sprintf("page message %d", i) || message.EncryptionError || !message.IdentityVerified || !message.AuthenticityVerified {
			t.Fatalf("product receipt %d did not retain its authenticated content", i)
		}
	}
	mu.Lock()
	if encryptionReads != 1 || eventReads != 50 {
		t.Fatalf("product adapter did not use the bounded page: state=%d event=%d", encryptionReads, eventReads)
	}
	readDelay = 0
	readEvents["$plaintext"] = json.RawMessage(`{"event_id":"$plaintext","room_id":"!room:matrix.test","sender":"@sytue_person:matrix.test","type":"m.room.message","content":{"body":"never display this"}}`)
	readEvents["$wrong-room"] = json.RawMessage(`{"event_id":"$wrong-room","room_id":"!other:matrix.test","type":"m.room.encrypted","content":{}}`)
	readEvents["$wrong-receipt"] = json.RawMessage(`{"event_id":"$different","room_id":"!room:matrix.test","type":"m.room.encrypted","content":{}}`)
	mu.Unlock()
	invalidReferences := []EventRef{references[0], {RoomID: "!foreign:matrix.test", EventID: "$page-1"}, {RoomID: "!room:matrix.test", EventID: ""}, {RoomID: "!room:matrix.test", EventID: "$plaintext"}, {RoomID: "!room:matrix.test", EventID: "$wrong-room"}, {RoomID: "!room:matrix.test", EventID: "$wrong-receipt"}, references[1]}
	results, readErr = c.ReadMany(t.Context(), "!room:matrix.test", invalidReferences)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for i, result := range results {
		if i == 0 || i == len(results)-1 {
			if result.Err != nil {
				t.Fatalf("one bad receipt prevented a valid neighbor: %v", result.Err)
			}
		} else if !errors.Is(result.Err, ErrIdentity) || len(result.Event.Content) != 0 {
			t.Fatalf("unbound, plaintext or substituted receipt %d returned content: %v", i, result.Err)
		}
	}
	badMetadata := append([]api.RoomsMessage(nil), metadata...)
	badMetadata[5].AuthorSub = "another-person"
	badMetadata[6].MatrixEvent = &EventRef{RoomID: "!foreign:matrix.test", EventID: "$page-6"}
	hydrated, hydrateErr = service.hydrateMany(t.Context(), badMetadata, service.revision())
	if hydrateErr != nil {
		t.Fatalf("one invalid metadata receipt discarded the whole page: %v", hydrateErr)
	}
	for i, message := range hydrated {
		if i == 5 || i == 6 {
			if !message.EncryptionError || message.Body != "" || message.IdentityVerified || message.AuthenticityVerified {
				t.Fatalf("forged author or room displayed receipt %d", i)
			}
		} else if message.EncryptionError || message.Body != fmt.Sprintf("page message %d", i) {
			t.Fatalf("invalid neighbor hid valid receipt %d", i)
		}
	}
	mu.Lock()
	unsupportedEncryption, encryptionReads, eventReads = true, 0, 0
	mu.Unlock()
	if values, err := c.ReadMany(t.Context(), "!room:matrix.test", references); !errors.Is(err, ErrNotEncrypted) || len(values) != 0 {
		t.Fatalf("page reused a stale encryption decision: %v", err)
	}
	mu.Lock()
	if encryptionReads != 1 || eventReads != 0 {
		t.Fatalf("unsupported encryption fetched message content: state=%d event=%d", encryptionReads, eventReads)
	}
	unsupportedEncryption = false
	readCtx, stopRead := context.WithCancel(t.Context())
	defer stopRead()
	cancelRead, encryptionReads, eventReads = stopRead, 0, 0
	mu.Unlock()
	results, readErr = c.ReadMany(readCtx, "!room:matrix.test", references)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, result := range results {
		if !errors.Is(result.Err, context.Canceled) || len(result.Event.Content) != 0 {
			t.Fatalf("canceled page returned content: %v", result.Err)
		}
	}
	mu.Lock()
	if eventReads != 1 {
		t.Fatalf("cancellation fetched the rest of the page: %d", eventReads)
	}
	cancelRead = nil
	mu.Unlock()
	if err = c.Backup(context.Background()); err != nil {
		t.Fatalf("backup: %v", err)
	}
	mu.Lock()
	backupBody := bytes.Clone(keyBackup)
	mu.Unlock()
	if len(backupBody) == 0 || bytes.Contains(backupBody, []byte(secret)) || bytes.Contains(backupBody, []byte(`"session_key"`)) {
		t.Fatal("backup exposed unencrypted session material")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err = Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var encryptedContent event.EncryptedEventContent
	if err = json.Unmarshal(ciphertext, &encryptedContent); err != nil {
		t.Fatal(err)
	}
	// Remove the inbound key before recovery: successful decryption must come
	// from the encrypted server backup, not from the original local session.
	if err = c.crypto.Machine().CryptoStore.RedactGroupSession(context.Background(), "!room:matrix.test", encryptedContent.SessionID, "test recovery"); err != nil {
		t.Fatal(err)
	}
	if err = c.Recover(context.Background(), recovery); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	if err = c.Recover(context.Background(), "invalid recovery key"); err == nil {
		t.Fatal("invalid recovery key accepted")
	}
	encrypted := &event.Event{ID: "$event", RoomID: "!room:matrix.test", Sender: "@sytue_person:matrix.test", Type: event.EventEncrypted, Content: event.Content{VeryRaw: ciphertext}}
	if err = encrypted.Content.ParseRaw(encrypted.Type); err != nil {
		t.Fatal(err)
	}
	clearEvent, err := c.crypto.Decrypt(context.Background(), encrypted)
	if err != nil || clearEvent.Content.AsMessage().Body != secret {
		t.Fatalf("restart decryption failed: %v", err)
	}
	c.ready.Store(true)
	fileText := "Artificial Go → JS encrypted attachment · Áé🙂"
	_, err = c.SendAttachment(context.Background(), "!room:matrix.test", Attachment{Reader: strings.NewReader(fileText), Name: "private-name.txt", MIME: "text/plain", Size: int64(len(fileText))})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	media, eventBody, name := bytes.Clone(uploaded), bytes.Clone(sent), contentName
	mu.Unlock()
	if name != "" || bytes.Equal(media, []byte(fileText)) || bytes.Contains(eventBody, []byte("private-name")) || bytes.Contains(eventBody, []byte(fileText)) {
		t.Fatal("attachment content, name or descriptor escaped encryption")
	}
	encrypted.ID = "$attachment"
	encrypted.Content = event.Content{VeryRaw: eventBody}
	if err = encrypted.Content.ParseRaw(encrypted.Type); err != nil {
		t.Fatal(err)
	}
	clearEvent, err = c.crypto.Decrypt(context.Background(), encrypted)
	if err != nil {
		t.Fatal(err)
	}
	file := clearEvent.Content.AsMessage().File
	if file == nil {
		t.Fatal("encrypted descriptor is missing from decrypted message")
	}
	emitInteropVector(t, c, ciphertext, secret, *file, media, fileText)
	if err = file.DecryptInPlace(media); err != nil || string(media) != fileText {
		t.Fatalf("attachment did not decrypt: %v", err)
	}

	// The product envelope is shared with web, including several attachments. Its
	// descriptor and draft body must remain absent from every transport body.
	if _, err = c.SendContent(t.Context(), "!room:matrix.test", MessageContent{Version: 1, Body: "private envelope", Attachments: []MessageAttachment{{FileID: string(file.URL), Name: "private-name.txt", ContentType: "text/plain", SizeBytes: int64(len(fileText)), MatrixFile: *file}}}, Relation{}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	envelopeWire := bytes.Clone(sent)
	mu.Unlock()
	if bytes.Contains(envelopeWire, []byte("private envelope")) || bytes.Contains(envelopeWire, []byte("matrixFile")) {
		t.Fatal("product envelope escaped encryption")
	}
	encrypted.ID = "$envelope"
	encrypted.Content = event.Content{VeryRaw: envelopeWire}
	_ = encrypted.Content.ParseRaw(encrypted.Type)
	envelope, err := c.crypto.Decrypt(t.Context(), encrypted)
	if err != nil {
		t.Fatal(err)
	}
	value, err := c.decryptedEvent(t.Context(), envelope)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMessage(value)
	if err != nil || decoded.Body != "private envelope" || len(decoded.Attachments) != 1 || decoded.Attachments[0].Name != "private-name.txt" {
		t.Fatalf("product envelope mismatch: %v", err)
	}
	c.cfg.Bootstrap.DraftRoomID = "!room:matrix.test"
	product := NewRoomsService(nil, func(context.Context) (*Client, error) { return c, nil })
	// A real encrypted upload can finish at the homeserver after the UI logs out.
	// Its late receipt must never repopulate the next account's descriptor cache.
	uploading, finishUpload := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-finishUpload:
		default:
			close(finishUpload)
		}
	}()
	mu.Lock()
	beforeUploadReceipt = func() { close(uploading); <-finishUpload }
	mu.Unlock()
	uploadResult := make(chan error, 1)
	go func() {
		_, uploadErr := product.UploadToChannel(t.Context(), "channel", "late-private.txt", "text/plain", strings.NewReader(fileText), int64(len(fileText)))
		uploadResult <- uploadErr
	}()
	select {
	case <-uploading:
	case <-time.After(10 * time.Second):
		t.Fatal("encrypted media upload did not reach the homeserver")
	}
	product.Forget()
	close(finishUpload)
	if err := <-uploadResult; !errors.Is(err, ErrIdentity) {
		t.Fatalf("late encrypted upload survived logout: %v", err)
	}
	product.mu.Lock()
	remaining := len(product.files)
	product.mu.Unlock()
	mu.Lock()
	beforeUploadReceipt = nil
	lateMedia, lateName := bytes.Clone(uploaded), contentName
	mu.Unlock()
	if remaining != 0 || bytes.Equal(lateMedia, []byte(fileText)) || lateName != "" {
		t.Fatal("late upload restored a key or exposed plaintext/name")
	}
	if err = product.PutDraft(t.Context(), "channel", "parent", "private draft"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	draftWire := bytes.Clone(sent)
	mu.Unlock()
	if bytes.Contains(draftWire, []byte("private draft")) || bytes.Contains(draftWire, []byte("org.sytue.rooms.draft")) {
		t.Fatal("draft escaped encryption")
	}
	drafts, err := product.Drafts(t.Context())
	if err != nil || len(drafts) != 1 || drafts[0].Body != "private draft" || drafts[0].ParentID == nil || *drafts[0].ParentID != "parent" {
		t.Fatalf("encrypted draft roundtrip: %#v %v", drafts, err)
	}
	callContent := CallKeyContent{Version: 1, CallID: "artificial-call", Generation: 0, Algorithm: "AES-GCM", ExpiresAt: time.Now().Add(time.Minute).UnixMilli(), KeyBase64: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))}
	if _, err = c.SendEncryptedControl(t.Context(), "!room:matrix.test", CallKeyEvent, callContent); !errors.Is(err, ErrUnsupported) {
		t.Fatal("room-wide media key transport was allowed")
	}
	clearControl := &event.Event{RoomID: "!room:matrix.test", Type: controlType(CallKeyEvent), Content: event.Content{Parsed: &callContent}}
	clearControl.Mautrix.WasEncrypted = true
	clearControl.Mautrix.TrustState = id.TrustStateBackup
	if _, err = c.decryptedEvent(t.Context(), clearControl); !errors.Is(err, ErrUnverified) {
		t.Fatal("historical backup could authorize a call key")
	}
	var beforeRotation event.EncryptedEventContent
	_ = json.Unmarshal(draftWire, &beforeRotation)
	c.crypto.Machine().HandleMemberEvent(t.Context(), &event.Event{RoomID: "!room:matrix.test", Type: event.StateMember, StateKey: new("@removed:matrix.test"), Unsigned: event.Unsigned{PrevContent: &event.Content{Parsed: &event.MemberEventContent{Membership: event.MembershipJoin}}}, Content: event.Content{Parsed: &event.MemberEventContent{Membership: event.MembershipLeave}}})
	if _, err = c.SendText(t.Context(), "!room:matrix.test", "after member left"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	afterWire := bytes.Clone(sent)
	mu.Unlock()
	var afterRotation event.EncryptedEventContent
	_ = json.Unmarshal(afterWire, &afterRotation)
	if beforeRotation.SessionID == afterRotation.SessionID {
		t.Fatal("membership departure reused the old encryption session")
	}
}
