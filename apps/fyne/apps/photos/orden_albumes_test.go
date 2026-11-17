// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los mismos casos que comprueba la web (`ordenAlbumes.test.ts`): las dos apps ordenan
// igual la misma lista.
func TestOrdenAlbumesComoLaWeb(t *testing.T) {
	datos, err := os.ReadFile("testdata/orden-albumes.json")
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Albums []api.Album `json:"albums"`
		Casos  []struct {
			Orden  OrdenAlbum `json:"orden"`
			Locale string     `json:"locale"`
			IDs    []string   `json:"ids"`
		} `json:"casos"`
	}
	if err := json.Unmarshal(datos, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Casos) != len(OrdenesAlbum) {
		t.Fatalf("hay %d casos para %d órdenes", len(c.Casos), len(OrdenesAlbum))
	}
	antes := idsDeAlbumes(c.Albums)
	for _, caso := range c.Casos {
		got := idsDeAlbumes(OrdenarAlbumes(c.Albums, caso.Orden, i18n.Locale(caso.Locale)))
		if !slices.Equal(got, caso.IDs) {
			t.Errorf("%s: %v, se esperaba %v", caso.Orden, got, caso.IDs)
		}
	}
	if !slices.Equal(idsDeAlbumes(c.Albums), antes) {
		t.Error("OrdenarAlbumes tocó la lista de entrada")
	}
	if !EsOrdenAlbum("count") || EsOrdenAlbum("size") || EsOrdenAlbum("") {
		t.Error("EsOrdenAlbum reconoce lo que no es un orden")
	}
}

func idsDeAlbumes(l []api.Album) []string {
	out := make([]string, len(l))
	for i, a := range l {
		out[i] = a.ID
	}
	return out
}
