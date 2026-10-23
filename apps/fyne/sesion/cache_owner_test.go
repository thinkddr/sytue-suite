// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

func suiteCacheIdentity(apex, issuer, sub, tenant string) *Suite {
	claims, _ := json.Marshal(map[string]string{"tenant": tenant})
	return &Suite{apex: apex, oidc: &auth.OIDC{Issuer: issuer}, Sesion: auth.NewSession(auth.Tokens{
		Subject: sub, Access: "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".artificial",
	}, nil, nil)}
}

func TestHTTPCacheOwnerSeparatesDeploymentsIssuersAndUnambiguousComponents(t *testing.T) {
	store, err := offline.Open(filepath.Join(t.TempDir(), "offline"))
	if err != nil {
		t.Fatal(err)
	}
	const id = "https://same-api.invalid/api/list"
	original := suiteCacheIdentity("prod.invalid", "https://issuer.invalid", "ada", "acme")
	if err := store.Write(original.dueñoCache(), "http:mail", id, "artificial"); err != nil {
		t.Fatal(err)
	}
	for _, identity := range []*Suite{
		suiteCacheIdentity("other.invalid", "https://issuer.invalid", "ada", "acme"),
		suiteCacheIdentity("prod.invalid", "https://other-issuer.invalid", "ada", "acme"),
		suiteCacheIdentity("prod.invalid", "https://issuer.invalid", "ada", "other"),
	} {
		var value string
		if ok, err := store.Read(identity.dueñoCache(), "http:mail", id, &value); err != nil || ok {
			t.Fatal("cache crossed identity", ok, value, err)
		}
	}
	if suiteCacheIdentity("prod.invalid", "https://issuer.invalid", "ada/x", "acme").dueñoCache() == suiteCacheIdentity("prod.invalid", "https://issuer.invalid", "ada", "x/acme").dueñoCache() {
		t.Fatal("ambiguous key components")
	}
	updated := suiteCacheIdentity("prod.invalid", "https://issuer.invalid/", "ada", "acme")
	var value string
	if ok, err := store.Read(updated.dueñoCache(), "http:mail", id, &value); err != nil || !ok || value != "artificial" {
		t.Fatal("same deployment lost stable cache", ok, value, err)
	}
}

func TestHTTPNamespaceChangePreservesNotesQueueIdentity(t *testing.T) {
	s := suiteCacheIdentity("prod.invalid", "https://issuer.invalid", "ada", "acme")
	if s.dueñoNotas() != "ada/acme" || s.dueñoCache() == s.dueñoNotas() {
		t.Fatal("queue/draft namespace changed")
	}
	if (&Suite{}).dueñoCache() != "" || (&Suite{}).dueñoNotas() != "" {
		t.Fatal("anonymous cache owner")
	}
}
