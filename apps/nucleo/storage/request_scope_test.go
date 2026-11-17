// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"net/http"
	"strings"
	"testing"
)

func TestRequestScopeRequiresExactAuthenticatedScopeAndOneHeader(t *testing.T) {
	scope := Scope{Tenant: "acme", Subject: "alice", Product: "contacts"}
	value, err := EncodeScopeHeader(scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequestScope(http.Header{ScopeHeader: {value}}, scope); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequestScope(http.Header{strings.ToLower(ScopeHeader): {value}}, scope); err != nil {
		t.Fatal(err)
	}
	for _, header := range []http.Header{
		nil,
		{},
		{ScopeHeader: {}},
		{ScopeHeader: {value, value}},
		{ScopeHeader: {value}, strings.ToLower(ScopeHeader): {value}},
		{ScopeHeader: {""}},
		{ScopeHeader: {"v1.invalid"}},
		{ScopeHeader: {value + "," + value}},
	} {
		if ValidateRequestScope(header, scope) == nil {
			t.Fatalf("accepted malformed scope headers: keys=%d", len(header))
		}
	}
	for _, other := range []Scope{
		{},
		{Tenant: "other", Subject: scope.Subject, Product: scope.Product},
		{Tenant: scope.Tenant, Subject: "bob", Product: scope.Product},
		{Tenant: scope.Tenant, Subject: scope.Subject, Product: "mail"},
	} {
		if ValidateRequestScope(http.Header{ScopeHeader: {value}}, other) == nil {
			t.Fatal("accepted another authenticated scope")
		}
	}
}
