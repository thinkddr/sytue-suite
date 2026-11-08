// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSenderLogoDevuelveElPNGYErrSinLogoEnUn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sender-logo" {
			t.Errorf("ruta %s", r.URL.Path)
		}
		switch r.URL.Query().Get("d") {
		case "corp.vodafone.es":
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, "\x89PNG")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()
	png, err := c.SenderLogo(ctx, "corp.vodafone.es")
	if err != nil || string(png) != "\x89PNG" {
		t.Fatalf("logo: %q %v", png, err)
	}
	if _, err := c.SenderLogo(ctx, "gmail.com"); !errors.Is(err, ErrSinLogo) {
		t.Fatalf("404: %v, quiero ErrSinLogo", err)
	}
}
