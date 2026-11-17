// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"encoding/json"
	"os"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestAndroidWorkContextUsesCanonicalGoIdentityAndScope(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Name         string        `json:"name"`
			Issuer       string        `json:"issuer"`
			Client       string        `json:"client"`
			Subject      string        `json:"subject"`
			Organization string        `json:"organization"`
			Context      string        `json:"context"`
			Identity     string        `json:"identity_hash"`
			Scope        storage.Scope `json:"scope"`
			Header       string        `json:"scope_header"`
			ScopeValid   bool          `json:"scope_valid"`
		} `json:"cases"`
	}
	raw, err := os.ReadFile("testdata/android-work-context.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) < 5 {
		t.Fatal("cross-runtime work context vectors are missing")
	}
	for _, v := range fixture.Cases {
		t.Run(v.Name, func(t *testing.T) {
			identity := IdentitySnapshot{Issuer: v.Issuer, Client: v.Client, Subject: v.Subject, Organization: v.Organization}
			if string(identity.contextoSesion()) != v.Context || identity.IdentidadSesion() != v.Identity {
				t.Fatal("Android fixture changed canonical Go session identity")
			}
			header, err := storage.EncodeScopeHeader(v.Scope)
			if v.Scope.Valid() != v.ScopeValid || (err == nil) != v.ScopeValid || header != v.Header {
				t.Fatal("Android fixture changed canonical Go storage scope")
			}
		})
	}
}
