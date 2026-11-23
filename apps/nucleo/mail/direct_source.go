// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/limits"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// El correo sobre la bóveda post-cuántica (ADR 0087) no tiene identidad 1 de
// Matrix de la que migrar: el destino de la importación inicial del correo del
// servidor es directamente su propio almacén. Son las mismas piezas que el
// origen histórico (la época se fija en el diario antes de pedir nada al
// servidor, el recibo se publica el último y todo se comprueba antes de
// exponer un buzón), pero sin repositorio intermedio ni corte de identidad.

// CaptureDirectSource fija y, si hace falta, empieza en el servidor la época del
// correo retenido para ESTE destino. Con el servidor aún en `legacy`, el almacén
// tiene que estar vacío: nunca se mezcla una importación con datos que ya estaban.
// Devuelve el estado aceptado, con el que se construye el repositorio (New).
func CaptureDirectSource(parent context.Context, cfg Config, source HistoricalSourceEnrollment) (storage.LegacyMigrationState, error) {
	if !validDirectSourceConfig(cfg) || source == nil {
		return storage.LegacyMigrationState{}, ErrRecord
	}
	maximum, err := limits.MessageBytes(cfg.MaxMessageBytes)
	if err != nil {
		return storage.LegacyMigrationState{}, ErrRecord
	}
	cfg.MaxMessageBytes = maximum
	gate := offline.NewWorkGateWithLease(cfg.Context, cfg.WorkLease) //nolint:contextcheck // Admission inherits both the repository lifetime and parent via Start.
	defer gate.Close()                                               //nolint:contextcheck // Close cancels this temporary gate and waits for its retained permission.
	ctx, done, err := gate.Start(parent)
	if err != nil {
		return storage.LegacyMigrationState{}, err
	}
	defer done()
	return captureHistoricalSource(ctx, cfg, source)
}

func validDirectSourceConfig(cfg Config) bool {
	return cfg.Context != nil && cfg.WorkLease != nil && cfg.Crypto != nil && cfg.Scope.Valid() && cfg.Scope.Product == "mail" && cfg.Crypto.StorageIdentityVersion() == storage.IsolatedIdentity && cfg.Journal != nil && cfg.Journal.Encrypted()
}

// MigrationState es la época retenida con la que se construyó el repositorio.
func (r *Repository) MigrationState() storage.LegacyMigrationState {
	state := r.cfg.MigrationState
	if state.StartedAt != nil {
		started := *state.StartedAt
		state.StartedAt = &started
	}
	return state
}

// ImportDirectSource abre el repositorio y, si aún no tiene el recibo de la
// importación de su época, importa el correo retenido del servidor directamente
// en él. Al terminar retira la intención de la época del diario.
func (r *Repository) ImportDirectSource(ctx context.Context, source Source) error {
	if r == nil || source == nil {
		return ErrRecord
	}
	err := r.Initialize(ctx)
	if errors.Is(err, ErrNotReady) {
		err = r.ImportRetainedSource(ctx, source)
	}
	if err != nil {
		return err
	}
	return r.retireSourceBegin()
}
