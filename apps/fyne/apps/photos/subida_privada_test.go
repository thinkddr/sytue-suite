// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// drivePrivadoFalso es el Drive privado (la bóveda): como el de verdad, subir con un
// nombre que ya está SUSTITUYE ese fichero (una versión nueva), así que pisar uno aquí
// es un fallo de la prueba. Lo demás de la fachada no se usa.
type drivePrivadoFalso struct {
	api.DriveData
	mu      sync.Mutex
	t       *testing.T
	fotos   map[string]string // nombre → contenido, en «/Fotos»
	carpeta bool
}

func (*drivePrivadoFalso) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "ana", Product: "drive"}
}

func (*drivePrivadoFalso) Root(context.Context) (api.Folder, error) {
	return api.Folder{ID: "raiz-privada", IsRoot: true}, nil
}

func (d *drivePrivadoFalso) Children(_ context.Context, carpeta, _ string) (api.Children, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var c api.Children
	switch carpeta {
	case "raiz-privada":
		if d.carpeta {
			c.Folders = []api.Folder{{ID: "fotos-privada", Name: CarpetaFotos}}
		}
	case "fotos-privada":
		for n := range d.fotos {
			c.Files = append(c.Files, api.File{ID: "p-" + n, Name: n})
		}
	}
	return c, nil
}

func (d *drivePrivadoFalso) CreateFolder(_ context.Context, padre, nombre string) (api.Folder, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if padre != "raiz-privada" || nombre != CarpetaFotos || d.carpeta {
		d.t.Errorf("carpeta inesperada %q en %q", nombre, padre)
	}
	d.carpeta = true
	return api.Folder{ID: "fotos-privada", Name: nombre}, nil
}

func (d *drivePrivadoFalso) Upload(_ context.Context, carpeta, nombre, _ string, cuerpo io.Reader, _ int64) (api.File, error) {
	datos, err := io.ReadAll(cuerpo)
	if err != nil {
		return api.File{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if carpeta != "fotos-privada" {
		d.t.Errorf("subida a %q", carpeta)
	}
	if _, ya := d.fotos[nombre]; ya {
		d.t.Errorf("se ha sustituido %q en el Drive privado", nombre)
	}
	d.fotos[nombre] = string(datos)
	return api.File{ID: "p-" + nombre, Name: nombre}, nil
}

// TestConElDriveEnLaBovedaSeSubeCifradoSinIndexar: con el Drive del servidor en
// almacenamiento privado, «Añadir fotos» sube al Drive privado (cifrado, sin pisar
// la que tiene el mismo nombre) y no le pide nada al índice del servidor, que no
// podría leerla. Y lo dice con su propio mensaje.
func TestConElDriveEnLaBovedaSeSubeCifradoSinIndexar(t *testing.T) {
	g, servidor := nuevaGaleria(t), &driveFalso{}
	a := appConCopia(t, g, servidor)
	privado := &drivePrivadoFalso{t: t, fotos: map[string]string{"IMG_0001.jpg": "la vieja"}, carpeta: true}
	a.DriveDeSubida = func(context.Context) (*api.Drive, error) {
		return a.Drive.WithPrivateData(privado)
	}
	a.Arrancar()
	ruta := filepath.Join(t.TempDir(), "IMG_0001.jpg")
	if err := os.WriteFile(ruta, []byte("la nueva"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.subir([]string{ruta, ruta})
	quiero := a.v.T.T("gallery.uploadedEncrypted", i18n.Vars{"count": 2})
	esperar(t, "no se dice que se guardaron cifradas", func() bool { return a.g.subida == nil && ultimoAviso(a).Titulo == quiero })
	privado.mu.Lock()
	defer privado.mu.Unlock()
	if privado.fotos["IMG_0001.jpg"] != "la vieja" || privado.fotos["IMG_0001 (2).jpg"] != "la nueva" || privado.fotos["IMG_0001 (3).jpg"] != "la nueva" {
		t.Fatalf("Drive privado: %v", privado.fotos)
	}
	servidor.mu.Lock()
	defer servidor.mu.Unlock()
	if len(servidor.subidos) != 0 || servidor.indexados != 0 {
		t.Fatalf("al servidor: subidas %v, indexadas %d", servidor.subidos, servidor.indexados)
	}
}

// TestConElDriveEnLaBovedaSinAbrirSeDiceQueHacer: si el Drive privado aún no está
// abierto en este dispositivo, no se sube nada y el aviso dice que hay que
// desbloquearlo, no un «no se pudo subir» a secas.
func TestConElDriveEnLaBovedaSinAbrirSeDiceQueHacer(t *testing.T) {
	g, servidor := nuevaGaleria(t), &driveFalso{}
	a := appConCopia(t, g, servidor)
	a.DriveDeSubida = func(context.Context) (*api.Drive, error) { return nil, ErrDriveCifradoCerrado }
	a.Arrancar()
	ruta := filepath.Join(t.TempDir(), "IMG_0002.jpg")
	if err := os.WriteFile(ruta, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.subir([]string{ruta})
	quiero := a.v.Traduce("gallery.uploadVaultLocked")
	esperar(t, "no se dice que hay que desbloquear el Drive", func() bool { return a.g.subida == nil && ultimoAviso(a).Titulo == quiero })
	servidor.mu.Lock()
	defer servidor.mu.Unlock()
	if len(servidor.subidos) != 0 {
		t.Fatalf("subió al servidor: %v", servidor.subidos)
	}
}

func TestMotivoSubida(t *testing.T) {
	cifrada := &api.Error{Status: http.StatusConflict, Body: `{"error":{"code":"private_storage_migration_required"}}`}
	for err, quiero := range map[error]string{
		ErrDriveCifradoCerrado: "gallery.uploadVaultLocked",
		errors.Join(ErrDriveCifradoNoDisponible, errors.New("sin diario")): "gallery.uploadVaultUnavailable",
		cifrada: "gallery.uploadVaultLocked",
		&api.Error{Status: http.StatusConflict, Body: `{"error":{"code":"name_conflict"}}`}: "",
		errors.New("red"): "",
	} {
		if got := motivoSubida(err); got != quiero {
			t.Errorf("motivoSubida(%v) = %q, quiero %q", err, got, quiero)
		}
	}
	if !errors.Is(errors.Join(ErrNoIndexable, ErrEnBoveda), ErrNoIndexable) {
		t.Fatal("una subida cifrada tiene que contar como hecha para la copia")
	}
}
