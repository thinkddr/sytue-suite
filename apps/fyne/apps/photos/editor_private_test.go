// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateEditorData struct {
	api.PhotosData
	current api.Receta
	pending *api.EdicionPhotos
	save    func(context.Context, string, api.Receta, string) (string, error)
	raw     []byte
}

func (*privateEditorData) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
}

func (p *privateEditorData) LeerReceta(ctx context.Context, _ string) (api.Receta, error) {
	return p.current.Clonar(), ctx.Err()
}

func (p *privateEditorData) GuardarRecetaDesde(ctx context.Context, id string, recipe api.Receta, base string) (string, error) {
	return p.save(ctx, id, recipe, base)
}

func (p *privateEditorData) EdicionPendiente(ctx context.Context, _ string) (*api.EdicionPhotos, error) {
	if p.pending == nil {
		return nil, ctx.Err()
	}
	return &api.EdicionPhotos{Receta: p.pending.Receta.Clonar(), Base: p.pending.Base}, ctx.Err()
}

func (p *privateEditorData) VariantDe(ctx context.Context, _, variant, fingerprint string) (io.ReadCloser, error) {
	if variant != api.PhotoPreview || fingerprint != api.RecetaVacia().Huella() {
		return nil, api.ErrPrivatePhotosOperation
	}
	return io.NopCloser(bytes.NewReader(p.raw)), ctx.Err()
}

func TestPrivateEditorReopensAndRetriesExactAttemptAfterLostACK(t *testing.T) {
	service := &servicioFalso{}
	application, _ := appDePrueba(t, service)
	data := &privateEditorData{current: api.RecetaVacia(), raw: pngDe(bandas(40, 20))}
	base := data.current.Huella()
	writes := 0
	data.save = func(_ context.Context, _ string, recipe api.Receta, expected string) (string, error) {
		writes++
		if expected != base || len(recipe.Pasos) != 1 || recipe.Pasos[0].Tipo != api.PasoGiro {
			t.Fatal("editor replaced its captured recipe/base", recipe, expected)
		}
		if writes == 1 {
			data.current = recipe.Clonar()
			data.pending = &api.EdicionPhotos{Receta: recipe.Clonar(), Base: expected}
			return "", errors.New("lost content ACK")
		}
		data.pending = nil
		return recipe.Huella(), nil
	}
	private, err := application.cli.WithPrivateData(data)
	if err != nil {
		t.Fatal(err)
	}
	application.cli = private
	foto := fotosDePrueba(1)[0]
	application.AbrirEditor(foto, "")
	if application.ed.cargando || application.ed.original == nil {
		t.Fatal("private editor did not load authenticated preview")
	}
	application.anyadirPaso(api.PasoReceta{Tipo: api.PasoGiro, Params: map[string]float64{"grados": 90}})
	application.guardarEdicion()
	if application.ed.pendiente == nil || !application.ed.sucia() || application.ed.editable() {
		t.Fatal("unacknowledged editor attempt became replaceable")
	}
	application.cerrarEditor()
	application.AbrirEditor(foto, "")
	e := application.ed
	if e.base != base || !e.sucia() || e.pendiente == nil || !e.receta.Igual(data.current) {
		t.Fatal("reopen rebuilt the attempt from accepted state", e.base, e.receta)
	}
	application.anyadirPaso(api.PasoReceta{Tipo: api.PasoGiro, Params: map[string]float64{"grados": 90}})
	application.deshacerEditor()
	e.mandos[api.PasoContraste].Poner(.5)
	if !e.receta.Igual(data.current) {
		t.Fatal("editing changed an unacknowledged intent")
	}
	application.guardarEdicion()
	if e.pendiente != nil || e.sucia() || !e.editable() || writes != 2 || application.Huella(foto.ID) != data.current.Huella() || len(service.vistas()) != 0 {
		t.Fatal("editor retry did not settle its original intent", writes, service.vistas())
	}
	application.Cerrar()
}

func TestPrivateEditorRequiresAuthenticatedPreviewBeforeSaving(t *testing.T) {
	service := &servicioFalso{}
	application, _ := appDePrueba(t, service)
	writes := 0
	data := &privateEditorData{current: api.RecetaVacia()}
	data.save = func(context.Context, string, api.Receta, string) (string, error) { writes++; return "", nil }
	private, err := application.cli.WithPrivateData(data)
	if err != nil {
		t.Fatal(err)
	}
	application.cli = private
	application.AbrirEditor(fotosDePrueba(1)[0], "")
	if application.ed.editable() {
		t.Fatal("failed preview admitted editing without an observed base")
	}
	application.anyadirPaso(api.PasoReceta{Tipo: api.PasoGiro, Params: map[string]float64{"grados": 90}})
	if application.ed.sucia() {
		t.Fatal("failed preview accepted an edit")
	}
	application.ed.receta = conPaso(application.ed.receta, api.PasoReceta{Tipo: api.PasoGiro, Params: map[string]float64{"grados": 90}})
	application.guardarEdicion()
	if writes != 0 || len(service.vistas()) != 0 {
		t.Fatal("failed preview published or reached HTTP", writes, service.vistas())
	}
	application.Cerrar()
}
