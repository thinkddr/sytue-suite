// SPDX-License-Identifier: Apache-2.0
package web

import (
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

// Native SDK values retain their CAS revision in memory. The browser editor
// needs an explicit opaque field because those native fields are json:"-".
// It grants no authority: the repository compares it with the current record.
type filterEditor struct {
	api.MailFilter
	Revision string `json:"revision"`
}

func editFilter(value api.MailFilter) filterEditor {
	return filterEditor{MailFilter: value, Revision: value.PrivateRevision}
}

func (e filterEditor) value() (api.MailFilter, error) {
	if e.Revision != "" && !webport.ValidID(e.Revision) {
		return api.MailFilter{}, webport.ErrRejected
	}
	value := e.MailFilter
	value.PrivateRevision = e.Revision
	return value, nil
}

type vacationEditor struct {
	api.MailVacation
	Revision string `json:"revision"`
}

func editVacation(value api.MailVacation) vacationEditor {
	return vacationEditor{MailVacation: value, Revision: value.PrivateRevision}
}

func (e vacationEditor) value() (api.MailVacation, error) {
	if e.Revision != "" && !webport.ValidID(e.Revision) {
		return api.MailVacation{}, webport.ErrRejected
	}
	value := e.MailVacation
	value.PrivateRevision = e.Revision
	return value, nil
}
