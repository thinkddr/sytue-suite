// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package catalogo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"testing"

	"git.sytue.eu/sytue/one/services/pages/internal/testutil"
)

func TestMaterializarCatalogoVerificaBytesReparaYFalla(t *testing.T) {
	cat, err := CargarPorDefecto()
	if err != nil {
		t.Fatal(err)
	}
	almacen := testutil.NuevoAlmacen(t)
	ctx := context.Background()
	if err := cat.Materializar(ctx, almacen.Objetos); err != nil {
		t.Fatal(err)
	}
	for nombre, rec := range cat.Recursos() {
		got := almacen.Leer(rec.Clave)
		hash := sha256.Sum256(got)
		if !bytes.Equal(got, rec.Contenido) || hex.EncodeToString(hash[:16])+path.Ext(nombre) != nombre {
			t.Fatalf("bytes/hash incorrecto: %s len=%d want=%d inicio=%q", nombre, len(got), len(rec.Contenido), got[:min(120, len(got))])
		}
	}
	escritos := almacen.Escrituras()
	if escritos != len(cat.Recursos()) {
		t.Fatalf("escrituras=%d", escritos)
	}
	if err := cat.Materializar(ctx, almacen.Objetos); err != nil {
		t.Fatal(err)
	}
	if almacen.Escrituras() != escritos {
		t.Fatal("segunda inicialización duplica bytes")
	}
	for _, rec := range cat.Recursos() {
		almacen.Escribir(rec.Clave, []byte("corrupto"))
		break
	}
	if err := cat.Materializar(ctx, almacen.Objetos); err != nil {
		t.Fatal(err)
	}
	if almacen.Escrituras() != escritos+1 {
		t.Fatal("objeto corrupto no reparado")
	}
	almacen.Fallar(true)
	if err := cat.Materializar(ctx, almacen.Objetos); err == nil {
		t.Fatal("almacén caído anunciado como materializado")
	}
}
