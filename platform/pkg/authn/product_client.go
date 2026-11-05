// SPDX-License-Identifier: Apache-2.0
package authn

import (
	"context"
	"slices"

	"git.sytue.eu/sytue/one/platform/pkg/config"
)

// FirstPartyNativeClient uses only middleware-verified claims. Ambiguous native
// audiences and delegated tokens cannot authorize device enrollment or peers.
func FirstPartyNativeClient(ctx context.Context, actor string) (string, bool) {
	claims, ok := FromContext(ctx)
	if !ok || actor == "" || claims.Subject != actor || claims.Delegado {
		return "", false
	}
	var client string
	for _, allowed := range []string{"sytue-desktop", "sytue-mobile"} {
		if slices.Contains(claims.Audience, allowed) {
			if client != "" {
				return "", false
			}
			client = allowed
		}
	}
	return client, client != ""
}

// FirstPartyProductClient shares the current catalog policy across the own-
// session facade and resource-authorized public peer evidence. Legacy service
// audiences never substitute for the product's current first-party client.
func FirstPartyProductClient(ctx context.Context, actor, product string) (string, bool) {
	claims, ok := FromContext(ctx)
	if !ok || actor == "" || claims.Subject != actor || claims.Delegado {
		return "", false
	}
	client := config.FirstPartyProductClientID(product)
	if client == "" {
		return "", false
	}
	if slices.Contains(claims.Audience, "sytue-desktop") || slices.Contains(claims.Audience, "sytue-mobile") {
		return FirstPartyNativeClient(ctx, actor)
	}
	return client, slices.Contains(claims.Audience, client)
}
