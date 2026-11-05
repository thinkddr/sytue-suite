// SPDX-License-Identifier: Apache-2.0
package authn_test

import (
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/config"
)

func TestFirstPartyProductClientRequiresKnownProductBeforeNativeAudience(t *testing.T) {
	for _, audience := range []string{"sytue-desktop", "sytue-mobile", "sytue-docus"} {
		ctx := authn.Into(t.Context(), authn.Claims{Subject: "alice", Audience: []string{audience}})
		for _, product := range []string{"", "unknown-product"} {
			if client, ok := authn.FirstPartyProductClient(ctx, "alice", product); ok || client != "" {
				t.Fatalf("native/web audience authorized unknown product: %q %q", audience, product)
			}
		}
		if client, ok := authn.FirstPartyProductClient(ctx, "alice", "docs"); !ok || client != audience {
			t.Fatalf("known product rejected: %q", audience)
		}
	}
	for _, claims := range []authn.Claims{
		{Subject: "alice", Audience: []string{"sytue-desktop", "sytue-mobile"}},
		{Subject: "alice", Audience: []string{"sytue-desktop"}, Delegado: true},
		{Subject: "other", Audience: []string{"sytue-desktop"}},
	} {
		if _, ok := authn.FirstPartyProductClient(authn.Into(t.Context(), claims), "alice", "docs"); ok {
			t.Fatal("catalog validation removed actor/delegation/ambiguous-native guard")
		}
	}
}

func TestFirstPartyProductClientKeepsEveryCatalogAudienceIsolated(t *testing.T) {
	clients := map[string]string{"hub": "sytue-one", "mail": "sytue-mail", "drive": "sytue-drive", "calendar": "sytue-calendar", "contacts": "sytue-contacts", "docs": "sytue-docus", "sheets": "sytue-calcus", "slides": "sytue-diapos", "pdf": "sytue-certipdf", "notes": "sytue-notes", "photos": "sytue-syphotos", "rooms": "sytue-chat", "cast": "sytue-cast", "erp": "sytue-syerp", "bases": "sytue-bases", "pages": "sytue-pages"}
	audiences := strings.Split(config.FirstPartyAudiences(), ",")
	for product, expected := range clients {
		if config.FirstPartyProductClientID(product) != expected || !slices.Contains(audiences, expected) {
			t.Fatal("catalog and audience registry diverged", product)
		}
		for other, audience := range clients {
			ctx := authn.Into(t.Context(), authn.Claims{Subject: "alice", Audience: []string{audience}})
			client, allowed := authn.FirstPartyProductClient(ctx, "alice", product)
			if allowed != (product == other) || client != expected {
				t.Fatal("web token crossed product client boundary", product, other)
			}
		}
		for _, native := range []string{"sytue-desktop", "sytue-mobile"} {
			client, ok := authn.FirstPartyProductClient(authn.Into(t.Context(), authn.Claims{Subject: "alice", Audience: []string{native}}), "alice", product)
			if !ok || client != native {
				t.Fatal("known product rejected native client", product, native)
			}
		}
	}
	if config.FirstPartyProductClientID("unknown") != "" {
		t.Fatal("unknown product acquired catalog client")
	}
}
