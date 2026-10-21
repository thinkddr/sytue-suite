// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package documentos

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// doble es un Drive en memoria: una raíz «r» con la carpeta «c1» y el fichero «f1».
type doble struct {
	contenido map[string]string
	subidas   []string // carpeta/nombre=contenido
	borrados  []string
	nombres   map[string]string
}

func nuevoDoble() *doble {
	return &doble{contenido: map[string]string{"f1": "hola"}, nombres: map[string]string{}}
}

var padreR = "r"

func (d *doble) Root(context.Context) (api.Folder, error) {
	return api.Folder{ID: "r", Name: "Mi unidad", IsRoot: true}, nil
}

func (d *doble) Children(_ context.Context, carpeta, cursor string) (api.Children, error) {
	if carpeta != "r" {
		return api.Children{}, errors.New("carpeta desconocida")
	}
	if cursor == "" { // dos páginas: que se sigan
		return api.Children{Folders: []api.Folder{{ID: "c1", Name: "Facturas", ParentID: &padreR}}, NextCursor: "2"}, nil
	}
	return api.Children{Files: []api.File{{ID: "f1", FolderID: "r", Name: "a.txt", SizeBytes: 4, ContentType: "text/plain"}}}, nil
}

func (d *doble) File(_ context.Context, id string) (api.File, error) {
	if id != "f1" {
		return api.File{}, errors.New("no existe")
	}
	return api.File{ID: "f1", FolderID: "r", Name: "a.txt", ContentType: "text/plain"}, nil
}

func (d *doble) Folder(_ context.Context, id string) (api.Folder, error) {
	switch id {
	case "c1":
		return api.Folder{ID: "c1", Name: "Facturas", ParentID: &padreR}, nil
	case "r":
		return api.Folder{ID: "r", Name: "Mi unidad", IsRoot: true}, nil
	}
	return api.Folder{}, errors.New("no existe")
}

func (d *doble) Download(_ context.Context, id string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(d.contenido[id])), nil
}

func (d *doble) Upload(_ context.Context, carpeta, nombre, _ string, cuerpo io.Reader, _ int64) (api.File, error) {
	b, _ := io.ReadAll(cuerpo)
	d.subidas = append(d.subidas, carpeta+"/"+nombre+"="+string(b))
	return api.File{ID: "f9", FolderID: carpeta, Name: nombre}, nil
}

func (d *doble) CreateFolder(_ context.Context, padre, nombre string) (api.Folder, error) {
	return api.Folder{ID: "c9", ParentID: &padre, Name: nombre}, nil
}

func (d *doble) Rename(_ context.Context, _ api.DriveKind, id, nombre string) error {
	d.nombres[id] = nombre
	return nil
}

func (d *doble) Delete(_ context.Context, _ api.DriveKind, id string) error {
	d.borrados = append(d.borrados, id)
	return nil
}

func (d *doble) Search(context.Context, string) (api.Children, error) {
	return api.Children{Files: []api.File{{ID: "f1", Name: "a.txt"}}}, nil
}

// TestSincronizarCarpeta: sincronización bidireccional entre Sytue Drive y la carpeta local
// (en iOS ~/Documents, expuesta en la app «Archivos»).
func TestSincronizarCarpeta(t *testing.T) {
	ctx := context.Background()
	d := nuevoDoble()
	dir := t.TempDir()

	// 1. Primera sincronización: baja f1 ("a.txt") y crea la carpeta "Facturas"
	if err := Sincronizar(ctx, d, dir); err != nil {
		t.Fatalf("primera sincronización: %v", err)
	}

	ficLocal := filepath.Join(dir, "a.txt")
	b, err := os.ReadFile(ficLocal)
	if err != nil || string(b) != "hola" {
		t.Fatalf("a.txt no se descargó correctamente: %q, err=%v", string(b), err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "Facturas")); err != nil || !fi.IsDir() {
		t.Fatalf("Facturas no se creó localmente: %v", err)
	}

	// 2. Modificación local (in-place en «Archivos»): debe subir la nueva versión
	// Cambiamos tiempo de modificación para garantizar detección
	if err := os.WriteFile(ficLocal, []byte("edición in-place"), 0o600); err != nil {
		t.Fatal(err)
	}
	ahora := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(ficLocal, ahora, ahora)

	if err := Sincronizar(ctx, d, dir); err != nil {
		t.Fatalf("segunda sincronización tras edición: %v", err)
	}
	if !slices.Contains(d.subidas, "r/a.txt=edición in-place") {
		t.Errorf("la edición local no se subió: %v", d.subidas)
	}

	// 3. Fichero nuevo añadido localmente en «Archivos»
	nuevoFic := filepath.Join(dir, "Facturas", "recibo.txt")
	if err := os.WriteFile(nuevoFic, []byte("total 42€"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Sincronizar(ctx, d, dir); err != nil {
		t.Fatalf("tercera sincronización tras añadir fichero: %v", err)
	}
	if !slices.Contains(d.subidas, "c1/recibo.txt=total 42€") {
		t.Errorf("el fichero nuevo no se subió a su carpeta: %v", d.subidas)
	}

	// 4. Borrado local: debe borrar en remoto
	if err := os.Remove(ficLocal); err != nil {
		t.Fatal(err)
	}
	if err := Sincronizar(ctx, d, dir); err != nil {
		t.Fatalf("cuarta sincronización tras borrado: %v", err)
	}
	if !slices.Contains(d.borrados, "f1") {
		t.Errorf("el fichero borrado localmente no se borró en remoto: %v", d.borrados)
	}
}
