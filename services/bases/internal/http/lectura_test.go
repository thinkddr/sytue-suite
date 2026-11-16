// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

func TestLecturaAdHoc(t *testing.T) {
	v := url.Values{"q": {"  hola "}, "sort": {"f1:desc, f2"}}
	got, ok := lecturaAdHoc(v)
	if !ok || got.Search != "hola" || len(got.Sort) != 2 ||
		got.Sort[0] != (domain.Orden{Field: "f1", Dir: "desc"}) || got.Sort[1] != (domain.Orden{Field: "f2", Dir: "asc"}) {
		t.Fatalf("lectura mal interpretada: %+v %v", got, ok)
	}
	for _, mal := range []url.Values{
		{"sort": {"f1:arriba"}},
		{"sort": {":desc"}},
		{"sort": {"a,b,c,d"}},
		{"q": {strings.Repeat("x", maxBusqueda+1)}},
	} {
		if _, ok := lecturaAdHoc(mal); ok {
			t.Errorf("debería rechazarse: %v", mal)
		}
	}
	if got, ok := lecturaAdHoc(url.Values{}); !ok || got.Search != "" || got.Sort != nil {
		t.Errorf("sin parámetros no se pide nada: %+v", got)
	}
}
