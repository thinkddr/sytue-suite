// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type privateRecipeFixture struct {
	privatePhotosFixture
	read    func(context.Context, string) (Receta, error)
	save    func(context.Context, string, Receta, string) (string, error)
	pending *EdicionPhotos
}

func (p *privateRecipeFixture) LeerReceta(ctx context.Context, id string) (Receta, error) {
	return p.read(ctx, id)
}

func (p *privateRecipeFixture) GuardarRecetaDesde(ctx context.Context, id string, recipe Receta, base string) (string, error) {
	return p.save(ctx, id, recipe, base)
}

func (p *privateRecipeFixture) EdicionPendiente(context.Context, string) (*EdicionPhotos, error) {
	return p.pending, nil
}

func TestPrivateRecipeAndRotationRetainObservedBaseAndNeverUseLegacyHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	legacy := NewPhotos(server.URL, nil, server.Client())
	fixture := &privateRecipeFixture{}
	current := RecetaVacia()
	current.Pasos = append(current.Pasos, PasoReceta{Tipo: PasoContraste, Params: map[string]float64{"valor": .25}})
	reads, writes := 0, 0
	fixture.read = func(context.Context, string) (Receta, error) { reads++; return current.Clonar(), nil }
	fixture.save = func(_ context.Context, id string, recipe Receta, base string) (string, error) {
		writes++
		if id != "photo" || base != current.Huella() || len(recipe.Pasos) != 2 || recipe.Pasos[1].Tipo != PasoGiro {
			t.Fatal("rotation lost its observed base", base, recipe)
		}
		return recipe.Huella(), nil
	}
	private, err := legacy.WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := private.Rotar(t.Context(), "photo"); err != nil || reads != 1 || writes != 1 {
		t.Fatal("private rotation bypassed its capability", err)
	}
	wanted := current.Clonar()
	wanted.Pasos = append(wanted.Pasos, PasoReceta{Tipo: PasoGiro, Params: map[string]float64{"grados": 90}})
	fixture.pending = &EdicionPhotos{Receta: wanted, Base: current.Huella()}
	if _, err := private.Rotar(t.Context(), "photo"); err != nil || reads != 1 || writes != 2 {
		t.Fatal("retry reconstructed rotation from current state", err)
	}
	fixture.pending = nil
	fixture.save = func(_ context.Context, _ string, _ Receta, base string) (string, error) {
		if base != "" {
			t.Fatal("plain save replaced its original empty base")
		}
		return "", context.Canceled
	}
	if _, err := private.GuardarReceta(t.Context(), "photo", wanted); !errors.Is(err, context.Canceled) || reads != 1 {
		t.Fatal("provider failure or stable request changed", err)
	}
	unsupported, err := legacy.WithPrivateData(privatePhotosFixture{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unsupported.LeerReceta(t.Context(), "photo"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal(err)
	}
	if _, err := unsupported.GuardarRecetaDesde(t.Context(), "photo", wanted, "old"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal(err)
	}
	if _, err := unsupported.EdicionPendiente(t.Context(), "photo"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal(err)
	}
	if _, err := unsupported.Rotar(t.Context(), "photo"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Fatal("private recipe reached legacy HTTP", requests)
	}
}
