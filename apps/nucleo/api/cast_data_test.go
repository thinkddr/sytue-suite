// SPDX-License-Identifier: Apache-2.0
package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateCastFixture struct {
	scope  storage.Scope
	opened int
}

func (f *privateCastFixture) PrivateStorageScope() storage.Scope { return f.scope }
func (*privateCastFixture) Videos(context.Context, string, string, string, string, int) (CastPage, error) {
	return CastPage{Videos: []CastVideo{{ID: "video"}}}, nil
}

func (*privateCastFixture) Video(context.Context, string) (CastVideo, error) {
	return CastVideo{ID: "video"}, nil
}

func (*privateCastFixture) Play(context.Context, string) (CastPlay, error) {
	return CastPlay{WebMURL: "sytue-private-cast:fixture"}, nil
}

func (f *privateCastFixture) OpenMedia(context.Context, string) (CastPrivateMedia, error) {
	f.opened++
	return &privateCastMediaFixture{Reader: bytes.NewReader([]byte("private authenticated media"))}, nil
}

type privateCastMediaFixture struct{ *bytes.Reader }

func (*privateCastMediaFixture) Close() error { return nil }

func TestPrivateCastFacadeUsesDeviceMediaAndBlocksAllLegacyDoors(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	session := &privateBoundarySession{}
	original := NewCast(server.URL, session, server.Client())
	data := &privateCastFixture{scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "cast"}}
	private, err := original.WithPrivateData(data)
	if err != nil {
		t.Fatal(err)
	}
	var absent *Cast
	if absent.UsesPrivateData() || original.UsesPrivateData() || !private.UsesPrivateData() {
		t.Fatal("private facade marker does not follow its bound data")
	}
	if page, err := private.Videos(t.Context(), "mine", "", "", "", 10); err != nil || len(page.Videos) != 1 {
		t.Fatal(err)
	}
	if video, err := private.Video(t.Context(), "video"); err != nil || video.ID != "video" {
		t.Fatal(err)
	}
	play, err := private.Play(t.Context(), "video")
	if err != nil {
		t.Fatal(err)
	}
	media, err := private.Media(t.Context(), play.WebMURL)
	if err != nil {
		t.Fatal(err)
	}
	if media.Size() != int64(len("private authenticated media")) {
		t.Fatal("size")
	}
	if _, err := media.Seek(-5, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	tail, err := io.ReadAll(media)
	if err != nil || string(tail) != "media" {
		t.Fatal("seek failed", err)
	}
	_ = media.Close()
	if private.MediaURL(play.WebMURL) != "" || private.MediaURL(server.URL+"/media") != "" {
		t.Fatal("private URL escaped")
	}
	if _, err := private.Poster(t.Context(), play.WebMURL); err != nil {
		t.Fatal(err)
	}
	checks := []func() error{
		func() error { _, err := private.Explore(t.Context(), "", 10); return err },
		func() error { _, err := private.PublicVideo(t.Context(), "video"); return err },
		func() error { _, err := private.PublicPlay(t.Context(), "video"); return err },
		func() error { _, err := private.Shared(t.Context(), ""); return err },
		func() error { _, err := private.Create(t.Context(), CastNew{Title: "private title"}); return err },
		func() error {
			_, err := private.Upload(t.Context(), "video", bytes.NewBufferString("private original"), 16, "video/webm")
			return err
		},
		func() error { _, err := private.Update(t.Context(), "video", CastChanges{}); return err },
		func() error { return private.Delete(t.Context(), "video") },
	}
	for _, check := range checks {
		if err := check(); !errors.Is(err, ErrPrivateCastOperation) {
			t.Fatal("legacy door did not reject", err)
		}
	}
	if requests.Load() != 0 || session.calls.Load() != 0 || data.opened != 2 {
		t.Fatal("private facade used HTTP or session")
	}
	if _, err := private.WithPrivateData(data); !errors.Is(err, ErrPrivateCastOperation) {
		t.Fatal("rebind accepted", err)
	}
	data.scope.Product = "mail"
	if _, err := original.WithPrivateData(data); !errors.Is(err, ErrPrivateCastOperation) {
		t.Fatal("foreign product accepted", err)
	}
}
