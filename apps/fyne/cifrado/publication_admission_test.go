// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

type publicationResourceContextKey struct{}

func TestNativePublicationBinderBelongsToTheOriginalResourcesLifetime(t *testing.T) {
	factory, ctx, session, devices := factoryFixture(t)
	first, err := factory.Open(ctx, devices)
	if err != nil {
		t.Fatal(err)
	}
	second, err := factory.Open(session.WithAuthorizationIntent(t.Context()), devices)
	if err != nil {
		t.Fatal(err)
	}
	if first.storagePublicationBinder() != nil || second.storagePublicationBinder() != nil {
		t.Fatal("unapproved resources acquired a writer")
	}
	if err := first.bindStoragePublication(ctx, nil); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("nil publication binder accepted", err)
	}
	called := 0
	if err := first.bindStoragePublication(ctx, func(ctx context.Context) (context.Context, error) {
		called++
		return context.WithValue(ctx, publicationResourceContextKey{}, "first"), nil
	}); err != nil {
		t.Fatal(err)
	}
	if second.storagePublicationBinder() != nil {
		t.Fatal("publication binder leaked into another resources lifetime")
	}
	bound, err := first.storagePublicationBinder()(first.Context())
	if err != nil || bound.Value(publicationResourceContextKey{}) != "first" || called != 1 {
		t.Fatal("original publication adapter was not retained", err)
	}
	factoryAdopt(t, first.session.witness, session, factoryTokens("bob"))
	factoryAdopt(t, first.session.witness, session, factoryTokens("alice"))
	if _, err := first.storagePublicationBinder()(context.WithoutCancel(first.Context())); !errors.Is(err, auth.ErrTrabajoSesion) || called != 1 {
		t.Fatal("publication binder revived A after account ABA", err)
	}
	if err := first.bindStoragePublication(context.WithoutCancel(ctx), func(ctx context.Context) (context.Context, error) { return ctx, nil }); !errors.Is(err, auth.ErrTrabajoSesion) {
		t.Fatal("retired resources accepted a replacement writer", err)
	}
}

func TestNativePublicationBinderRejectsIdentityChangeDuringBinding(t *testing.T) {
	factory, ctx, session, devices := factoryFixture(t)
	resources, err := factory.Open(ctx, devices)
	if err != nil {
		t.Fatal(err)
	}
	if err := resources.bindStoragePublication(ctx, func(ctx context.Context) (context.Context, error) {
		factoryAdopt(t, resources.session.witness, session, factoryTokens("bob"))
		return ctx, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := resources.storagePublicationBinder()(resources.Context()); !errors.Is(err, auth.ErrTrabajoSesion) {
		t.Fatal("publication binding skipped its post-session fence", err)
	}
}
