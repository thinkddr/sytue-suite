// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateBases "git.sytue.eu/sytue/one/apps/nucleo/bases"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

func basesPlan(ctx context.Context, resources *cifrado.Resources[*mx.Client], suite *sesion.Suite) (cifrado.ApplicationPlan[*privateBases.Repository], error) {
	var empty cifrado.ApplicationPlan[*privateBases.Repository]
	if resources == nil || suite == nil {
		return empty, cifrado.ErrPrivateResources
	}
	source, err := privateBases.NewSource(api.NewBases(suite.URL("bases"), resources.HTTP(), nil))
	if err != nil {
		return empty, err
	}
	return cifrado.NewRecordApplicationPlan(ctx, resources, cifrado.RecordApplicationConfig[*privateBases.Repository]{
		Gateway:    api.NewDrive(suite.URL("drive"), resources.HTTP(), nil),
		Source:     source,
		Service:    api.NewRooms(suite.URL("rooms"), resources.HTTP(), nil),
		New:        privateBases.New,
		Historical: privateBases.NewHistoricalSource,
	})
}
