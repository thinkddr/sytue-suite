// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain preserves the Photos service model while the same domain
// rules run on authorized devices.
package domain

import "git.sytue.eu/sytue/one/apps/nucleo/photos/model"

type (
	Orientacion    = model.Orientacion
	Transformacion = model.Transformacion
	Activo         = model.Activo
)

var (
	ErrNotFound     = model.ErrNotFound
	ErrInvalidInput = model.ErrInvalidInput
	ErrForbidden    = model.ErrForbidden
	OrientacionDe   = model.OrientacionDe
)
