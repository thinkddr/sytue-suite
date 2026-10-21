// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package seeds embebe el catálogo declarativo de plantillas Tier-1 del Sytue
// Integration Gateway (OLA 3) y lo carga al arrancar: cada fichero templates/*.json es
// una plantilla (clave + tier + manifiesto) que se valida contra el esquema y se hace
// upsert en app_templates. Así, añadir un SaaS al catálogo es solo añadir un fichero —
// sin migración ni código. Los iconos son SVG propios curados en el manifiesto (nunca
// hotlink a terceros).
package seeds

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"

	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

//go:embed templates/*.json
var files embed.FS

// Seed es una plantilla del catálogo lista para upsert: su clave, tier y el manifiesto
// crudo (ya validado) que persiste en app_templates.manifest.
type Seed struct {
	Key      string
	Tier     string
	Manifest json.RawMessage
}

// seedFile es la forma en disco de un fichero de semilla.
type seedFile struct {
	Key      string          `json:"key"`
	Tier     string          `json:"tier"`
	Manifest json.RawMessage `json:"manifest"`
}

// Load lee, valida y devuelve todas las semillas embebidas, ordenadas por clave (orden
// estable). Un manifiesto que no cumple el esquema es un ERROR de build/arranque (una
// semilla mal escrita no debe llegar silenciosa al catálogo).
func Load() ([]Seed, error) {
	entries, err := fs.ReadDir(files, "templates")
	if err != nil {
		return nil, fmt.Errorf("seeds: leer directorio: %w", err)
	}
	out := make([]Seed, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := files.ReadFile("templates/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("seeds: leer %s: %w", e.Name(), err)
		}
		var sf seedFile
		if err := json.Unmarshal(raw, &sf); err != nil {
			return nil, fmt.Errorf("seeds: %s: JSON inválido: %w", e.Name(), err)
		}
		if sf.Key == "" {
			return nil, fmt.Errorf("seeds: %s: falta key", e.Name())
		}
		if _, err := manifest.Parse(sf.Manifest); err != nil {
			return nil, fmt.Errorf("seeds: %s: %w", e.Name(), err)
		}
		out = append(out, Seed(sf))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
