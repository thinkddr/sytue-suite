// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package plantillas

import (
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// BLOQUES REUTILIZABLES (ROADMAP-OFFICE plantillas-1): piezas que se insertan en un
// documento que ya existe, desde la cinta. Comparten con la galería la máquina de
// modelos con huecos y los textos en los 26 idiomas; lo que cambia es la forma:
//
//   - Documento (`bloques/<clave>.html`): un fragmento del HTML del editor.
//   - Presentación (`bloques/<clave>.json`): UNA diapositiva `{id, boxes}`; quien la
//     inserta le pone ids nuevos.
//   - Hoja (`bloques/<clave>.json`): `{cells, formats, styles, charts}` con las
//     referencias contadas desde A1; quien lo inserta lo desplaza a la celda activa.
//     Una celda con `"tema"` en su estilo es la que se viste con el tema o la marca.
//
// El contenido es obra original de Sytue, como el de la galería.

// catalogoBloques fija el orden en la cinta y el tipo de cada bloque.
var catalogoBloques = []struct {
	clave string
	kind  domain.Kind
}{
	{"b_portada", domain.KindDocument},
	{"b_firmas", domain.KindDocument},
	{"b_resumen", domain.KindDocument},
	{"b_legal", domain.KindDocument},
	{"b_cabecera", domain.KindSpreadsheet},
	{"b_totales", domain.KindSpreadsheet},
	{"b_grafico", domain.KindSpreadsheet},
	{"b_agenda", domain.KindPresentation},
	{"b_comparativa", domain.KindPresentation},
	{"b_cierre", domain.KindPresentation},
}

// ListarBloques devuelve los bloques de un tipo en el idioma pedido, en su orden.
func ListarBloques(kind domain.Kind, idioma string) ([]Info, error) {
	if err := cargar(); err != nil {
		return nil, err
	}
	idioma = Idioma(idioma)
	out := []Info{}
	for _, b := range catalogoBloques {
		if b.kind == kind {
			out = append(out, info(b.clave, b.kind, idioma))
		}
	}
	return out, nil
}

// GenerarBloque devuelve el bloque en el idioma pedido y vestido con `tema` (nil = sin
// tema: el bloque sale neutro). ErrNoExiste si la clave no está.
func GenerarBloque(clave, idioma string, tema *domain.TemaComun) (Info, string, error) {
	if err := cargar(); err != nil {
		return Info{}, "", err
	}
	idioma = Idioma(idioma)
	for _, b := range catalogoBloques {
		if b.clave != clave {
			continue
		}
		contenido, err := rellenar("bloques/"+clave, b.kind, idioma)
		if err != nil {
			return Info{}, "", err
		}
		t := domain.TemaComun{ID: domain.IDTemaPapel}
		if tema != nil {
			t = *tema
		}
		contenido, err = domain.AplicarTemaBloque(b.kind, contenido, t)
		if err != nil {
			return Info{}, "", err
		}
		return info(clave, b.kind, idioma), contenido, nil
	}
	return Info{}, "", ErrNoExiste
}
