// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Pegar un bloque en la rejilla: lo que se copia de una hoja de cálculo (columnas
// separadas por tabuladores, filas por saltos de línea) entra a partir de la celda con
// el foco, cada valor en su campo. Las filas que no caben en los registros que hay se
// AÑADEN como registros nuevos: así se llena una tabla de golpe desde otra hoja. Lo
// mismo pasa al soltar ese texto sobre la rejilla.
//
// Cada valor se lee como en la celda (valorEscrito); uno que no encaja en su campo, o
// que va a un campo que no se escribe así (enlace, fórmula, adjuntos), se deja sin
// tocar y se avisa.

// bloque parte un texto copiado de una hoja de cálculo en filas y columnas.
func bloque(texto string) [][]string {
	texto = strings.ReplaceAll(texto, "\r\n", "\n")
	texto = strings.TrimSuffix(texto, "\n")
	if texto == "" {
		return nil
	}
	lineas := strings.Split(texto, "\n")
	out := make([][]string, len(lineas))
	for i, l := range lineas {
		out[i] = strings.Split(l, "\t")
	}
	return out
}

// valorPegado es un texto pegado → el valor de ese campo. `ok` falso: no encaja o el
// campo no se escribe pegando.
func valorPegado(campo api.BasesField, texto string) (any, bool) {
	t := strings.TrimSpace(texto)
	switch campo.Kind {
	case api.BasesKindLink, api.BasesKindFormula, api.BasesKindAttachment:
		return nil, false
	case api.BasesKindCheckbox:
		switch strings.ToLower(t) {
		case "", "0", "no", "false", "falso", "✗":
			return nil, true
		}
		return true, true
	case api.BasesKindMultiselect:
		var elegidos []any
		for _, parte := range strings.Split(t, ",") {
			parte = strings.TrimSpace(parte)
			if parte == "" {
				continue
			}
			if !slices.Contains(campo.Choices, parte) {
				return nil, false
			}
			elegidos = append(elegidos, parte)
		}
		if len(elegidos) == 0 {
			return nil, true
		}
		return elegidos, true
	case api.BasesKindDate:
		// De una hoja llega tanto 05/10/2026 como 2026-10-05.
		if v, ok := valorDeTexto(campo, t); ok {
			return v, true
		}
	}
	return valorEscrito(campo, t)
}

// pegar escribe un bloque a partir de la celda `desde` de la rejilla.
func (p *Pantalla) pegar(desde celda, texto string) {
	b := p.abierta
	if b == nil || b.rejilla == nil || !puedeEscribir(b.rol()) {
		return
	}
	g := b.rejilla
	filas := bloque(texto)
	if len(filas) == 0 || desde.col < 0 || desde.col >= len(g.Campos) {
		return
	}
	saltados := 0
	// valores de una fila del bloque, por id de campo.
	valores := func(linea []string) map[string]any {
		datos := map[string]any{}
		for j, t := range linea {
			col := desde.col + j
			if col >= len(g.Campos) {
				break
			}
			campo := g.Campos[col]
			v, ok := valorPegado(campo, t)
			if !ok {
				if strings.TrimSpace(t) != "" {
					saltados++
				}
				continue
			}
			datos[campo.ID] = v
		}
		return datos
	}
	var nuevos []map[string]any
	for i, linea := range filas {
		datos := valores(linea)
		fila := desde.fila + i
		if fila >= len(g.Registros) {
			nuevos = append(nuevos, datos)
			continue
		}
		r := g.Registros[fila]
		for id, v := range datos {
			if textoDeValor(r.Data[id]) == textoDeValor(v) {
				delete(datos, id)
			}
		}
		if len(datos) > 0 {
			p.cambiar(r, datos, nil)
		}
	}
	b.Estado = ""
	if saltados > 0 {
		b.Estado = p.t("grid.invalidValue")
	}
	if len(nuevos) > 0 {
		p.anadirConDatos(nuevos)
	}
	p.Repintar()
}

// anadirConDatos crea registros con sus valores, en orden, y los pone al final.
func (p *Pantalla) anadirConDatos(filas []map[string]any) {
	b := p.abierta
	tabla := b.Activa
	var creados []api.BasesRecord
	p.hacer(func() error {
		ctx, cancelar := p.ctx()
		defer cancelar()
		for _, datos := range filas {
			r, err := p.svc.CreateRecord(ctx, tabla, datos, nil)
			if err != nil {
				return err
			}
			creados = append(creados, r)
		}
		return nil
	}, func() {
		if b.Activa != tabla {
			return
		}
		for _, r := range creados {
			b.vent.anadir(r)
		}
		if b.Estado == "" {
			b.Estado = p.t("grid.added")
		}
		p.ponerRejilla()
		p.Repintar()
	})
}

// SoltarTexto recibe el texto soltado sobre la ventana (el `soltar` de la suite): si cae
// sobre una celda de la rejilla, se pega desde ella.
func (p *Pantalla) SoltarTexto(texto string, pos fyne.Position) {
	b := p.abierta
	if b == nil || b.rejilla == nil || p.ficha != nil || !puedeEscribir(b.rol()) {
		return
	}
	g := b.rejilla
	if g.r == nil || !g.Visible() {
		return
	}
	local := pos.Subtract(fyne.CurrentApp().Driver().AbsolutePositionForObject(g))
	c, ok := g.r.celdaEn(local)
	if !ok {
		return
	}
	g.enfocar(c)
	p.pegar(c, texto)
}
