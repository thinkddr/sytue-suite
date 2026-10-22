// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ir

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ErrorValidacion envuelve los problemas detectados al decodificar un IR.
type ErrorValidacion struct {
	Problemas []Problema
}

func (e *ErrorValidacion) Error() string {
	return fmt.Sprintf("ir: %d problemas de validación", len(e.Problemas))
}

// codificarSinEscape serializa v en JSON compacto sin escapar HTML (<, >, &).
func codificarSinEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b, nil
}

// Codificar emite la representación JSON compacta del Sitio sin escapar HTML.
func Codificar(s *Sitio) ([]byte, error) {
	return codificarSinEscape(s)
}

// BytesIR devuelve la longitud en bytes UTF-8 de la serialización compacta.
func BytesIR(v any) (int, error) {
	b, err := codificarSinEscape(v)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// Decodificar valida estructuralmente el JSON con ValidarCrudo,
// si la versión es anterior a VersionIR aplica Migrar, y desglosa en *Sitio.
func Decodificar(b []byte) (*Sitio, error) {
	if problemas := ValidarCrudo(b); len(problemas) > 0 {
		return nil, &ErrorValidacion{Problemas: problemas}
	}
	var ver struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(b, &ver); err != nil {
		return nil, err
	}
	if ver.Version < VersionIR {
		var err error
		b, err = Migrar(b)
		if err != nil {
			return nil, err
		}
	}
	var s Sitio
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
