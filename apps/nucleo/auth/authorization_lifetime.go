// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/autorizacion"
)

// WithoutAuthorizationIntent removes only the short action intent. Native
// application lifetimes call it AFTER capturing a persisted work witness and
// binding their HTTP session to that identity and witness. It grants no authority
// itself: every request must still acquire and validate its AuthorizationScope.
// Cancellation, deadlines, ScopeProof and unrelated values remain inherited.
func WithoutAuthorizationIntent(ctx context.Context) context.Context {
	return autorizacion.WithoutAuthorizationIntent(ctx)
}
