// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Lo que puede salir mal al compartir: borrar la subida propia sin haber
// asegurado la compartida (pérdida de datos), o borrar de S3 una clave
// compartida al purgar una fila (pérdida para otra organización).
func TestCompartirBlob(t *testing.T) {
	ctx := context.Background()
	acme := tenancy.Tenant{Slug: "acme"}
	hash := "ab12"
	clave := "shared/drive/acme/" + hash

	t.Run("primera vez: copia a la clave por contenido y borra la propia", func(t *testing.T) {
		fb := &fakeBlobs{existen: map[string]bool{}}
		s := &Service{blobs: fb}
		got := s.compartirBlob(ctx, acme, "files/x/propia", hash)
		if got != clave {
			t.Fatalf("clave = %q", got)
		}
		if len(fb.copias) != 1 || fb.copias[0] != "files/x/propia→"+clave {
			t.Fatalf("copias = %v", fb.copias)
		}
		if len(fb.delKeys) != 1 || fb.delKeys[0] != "files/x/propia" {
			t.Fatalf("borrados = %v", fb.delKeys)
		}
	})

	t.Run("ya existía: no copia, solo suelta la propia", func(t *testing.T) {
		fb := &fakeBlobs{existen: map[string]bool{clave: true}}
		s := &Service{blobs: fb}
		if got := s.compartirBlob(ctx, acme, "files/x/propia", hash); got != clave {
			t.Fatalf("clave = %q", got)
		}
		if len(fb.copias) != 0 || len(fb.delKeys) != 1 {
			t.Fatalf("copias=%v borrados=%v", fb.copias, fb.delKeys)
		}
	})

	// Decisión del dueño (2026-10-16): que otra organización tenga el mismo
	// fichero no se nota. Ni la clave común de antes ni la de otra organización
	// cuentan como «ya existía»: se copia, como la primera vez.
	t.Run("lo de otra organización no cuenta: copia a la clave de la suya", func(t *testing.T) {
		fb := &fakeBlobs{existen: map[string]bool{"shared/drive/" + hash: true, "shared/drive/otra/" + hash: true}}
		s := &Service{blobs: fb}
		if got := s.compartirBlob(ctx, acme, "files/x/propia", hash); got != clave {
			t.Fatalf("clave = %q; quiero %q", got, clave)
		}
		if len(fb.copias) != 1 {
			t.Fatalf("debía copiar, porque en su organización no existía: copias = %v", fb.copias)
		}
	})

	t.Run("si la copia falla, se queda con la propia y no borra nada", func(t *testing.T) {
		fb := &fakeBlobs{existen: map[string]bool{}, copiaErr: errors.New("s3")}
		s := &Service{blobs: fb}
		if got := s.compartirBlob(ctx, acme, "files/x/propia", hash); got != "files/x/propia" {
			t.Fatalf("clave = %q", got)
		}
		if len(fb.delKeys) != 0 {
			t.Fatalf("no debía borrar la subida propia: %v", fb.delKeys)
		}
	})

	t.Run("sin hash no hay nada que compartir", func(t *testing.T) {
		fb := &fakeBlobs{}
		s := &Service{blobs: fb}
		if got := s.compartirBlob(ctx, acme, "files/x/propia", ""); got != "files/x/propia" {
			t.Fatalf("clave = %q", got)
		}
	})
}

func TestSoltarBlobNoBorraLasCompartidasEnElActo(t *testing.T) {
	ctx := context.Background()
	fb := &fakeBlobs{}
	s := &Service{blobs: fb} // sin store de mantenimiento: la compartida solo se anota
	s.soltarBlob(ctx, tenancy.Tenant{Slug: "acme"}, "files/x/propia")
	s.soltarBlob(ctx, tenancy.Tenant{Slug: "acme"}, "shared/drive/ab12")
	if len(fb.delKeys) != 1 || fb.delKeys[0] != "files/x/propia" {
		t.Fatalf("solo la propia se borra en el acto: %v", fb.delKeys)
	}
}
