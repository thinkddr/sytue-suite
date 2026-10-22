// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestAttachmentDescriptorCannotCrossRoomOrClient(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected attachment request", http.StatusForbidden)
	}))
	defer server.Close()
	cfg := configFor(t, server)
	cfg.Bootstrap.Channels = append(cfg.Bootstrap.Channels, RoomBinding{ChannelID: "other", RoomID: "!other:matrix.test"})
	first, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newClient(cfg) // Even the same subject's replacement device is a new scope.
	if err != nil {
		t.Fatal(err)
	}
	current := first
	r := NewRoomsService(nil, func(context.Context) (*Client, error) { return current, nil })
	file := MessageAttachment{FileID: "mxc://matrix.test/private", Name: "private.txt", ContentType: "text/plain", MatrixFile: event.EncryptedFileInfo{URL: id.ContentURIString("mxc://matrix.test/private")}}
	r.files[attachmentKey{first, "!room:matrix.test", file.FileID}] = cachedAttachment{file: file, channel: "channel", temporary: true}
	if content, err := r.content("", api.RoomsSend{FileIDs: []string{file.FileID}}, first, "!room:matrix.test", r.revision()); err != nil || len(content.Attachments) != 1 {
		t.Fatalf("original room lost its attachment: %#v %v", content, err)
	}
	for _, channel := range []string{"other", "channel"} {
		if channel == "channel" {
			current = second
		}
		if _, err := r.PostWith(t.Context(), channel, "", api.RoomsSend{FileIDs: []string{file.FileID}}); !errors.Is(err, ErrIdentity) {
			t.Fatalf("foreign room/client was allowed to send the descriptor: %v", err)
		}
		if _, err := r.ChannelContent(t.Context(), channel, file.FileID, false); !errors.Is(err, ErrIdentity) {
			t.Fatalf("foreign room/client was allowed to download with the key: %v", err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("scope rejection happened after network IO: %d", requests.Load())
	}

	// Editing must not restore the first device's decrypted attachment through
	// the cached message instead of the scoped attachment lookup.
	r.messages["message"] = cachedMessage{value: api.RoomsMessage{ID: "message", ChannelID: "channel", MatrixEvent: &EventRef{RoomID: "!room:matrix.test", EventID: "$message"}, Attachments: []api.RoomsAttachment{{FileID: file.FileID}}}, client: first, room: "!room:matrix.test"}
	if _, err := r.Edit(t.Context(), "message", "updated"); !errors.Is(err, ErrIdentity) {
		t.Fatalf("edit reused another device's descriptor: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("edit made a request before rejecting the old scope")
	}
}

func TestForgetInvalidatesInFlightDescriptorAndOnlyDraftCleanup(t *testing.T) {
	c := &Client{}
	r := NewRoomsService(nil, func(context.Context) (*Client, error) { return c, nil })
	temporary := attachmentKey{c, "!room", "temporary"}
	received := attachmentKey{c, "!room", "received"}
	other := attachmentKey{c, "!other", "other"}
	r.files[temporary] = cachedAttachment{channel: "channel", temporary: true}
	r.files[received] = cachedAttachment{channel: "channel"}
	r.files[other] = cachedAttachment{channel: "other", temporary: true}
	r.ForgetUploads("channel")
	if _, ok := r.files[temporary]; ok {
		t.Fatal("abandoned upload kept its process-local key")
	}
	if len(r.files) != 2 {
		t.Fatal("draft cleanup removed a received or other-room attachment")
	}
	r.ForgetUpload("other", "other")
	if len(r.files) != 1 {
		t.Fatal("individual draft cleanup removed a received attachment")
	}
	epoch := r.revision()
	r.Forget()
	if _, err := r.content("", api.RoomsSend{}, c, "!room", epoch); !errors.Is(err, ErrIdentity) {
		t.Fatalf("in-flight operation survived identity reset: %v", err)
	}
	if len(r.files) != 0 || len(r.messages) != 0 {
		t.Fatal("identity reset retained decrypted state")
	}
}
