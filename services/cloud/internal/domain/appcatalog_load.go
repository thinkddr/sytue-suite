// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed apps/*.json
var appFiles embed.FS

// AppCatalog es el catálogo cargado: lo que se puede ofrecer y lo que aún no.
type AppCatalog struct {
	// Apps son las entradas COMPLETAS: revisadas, con licencia comprobada y con
	// la imagen ya replicada. Son las únicas que se pueden aprovisionar.
	Apps []CatalogApp
	// PendingMirror son las que están escritas y validadas pero cuya imagen aún
	// no se ha replicado. Se devuelven aparte, y NO en silencio: enseñarlas como
	// «en camino» es honesto; esconderlas haría que nadie replicase nunca la
	// imagen porque nadie las echaría de menos.
	PendingMirror []string
}

// LoadAppCatalog lee y valida el catálogo embebido.
//
// Un fichero mal formado o que incumpla un invariante es un ERROR, no una
// entrada que se salta: el catálogo lo escriben personas y la diferencia entre
// una etiqueta y un digest no se ve en un diff.
func LoadAppCatalog(allowedRegistry string) (AppCatalog, error) {
	entries, err := fs.ReadDir(appFiles, "apps")
	if err != nil {
		return AppCatalog{}, fmt.Errorf("catálogo de apps: leer directorio: %w", err)
	}
	var cat AppCatalog
	// todas incluye las pendientes de espejo: una integración puede apuntar a una
	// app cuya imagen todavía no se ha replicado sin estar mal escrita.
	todas := make([]CatalogApp, 0, len(entries))
	seen := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, rerr := appFiles.ReadFile("apps/" + e.Name())
		if rerr != nil {
			return AppCatalog{}, fmt.Errorf("catálogo de apps: leer %s: %w", e.Name(), rerr)
		}
		var a CatalogApp
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		// Un campo que no existe en el struct casi siempre es una errata —
		// `memory_mb` en vez de `memory_mib`— y aceptarlo en silencio deja el
		// valor a cero, que aquí significa «sin límite pedido».
		dec.DisallowUnknownFields()
		if derr := dec.Decode(&a); derr != nil {
			return AppCatalog{}, fmt.Errorf("catálogo de apps: %s: %w", e.Name(), derr)
		}
		if prev, dup := seen[a.ID]; dup {
			return AppCatalog{}, fmt.Errorf(
				"catálogo de apps: %s: id %q duplicado (ya lo usa %s)", e.Name(), a.ID, prev,
			)
		}
		seen[a.ID] = e.Name()

		// Sin copia todavía: la entrada está escrita y revisada, pero su imagen no
		// se ha replicado. Se valida IGUAL —licencia, límites, plantilla de
		// exports— para que el repaso ocurra cuando se escribe y no meses después.
		pending := a.MirroredDigest == ""
		if verr := ValidateCatalogApp(a, allowedRegistry); verr != nil {
			return AppCatalog{}, fmt.Errorf("catálogo de apps: %s: %w", e.Name(), verr)
		}
		if pending {
			cat.PendingMirror = append(cat.PendingMirror, a.ID)
			todas = append(todas, a)
			continue
		}
		cat.Apps = append(cat.Apps, a)
		todas = append(todas, a)
	}
	if err := validarIntegracionesCruzadas(todas); err != nil {
		return AppCatalog{}, err
	}
	SortCatalogApps(cat.Apps)
	return cat, nil
}

// validarIntegracionesCruzadas comprueba que cada integración apunta a una app
// que EXISTE. No se puede hacer al validar una entrada suelta —no conoce a las
// demás— y sin esto una errata en el id se descubriría en el namespace de un
// cliente, con la integración simplemente sin aparecer y nada que lo delatara.
//
// Vale que la otra esté pendiente de espejo: la integración está bien escrita, y
// se activará el día que su imagen se replique.
func validarIntegracionesCruzadas(todas []CatalogApp) error {
	existe := make(map[string]bool, len(todas))
	for _, a := range todas {
		existe[a.ID] = true
	}
	for _, a := range todas {
		for _, in := range a.Integrations {
			if !existe[in.App] {
				return fmt.Errorf(
					"catálogo de apps: %s se integra con %q, que no está en el catálogo", a.ID, in.App,
				)
			}
		}
	}
	return nil
}
