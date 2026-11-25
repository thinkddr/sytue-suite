// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

func docusPlan(ctx context.Context, resources *cifrado.Resources[*mx.Client], suite *sesion.Suite) (cifrado.ApplicationPlan[*office.Repository], error) {
	var empty cifrado.ApplicationPlan[*office.Repository]
	if resources == nil || suite == nil {
		return empty, cifrado.ErrPrivateResources
	}
	source, err := office.NewDocsSource(api.NewDocs(suite.URL("docs"), resources.HTTP(), nil))
	if err != nil {
		return empty, err
	}
	return cifrado.NewRecordApplicationPlan(ctx, resources, cifrado.RecordApplicationConfig[*office.Repository]{
		Gateway:    api.NewDrive(suite.URL("drive"), resources.HTTP(), nil),
		Source:     source,
		Service:    api.NewRooms(suite.URL("rooms"), resources.HTTP(), nil),
		New:        office.New,
		Historical: office.NewHistoricalSource,
		OnWrite:    suite.AvisarEscritura,
	})
}
