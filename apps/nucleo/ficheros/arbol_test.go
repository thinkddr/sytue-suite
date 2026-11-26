// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"errors"
	"io/fs"
	"testing"
	"time"
)

// Lo que contesta ArbolDocumentos (apps/fyne/android/java), tal como lo escribe Java: la
// prueba de Java (ArbolDocumentosPrueba) comprueba el otro lado.
func TestLeerLoQueContestaArbolDocumentos(t *testing.T) {
	e, err := leerElegido("content://com.android.externalstorage.documents/tree/primary%3ADownload\nDownload\nrw")
	if err != nil || e.Nombre != "Download" || !e.Persistente || e.URI != "content://com.android.externalstorage.documents/tree/primary%3ADownload" {
		t.Fatal(e, err)
	}
	if e, err := leerElegido("content://x/tree/y\nY\nt"); err != nil || e.Persistente {
		t.Fatal("permiso temporal", e, err)
	}
	// Solo lectura: el error, y lo elegido, que aún sirve para leer (subir una carpeta).
	if e, err := leerElegido("content://x/tree/y\nY\nr"); !errors.Is(err, ErrSoloLectura) || !e.SoloLectura || e.URI != "content://x/tree/y" || e.Persistente {
		t.Fatal("solo lectura", e, err)
	}
	if e, err := leerElegido(""); err != nil || e.URI != "" {
		t.Fatal("cancelar", e, err)
	}
	if _, err := leerElegido("!"); !errors.Is(err, ErrNoDisponible) {
		t.Fatal("sin ventana", err)
	}
	if _, err := leerElegido("basura"); err == nil {
		t.Fatal("ilegible")
	}

	d, err := leerDocumento(`primary:Download/a\tb` + "\t0\t4\t1764064800000\t" + `a\tb\nc\\d.txt`)
	if err != nil {
		t.Fatal(err)
	}
	quiero := DocumentoArbol{ID: "primary:Download/a\tb", Nombre: "a\tb\nc\\d.txt", Tamano: 4, Modificado: time.UnixMilli(1764064800000)}
	if d != quiero {
		t.Fatalf("%+v", d)
	}
	ds, err := leerDocumentos("x\t1\t-1\t0\tCarpeta\ny\t0\t0\t0\tvacío")
	if err != nil || len(ds) != 2 || !ds[0].Carpeta || ds[0].Tamano != -1 || !ds[0].Modificado.IsZero() || ds[1].Nombre != "vacío" {
		t.Fatal(ds, err)
	}
	if ds, err := leerDocumentos(""); err != nil || ds != nil {
		t.Fatal("carpeta vacía", ds, err)
	}
	if _, err := leerDocumento("a\tb"); err == nil {
		t.Fatal("línea corta")
	}

	if total, libre, err := leerEspacio("1000\t400"); err != nil || total != 1000 || libre != 400 {
		t.Fatal(total, libre, err)
	}
	if _, _, err := leerEspacio(""); !errors.Is(err, ErrNoDisponible) {
		t.Fatal("espacio desconocido", err)
	}

	for crudo, quiero := range map[string]error{
		"java.io.FileNotFoundException: no está":                 fs.ErrNotExist,
		"java.lang.UnsupportedOperationException: Move not supp": ErrNoSoportado,
		"java.lang.SecurityException: Permission Denial":         fs.ErrPermission,
	} {
		if err := errorJava(errors.New(crudo)); !errors.Is(err, quiero) {
			t.Fatal(crudo, err)
		}
	}
	if errorJava(nil) != nil {
		t.Fatal("nil")
	}
}

func TestArbolFueraDeAndroidNoEsta(t *testing.T) {
	if _, err := ElegirArbol(); !errors.Is(err, ErrNoDisponible) {
		t.Fatal(err)
	}
	if ArbolVigente("content://x/tree/y") {
		t.Fatal("vigente fuera de Android")
	}
	if _, err := (Arbol{URI: "content://x/tree/y"}).Raiz(); !errors.Is(err, ErrNoDisponible) {
		t.Fatal(err)
	}
}
