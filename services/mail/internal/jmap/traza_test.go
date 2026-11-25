// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// Cada regla de la traza casa con una ruta que existe: una ruta renombrada
// dejaría de anotarse en silencio.
func TestReglasDeTrazaExisten(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	r := chi.NewRouter()
	a.Mount(r, nil)
	if faltan := auditlog.ReglasSinRuta(r, ReglasDeTraza); len(faltan) > 0 {
		t.Fatalf("reglas de la traza sin ruta: %+v", faltan)
	}
}
