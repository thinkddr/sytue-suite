// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// La paridad de la cinta con la web, MEDIDA: qué comandos del registro común no tienen
// control en la cinta nativa de un tipo de documento.
//
// El registro (`office.Comandos`) dice qué comandos tiene cada tipo, en qué pestaña y con
// qué rótulo (`labelKey`). La cinta nativa los pinta con ese mismo rótulo como nombre
// accesible, así que mirar los controles que construye cada pestaña dice cuáles están y
// cuáles no, sin una lista a mano que se olvide al añadir un botón.

// rotulado es todo control de la cinta con nombre accesible (`activable`: el botón, el
// selector y la muestra de color).
type rotulado interface{ AccessibilityLabel() string }

// RotulosDe son los nombres accesibles de todos los controles de un cuerpo de pestaña,
// y los rótulos de sus grupos. Un cuerpo es la banda (`Grupos`) que construye una pestaña.
func RotulosDe(o fyne.CanvasObject) []string {
	var out []string
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			out = append(out, RotulosDe(h)...)
		}
	case *Grupo:
		out = append(out, x.Rotulo)
		for _, h := range x.Controles {
			out = append(out, RotulosDe(h)...)
		}
	case rotulado:
		out = append(out, x.AccessibilityLabel())
	}
	return out
}

// ComandosSinControl devuelve, ordenados, los ids del registro común que existen en el
// tipo de documento del contexto y a los que NINGUNA pestaña de `r` pinta un control cuyo
// nombre accesible contenga su rótulo. Es la brecha de paridad con la web, medida igual que
// ella (`test/e2e/paridad-cinta-web.mjs`: el rótulo en español, contenido en el nombre de
// algún control de alguna pestaña): un test la compara con su lista de pendientes por
// IGUALDAD, así que cerrar una brecha obliga a quitarla de la lista y abrir una nueva
// obliga a anotarla.
//
// Contenido y no igual porque un control puede decir más que el registro («Insertar
// segmentación de datos» contiene «Insertar segmentación»), y en cualquier pestaña porque
// la web también deja que un comando viva en otra (el relleno con IA está en «Fórmulas»
// en nativo y en «Datos» en la web). El contexto lleva el traductor real: con las claves a
// la vista, dos claves distintas con el mismo texto («sheet.borders» y
// «sheet.bordersAndFit») darían una brecha que no existe.
func ComandosSinControl(r *Registro, c *Contexto) []string {
	kind := c.Tipo.kindDe()
	var nombres []string // en minúsculas, de todas las pestañas visibles de este tipo
	for _, p := range r.Visibles(c.Tipo) {
		if p.Cuerpo != nil {
			for _, n := range RotulosDe(p.Cuerpo(c)) {
				nombres = append(nombres, strings.ToLower(n))
			}
		}
	}
	var faltan []string
	for id, cmd := range office.Comandos() {
		if !cmd.Existe(kind) {
			continue
		}
		rotulo := strings.ToLower(c.Traducir(cmd.LabelKey))
		if !slices.ContainsFunc(nombres, func(n string) bool { return strings.Contains(n, rotulo) }) {
			faltan = append(faltan, id)
		}
	}
	slices.Sort(faltan)
	return faltan
}

// CotejarParidad compara la brecha medida con la lista de pendientes de un producto y con la
// de la web, y devuelve lo que no cuadra (vacío = todo en orden):
//
//   - lo medido tiene que ser IGUAL a lo anotado: cerrar una brecha obliga a quitarla de la
//     lista, y abrir una nueva obliga a anotarla;
//   - cada pendiente lleva su motivo, y el motivo empieza por «web:» exactamente cuando la web
//     tampoco pinta ese comando. Así la lista dice qué parte de la brecha es del registro (o de
//     la web) y qué parte es de la cinta nativa, que es la que hay que cerrar.
func CotejarParidad(medidos []string, pendientes map[string]string, web []string) []string {
	var problemas []string
	for _, id := range medidos {
		if _, ok := pendientes[id]; !ok {
			problemas = append(problemas, id+": sin control y sin anotar en los pendientes")
		}
	}
	for id, motivo := range pendientes {
		if !slices.Contains(medidos, id) {
			problemas = append(problemas, id+": anotado como pendiente pero ya tiene control; quítalo de la lista")
			continue
		}
		enWeb := slices.Contains(web, id)
		switch {
		case strings.TrimSpace(motivo) == "":
			problemas = append(problemas, id+": un pendiente sin motivo")
		case enWeb && !strings.HasPrefix(motivo, "web:"):
			problemas = append(problemas, id+": la web tampoco lo pinta; el motivo debe empezar por «web:»")
		case !enWeb && strings.HasPrefix(motivo, "web:"):
			problemas = append(problemas, id+": la web SÍ lo pinta; es una brecha de la cinta nativa y el motivo no puede empezar por «web:»")
		}
	}
	slices.Sort(problemas)
	return problemas
}

// CotejarPestanas compara, por IGUALDAD, las pestañas que una cinta nativa pinta para un
// tipo de documento con las que la web pinta para ese mismo tipo: las mismas, en el mismo
// orden y con el mismo rótulo. `web` es el `pestanas.json` que la web lleva en su build
// (la copia de `apps/nucleo/office/pestanas.json`), no la fuente nativa: así también cae
// si la copia de build se queda atrás. Vacío = todo en orden.
func CotejarPestanas(r *Registro, t Tipo, web []byte) []string {
	var datos struct {
		Pestanas []struct {
			ID       string   `json:"id"`
			LabelKey string   `json:"labelKey"` //nolint:tagliatelle // el JSON es el de la web
			Kinds    []string `json:"kinds"`
		} `json:"pestanas"`
	}
	if err := json.Unmarshal(web, &datos); err != nil {
		return []string{"pestanas.json de la web no se lee: " + err.Error()}
	}
	kind := map[Tipo]string{Documento: office.Documento, Hoja: office.Hoja, Presentacion: office.Presentacion}[t]
	var quiere, hay []string
	for _, p := range datos.Pestanas {
		if slices.Contains(p.Kinds, kind) {
			quiere = append(quiere, p.ID+" "+p.LabelKey)
		}
	}
	for _, p := range r.Visibles(t) {
		hay = append(hay, p.ID+" "+p.Rotulo)
	}
	if !slices.Equal(quiere, hay) {
		return []string{fmt.Sprintf("las pestañas de %s no son las de la web:\n  la web pinta %v\n  la nativa pinta %v", kind, quiere, hay)}
	}
	return nil
}

// PestanasWeb es la copia de build de la taxonomía de pestañas que lleva la web.
const PestanasWeb = "apps/frontend/packages/office/src/editor/pestanas.json"
