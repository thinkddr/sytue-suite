// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func TestOwnAndSharedCalendarRequestsOverlap(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/calendars", "/api/v1/calendars/shared-with-me":
			started <- r.URL.Path
			<-release
			_, _ = io.WriteString(w, `{"calendars":[]}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	})
	a := appDePrueba(t, handler)
	done := make(chan struct{})
	go func() { a.Arrancar(); close(done) }()
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Error("own/shared inventories serialized independent service reads")
		}
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("calendar did not finish after both inventories were released")
	}
}
