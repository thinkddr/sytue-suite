// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Buscar y reemplazar en TODO el documento.
//
// La edición de texto ya existía, pero por fragmento y página: para cambiar el
// nombre de una empresa en un contrato de cuarenta páginas había que ir página
// a página buscando a ojo. Es de lo que más se usa en un editor de PDF de pago
// y de lo que peor se hace a mano.
//
// La diferencia con un procesador de textos, y hay que decirla antes: un PDF
// guarda posiciones, no párrafos. Sustituir «Acme» por «Corporación Industrial
// Acme» NO empuja el resto de la línea; el texto se encoge para caber, y pasado
// cierto punto se nota. Por eso la búsqueda devuelve, para cada coincidencia,
// si el reemplazo CABE — y la interfaz lo enseña antes de tocar nada.
package domain

import (
	"fmt"
	"strings"
)

// BusquedaReemplazo describe qué cambiar.
type BusquedaReemplazo struct {
	Buscar     string `json:"buscar"`
	Reemplazar string `json:"reemplazar"`
	// DistinguirMayusculas: por defecto NO, que es lo que espera quien busca el
	// nombre de una empresa escrito de tres formas distintas en el mismo pliego.
	DistinguirMayusculas bool `json:"distinguir_mayusculas"`
	// SoloPaginas acota la búsqueda (0-based). Vacío = todo el documento.
	SoloPaginas []int `json:"solo_paginas,omitempty"`
}

// Coincidencia es un fragmento que casa, con lo que pasaría al cambiarlo.
type Coincidencia struct {
	Pagina  int    `json:"pagina"`
	ID      int    `json:"id"`
	Antes   string `json:"antes"`
	Despues string `json:"despues"`
	// Cabe dice si el texto nuevo entra en el hueco del viejo SIN encogerlo
	// tanto que se note. Falso no impide el cambio: avisa de que ese fragmento
	// va a quedar apretado, que es la única forma honesta de ofrecer esto.
	Cabe bool `json:"cabe"`
}

// maxCoincidencias acota la respuesta. Buscar «de» en un pliego devuelve miles
// y ninguna interfaz enseña eso.
const maxCoincidencias = 500

// encogidoQueSeNota es el límite a partir del cual el ajuste entre caracteres
// deja de ser invisible. Un 12 % de más ya se lee apretado en un párrafo.
const encogidoQueSeNota = 1.12

// holguraEnCaracteres es el crecimiento absoluto que se da por bueno pase lo que
// pase con la proporción. Dos caracteres los absorbe el interletraje de
// cualquier línea; es el umbral relativo el que manda en cuanto se pasa de ahí.
const holguraEnCaracteres = 2

// BuscarParaReemplazar devuelve lo que cambiaría, sin cambiar nada.
//
// Va antes del reemplazo a propósito: es lo que permite enseñar la lista, decir
// cuáles quedan apretadas y dejar que se elijan.
func (d *PDFDoc) BuscarParaReemplazar(b BusquedaReemplazo) ([]Coincidencia, error) {
	aguja := strings.TrimSpace(b.Buscar)
	if aguja == "" {
		return nil, fmt.Errorf("pdf: no se ha indicado qué buscar")
	}
	paginas, err := paginasABuscar(d, b.SoloPaginas)
	if err != nil {
		return nil, err
	}

	var out []Coincidencia
	for _, i := range paginas {
		if len(out) >= maxCoincidencias {
			break
		}
		a, err := d.AnalizarEdicion(i)
		if err != nil {
			// Una página ilegible no tumba la búsqueda: se sigue con las demás y
			// quien mira la lista ve lo que sí se pudo leer.
			continue
		}
		for _, f := range a.Fragmentos {
			nuevo, hay := sustituir(f.Texto, aguja, b.Reemplazar, b.DistinguirMayusculas)
			if !hay {
				continue
			}
			out = append(out, Coincidencia{
				Pagina: i, ID: f.ID, Antes: f.Texto, Despues: nuevo,
				Cabe: cabeEnElHueco(f, nuevo),
			})
			if len(out) >= maxCoincidencias {
				break
			}
		}
	}
	return out, nil
}

