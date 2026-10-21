// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ir_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

func TestEspejoDeConstantes(t *testing.T) {
	// Lee tipos.ts por ruta relativa (patrón services/hub/internal/instaladores/instaladores_test.go:205)
	ruta := filepath.Join("..", "..", "..", "..", "apps", "frontend", "packages", "pages-render", "src", "ir", "tipos.ts")
	b, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("no se pudo leer tipos.ts en %s: %v", ruta, err)
	}
	contenido := string(b)

	// VERSION_IR
	reVer := regexp.MustCompile(`export const VERSION_IR\s*=\s*(\d+)`)
	mVer := reVer.FindStringSubmatch(contenido)
	if len(mVer) < 2 {
		t.Fatal("no se encontró VERSION_IR en tipos.ts")
	}
	verTS, _ := strconv.Atoi(mVer[1])
	if ir.VersionIR != verTS {
		t.Errorf("VersionIR Go (%d) != TS (%d)", ir.VersionIR, verTS)
	}

	// Helper para arrays de cadenas
	extraerArray := func(nombre string) []string {
		re := regexp.MustCompile(`export const ` + nombre + `\s*=\s*\[([\s\S]*?)\]\s*as const;`)
		m := re.FindStringSubmatch(contenido)
		if len(m) < 2 {
			t.Fatalf("no se encontró array %s en tipos.ts", nombre)
		}
		reCad := regexp.MustCompile(`'([^']+)'`)
		cads := reCad.FindAllStringSubmatch(m[1], -1)
		res := make([]string, 0, len(cads))
		for _, c := range cads {
			res = append(res, c[1])
		}
		return res
	}

	compararSlice := func(nombre string, goVals, tsVals []string) {
		t.Run(nombre, func(t *testing.T) {
			if !slices.Equal(goVals, tsVals) {
				t.Errorf("espejo roto en %s:\nGo: %v\nTS: %v", nombre, goVals, tsVals)
			}
		})
	}

	compararSlice("IDIOMAS", ir.Idiomas, extraerArray("IDIOMAS"))
	compararSlice("IDIOMAS_VISITANTE", ir.IdiomasVisitante, extraerArray("IDIOMAS_VISITANTE"))
	compararSlice("TIPOS_NODO", ir.TiposNodo, extraerArray("TIPOS_NODO"))
	compararSlice("TIPOS_SECCION", ir.TiposSeccion, extraerArray("TIPOS_SECCION"))
	compararSlice("ICONOS", ir.Iconos, extraerArray("ICONOS"))
	compararSlice("PROVEEDORES_INCRUSTADO", ir.ProveedoresIncrustado, extraerArray("PROVEEDORES_INCRUSTADO"))
	compararSlice("COLORES_OBLIGATORIOS", ir.ColoresObligatorios, extraerArray("COLORES_OBLIGATORIOS"))
	compararSlice("ESTILOS_TEXTO", ir.EstilosTexto, extraerArray("ESTILOS_TEXTO"))
	compararSlice("SLUGS_RESERVADOS", ir.SlugsReservados, extraerArray("SLUGS_RESERVADOS"))
	compararSlice("ANCLAS_RESERVADAS", ir.AnclasReservadas, extraerArray("ANCLAS_RESERVADAS"))

	// LIMITES
	reLim := regexp.MustCompile(`export const LIMITES\s*=\s*\{([\s\S]*?)\}\s*as const;`)
	mLim := reLim.FindStringSubmatch(contenido)
	if len(mLim) < 2 {
		t.Fatal("no se encontró LIMITES en tipos.ts")
	}
	rePar := regexp.MustCompile(`([a-zA-Z]+):\s*([0-9_]+)`)
	pares := rePar.FindAllStringSubmatch(mLim[1], -1)
	limitesTS := make(map[string]int)
	for _, p := range pares {
		limpio := strings.ReplaceAll(p[2], "_", "")
		n, _ := strconv.Atoi(limpio)
		limitesTS[strings.ToLower(p[1])] = n
	}

	checkLim := func(nombre string, goVal int) {
		tsVal, ok := limitesTS[strings.ToLower(nombre)]
		if !ok {
			t.Errorf("límite %s no encontrado en TS", nombre)
			return
		}
		if goVal != tsVal {
			t.Errorf("límite %s: Go=%d, TS=%d", nombre, goVal, tsVal)
		}
	}

	checkLim("irBytes", ir.Limites.IRBytes)
	checkLim("paginas", ir.Limites.Paginas)
	checkLim("seccionesPorPagina", ir.Limites.SeccionesPorPagina)
	checkLim("nodosPorPagina", ir.Limites.NodosPorPagina)
	checkLim("profundidad", ir.Limites.Profundidad)
	checkLim("componentes", ir.Limites.Componentes)
	checkLim("nodosPorComponente", ir.Limites.NodosPorComponente)
	checkLim("caracteresPorTexto", ir.Limites.CaracteresPorTexto)
	checkLim("coloresTema", ir.Limites.ColoresTema)
	checkLim("camposFormulario", ir.Limites.CamposFormulario)
	checkLim("elementosMenu", ir.Limites.ElementosMenu)
	checkLim("hijosPorElementoMenu", ir.Limites.HijosPorElementoMenu)
	checkLim("productosEnRejilla", ir.Limites.ProductosEnRejilla)
	checkLim("filasTabla", ir.Limites.FilasTabla)
	checkLim("columnasTabla", ir.Limites.ColumnasTabla)
	checkLim("slug", ir.Limites.Slug)
	checkLim("url", ir.Limites.URL)
}
