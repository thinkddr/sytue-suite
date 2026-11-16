// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// TestFileCursorRoundTripPorOrden: el token lleva su orden y la clave de ese
// orden sin perder precisión (la fecha, al microsegundo de timestamptz).
func TestFileCursorRoundTripPorOrden(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	fecha := time.Date(2026, 11, 9, 10, 11, 12, 345678000, time.UTC)
	casos := []store.FileCursor{
		{Orden: store.OrdenFicheros{Campo: store.OrdenNombre, Desc: true}, NameLower: "zeta.txt", ID: id},
		{Orden: store.OrdenFicheros{Campo: store.OrdenModificado, Desc: true}, UpdatedAt: fecha, ID: id},
		{Orden: store.OrdenFicheros{Campo: store.OrdenModificado}, UpdatedAt: fecha, ID: id},
		{Orden: store.OrdenFicheros{Campo: store.OrdenTamano}, SizeBytes: 1 << 40, ID: id},
		{Orden: store.OrdenFicheros{Campo: store.OrdenTamano, Desc: true}, SizeBytes: 0, ID: id},
	}
	for _, c := range casos {
		back, err := decodeFileCursor(encodeFileCursor(c))
		if err != nil {
			t.Fatalf("%+v: %v", c.Orden, err)
		}
		if back != c {
			t.Fatalf("roundtrip %+v:\n got %+v\nwant %+v", c.Orden, back, c)
		}
	}
}

// TestCursorAntiguoEsPorNombre: un token emitido antes del orden en el servicio
// (solo `n` e `i`) sigue valiendo y es por nombre ascendente.
func TestCursorAntiguoEsPorNombre(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	tok := base64.RawURLEncoding.EncodeToString([]byte(`{"n":"b.txt","i":"` + id.String() + `"}`))
	c, err := cursorEnOrden(tok, store.OrdenFicheros{})
	if err != nil {
		t.Fatalf("cursor antiguo: %v", err)
	}
	if c.Orden != (store.OrdenFicheros{Campo: store.OrdenNombre}) || c.NameLower != "b.txt" || c.ID != id {
		t.Fatalf("cursor antiguo = %+v", c)
	}
}

// TestListFilesPageOrdenEnElCursor: el orden pedido llega al store, el cursor de
// la página siguiente lo conserva y cambiar de orden a mitad es entrada inválida.
func TestListFilesPageOrdenEnElCursor(t *testing.T) {
	porFecha := store.OrdenFicheros{Campo: store.OrdenModificado, Desc: true}
	fecha := time.Date(2026, 11, 9, 8, 0, 0, 123000, time.UTC)
	next := store.FileCursor{Orden: porFecha, UpdatedAt: fecha, ID: uuid.New()}
	fs := &fakeStore{page: store.FilePage{Files: []domain.File{{ID: uuid.New()}}, Next: &next}}
	s := newSvc(fs, &fakeBlobs{}, nil)

	page, err := s.ListFilesPage(tenantCtx(t), uuid.New(), porFecha, "", 10)
	if err != nil {
		t.Fatalf("primera página: %v", err)
	}
	if fs.lastAfter != (store.FileCursor{Orden: porFecha}) {
		t.Fatalf("primera página al store = %+v, quiero el inicio del orden por fecha", fs.lastAfter)
	}
	if _, err := s.ListFilesPage(tenantCtx(t), uuid.New(), porFecha, page.NextCursor, 10); err != nil {
		t.Fatalf("página siguiente: %v", err)
	}
	if fs.lastAfter != next {
		t.Fatalf("página siguiente al store = %+v, quiero %+v", fs.lastAfter, next)
	}
	_, err = s.ListFilesPage(tenantCtx(t), uuid.New(), store.OrdenFicheros{Campo: store.OrdenTamano}, page.NextCursor, 10)
	if !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("cursor de otro orden: %v, quiero ErrInvalidName", err)
	}
}

// TestOrdenarCarpetas: las subcarpetas siguen la regla de los ficheros; por
// tamaño (no tienen) se quedan por nombre ascendente.
func TestOrdenarCarpetas(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	// Llegan por nombre ascendente, como las devuelve el store.
	nuevas := func() []domain.Folder {
		return []domain.Folder{
			{Name: "a", UpdatedAt: base.Add(2 * time.Hour)},
			{Name: "b", UpdatedAt: base},
			{Name: "c", UpdatedAt: base.Add(2 * time.Hour)},
		}
	}
	nombres := func(fs []domain.Folder) string {
		s := ""
		for _, f := range fs {
			s += f.Name
		}
		return s
	}
	casos := []struct {
		orden store.OrdenFicheros
		want  string
	}{
		{store.OrdenFicheros{}, "abc"},
		{store.OrdenFicheros{Campo: store.OrdenNombre, Desc: true}, "cba"},
		// A igual fecha, el nombre (estable).
		{store.OrdenFicheros{Campo: store.OrdenModificado}, "bac"},
		{store.OrdenFicheros{Campo: store.OrdenModificado, Desc: true}, "acb"},
		{store.OrdenFicheros{Campo: store.OrdenTamano, Desc: true}, "abc"},
	}
	for _, c := range casos {
		fs := nuevas()
		OrdenarCarpetas(fs, c.orden)
		if got := nombres(fs); got != c.want {
			t.Errorf("%+v: %q, quiero %q", c.orden, got, c.want)
		}
	}
}
