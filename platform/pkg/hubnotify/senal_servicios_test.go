// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// senalDe es qué productos manda cada servicio con TrasEscribir: Drive también el de
// CertiPDF (los PDF de su carpeta), y Docs los tres editores y las peticiones de firma.
var senalDe = map[string][]string{
	"bases": {"bases"}, "calendar": {"calendar"}, "cast": {"cast"}, "contacts": {"contacts"},
	"docs": {"docs", "pdf", "sheets", "slides"}, "drive": {"drive", "pdf"}, "mail": {"mail"},
	"notes": {"notes"}, "rooms": {"rooms"}, "syerp": {"erp"}, "syphotos": {"photos"},
}

// Cada servicio de un producto con widget monta la señal en sus escrituras, y entre
// todos cubren la lista cerrada. Sin ella el widget no se entera de lo que cambia en
// la web, en otro aparato o por otra persona hasta el latido de 10 minutos. Visto en
// rojo quitando el `api.Use(cambios)` de Contacts (y su CardDAV).
func TestCadaServicioMandaLaSenalDeCambios(t *testing.T) {
	producto := regexp.MustCompile(`"([a-z]+)"`)
	cubiertos := map[string]bool{}
	for svc, quiero := range senalDe {
		vistos := map[string]bool{}
		raiz := filepath.Join("..", "..", "..", "services", svc)
		err := filepath.WalkDir(raiz, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, productos := range productosDeTrasEscribir(string(b)) {
				for _, q := range producto.FindAllStringSubmatch(productos, -1) {
					vistos[q[1]] = true
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		tengo := make([]string, 0, len(vistos))
		for p := range vistos {
			tengo = append(tengo, p)
			cubiertos[p] = true
		}
		slices.Sort(tengo)
		if !slices.Equal(tengo, quiero) {
			t.Errorf("services/%s manda la señal de %v, quiero %v", svc, tengo, quiero)
		}
	}
	for _, p := range ProductosDeDatos() {
		if !cubiertos[p] {
			t.Errorf("ningún servicio manda la señal de %q", p)
		}
	}
}

// productosDeTrasEscribir es, de cada llamada a TrasEscribir del código, lo que va
// detrás de su primer argumento (`saltar`, que puede ser una función entera): los
// productos.
func productosDeTrasEscribir(codigo string) []string {
	var out []string
	for resto := codigo; ; {
		i := strings.Index(resto, "TrasEscribir(")
		if i < 0 {
			return out
		}
		resto = resto[i+len("TrasEscribir("):]
		nivel, coma := 0, -1
		for j, c := range resto {
			switch c {
			case '(', '{', '[':
				nivel++
			case ')', '}', ']':
				nivel--
			case ',':
				if nivel == 0 && coma < 0 {
					coma = j
				}
			}
			if nivel < 0 {
				if coma >= 0 {
					out = append(out, resto[coma:j])
				}
				break
			}
		}
	}
}
