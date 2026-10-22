// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package plantillas es la galería de Sytue: los documentos, hojas y
// presentaciones de partida que se ofrecen al crear, junto a «En blanco».
//
// Cada plantilla es un MODELO con huecos (`modelos/`) más su texto en los 26
// idiomas del editor (`textos/<idioma>.json`). El modelo es el valor que el
// editor guarda en su registro de contenido (HTML para un documento, el JSON de
// SheetModel o SlidesModel para hoja y presentación), así que lo que sale de
// aquí se siembra tal cual con una sola operación `set`, en el servidor y en la
// misma transacción que crea el documento.
//
// Separar estructura y texto es lo que permite tener una sola plantilla y no
// veintiséis copias que acabarían divergiendo: un hueco es `{{clave}}` (el texto
// se escapa como HTML, que es lo que guardan celdas y cajas) o `{{=clave}}` (texto
// crudo, para lo que el editor NO interpreta como HTML: los valores de una lista
// de validación y la rejilla de un gráfico).
//
// El contenido es obra original de Sytue, sobrio y sin emojis: lo vigila
// plantillas_test.go junto con que cada idioma tenga exactamente las claves del
// inglés y cada modelo salga válido en todos.
package plantillas

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"path"
	"regexp"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

//go:embed modelos/* bloques/* textos/*.json
var fsys embed.FS

// ErrNoExiste es una clave de plantilla que la galería no tiene.
var ErrNoExiste = errors.New("plantillas: no existe")

// idiomaBase es el que se usa cuando el pedido no tiene textos (y la fuente de
// verdad de las claves, como en el catálogo de la web).
const idiomaBase = "en"

