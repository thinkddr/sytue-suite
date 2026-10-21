// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"os"

	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Administrative commands keep an already-enabled SQL guard; they never enable
// encrypted calls or expose the identity proof API themselves.
func configureCommandCallIdentity(ctx context.Context, st *store.Store) error {
	base := os.Getenv("SYTUE_ID_ROOMS_CALL_ACCESS_URL")
	if base == "" {
		base = "http://rooms:8085"
	}
	var revoker callaccess.SubjectRevoker
	if client := callaccess.New(base, os.Getenv("SYTUE_ID_ROOMS_CALL_ACCESS_SECRET")); client != nil {
		revoker = client
	}
	return st.ConfigureCallIdentity(ctx, false, revoker)
}
