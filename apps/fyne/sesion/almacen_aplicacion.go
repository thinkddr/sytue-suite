// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js

package sesion

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// AlmacenPrivadoAplicacion is called after the native factory has verified both
// devices and acknowledged recovery. Apple sandbox journals are suite-visible,
// while their cache keys remain private to this installed app. Desktop shared
// caches and the Android broker retain their existing authority boundaries.
func (s *Suite) AlmacenPrivadoAplicacion(ctx context.Context, product string, witness auth.TestigoTrabajo) (*offline.Store, error) {
	return s.almacenPrivadoAplicacion(ctx, product, witness, almacenAplicacionSeparado)
}

func (s *Suite) almacenPrivadoAplicacion(ctx context.Context, product string, witness auth.TestigoTrabajo, separado bool) (*offline.Store, error) {
	consumer, err := s.ConsumidorAlmacenamiento(product)
	if err != nil {
		return nil, err
	}
	if err := s.validarAlmacenAplicacion(ctx, witness); err != nil {
		return nil, err
	}
	if !separado {
		if consumer != "" {
			return AlmacenConsumidor(ctx, consumer)
		}
		return AlmacenConContexto(ctx)
	}
	available, err := ajustes.EstadoTrabajoSesionCompartido()
	if err != nil || !available {
		return nil, errors.Join(ajustes.ErrTrabajoCompartidoNoDisponible, err)
	}
	app := product
	if consumer != "" {
		app = consumer
	}
	source, err := directorioAlmacen()
	if err != nil {
		return nil, err
	}
	target, err := directorioAlmacenAplicacion(app)
	if err != nil {
		return nil, err
	}
	acquire := func(job context.Context) (func(), error) {
		release, err := witness.Exclusivo(job)
		if err != nil {
			return nil, err
		}
		if err := s.validarAlmacenAplicacion(job, witness); err != nil {
			release()
			return nil, err
		}
		return release, nil
	}
	journal, err := offline.RelocateJournalContext(ctx, source, target, acquire)
	if err != nil {
		return nil, err
	}
	if err := s.validarAlmacenAplicacion(ctx, witness); err != nil {
		return nil, err
	}
	// Atomic publication changes layout only. HTTP owners and native work retain
	// their independent session/Scope checks; no device or recovery key is shared.
	s.cacheAlmacen.Store(&almacenCache{store: journal, dir: target, source: source})
	return journal, nil
}

func (s *Suite) validarAlmacenAplicacion(ctx context.Context, witness auth.TestigoTrabajo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.Sesion == nil || s.almacen == nil || witness.Almacen != s.almacen.Identificador() || witness.Acceso.Identidad != s.Sesion.SnapshotIdentity().IdentidadSesion() {
		return auth.ErrTrabajoSesion
	}
	return witness.Validar()
}
