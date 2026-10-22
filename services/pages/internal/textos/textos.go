// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package textos carga los textos que el SERVICIO escribe para el visitante de una
// web o para el comprador —correos de la tienda, la página «esta web no está
// disponible»—, un fichero `<idioma>.json` = {clave: texto} por idioma y área
// (PAGES-ENCARGO §10.5, §8.1). Los de la interfaz de la app viven en la web.
//
// Todos los idiomas de un área llevan exactamente las claves del inglés, que es la
// base: una clave que falta en una lengua sale al cargar, no en el correo de un
// cliente.
package textos

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
)

// Base es el idioma de respaldo: el de las claves de referencia.
const Base = "en"

// Catalogo son los textos de un área, por idioma.
type Catalogo struct {
	por map[string]map[string]string
}

// Cargar lee dir/*.json de fsys (normalmente un embed.FS del área).
func Cargar(fsys fs.FS, dir string) (*Catalogo, error) {
	ficheros, err := fs.Glob(fsys, path.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	c := &Catalogo{por: map[string]map[string]string{}}
	for _, f := range ficheros {
		b, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var m map[string]string
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("textos: %s: %w", f, err)
		}
		c.por[strings.TrimSuffix(path.Base(f), ".json")] = m
	}
	base, ok := c.por[Base]
	if !ok {
		return nil, fmt.Errorf("textos: %s no tiene %s.json, que es la base", dir, Base)
	}
	claves := slices.Sorted(maps.Keys(base))
	for idioma, m := range c.por {
		if otras := slices.Sorted(maps.Keys(m)); !slices.Equal(otras, claves) {
			return nil, fmt.Errorf("textos: %s/%s.json no tiene las mismas claves que %s.json", dir, idioma, Base)
		}
	}
	return c, nil
}

// Texto devuelve el texto de una clave en un idioma, con `{nombre}` sustituido por
// datos[nombre]. Un idioma sin catálogo cae a la base; una clave que no existe se
// devuelve tal cual, a la vista, antes que un hueco en blanco.
func (c *Catalogo) Texto(idioma, clave string, datos map[string]string) string {
	m, ok := c.por[idioma]
	if !ok {
		m = c.por[Base]
	}
	t, ok := m[clave]
	if !ok {
		return clave
	}
	if len(datos) == 0 {
		return t
	}
	pares := make([]string, 0, 2*len(datos))
	for _, k := range slices.Sorted(maps.Keys(datos)) {
		pares = append(pares, "{"+k+"}", datos[k])
	}
	return strings.NewReplacer(pares...).Replace(t)
}

// Idiomas son los idiomas con catálogo, ordenados.
func (c *Catalogo) Idiomas() []string { return slices.Sorted(maps.Keys(c.por)) }
