// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/base64"
	"fmt"
)

// ClaveWire es una contraseña ya derivada tal y como viaja: con los nombres del esquema de
// OOXML (algorithmName, saltValue, spinCount, hashValue; sal y hash en base64). Es lo que
// sale en una exportación a .xlsx y lo que vuelve de una importación.
//
//nolint:tagliatelle // los nombres los fija el esquema de OOXML
type ClaveWire struct {
	Algoritmo string `json:"algorithmName"`
	Sal       string `json:"saltValue"`
	Vueltas   int    `json:"spinCount"`
	Hash      string `json:"hashValue"`
}

// Tope de vueltas al IMPORTAR (entrada no confiable): cada comprobación cuesta una vuelta
// de SHA-512 por cada una, y un fichero hostil con diez millones paralizaría un núcleo
// durante segundos en cada intento. Excel escribe 100 000.
const claveMaxVueltasImportada = 1_000_000

// ErrHashInvalido: el hash de una contraseña que viene de fuera no es de los que sabe
// verificar el servicio.
var ErrHashInvalido = fmt.Errorf("%w: hash de contraseña no soportado", ErrInvalidOp)

// Wire es el hash para viajar.
func (c ClaveHash) Wire() ClaveWire {
	return ClaveWire{
		Algoritmo: c.Algoritmo,
		Sal:       base64.StdEncoding.EncodeToString(c.Sal),
		Vueltas:   c.Vueltas,
		Hash:      base64.StdEncoding.EncodeToString(c.Hash),
	}
}

// aHash valida el hash que viene de fuera y lo convierte. Solo el esquema que verifica el
// servicio: SHA-512 con sal y vueltas acotadas. Cualquier otro (SHA-1, MD5, la contraseña
// de 16 bits de Excel antiguo) no se puede comprobar aquí y no se acepta como contraseña.
func (w ClaveWire) aHash() (ClaveHash, error) {
	sal, err1 := base64.StdEncoding.DecodeString(w.Sal)
	hash, err2 := base64.StdEncoding.DecodeString(w.Hash)
	if w.Algoritmo != claveAlgoritmo || err1 != nil || err2 != nil ||
		len(sal) < 8 || len(sal) > 64 || len(hash) != 64 ||
		w.Vueltas < 1 || w.Vueltas > claveMaxVueltasImportada {
		return ClaveHash{}, ErrHashInvalido
	}
	return ClaveHash{Algoritmo: w.Algoritmo, Sal: sal, Vueltas: w.Vueltas, Hash: hash}, nil
}