// Info es lo que la galería enseña de una plantilla, ya en el idioma pedido.
type Info struct {
	Key         string      `json:"key"`
	Kind        domain.Kind `json:"kind"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
}

// catalogo fija el ORDEN de la galería y el tipo de cada plantilla. El modelo es
// `modelos/<clave>.html` (documento) o `modelos/<clave>.json` (hoja y
// presentación).
var catalogo = []struct {
	clave string
	kind  domain.Kind
}{
	{"carta", domain.KindDocument},
	{"informe", domain.KindDocument},
	{"acta", domain.KindDocument},
	{"orden", domain.KindDocument},
	{"presupuesto", domain.KindSpreadsheet},
	{"gastos", domain.KindSpreadsheet},
	{"planificacion", domain.KindSpreadsheet},
	{"factura", domain.KindSpreadsheet},
	{"proyecto", domain.KindPresentation},
	{"trimestral", domain.KindPresentation},
	{"propuesta", domain.KindPresentation},
	{"mensual", domain.KindSpreadsheet},
	{"inventario", domain.KindSpreadsheet},
	{"semanal", domain.KindSpreadsheet},
	{"contactos", domain.KindSpreadsheet},
	{"horas", domain.KindSpreadsheet},
	{"cobros", domain.KindSpreadsheet},
	{"equipo", domain.KindPresentation},
	{"lanzamiento", domain.KindPresentation},
	{"formacion", domain.KindPresentation},
	{"cierre", domain.KindPresentation},
	{"estrategia", domain.KindPresentation},
	{"bienvenida", domain.KindPresentation},
}

// hueco es `{{clave}}` o `{{=clave}}`.
var hueco = regexp.MustCompile(`\{\{(=?)([a-z0-9_.]+)\}\}`)

var (
	cargaUnaVez sync.Once
	textos      map[string]map[string]string // idioma → clave → texto
	errCarga    error
)

// cargar lee los textos embebidos una vez por proceso.
func cargar() error {
	cargaUnaVez.Do(func() {
		entradas, err := fsys.ReadDir("textos")
		if err != nil {
			errCarga = err
			return
		}
		textos = make(map[string]map[string]string, len(entradas))
		for _, e := range entradas {
			datos, err := fsys.ReadFile("textos/" + e.Name())
			if err != nil {
				errCarga = err
				return
			}
			m := map[string]string{}
			if err := json.Unmarshal(datos, &m); err != nil {
				errCarga = fmt.Errorf("plantillas: %s: %w", e.Name(), err)
				return
			}
			textos[strings.TrimSuffix(e.Name(), ".json")] = m
		}
		if textos[idiomaBase] == nil {
			errCarga = fmt.Errorf("plantillas: faltan los textos en %q", idiomaBase)
		}
	})
	return errCarga
}

// Idioma normaliza lo que llega («es-ES», «pt_PT», «EN») al código con textos;
// lo que no tiene textos cae al inglés.
func Idioma(pedido string) string {
	if cargar() != nil {
		return idiomaBase
	}
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(pedido)), "-")
	base, _, _ = strings.Cut(base, "_")
	if _, ok := textos[base]; ok {
		return base
	}
	return idiomaBase
}

// texto es la cadena de `clave` en `idioma`, con el inglés de respaldo.
func texto(idioma, clave string) string {
	if s, ok := textos[idioma][clave]; ok {
		return s
	}
	return textos[idiomaBase][clave]
}

// Listar devuelve la galería en el idioma pedido, en su orden.
func Listar(idioma string) ([]Info, error) {
	if err := cargar(); err != nil {
		return nil, err
	}
	idioma = Idioma(idioma)
	out := make([]Info, 0, len(catalogo))
	for _, p := range catalogo {
		out = append(out, info(p.clave, p.kind, idioma))
	}
	return out, nil
}

func info(clave string, kind domain.Kind, idioma string) Info {
	return Info{
		Key: clave, Kind: kind,
		Name:        texto(idioma, clave+".nombre"),
		Description: texto(idioma, clave+".descripcion"),
	}
}

// Generar devuelve la plantilla y su contenido inicial (el valor del registro de
// contenido del editor) en el idioma pedido. ErrNoExiste si la clave no está.
func Generar(clave, idioma string) (Info, string, error) {
	if err := cargar(); err != nil {
		return Info{}, "", err
	}
	idioma = Idioma(idioma)
	for _, p := range catalogo {
		if p.clave != clave {
			continue
		}
		contenido, err := rellenar("modelos/"+clave, p.kind, idioma)
		if err != nil {
			return Info{}, "", err
		}
		return info(clave, p.kind, idioma), contenido, nil
	}
	return Info{}, "", ErrNoExiste
}

// rellenar lee el modelo `base`.html (documento) o `base`.json y sustituye sus huecos por
// los textos de `idioma`. Lo comparten la galería y los bloques reutilizables.
func rellenar(base string, kind domain.Kind, idioma string) (string, error) {
	fichero := base + ".json"
	if kind == domain.KindDocument {
		fichero = base + ".html"
	}
	modelo, err := fsys.ReadFile(fichero)
	if err != nil {
		return "", fmt.Errorf("plantillas: %s: %w", fichero, err)
	}
	enJSON := path.Ext(fichero) == ".json"
	fuente := string(modelo)
	if !enJSON {
		// El modelo lleva un bloque por línea para poder leerlo; el HTML del
		// editor no lleva saltos entre bloques (serían nodos de texto sueltos).
		fuente = strings.ReplaceAll(fuente, "\n", "")
	}
	contenido := hueco.ReplaceAllStringFunc(fuente, func(m string) string {
		partes := hueco.FindStringSubmatch(m)
		s := texto(idioma, partes[2])
		if partes[1] != "=" {
			s = html.EscapeString(s)
		}
		if enJSON {
			s = cadenaJSON(s)
		}
		return s
	})
	return strings.TrimSpace(contenido), nil
}

// cadenaJSON escapa `s` para ir DENTRO de una cadena JSON ya entrecomillada en
// el modelo (sin las comillas de fuera).
func cadenaJSON(s string) string {
	// Sin escapar <, > ni &: json.Marshal los pasa a \u003c, válido pero ilegible
	// en el registro de contenido, y aquí el HTML ya se escapó (o es texto crudo).
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // una cadena siempre se serializa
	r := strings.TrimSuffix(b.String(), "\n")
	return r[1 : len(r)-1]
}
