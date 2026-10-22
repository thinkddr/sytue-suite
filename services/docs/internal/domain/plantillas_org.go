// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "time"

// PrefijoPlantillaOrg es el prefijo de la clave con que una plantilla de la organización
// viaja en la galería y en «crear desde plantilla»: «org:<id del documento>».
const PrefijoPlantillaOrg = "org:"

// Acciones del registro de una plantilla de organización.
const (
	AccionPlantillaMarcada    = "marked"
	AccionPlantillaPublicada  = "published"
	AccionPlantillaRenombrada = "renamed"
	AccionPlantillaQuitada    = "unmarked"
	AccionPlantillaUsada      = "used"
)

// VersionPlantilla es una versión PUBLICADA de una plantilla de organización: una
// instantánea del documento (doc_versions) con su número, el 1 de la primera.
type VersionPlantilla struct {
	No         int       `json:"no"`
	VersionID  string    `json:"id"`
	SnapshotID string    `json:"-"`
	AuthorSub  string    `json:"author_sub"`
	CreatedAt  time.Time `json:"created_at"`
}

// PlantillaOrg es un documento marcado como plantilla de su organización.
type PlantillaOrg struct {
	// Key es la clave con que la galería la ofrece («org:<id del documento>»).
	Key         string    `json:"key"`
	DocumentID  string    `json:"document_id"`
	Kind        Kind      `json:"kind"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	MarkedBy    string    `json:"marked_by"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Block: no se ofrece para crear documentos sino para insertarse en uno (bloques.go).
	Block bool `json:"block,omitempty"`
	// Versions son las publicadas, de la más reciente a la más antigua; la primera es
	// la vigente. Vacía solo si nadie publicó nada (no ocurre al marcar).
	Versions []VersionPlantilla `json:"versions"`
}

// EntradaRegistroPlantilla es una línea del registro de una plantilla.
type EntradaRegistroPlantilla struct {
	Action    string    `json:"action"`
	ActorSub  string    `json:"actor_sub"`
	VersionNo *int      `json:"version_no,omitempty"`
	At        time.Time `json:"at"`
}
