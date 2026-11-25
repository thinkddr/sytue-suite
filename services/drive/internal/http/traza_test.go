// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
)

// Cada regla de la traza casa con una ruta que existe. Una ruta renombrada no
// rompe nada visible: deja de anotarse en silencio, y una auditoría que calla
// es peor que no tenerla.
func TestReglasDeTrazaExisten(t *testing.T) {
	r, ok := NewRouter(Deps{
		Cfg:          config.Config{PublicURL: "https://drive.sytue.eu"},
		AppPasswords: authn.NewAppPasswords("http://127.0.0.1:1", "secreto-de-prueba"),
		WebDAV:       http.NotFoundHandler(),
		Traza:        auditlog.NuevoRegistro(nil, nil),
	}).(chi.Routes)
	if !ok {
		t.Fatal("el router de Drive no es un chi.Routes")
	}
	if faltan := auditlog.ReglasSinRuta(r, ReglasDeTraza); len(faltan) > 0 {
		t.Fatalf("reglas de la traza sin ruta: %+v", faltan)
	}
}
