// SPDX-License-Identifier: Apache-2.0
package matrixprooftest

import "testing"

func TestDisposableFixtureOriginCannotReceiveProductionCredentialsOrURLs(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:18643", "http://[::1]:18643/"} {
		if _, err := DisposableHTTPOrigin(raw); err != nil {
			t.Fatal("disposable loopback origin rejected", err)
		}
	}
	for _, raw := range []string{"https://production.invalid", "http://localhost:18643", "http://192.0.2.1:18643", "http://127.0.0.1:80", "http://user:synthetic@127.0.0.1:18643", "http://127.0.0.1:18643/?credential=synthetic", "http://127.0.0.1:18643/private", "http://127.0.0.1:18643/#synthetic"} {
		if _, err := DisposableHTTPOrigin(raw); err == nil {
			t.Fatal("non-fixture origin was admitted", raw)
		}
	}
}
