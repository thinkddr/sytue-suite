// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// # Lo que queda de un almacén que pasó a una base
//
// Photos y Contacts de Sytue Office pasaron su almacén a SQLite (`local/fototeca`,
// `local/libreta`) y lo dejaron en su carpeta, sin tocar, hasta que la persona lo borre
// (Ajustes → Datos). [Restos] dice qué es del almacén en esa carpeta, [Medir] cuánto
// ocupa y [Borrar] lo quita; decidir que ya no hace falta es de quien migró (su marca).

// Antes es lo que queda de un almacén de registros que pasó a una base.
type Antes struct {
	// Migrada: lo del almacén está en la base y se comprobó al pasarlo (la marca de la
	// migración). Sin ella no se borra nada ni se ofrece borrarlo.
	Migrada bool
	// Ficheros y Bytes es lo que queda del almacén: lo que borrarlo quita (o quitó).
	Ficheros int
	Bytes    int64
}

// Restos son las rutas de dir que son del almacén y SOLO de él, las que haya: su diario
// (`registros.jsonl`), el diario transitorio del motor y su clave, el estado del origen y
// los temporales de compactar y de escribir. No incluye `objetos/`, que una base que
// sustituye al almacén puede seguir usando (la fototeca de SQLite cita sus ficheros):
// quien sabe que no, lo añade. Tampoco el cerrojo, que se suelta y se quita al final
// ([QuitarCerrojo]). Nada más de dir (la base y lo suyo) sale aquí.
func Restos(dir string) ([]string, error) {
	entradas, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entradas {
		n := e.Name()
		switch {
		case n == ficheroDiario, n == "origen.json", n == "diario", n == "diario.clave":
		case strings.HasPrefix(n, prefijoTemporal):
		case strings.HasPrefix(n, ".origen.json.") && strings.HasSuffix(n, ".tmp"):
		default:
			continue
		}
		out = append(out, filepath.Join(dir, n))
	}
	return out, nil
}

// Medir cuenta los ficheros y los bytes de rutas (una carpeta, con lo que tiene dentro;
// un enlace, como el enlace). Lo que no está no cuenta.
func Medir(rutas ...string) (ficheros int, bytes int64, err error) {
	for _, r := range rutas {
		err := filepath.WalkDir(r, func(_ string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			ficheros++
			bytes += info.Size()
			return nil
		})
		if err != nil {
			return ficheros, bytes, err
		}
	}
	return ficheros, bytes, nil
}

// Borrar quita rutas (las carpetas, enteras) y dice cuánto se fue. Sigue con las demás
// si una falla, y devuelve los fallos juntos.
func Borrar(rutas ...string) (ficheros int, bytes int64, err error) {
	var errs []error
	for _, r := range rutas {
		n, b, err := Medir(r)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.RemoveAll(r); err != nil {
			errs = append(errs, fmt.Errorf("almacen: no se pudo borrar %s: %w", filepath.Base(r), err))
			continue
		}
		ficheros += n
		bytes += b
	}
	return ficheros, bytes, errors.Join(errs...)
}

// QuitarCerrojo quita el fichero del cerrojo de dir, si está: lo último al borrar el
// almacén, después de soltarlo (en Windows no se borra abierto). Si no se puede, se
// queda: está vacío y no estorba.
func QuitarCerrojo(dir string) {
	_ = os.Remove(filepath.Join(dir, ficheroCerrojo))
}
