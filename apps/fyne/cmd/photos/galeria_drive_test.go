// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestGaleriaDriveSubePorElDriveOcultoYCierraElResto(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
	driveScope := scope
	driveScope.Product = "drive"
	asked := 0
	data := galeriaDriveData{
		resolve: func(context.Context) (*privateDrive.Repository, error) {
			asked++
			return nil, privatePhotos.ErrNotReady
		},
		scope: driveScope,
	}
	facade, err := api.NewDrive("http://drive.invalid", nil, nil).WithPrivateData(data)
	if err != nil {
		t.Fatal(err)
	}
	if data.PrivateStorageScope() != driveScope || data.PrivateStorageScope().Product != "drive" {
		t.Fatal("gallery Drive facade lost the hidden repository scope", data.PrivateStorageScope())
	}
	for _, subir := range []struct {
		nombre string
		llama  func() error
	}{
		{"Root", func() error { _, err := facade.Root(t.Context()); return err }},
		{"Children", func() error { _, err := facade.Children(t.Context(), "carpeta", ""); return err }},
		{"CreateFolder", func() error { _, err := facade.CreateFolder(t.Context(), "carpeta", "Fotos"); return err }},
		{"Upload", func() error {
			_, err := facade.Upload(t.Context(), "carpeta", "foto.png", "image/png", nil, 0)
			return err
		}},
	} {
		if err := subir.llama(); !errors.Is(err, privatePhotos.ErrNotReady) {
			t.Fatal(subir.nombre, "did not resolve the hidden Drive repository", err)
		}
	}
	if asked != 4 {
		t.Fatal("facade did not resolve per gallery upload", asked)
	}
	for _, cerrado := range []struct {
		nombre string
		llama  func() error
	}{
		{"File", func() error { _, err := facade.File(t.Context(), "f"); return err }},
		{"Download", func() error { _, err := facade.Download(t.Context(), "f"); return err }},
		{"Delete", func() error { return facade.Delete(t.Context(), api.DriveKind("folder"), "f") }},
		{"Trash", func() error { _, err := facade.Trash(t.Context()); return err }},
		{"Zip", func() error { _, _, err := facade.Zip(t.Context(), nil, nil, "fotos.zip"); return err }},
	} {
		if err := cerrado.llama(); !errors.Is(err, api.ErrPrivateDriveOperation) {
			t.Fatal(cerrado.nombre, "was not closed on the gallery facade", err)
		}
	}
	if asked != 4 {
		t.Fatal("closed operations resolved the hidden repository", asked)
	}
}
