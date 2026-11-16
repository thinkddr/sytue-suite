// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

func NewPlannedHTTPSource(scope storage.Scope, client *api.Photos, plans *SourcePlanStore) (*PlannedSource, error) {
	if client == nil {
		return nil, ErrSourcePlan
	}
	backend, err := records.NewAppSource(client)
	if err != nil {
		return nil, err
	}
	return NewPlannedSource(scope, backend, client, plans)
}
