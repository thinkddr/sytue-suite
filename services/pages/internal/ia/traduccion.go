// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"errors"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

var (
	// ErrAlcanceTraduccion indica un alcance mal formado, nunca «todo el sitio».
	ErrAlcanceTraduccion = errors.New("ia: invalid translation scope")
	// ErrTraduccionTiendaNoDisponible evita modificar un catálogo hasta que
	// exista la aplicación atómica con snapshot de valores anteriores para deshacer.
	ErrTraduccionTiendaNoDisponible = errors.New("ia: store translation is not available")
	// ErrIdiomasTraduccion impide generar textos que el IR no puede guardar.
	ErrIdiomasTraduccion = errors.New("ia: invalid translation languages")
)

// ValidarAlcanceTraduccion se usa antes de reservar cupo y también en la ruta
// HTTP. El alcance de página lleva el id real, tal como establece el contrato.
func ValidarAlcanceTraduccion(scope string) error {
	switch scope {
	case "", "site":
		return nil
	case "store":
		return ErrTraduccionTiendaNoDisponible
	}
	id, page := strings.CutPrefix(scope, "page:")
	if !page || id == "" || len(id) > 64 || strings.ContainsAny(id, " /:\t\r\n") {
		return ErrAlcanceTraduccion
	}
	return nil
}

// ValidarIdiomasTraduccion comparte los idiomas con el validador y el visitante.
// El origen debe estar habilitado en el borrador; el destino puede ser nuevo.
func ValidarIdiomasTraduccion(in domain.TranslateInput) error {
	if !slices.Contains(ir.IdiomasVisitante, in.From) || len(in.To) == 0 || len(in.To) > len(ir.IdiomasVisitante) {
		return ErrIdiomasTraduccion
	}
	vistos := make(map[string]bool)
	for _, target := range in.To {
		if !slices.Contains(ir.IdiomasVisitante, target) || target == in.From || vistos[target] {
			return ErrIdiomasTraduccion
		}
		vistos[target] = true
	}
	return nil
}

func textosDeAlcance(s *ir.Sitio, idioma, scope string) (map[string]ir.TextoTraducible, error) {
	if err := ValidarAlcanceTraduccion(scope); err != nil {
		return nil, err
	}
	if id, page := strings.CutPrefix(scope, "page:"); page {
		texts, found := ir.TextosDePagina(s, idioma, id)
		if !found {
			return nil, domain.ErrNoExiste
		}
		return texts, nil
	}
	return ir.TextosDe(s, idioma), nil
}

func textosParaTraducir(s *ir.Sitio, in domain.TranslateInput, target string) (map[string]ir.TextoTraducible, error) {
	source, err := textosDeAlcance(s, in.From, in.Scope)
	if err != nil {
		return nil, err
	}
	existing, err := textosDeAlcance(s, target, in.Scope)
	if err != nil {
		return nil, err
	}
	for key, text := range source {
		if strings.TrimSpace(text.Texto) == "" || (!in.Overwrite && strings.TrimSpace(existing[key].Texto) != "") {
			delete(source, key)
		}
	}
	return source, nil
}
