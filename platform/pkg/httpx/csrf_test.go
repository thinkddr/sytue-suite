// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func csrfHandler() http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return CSRF(CSRFConfig{})(next)
}

func TestCSRFSeedsOnSafeMethod(t *testing.T) {
	rec := httptest.NewRecorder()
	csrfHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status=%d", rec.Code)
	}
	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == DefaultCSRFCookieName {
			found = true
			if c.HttpOnly {
				t.Error("la cookie CSRF no debe ser HttpOnly (double-submit)")
			}
			if !c.Secure {
				t.Error("la cookie CSRF debe ser Secure")
			}
		}
	}
	if !found {
		t.Fatal("no se sembró cookie CSRF en método seguro")
	}
}

func TestCSRFRejectsMissingHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req.AddCookie(&http.Cookie{Name: DefaultCSRFCookieName, Value: "tok123"})
	rec := httptest.NewRecorder()
	csrfHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST sin header debe ser 403, got %d", rec.Code)
	}
}

func TestCSRFRejectsMismatch(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req.AddCookie(&http.Cookie{Name: DefaultCSRFCookieName, Value: "tok123"})
	req.Header.Set(HeaderCSRFToken, "otro")
	rec := httptest.NewRecorder()
	csrfHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("token no coincidente debe ser 403, got %d", rec.Code)
	}
}

func TestCSRFAcceptsMatch(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req.AddCookie(&http.Cookie{Name: DefaultCSRFCookieName, Value: "tok123"})
	req.Header.Set(HeaderCSRFToken, "tok123")
	rec := httptest.NewRecorder()
	csrfHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("double-submit válido debe pasar, got %d", rec.Code)
	}
}

func TestCSRFRejectsNoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req.Header.Set(HeaderCSRFToken, "tok123")
	rec := httptest.NewRecorder()
	csrfHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sin cookie debe ser 403, got %d", rec.Code)
	}
}
