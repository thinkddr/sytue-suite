// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

func calendarPlan(ctx context.Context, resources *cifrado.Resources[*mx.Client], suite *sesion.Suite) (cifrado.ApplicationPlan[*privateCalendar.Repository], error) {
	var empty cifrado.ApplicationPlan[*privateCalendar.Repository]
	if resources == nil || suite == nil {
		return empty, cifrado.ErrPrivateResources
	}
	source, err := privateCalendar.NewSource(api.NewCalendars(suite.URL("calendar"), resources.HTTP(), nil))
	if err != nil {
		return empty, err
	}
	return cifrado.NewRecordApplicationPlan(ctx, resources, cifrado.RecordApplicationConfig[*privateCalendar.Repository]{
		Gateway: api.NewDrive(suite.URL("drive"), resources.HTTP(), nil), Source: source,
		Service: api.NewRooms(suite.URL("rooms"), resources.HTTP(), nil), New: privateCalendar.New,
		Historical: privateCalendar.NewHistoricalSource,
	})
}
