// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package biblioteca

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Trasladar pone al día los documentos de la biblioteca de dir (`<Datos>/Documentos`),
// ya COPIADOS en su sitio nuevo, cuando la carpeta de datos entera ha pasado de `de` a
// `a` (Ajustes → Datos, «mover la carpeta de datos»). Un documento anclado a un fichero
// de dentro de la carpeta de datos (un `.docx` de Drive) guarda su ruta absoluta: pasa a
// la de `a`. Los anclados a un fichero de fuera se quedan como estaban. Solo se reescribe
// el documento que cambia, igual que lo guarda la biblioteca. No tiene que estar abierta.
func Trasladar(dir, de, a string) error {
	de, a = filepath.Clean(de), filepath.Clean(a)
	productos, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, p := range productos {
		if !p.IsDir() {
			continue
		}
		carpeta := filepath.Join(dir, p.Name())
		es, err := os.ReadDir(carpeta)
		if err != nil {
			return err
		}
		for _, e := range es {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			if err := trasladarDocumento(filepath.Join(carpeta, e.Name()), de, a); err != nil {
				return err
			}
		}
	}
	return nil
}

func trasladarDocumento(ruta, de, a string) error {
	raw, err := os.ReadFile(ruta) //nolint:gosec // G304: un documento de la biblioteca copiada
	if err != nil {
		return err
	}
	// Antes de descodificar entero, lo barato: si no cita la carpeta vieja, nada.
	if !strings.Contains(string(raw), `"fichero"`) {
		return nil
	}
	var r registro
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("biblioteca: %s ilegible: %w", filepath.Base(ruta), err)
	}
	if r.Fichero == nil {
		return nil
	}
	rel, ok := relativaDentro(de, r.Fichero.Ruta)
	if !ok {
		return nil
	}
	r.Fichero.Ruta = filepath.Join(a, rel)
	nuevo, err := json.MarshalIndent(&r, "", " ")
	if err != nil {
		return err
	}
	return EscribirAtomico(ruta, nuevo)
}

// relativaDentro es la ruta absoluta r relativa a base, si está dentro de ella.
func relativaDentro(base, r string) (string, bool) {
	if !filepath.IsAbs(r) {
		return "", false
	}
	rel, err := filepath.Rel(base, r)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}
