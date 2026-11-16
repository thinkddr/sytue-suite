// SPDX-License-Identifier: Apache-2.0
package cast

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateShareFixture struct{}

func (privateShareFixture) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "cast"}
}

func (privateShareFixture) Videos(context.Context, string, string, string, string, int) (api.CastPage, error) {
	return api.CastPage{}, nil
}

func (privateShareFixture) Video(_ context.Context, id string) (api.CastVideo, error) {
	return api.CastVideo{ID: id, Visibility: "private", Status: api.CastStatusReady, Title: "Vídeo privado"}, nil
}

func (privateShareFixture) Play(context.Context, string) (api.CastPlay, error) {
	return api.CastPlay{}, nil
}

func hasShareButton(object fyne.CanvasObject, text string) bool {
	if object == nil || !object.Visible() {
		return false
	}
	if button, ok := object.(*ui.Boton); ok && button.Texto == text {
		return true
	}
	var children []fyne.CanvasObject
	switch value := object.(type) {
	case *fyne.Container:
		children = value.Objects
	case fyne.Widget:
		children = test.WidgetRenderer(value).Objects()
	}
	for _, child := range children {
		if hasShareButton(child, text) {
			return true
		}
	}
	return false
}

func TestPrivateCastDoesNotOfferOrCopyLegacyShareLink(t *testing.T) {
	a, w := appDePrueba(t, servicioCompleto(), true)
	a.AbrirVideo("v01")
	label := a.v.Traduce("watch.share")
	if !hasShareButton(w.Content(), label) {
		t.Fatal("legacy share button missing")
	}
	legacy := a.cli
	private, err := legacy.WithPrivateData(privateShareFixture{})
	if err != nil {
		t.Fatal(err)
	}
	a.cli = private
	a.AbrirVideo("private-video")
	if a.ver.video.ID != "private-video" {
		t.Fatal("private video was not loaded through its provider")
	}
	if hasShareButton(w.Content(), label) {
		t.Fatal("private catalog exposed legacy sharing")
	}
	if share := a.datosCompartir(); share.Enlace != "" || share.Insercion != "" || share.AlCopiarEnlace != nil || share.AlCopiarInsercion != nil {
		t.Fatal("private catalog supplied legacy sharing data")
	}
	fyne.CurrentApp().Clipboard().SetContent("sentinel")
	a.abrirCompartir()
	a.copiarEnlace()
	a.copiarInsercion()
	if a.ver.compartir != nil || a.ultimoEnlace != "" || a.ultimaInsercion != "" || fyne.CurrentApp().Clipboard().Content() != "sentinel" {
		t.Fatal("private sharing action exposed a legacy link")
	}
	a.cli = legacy
	a.AbrirVideo("v02")
	if !hasShareButton(w.Content(), label) {
		t.Fatal("legacy sharing did not return after changing provider")
	}
}