// ReemplazarEnDocumentoPDF aplica el cambio y devuelve cuántos fragmentos tocó.
//
// `soloEstos` limita a unos IDs concretos —los que se hayan marcado en la
// lista—; vacío significa todos los que casen.
func ReemplazarEnDocumentoPDF(doc *PDFDoc, b BusquedaReemplazo, soloEstos []Coincidencia) ([]byte, int, error) {
	coincidencias, err := doc.BuscarParaReemplazar(b)
	if err != nil {
		return nil, 0, err
	}
	if len(coincidencias) == 0 {
		return nil, 0, fmt.Errorf("pdf: no se encontró %q en el documento", b.Buscar)
	}
	if len(soloEstos) > 0 {
		elegidos := make(map[[2]int]bool, len(soloEstos))
		for _, c := range soloEstos {
			elegidos[[2]int{c.Pagina, c.ID}] = true
		}
		filtradas := coincidencias[:0]
		for _, c := range coincidencias {
			if elegidos[[2]int{c.Pagina, c.ID}] {
				filtradas = append(filtradas, c)
			}
		}
		coincidencias = filtradas
		if len(coincidencias) == 0 {
			return nil, 0, fmt.Errorf("pdf: ninguna de las coincidencias elegidas sigue estando")
		}
	}

	cambios := make([]CambioDeTexto, 0, len(coincidencias))
	for _, c := range coincidencias {
		cambios = append(cambios, CambioDeTexto{Pagina: c.Pagina, ID: c.ID, Nuevo: c.Despues})
	}
	salida, err := EditarTextoPDF(doc, cambios)
	if err != nil {
		return nil, 0, err
	}
	return salida, len(cambios), nil
}

// paginasABuscar valida el filtro de páginas.
func paginasABuscar(d *PDFDoc, solo []int) ([]int, error) {
	if len(solo) == 0 {
		todas := make([]int, d.NumPaginas())
		for i := range todas {
			todas[i] = i
		}
		return todas, nil
	}
	for _, i := range solo {
		if i < 0 || i >= d.NumPaginas() {
			return nil, fmt.Errorf("pdf: la página %d no existe", i+1)
		}
	}
	return solo, nil
}

// sustituir cambia TODAS las apariciones dentro de un fragmento.
//
// Sin distinguir mayúsculas hay que recorrer a mano: `strings.ReplaceAll` sobre
// el texto en minúsculas devolvería el fragmento entero en minúsculas y se
// perdería el resto de la línea.
func sustituir(texto, aguja, nuevo string, distinguir bool) (string, bool) {
	if distinguir {
		if !strings.Contains(texto, aguja) {
			return texto, false
		}
		return strings.ReplaceAll(texto, aguja, nuevo), true
	}
	bajoTexto, bajoAguja := strings.ToLower(texto), strings.ToLower(aguja)
	if !strings.Contains(bajoTexto, bajoAguja) {
		return texto, false
	}
	var b strings.Builder
	for i := 0; i < len(texto); {
		j := strings.Index(bajoTexto[i:], bajoAguja)
		if j < 0 {
			b.WriteString(texto[i:])
			break
		}
		b.WriteString(texto[i : i+j])
		b.WriteString(nuevo)
		i += j + len(aguja)
	}
	return b.String(), true
}

// cabeEnElHueco estima si el texto nuevo entra sin apretarse de forma visible.
//
// Se compara por número de caracteres y no midiendo la fuente: el ancho exacto
// depende de la fuente vigente en ese punto del flujo, que aquí no está a mano,
// y errar por poco en un aviso es mejor que no dar el aviso. El fragmento
// conoce su ancho real, así que la proporción es la parte fiable.
//
// La proporción SOLA no vale para fragmentos cortos: en uno de cuatro letras,
// una letra más ya es un 25 % y saltaba el aviso. Cambiar «Acme» por «Sytue» no
// es lo que este aviso viene a evitar —eso es meter un párrafo donde había una
// palabra—, y un aviso que salta siempre se aprende a ignorar.
func cabeEnElHueco(f FragmentoEditable, nuevo string) bool {
	viejos := len([]rune(f.Texto))
	nuevos := len([]rune(nuevo))
	if nuevos <= viejos || viejos == 0 {
		return true
	}
	if nuevos-viejos <= holguraEnCaracteres {
		return true
	}
	return float64(nuevos)/float64(viejos) <= encogidoQueSeNota
}
