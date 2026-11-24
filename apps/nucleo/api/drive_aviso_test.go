// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// driveEscrituras es un Drive privado que apunta lo que le piden; `fallar` hace que
// cada escritura falle.
type driveEscrituras struct {
	DriveData
	fallar error
}

func (driveEscrituras) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "org", Subject: "owner", Product: "drive"}
}
func (driveEscrituras) Root(context.Context) (Folder, error) { return Folder{ID: "raiz"}, nil }
func (driveEscrituras) Children(context.Context, string, string) (Children, error) {
	return Children{}, nil
}
func (d driveEscrituras) CreateFolder(context.Context, string, string) (Folder, error) {
	return Folder{ID: "c"}, d.fallar
}
func (d driveEscrituras) Upload(context.Context, string, string, string, io.Reader, int64) (File, error) {
	return File{ID: "f"}, d.fallar
}
func (d driveEscrituras) Rename(context.Context, DriveKind, string, string) error { return d.fallar }
func (d driveEscrituras) Move(context.Context, DriveKind, string, string) error   { return d.fallar }
func (d driveEscrituras) Delete(context.Context, DriveKind, string) error         { return d.fallar }
func (d driveEscrituras) Restore(context.Context, DriveKind, string) error        { return d.fallar }
func (d driveEscrituras) CopyFile(context.Context, string) (File, error)          { return File{}, d.fallar }
func (d driveEscrituras) SetFavorite(context.Context, DriveKind, string, bool) error {
	return d.fallar
}
func (d driveEscrituras) RestoreVersion(context.Context, string, int) error { return d.fallar }
func (d driveEscrituras) SetLabel(context.Context, string, string) error    { return d.fallar }
func (d driveEscrituras) ReplaceFile(context.Context, File, string, io.Reader, int64) (File, error) {
	return File{}, d.fallar
}

// TestElDrivePrivadoAvisaDeCadaEscritura: Drive y CertiPDF escriben en el Drive privado
// (la bóveda o Matrix), que no pasa por HTTP y que ClienteConAviso no ve. Con ConAviso,
// cada escritura con éxito avisa una vez —el widget se vuelve a publicar— y ni las
// lecturas ni una escritura fallida avisan.
func TestElDrivePrivadoAvisaDeCadaEscritura(t *testing.T) {
	ctx := t.Context()
	for _, fallo := range []error{nil, errors.New("sin red")} {
		avisos := 0
		privado, err := NewDrive("https://drive.example", nil, nil).WithPrivateData(driveEscrituras{fallar: fallo})
		if err != nil {
			t.Fatal(err)
		}
		d := privado.ConAviso(func() { avisos++ })
		escrituras := map[string]func() error{
			"crear carpeta": func() error { _, e := d.CreateFolder(ctx, "raiz", "x"); return e },
			"subir":         func() error { _, e := d.Upload(ctx, "raiz", "x", "text/plain", strings.NewReader("x"), 1); return e },
			"subir conservando": func() error {
				_, e := d.UploadConservando(ctx, "raiz", "x", "text/plain", strings.NewReader("x"), 1)
				return e
			},
			"renombrar": func() error { return d.Rename(ctx, DriveFile, "f", "y") },
			"mover":     func() error { return d.Move(ctx, DriveFile, "f", "c") },
			"papelera":  func() error { return d.Delete(ctx, DriveFile, "f") },
			"restaurar": func() error { return d.Restore(ctx, DriveFile, "f") },
			"copiar":    func() error { _, e := d.CopyFile(ctx, "f"); return e },
			"destacar":  func() error { return d.SetFavorite(ctx, DriveFile, "f", true) },
			"versión":   func() error { return d.RestoreVersion(ctx, "f", 2) },
			"etiquetar": func() error { return d.SetLabel(ctx, "f", "") },
			"sustituir": func() error {
				_, e := d.ReplaceFile(ctx, File{ID: "f"}, "text/plain", strings.NewReader("x"), 1)
				return e
			},
		}
		for nombre, escribir := range escrituras {
			antes := avisos
			err := escribir()
			quiero := 1
			if fallo != nil {
				quiero = 0
			}
			if !errors.Is(err, fallo) || avisos-antes != quiero {
				t.Errorf("%s (fallo %v): error %v, %d avisos", nombre, fallo, err, avisos-antes)
			}
		}
		antes := avisos
		if _, err := d.Root(ctx); err != nil || avisos != antes {
			t.Fatalf("leer avisa: %v, %d", err, avisos-antes)
		}
		if _, err := d.Children(ctx, "raiz", ""); err != nil || avisos != antes {
			t.Fatalf("listar avisa: %v, %d", err, avisos-antes)
		}
		if privado.avisar != nil {
			t.Fatal("ConAviso cambió el cliente original")
		}
	}
	// Sin datos privados no cambia nada: el de HTTP avisa con su transporte.
	servidor := NewDrive("https://drive.example", nil, &http.Client{})
	if servidor.ConAviso(func() {}) != servidor {
		t.Fatal("ConAviso sobre el Drive del servidor")
	}
}

// TestCertiPDFAvisaDeLoQueEscribeEnElDrivePrivado: el PDF de CertiPDF sobre el Drive
// privado avisa al guardar en la carpeta «PDF».
func TestCertiPDFAvisaDeLoQueEscribeEnElDrivePrivado(t *testing.T) {
	avisos := 0
	pdf, err := NewPDF("https://docs.example", "https://drive.example", nil, nil).WithPrivateDrive(driveEscrituras{})
	if err != nil {
		t.Fatal(err)
	}
	conAviso := pdf.ConAviso(func() { avisos++ })
	if _, err := conAviso.Subir(t.Context(), "a.pdf", strings.NewReader("%PDF-1.7 x"), 10); err != nil || avisos != 2 {
		// La primera vez se crea la carpeta «PDF» y se sube: dos escrituras.
		t.Fatalf("subir un PDF: %v, %d avisos", err, avisos)
	}
	if NewPDF("https://docs.example", "https://drive.example", nil, nil).ConAviso(func() { avisos++ }) == nil {
		t.Fatal("sin Drive privado, el mismo cliente")
	}
}
