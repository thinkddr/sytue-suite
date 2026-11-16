// SPDX-License-Identifier: Apache-2.0
package photos

import "git.sytue.eu/sytue/one/apps/nucleo/storage/records"

// NewHistoricalSource fixes the same complete domain validation as New, while
// exposing only the shared source migration lifecycle, never historical CRUD.
func NewHistoricalSource(cfg Config) (*records.HistoricalSource, error) {
	configuration := recordsConfiguration(cfg)
	return records.NewHistoricalSource(configuration)
}

// IdentityCutover binds the shared migration to this exact private repository.
// Callers cannot substitute another target or weaken the domain's validators.
func (r *Repository) IdentityCutover(cfg records.IdentityCutoverConfig) (*records.IdentityCutover, error) {
	if r == nil || r.engine == nil || cfg.Scope != r.scope || cfg.Isolated != nil {
		return nil, ErrSource
	}
	cfg.Isolated = r.engine
	return records.NewIdentityCutover(cfg)
}
