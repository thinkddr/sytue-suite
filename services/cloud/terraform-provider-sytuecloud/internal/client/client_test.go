// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer returns a Client pointed at an httptest.Server driven by h.
func newTestServer(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL, "scl_test")
}

func TestProjectCRUD(t *testing.T) {
	var lastMethod, lastPath, lastAuth string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastMethod, lastPath, lastAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			var in Project
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.Slug != "blog" {
				t.Errorf("slug = %q, want blog", in.Slug)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(Project{ID: "p1", Slug: in.Slug, Name: in.Name})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/p1":
			_ = json.NewEncoder(w).Encode(Project{ID: "p1", Slug: "blog", Name: "Blog"})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/projects/p1":
			_ = json.NewEncoder(w).Encode(Project{ID: "p1", Slug: "blog", Name: "Renamed"})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/projects/p1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	created, err := c.CreateProject(ctx, Project{Slug: "blog", Name: "Blog"})
	if err != nil || created.ID != "p1" {
		t.Fatalf("create: %v id=%q", err, created.ID)
	}
	if lastAuth != "Bearer scl_test" {
		t.Errorf("auth header = %q", lastAuth)
	}

	got, err := c.GetProject(ctx, "p1")
	if err != nil || got.Name != "Blog" {
		t.Fatalf("get: %v name=%q", err, got.Name)
	}

	up, err := c.UpdateProject(ctx, "p1", Project{Name: "Renamed"})
	if err != nil || up.Name != "Renamed" {
		t.Fatalf("update: %v name=%q", err, up.Name)
	}

	if err := c.DeleteProject(ctx, "p1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if lastMethod != http.MethodDelete || lastPath != "/api/v1/projects/p1" {
		t.Errorf("last call = %s %s", lastMethod, lastPath)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"code":"not_found","message":"gone"}}`)
	})
	_, err := c.GetProject(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestAPIErrorTyped(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(w, `{"error":{"code":"spend_limit","message":"over budget"}}`)
	})
	_, err := c.CreateProject(context.Background(), Project{Slug: "x"})
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %T", err)
	}
	if ae.Status != http.StatusPaymentRequired || ae.Code != "spend_limit" {
		t.Errorf("APIError = %+v", ae)
	}
}

func TestMissingToken(t *testing.T) {
	c := New("http://example.invalid", "")
	if _, err := c.GetProject(context.Background(), "p1"); err == nil {
		t.Fatal("want error for missing token")
	}
}

func TestEnvAndDomainAndService(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/projects/p1/env":
			_ = json.NewEncoder(w).Encode(EnvVar{ID: "e1", ProjectID: "p1", Key: "API_TOKEN", IsSecret: true, Scope: "all"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/p1/env":
			_ = json.NewEncoder(w).Encode(map[string]any{"env": []EnvVar{{ID: "e1", Key: "API_TOKEN", IsSecret: true}}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/env/e1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/p1/domains":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"domain": Domain{ID: "d1", ProjectID: "p1", Hostname: "app.example.com", Status: "pending", VerifyToken: "tok"},
				"dns":    map[string]string{"cname_target": "cname.sytue.eu"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/p1/domains":
			_ = json.NewEncoder(w).Encode(map[string]any{"domains": []Domain{{ID: "d1", Hostname: "app.example.com"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/p1/services":
			_ = json.NewEncoder(w).Encode(ManagedService{ID: "s1", ProjectID: "p1", Kind: "postgres", Status: "provisioning"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services/s1":
			_ = json.NewEncoder(w).Encode(ManagedService{ID: "s1", ProjectID: "p1", Kind: "postgres", Status: "ready", Config: map[string]string{"version": "16"}})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	ev, err := c.PutEnv(ctx, "p1", EnvVar{Key: "API_TOKEN", Value: "secret", IsSecret: true})
	if err != nil || ev.ID != "e1" || !ev.IsSecret {
		t.Fatalf("putenv: %v %+v", err, ev)
	}
	list, err := c.ListEnv(ctx, "p1")
	if err != nil || len(list) != 1 || list[0].Key != "API_TOKEN" {
		t.Fatalf("listenv: %v %+v", err, list)
	}
	if err := c.DeleteEnv(ctx, "e1"); err != nil {
		t.Fatalf("delenv: %v", err)
	}

	dom, err := c.CreateDomain(ctx, "p1", "app.example.com")
	if err != nil || dom.ID != "d1" || dom.VerifyToken != "tok" {
		t.Fatalf("createdomain: %v %+v", err, dom)
	}
	doms, err := c.ListDomains(ctx, "p1")
	if err != nil || len(doms) != 1 {
		t.Fatalf("listdomains: %v %+v", err, doms)
	}

	svc, err := c.CreateService(ctx, "p1", ManagedService{Kind: "postgres"})
	if err != nil || svc.ID != "s1" {
		t.Fatalf("createservice: %v %+v", err, svc)
	}
	got, err := c.GetService(ctx, "s1")
	if err != nil || got.Status != "ready" || got.Config["version"] != "16" {
		t.Fatalf("getservice: %v %+v", err, got)
	}
}
