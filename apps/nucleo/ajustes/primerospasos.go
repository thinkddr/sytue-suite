// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
)

// «Ocultar» de la tarjeta «Primeros pasos» del Hub.
//
// En la web se recuerda en `localStorage` con la clave `sub` + organización
// (`clavePrimerosPasos`). Aquí va en los ajustes de la suite, y la clave lleva además
// el EMISOR: el mismo `sub` en otra instalación de Sytue ID es otra persona. No va en
// `sesion.PorPersona`, que tiene ese mismo ámbito pero es de UNA app: ocultarla vale para
// todas las de la suite de este equipo.

// HuellaPrimerosPasos es la clave de quien oculta: emisor, sujeto y organización. Vacía
// si no hay sujeto: sin nadie dentro no se recuerda nada.
func HuellaPrimerosPasos(emisor, sujeto, organizacion string) string {
	if sujeto == "" {
		return ""
	}
	suma := sha256.Sum256([]byte("sytue-primeros-pasos\x00" + emisor + "\x00" + sujeto + "\x00" + organizacion))
	return hex.EncodeToString(suma[:16])
}

// PrimerosPasosOcultos dice si esa persona, en esa organización, ocultó la tarjeta.
func PrimerosPasosOcultos(huella string) bool {
	return huella != "" && slices.Contains(strings.Fields(Leer().PrimerosPasosOcultos), huella)
}

// OcultarPrimerosPasos lo deja guardado para siempre en este equipo, para esa persona
// en esa organización y en todas las apps de la suite.
func OcultarPrimerosPasos(huella string) error {
	if huella == "" {
		return nil
	}
	return Cambiar(func(a *Ajustes) {
		if huellas := strings.Fields(a.PrimerosPasosOcultos); !slices.Contains(huellas, huella) {
			a.PrimerosPasosOcultos = strings.Join(append(huellas, huella), " ")
		}
	})
}
