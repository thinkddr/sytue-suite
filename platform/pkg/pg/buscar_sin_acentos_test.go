// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// BUSCAR SIN ACENTOS NI MAYÚSCULAS, EN TODAS PARTES. «reunion» encuentra «reunión» y
// «Muller» encuentra «Müller» porque cada buscador pasa por la MISMA regla: plegar()
// en SQL, buscar.Plegar/buscar.Contiene en Go (apps/nucleo/buscar) y plegar/contiene
// de @sytue/i18n en la web. Estos guardianes miran el CÓDIGO FUENTE, porque el fallo
// se comete al escribir: un `ILIKE` nuevo pasa todos los tests de su servicio y solo
// se nota cuando alguien busca «cafe» y no sale «Café».
//
// SI TE PARAN: usa plegar() / buscar.Contiene. Las excepciones son sitios que NO son
// un buscador (comprobar un nombre ocupado, una cabecera) y llevan su porqué.

var raizRepo = filepath.Join("..", "..", "..")

// excepcionesSQL: fichero → motivo. Se compara por IGUALDAD con lo encontrado, así
// que una excepción que ya no hace falta también falla.
var excepcionesSQL = map[string]string{
	"services/id/internal/store/users.go": "usernameLibre: qué nombres de usuario [a-z0-9.-] están ocupados, no un buscador",
}

// excepcionesGo: igual, para los filtros escritos en Go.
var excepcionesGo = map[string]string{
	"apps/fyne/calcus/condicional.go": "formato condicional «el texto contiene» de la hoja: la semántica de Excel " +
		"(y de format.ts en la web), que distingue acentos",
}

// excepcionesNFD: normalizadores hechos a mano que NO son un buscador.
var excepcionesNFD = map[string]string{
	"apps/nucleo/buscar/buscar.go":                     "es la regla",
	"apps/fyne/apps/notes/escritura.go":                "slugDe: el ancla `h=` de un título, igual que la de la web",
	"apps/fyne/apps/hub/bienvenida.go":                 "Slugify: el identificador ASCII de la organización, igual que el de la web",
	"services/docs/internal/domain/pdfcodificacion.go": "nombres de glifos Adobe: conserva mayúsculas y diacríticos al descomponer; no es un buscador",
	"services/docs/internal/domain/pdffont.go":         "métricas AFM de las 14 fuentes estándar PDF: aproxima el ancho de caracteres acentuados por su base ASCII; no es un buscador",
}

var (
	reILike    = regexp.MustCompile(`(?i)\bILIKE\b`)
	reLowerLik = regexp.MustCompile(`(?i)lower\([^)]*\)\s*(NOT\s+)?LIKE\b`)
	reTSQuery  = regexp.MustCompile(`(?i)to_tsquery\(`)
	// strings.Contains sobre un ToLower contra algo que NO es un literal: eso es buscar
	// lo que alguien ha escrito, no comprobar una cabecera. HasPrefix no entra: aquí
	// sirve para esquemas y prefijos técnicos, no para buscar.
	reGoFiltro = regexp.MustCompile(`strings\.Contains\(strings\.ToLower\([^)]*\),\s*[^"\s]`)
)

