// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExplicitJSONLimitAllowsAttachmentsButRemainsBounded(t *testing.T) {
	payload := `{"data":"` + strings.Repeat("a", maxBodyBytes+1) + `"}`
	var value struct {
		Data string `json:"data"`
	}
	if err := DecodeJSON(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload)), &value); AsError(err).Code != "body_too_large" {
		t.Fatalf("default limit: %v", err)
	}
	if err := DecodeJSONLimit(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload)), &value, 2<<20); err != nil || len(value.Data) != maxBodyBytes+1 {
		t.Fatalf("attachment payload: %v", err)
	}
	err := DecodeJSONLimit(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload)), &value, 128)
	if AsError(err).Code != "body_too_large" || AsError(err).Params["limit_bytes"] != 128 {
		t.Fatalf("custom limit: %v", err)
	}
}
