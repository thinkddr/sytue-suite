// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package publicar recibe la salida del renderizador, la valida contra
// permitido.json y la guarda como versión inmutable (PAGES-ENCARGO §7.4); también
// escribe los zip de las exportaciones (§7.6). El JS de las islas vive embebido en
// islas/ (dueño `render`).
package publicar

import (
	"fmt"
)

// Servicio valida y publica sitios web.
type Servicio struct {
	Permitido *Permitido
}

// Nuevo construye el servicio de publicación cargando permitido.json.
func Nuevo() *Servicio {
	p, err := CargarPermitido()
	if err != nil {
		panic(fmt.Sprintf("publicar: no se pudo cargar permitido.json: %v", err))
	}
	return &Servicio{
		Permitido: p,
	}
}

// Validar verifica la salida del renderizador contra la allowlist y restricciones de seguridad.
func (s *Servicio) Validar(ficheros []FicheroSalida, opts OpcionesValidacion) error {
	return ValidarSalida(s.Permitido, ficheros, opts)
}