func fuentesGo(t *testing.T, dirs ...string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, dir := range dirs {
		err := filepath.WalkDir(filepath.Join(raizRepo, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "vendor" || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(raizRepo, p)
			out[filepath.ToSlash(rel)] = strings.Split(string(b), "\n")
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func esComentario(l string) bool { return strings.HasPrefix(strings.TrimSpace(l), "//") }

func TestLasBusquedasSQLPasanPorPlegar(t *testing.T) {
	var malas []string
	encontradas := map[string]bool{}
	for rel, lineas := range fuentesGo(t, "services", "platform") {
		for i, l := range lineas {
			if esComentario(l) {
				continue
			}
			mala := reILike.MatchString(l) || reLowerLik.MatchString(l) ||
				(reTSQuery.MatchString(l) && !strings.Contains(l, "plegar("))
			if !mala {
				continue
			}
			if _, ok := excepcionesSQL[rel]; ok {
				encontradas[rel] = true
				continue
			}
			malas = append(malas, rel+":"+strconv.Itoa(i+1)+"  "+strings.TrimSpace(l))
		}
	}
	slices.Sort(malas)
	if len(malas) > 0 {
		t.Errorf("búsqueda SQL sin plegar(): «reunion» no encontraría «reunión». Usa "+
			"`plegar(col) LIKE '%%' || plegar($n) || '%%'` (y su índice sobre plegar(col)):\n  %s",
			strings.Join(malas, "\n  "))
	}
	for rel := range excepcionesSQL {
		if !encontradas[rel] {
			t.Errorf("%s ya no lleva el patrón: quita su excepción", rel)
		}
	}
}

func TestLosFiltrosGoPasanPorBuscar(t *testing.T) {
	var malos []string
	encontrados := map[string]bool{}
	for rel, lineas := range fuentesGo(t, "services", "platform", "apps/fyne", "apps/nucleo") {
		for i, l := range lineas {
			if esComentario(l) || !reGoFiltro.MatchString(l) {
				continue
			}
			if _, ok := excepcionesGo[rel]; ok {
				encontrados[rel] = true
				continue
			}
			malos = append(malos, rel+":"+strconv.Itoa(i+1)+"  "+strings.TrimSpace(l))
		}
	}
	slices.Sort(malos)
	if len(malos) > 0 {
		t.Errorf("filtro de texto sin buscar.Contiene/buscar.Plegar (apps/nucleo/buscar):\n  %s",
			strings.Join(malos, "\n  "))
	}
	for rel := range excepcionesGo {
		if !encontrados[rel] {
			t.Errorf("%s ya no lleva el patrón: quita su excepción", rel)
		}
	}
}

func TestNadieNormalizaAMano(t *testing.T) {
	var malos []string
	encontrados := map[string]bool{}
	for rel, lineas := range fuentesGo(t, "services", "platform", "apps/fyne", "apps/nucleo") {
		for i, l := range lineas {
			if esComentario(l) || !strings.Contains(l, "norm.NFD") {
				continue
			}
			if _, ok := excepcionesNFD[rel]; ok {
				encontrados[rel] = true
				continue
			}
			malos = append(malos, rel+":"+strconv.Itoa(i+1)+"  "+strings.TrimSpace(l))
		}
	}
	slices.Sort(malos)
	if len(malos) > 0 {
		t.Errorf("normalización hecha a mano: usa buscar.Plegar / buscar.Contiene / buscar.Rangos:\n  %s",
			strings.Join(malos, "\n  "))
	}
	for rel := range excepcionesNFD {
		if !encontrados[rel] {
			t.Errorf("%s ya no normaliza a mano: quita su excepción", rel)
		}
	}
}

// plegar() se define en una migración de cada base. Las definiciones tienen que ser
// IDÉNTICAS: una base que pliegue distinto encuentra otra cosa con la misma búsqueda.
func TestPlegarSeDefineIgualEnTodasLasBases(t *testing.T) {
	re := regexp.MustCompile(`(?s)CREATE EXTENSION IF NOT EXISTS unaccent;\s*CREATE OR REPLACE FUNCTION plegar\(.*?;\n`)
	ficheros, err := filepath.Glob(filepath.Join(raizRepo, "services", "*", "db", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	defs := map[string][]string{}
	for _, f := range ficheros {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "FUNCTION plegar(") {
			continue
		}
		m := re.FindString(string(b))
		if m == "" {
			t.Errorf("%s define plegar() sin el bloque canónico", f)
			continue
		}
		defs[m] = append(defs[m], filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(f)))))
	}
	if len(defs) != 1 {
		t.Errorf("plegar() se define de %d formas distintas: %v", len(defs), defs)
	}
	for _, bases := range defs {
		if len(bases) < 10 {
			t.Errorf("plegar() solo está en %v", bases)
		}
	}
}
