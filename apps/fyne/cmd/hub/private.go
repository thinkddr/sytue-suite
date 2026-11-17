// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateHub "git.sytue.eu/sytue/one/apps/nucleo/hub"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

func hubPlan(ctx context.Context, resources *cifrado.Resources[*mx.Client], suite *sesion.Suite) (cifrado.ApplicationPlan[*privateHub.Repository], error) {
	var empty cifrado.ApplicationPlan[*privateHub.Repository]
	if resources == nil || suite == nil {
		return empty, cifrado.ErrPrivateResources
	}
	source, err := privateHub.NewSource(api.NewHub(suite.URL("hub"), resources.HTTP(), nil))
	if err != nil {
		return empty, err
	}
	return cifrado.NewRecordApplicationPlan(ctx, resources, cifrado.RecordApplicationConfig[*privateHub.Repository]{
		Gateway:    api.NewDrive(suite.URL("drive"), resources.HTTP(), nil),
		Source:     source,
		Service:    api.NewRooms(suite.URL("rooms"), resources.HTTP(), nil),
		New:        privateHub.New,
		Historical: privateHub.NewHistoricalSource,
	})
}
