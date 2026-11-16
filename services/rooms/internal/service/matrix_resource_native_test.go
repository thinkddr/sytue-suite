// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestResourceNativeAudienceCannotSelectProductOrApproveItself(t *testing.T) {
	for _, client := range []string{"sytue-desktop", "sytue-mobile"} {
		ctx := authn.Into(t.Context(), authn.Claims{Subject: "alice", Audience: []string{client}})
		if got, err := resourceNativeAudience(ctx, "alice"); err != nil || got != client {
			t.Fatal("validated native client rejected", err)
		}
		s := &Service{}
		if err := s.ApproveMatrixResourceNative(ctx, "alice", "not-authority-from-native-body"); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("native app approved its own invisible consent", err)
		}
	}
	for _, claims := range []authn.Claims{
		{Subject: "alice", Audience: []string{"sytue-chat"}},
		{Subject: "alice", Audience: []string{"sytue-mail"}},
		{Subject: "alice", Audience: []string{"sytue-desktop", "sytue-mobile"}},
		{Subject: "bob", Audience: []string{"sytue-desktop"}},
		{Subject: "alice", Audience: []string{"sytue-desktop"}, Delegado: true},
	} {
		if _, err := resourceNativeAudience(authn.Into(t.Context(), claims), "alice"); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("client/actor/delegation authority crossed", err)
		}
	}
	if _, err := resourceNativeAudience(context.Background(), "alice"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("missing verified claims gained native purpose")
	}
}

func TestResourceProductWebAudienceIsIndependentAndNativeStillNeedsPurposeGrant(t *testing.T) {
	for _, test := range []struct {
		aud, product string
		allowed      bool
	}{
		{"sytue-docus", "docs", true},
		{"sytue-calcus", "sheets", true},
		{"sytue-diapos", "slides", true},
		{"sytue-certipdf", "pdf", true},
		{"sytue-mail", "docs", false},
		{"sytue-docus", "sheets", false},
		{"sytue-docs", "docs", false},
		{"sytue-chat", "docs", false},
		{"sytue-desktop", "docs", true},
	} {
		ctx := authn.Into(t.Context(), authn.Claims{Subject: "alice", Audience: []string{test.aud}})
		got, err := resourceProductClient(ctx, "alice", test.product)
		if test.allowed {
			if err != nil || got != test.aud {
				t.Fatal("current product audience rejected", test, err)
			}
		} else if !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("another app/service audience gained product enrollment", test, err)
		}
	}
}
