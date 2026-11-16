//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
)

// Actual normal-RLS source mutations occur AFTER the current PRE response,
// with no source SQL transaction held over HTTP. Cached metadata is insufficient
// to survive either an ACL version change or cancellation of its capture.
func TestOfficeResourcePeerOperationRechecksOriginalSQLAfterCurrentCapture(t *testing.T) {
	for _, change := range []string{"acl-version", "cancel-capture"} {
		t.Run(change, func(t *testing.T) {
			c := prepareOriginalCaptureRecoveryCase(t)
			var plan resource.SignedBootstrapPlan
			var manifest resource.SignedBootstrapRosterManifest
			if json.Unmarshal(c.pending.PublicPlanBytes(), &plan) != nil || json.Unmarshal(c.pending.PublicManifestBytes(), &manifest) != nil {
				t.Fatal("public immutable fixture malformed")
			}
			query := resource.BootstrapPeerQuery{Plan: plan, Manifest: manifest, Scope: c.registry.Record.Binding.Scope, DeviceID: c.registry.Record.Binding.DeviceID}
			secret := strings.Repeat("a", 64)
			var reads atomic.Int64
			var changed atomic.Bool
			remote := httptest.NewServer(httpx.SoloDesdeDentro(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q resourcepg.SessionOriginRequest
				if httpx.DecodeJSONLimit(w, r, &q, 4096) != nil || !q.ValidForService("docs") {
					httpx.Fail(w, r, httpx.ErrBadRequest)
					return
				}
				n := reads.Add(1)
				record, err := c.registry.CurrentResourceSession(r.Context(), q.Scope, q.DeviceID)
				if err != nil {
					httpx.Fail(w, r, httpx.ErrForbidden)
					return
				}
				response, err := resourcepg.SignSessionOriginResponse(secret, q, resourcepg.SessionOriginResponse{Version: 1, Record: record, Origin: c.registry.Chain.Origin})
				if err != nil {
					httpx.Fail(w, r, httpx.ErrUnavailable)
					return
				}
				if n == 2 {
					if change == "acl-version" {
						err = c.inspect(func(tx pgx.Tx) error {
							_, err := tx.Exec(c.ctx, `SELECT docs_record_call_fence($1,$2)`, c.key.ID, c.key.Tenant)
							return err
						})
					} else {
						err = c.capture.Cancel(c.ctx, c.pending)
					}
					if err != nil {
						t.Error("actual original SQL mutation was not reached", err)
						httpx.Fail(w, r, httpx.ErrUnavailable)
						return
					}
					changed.Store(true)
				}
				httpx.JSON(w, r, http.StatusOK, response)
			})))
			defer remote.Close()
			authority, err := resourcepg.NewSessionAuthorityClient("docs", remote.URL, secret, remote.Client().Transport)
			if err != nil {
				t.Fatal(err)
			}
			source, ok := c.original.(resourcepg.BootstrapPeerTransactionSource)
			if !ok {
				t.Fatal("actual original source missing transaction contract")
			}
			if _, err := c.capture.CurrentPeerEvidenceOperation(c.ctx, query, source, authority); err == nil {
				t.Fatal("PRE witness survived changed ORIGINAL SQL at POST", change)
			}
			if !changed.Load() {
				t.Fatal("original PRE/POST fault boundary was not executed", reads.Load())
			}
			if reads.Load() != 2 {
				t.Fatal("original SQL denial occurred after unnecessary CURRENT lookups", reads.Load())
			}
		})
	}
}
