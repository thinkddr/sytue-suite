// SPDX-License-Identifier: Apache-2.0
package calendar

import (
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// NewHistoricalSource uses Calendar's complete graph validation while exposing
// only the common read/prepare lifecycle. Historical CRUD is never mounted.
func NewHistoricalSource(cfg Config) (*records.HistoricalSource, error) {
	configuration, err := recordsConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	return records.NewHistoricalSource(configuration)
}

func (r *Repository) IdentityCutover(cfg records.IdentityCutoverConfig) (*records.IdentityCutover, error) {
	if r == nil || r.core == nil || cfg.Scope != r.scope || cfg.Isolated != nil {
		return nil, model.ErrSnapshot
	}
	cfg.Isolated = r.core
	return records.NewIdentityCutover(cfg)
}
