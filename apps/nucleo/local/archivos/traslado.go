// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Trasladar pone al día la raíz de Drive raiz, que ya está COPIADA en su sitio nuevo,
// cuando la carpeta de datos entera ha pasado de `de` a `a` (Ajustes → Datos, «mover la
// carpeta de datos»). El índice guarda rutas absolutas —recientes, favoritos, alias,
// papelera y etiquetas— y cada fichero tiene su carpeta de versiones por la huella de su
// ruta: todo lo que estaba bajo `de` pasa a estar bajo `a`, como hace [Archivos] al mover
// un fichero dentro de la raíz. Lo de fuera de `de` (un reciente de otro disco) se queda.
// Sin índice ni versiones no hace nada. La raíz no tiene que estar abierta.
func Trasladar(raiz, de, a string) error {
	de, a = filepath.Clean(de), filepath.Clean(a)
	cambiar := func(r string) (string, bool) {
		if !filepath.IsAbs(r) || !dentro(de, r) {
			return r, false
		}
		rel, err := filepath.Rel(de, r)
		if err != nil {
			return r, false
		}
		return filepath.Join(a, rel), true
	}
	meta := filepath.Join(raiz, Meta)
	if err := trasladarIndice(filepath.Join(meta, "indice.json"), cambiar); err != nil {
		return err
	}
	return trasladarVersiones(meta, cambiar)
}

func trasladarIndice(ruta string, cambiar func(string) (string, bool)) error {
	raw, err := os.ReadFile(ruta) //nolint:gosec // G304: el índice de la raíz de Drive copiada
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var ind indice
	if err := json.Unmarshal(raw, &ind); err != nil {
		return fmt.Errorf("archivos: índice ilegible: %w", err)
	}
	una := func(r *string) {
		if n, ok := cambiar(*r); ok {
			*r = n
		}
	}
	for k, v := range ind.Alias {
		una(&v)
		ind.Alias[k] = v
	}
	for i := range ind.Recientes {
		una(&ind.Recientes[i].Ruta)
	}
	for i := range ind.Favoritos {
		una(&ind.Favoritos[i])
	}
	ind.Etiquetas = moverClaves(ind.Etiquetas, cambiar)
	for i := range ind.Papelera {
		una(&ind.Papelera[i].Ruta)
		ind.Papelera[i].Etiquetas = moverClaves(ind.Papelera[i].Etiquetas, cambiar)
	}
	nuevo, err := json.MarshalIndent(ind, "", "  ")
	if err != nil {
		return err
	}
	return DelDisco.EscribirAtomico(ruta, append(nuevo, '\n'))
}

func moverClaves(m map[string][]string, cambiar func(string) (string, bool)) map[string][]string {
	if len(m) == 0 {
		return m
	}
	out := make(map[string][]string, len(m))
	for r, ls := range m {
		if n, ok := cambiar(r); ok {
			r = n
		}
		out[r] = ls
	}
	return out
}

// trasladarVersiones renombra la carpeta de versiones de cada fichero a la huella de su
// ruta nueva y le pone la ruta nueva en su historial.
func trasladarVersiones(meta string, cambiar func(string) (string, bool)) error {
	dir := filepath.Join(meta, "versiones")
	es, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range es {
		if !e.IsDir() {
			continue
		}
		viejo := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(filepath.Join(viejo, "historial.json")) //nolint:gosec // G304: el historial de la raíz de Drive copiada
		if err != nil {
			continue
		}
		var h historial
		if json.Unmarshal(raw, &h) != nil {
			continue
		}
		n, ok := cambiar(h.Ruta)
		if !ok {
			continue
		}
		h.Ruta = n
		nuevo := dirVersionesEn(meta, n)
		if nuevo != viejo {
			if _, err := os.Stat(nuevo); err == nil {
				return fmt.Errorf("archivos: las versiones de %s ya estaban en su sitio nuevo", n)
			}
			if err := os.Rename(viejo, nuevo); err != nil {
				return err
			}
		}
		out, err := json.MarshalIndent(h, "", "  ")
		if err != nil {
			return err
		}
		if err := DelDisco.EscribirAtomico(filepath.Join(nuevo, "historial.json"), append(out, '\n')); err != nil {
			return err
		}
	}
	return nil
}
