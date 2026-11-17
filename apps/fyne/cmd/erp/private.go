// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateERP "git.sytue.eu/sytue/one/apps/nucleo/erp"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

func erpPlan(ctx context.Context, resources *cifrado.Resources[*mx.Client], suite *sesion.Suite, idioma string) (cifrado.ApplicationPlan[*privateERP.Repository], error) {
	var empty cifrado.ApplicationPlan[*privateERP.Repository]
	if resources == nil || suite == nil {
		return empty, cifrado.ErrPrivateResources
	}
	source, err := privateERP.NewSource(api.NewERP(suite.URL("erp"), resources.HTTP(), nil, idioma))
	if err != nil {
		return empty, err
	}
	return cifrado.NewRecordApplicationPlan(ctx, resources, cifrado.RecordApplicationConfig[*privateERP.Repository]{
		Gateway:    api.NewDrive(suite.URL("drive"), resources.HTTP(), nil),
		Source:     source,
		Service:    api.NewRooms(suite.URL("rooms"), resources.HTTP(), nil),
		New:        privateERP.New,
		Historical: privateERP.NewHistoricalSource,
	})
}
