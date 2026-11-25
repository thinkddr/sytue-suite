// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
	chathttp "git.sytue.eu/sytue/one/services/rooms/internal/http"
	mediahttp "git.sytue.eu/sytue/one/services/rooms/internal/media/http"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

// Cada regla de la traza casa con una ruta del servicio entero —mensajería y
// videollamada, como se monta en cmd/rooms—, y la lectura admin se monta: una
// ruta renombrada dejaría de anotarse en silencio.
func TestReglasDeTrazaExisten(t *testing.T) {
	r, ok := chathttp.NewRouter(chathttp.Deps{
		Service:    &service.Service{},
		Traza:      auditlog.NuevoRegistro(nil, nil),
		MountExtra: func(api chi.Router) { mediahttp.MountAPI(api, mediahttp.Deps{}) },
	}).(chi.Routes)
	if !ok {
		t.Fatal("el router de Rooms no es un chi.Routes")
	}
	reglas := append([]auditlog.Regla{{Metodo: http.MethodGet, Ruta: "/api/v1/admin/audit-log"}}, chathttp.ReglasDeTraza...)
	reglas = append(reglas, chathttp.ReglasDeTrazaDeLlamadas...)
	if faltan := auditlog.ReglasSinRuta(r, reglas); len(faltan) > 0 {
		t.Fatalf("reglas de la traza sin ruta: %+v", faltan)
	}
}
