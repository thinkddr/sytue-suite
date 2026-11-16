// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateContacts "git.sytue.eu/sytue/one/apps/nucleo/contacts"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

func contactsPlan(ctx context.Context, resources *cifrado.Resources[*mx.Client], suite *sesion.Suite) (cifrado.ApplicationPlan[*privateContacts.Repository], error) {
	var empty cifrado.ApplicationPlan[*privateContacts.Repository]
	source, err := privateContacts.NewSource(api.NewContacts(suite.URL("contacts"), resources.HTTP(), nil))
	if err != nil {
		return empty, err
	}
	return cifrado.NewRecordApplicationPlan(ctx, resources, cifrado.RecordApplicationConfig[*privateContacts.Repository]{
		Gateway: api.NewDrive(suite.URL("drive"), resources.HTTP(), nil), Source: source,
		Service: api.NewRooms(suite.URL("rooms"), resources.HTTP(), nil), New: privateContacts.New, Historical: privateContacts.NewHistoricalSource,
	})
}
