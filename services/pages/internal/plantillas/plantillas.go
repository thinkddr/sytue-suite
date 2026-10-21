// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package plantillas

import (
	"embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

//go:embed *.json secciones/*.json
var plantillasFS embed.FS

// Plantilla representa una plantilla de sitio completa con sus metadatos y su IR.
type Plantilla struct {
	Key         string    `json:"key"`
	Name        ir.Textos `json:"name"`
	Description ir.Textos `json:"description"`
	Vertical    string    `json:"vertical"`
	// Category agrupa la plantilla en la galería (belleza, comida, salud...). Vacía en las plantillas
	// anteriores a la ampliación del catálogo.
	Category string `json:"category,omitempty"`
	// Keywords son sinónimos y palabras de uso común por idioma (separadas por espacios) para el
	// buscador de la galería: «pelu», «barbero», «fontanero»...
	Keywords ir.Textos `json:"keywords,omitempty"`
	Store    bool      `json:"store"`
	IR       ir.Sitio  `json:"ir"`
}

// PlantillaInfo contiene solo los metadatos de una plantilla (sin el árbol IR completo).
type PlantillaInfo struct {
	Key         string    `json:"key"`
	Name        ir.Textos `json:"name"`
	Description ir.Textos `json:"description"`
	Vertical    string    `json:"vertical"`
	Category    string    `json:"category,omitempty"`
	Keywords    ir.Textos `json:"keywords,omitempty"`
	Store       bool      `json:"store"`
}

// FS devuelve el sistema de ficheros embebido de plantillas.
func FS() embed.FS {
	return plantillasFS
}

var (
	listaOnce sync.Once
	listaInfo []PlantillaInfo
	listaErr  error
)

// ListarPlantillas devuelve los metadatos de todas las plantillas disponibles. Los ficheros son
// embebidos e inmutables, así que se leen una sola vez por proceso (el catálogo pasa de 70 ficheros).
func ListarPlantillas() ([]PlantillaInfo, error) {
	listaOnce.Do(func() { listaInfo, listaErr = leerPlantillas() })
	if listaErr != nil {
		return nil, listaErr
	}
	return slices.Clone(listaInfo), nil
}

func leerPlantillas() ([]PlantillaInfo, error) {
	entries, err := plantillasFS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("plantillas: leer directorio: %w", err)
	}
	var res []PlantillaInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "temas.json" {
			continue
		}
		data, err := plantillasFS.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("plantillas: leer %s: %w", e.Name(), err)
		}
		var p Plantilla
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("plantillas: decodificar %s: %w", e.Name(), err)
		}
		res = append(res, PlantillaInfo{
			Key:         p.Key,
			Name:        p.Name,
			Description: p.Description,
			Vertical:    p.Vertical,
			Category:    p.Category,
			Keywords:    p.Keywords,
			Store:       p.Store,
		})
	}
	return res, nil
}

// ObtenerPlantilla devuelve la plantilla completa por clave.
func ObtenerPlantilla(key string) (Plantilla, error) {
	filename := key + ".json"
	data, err := plantillasFS.ReadFile(filename)
	if err != nil {
		return Plantilla{}, fmt.Errorf("plantillas: no encontrada %q: %w", key, err)
	}
	var p Plantilla
	if err := json.Unmarshal(data, &p); err != nil {
		return Plantilla{}, fmt.Errorf("plantillas: decodificar %s: %w", filename, err)
	}
	return p, nil
}

// ListarSecciones devuelve todas las secciones de plantilla organizadas por kind.
func ListarSecciones() (map[string][]ir.Nodo, error) {
	entries, err := plantillasFS.ReadDir("secciones")
	if err != nil {
		return nil, fmt.Errorf("plantillas: leer secciones: %w", err)
	}
	res := make(map[string][]ir.Nodo)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := plantillasFS.ReadFile(filepath.Join("secciones", e.Name()))
		if err != nil {
			return nil, fmt.Errorf("plantillas: leer seccion %s: %w", e.Name(), err)
		}
		var nodo ir.Nodo
		if err := json.Unmarshal(data, &nodo); err != nil {
			return nil, fmt.Errorf("plantillas: decodificar seccion %s: %w", e.Name(), err)
		}
		res[nodo.Kind] = append(res[nodo.Kind], nodo)
	}
	return res, nil
}

// ObtenerSeccionesPorKind devuelve las secciones de un kind concreto.
func ObtenerSeccionesPorKind(kind string) ([]ir.Nodo, error) {
	todas, err := ListarSecciones()
	if err != nil {
		return nil, err
	}
	return todas[kind], nil
}
