// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCast "git.sytue.eu/sytue/one/apps/nucleo/cast"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

// castPlan traslada el catálogo completo del servicio real —vídeos, originales,
// pósteres y topología HLS— al repositorio privado del dispositivo verificado con
// el mismo corte de identidad que las demás apps de records.
func castPlan(ctx context.Context, resources *cifrado.Resources[*mx.Client], suite *sesion.Suite) (cifrado.ApplicationPlan[*privateCast.Repository], error) {
	var empty cifrado.ApplicationPlan[*privateCast.Repository]
	if resources == nil || suite == nil {
		return empty, cifrado.ErrPrivateResources
	}
	source, err := privateCast.NewHTTPSource(api.NewCast(suite.URL("cast"), resources.HTTP(), nil))
	if err != nil {
		return empty, err
	}
	return cifrado.NewRecordApplicationPlan(ctx, resources, cifrado.RecordApplicationConfig[*privateCast.Repository]{
		Gateway: api.NewDrive(suite.URL("drive"), resources.HTTP(), nil), Source: source,
		Service: api.NewRooms(suite.URL("rooms"), resources.HTTP(), nil), New: privateCast.New,
		Historical: privateCast.NewHistoricalSource, OnWrite: suite.AvisarEscritura,
	})
}
