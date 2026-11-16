// SPDX-License-Identifier: Apache-2.0
package domain

import "git.sytue.eu/sytue/one/apps/nucleo/photos/model"

type (
	TipoPaso = model.TipoPaso
	Paso     = model.Paso
	Receta   = model.Receta
)

const (
	VersionReceta   = model.VersionReceta
	PasoRecorte     = model.PasoRecorte
	PasoGiro        = model.PasoGiro
	PasoEspejo      = model.PasoEspejo
	PasoExposicion  = model.PasoExposicion
	PasoContraste   = model.PasoContraste
	PasoSaturacion  = model.PasoSaturacion
	PasoTemperatura = model.PasoTemperatura
)

var RecetaVacia = model.RecetaVacia
