// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ir_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

var update = flag.Bool("update", false, "actualiza los ficheros de oro en testdata/emitidos/")

func TestIdaYVueltaSemantica(t *testing.T) {
	archivos, err := filepath.Glob(filepath.Join("testdata", "validos", "*.json"))
	if err != nil || len(archivos) == 0 {
		t.Fatalf("no se encontraron fixtures válidas: %v", err)
	}

	for _, arch := range archivos {
		t.Run(filepath.Base(arch), func(t *testing.T) {
			b, err := os.ReadFile(arch)
			if err != nil {
				t.Fatalf("leer %s: %v", arch, err)
			}
			probs := ir.ValidarCrudo(b)
			if len(probs) > 0 {
				t.Fatalf("ValidarCrudo devolvió errores en %s: %+v", arch, probs)
			}

			s, err := ir.Decodificar(b)
			if err != nil {
				t.Fatalf("Decodificar falló en %s: %v", arch, err)
			}

			probsSem := ir.Validar(s, nil, ir.ModoPublicar)
			for _, p := range probsSem {
				if p.Severity == "error" {
					t.Errorf("error semántico en fixture válida %s: %+v", arch, p)
				}
			}

			b2, err := ir.Codificar(s)
			if err != nil {
				t.Fatalf("Codificar falló: %v", err)
			}

			var x1, x2 any
			if err := json.Unmarshal(b, &x1); err != nil {
				t.Fatalf("json.Unmarshal b: %v", err)
			}
			if err := json.Unmarshal(b2, &x2); err != nil {
				t.Fatalf("json.Unmarshal b2: %v", err)
			}

			if !reflect.DeepEqual(x1, x2) {
				t.Errorf("ida y vuelta semántica rota en %s", arch)
			}
		})
	}
}

func TestInvalidos(t *testing.T) {
	archivos, err := filepath.Glob(filepath.Join("testdata", "invalidos", "*.json"))
	if err != nil || len(archivos) == 0 {
		t.Fatalf("no se encontraron fixtures inválidas: %v", err)
	}

	for _, arch := range archivos {
		t.Run(filepath.Base(arch), func(t *testing.T) {
			b, err := os.ReadFile(arch)
			if err != nil {
				t.Fatalf("leer %s: %v", arch, err)
			}

			var wrapper struct {
				Modo   string        `json:"modo"`
				IR     any           `json:"ir"`
				Espera []ir.Problema `json:"espera"`
			}
			if err := json.Unmarshal(b, &wrapper); err != nil {
				t.Fatalf("unmarshal wrapper en %s: %v", arch, err)
			}

			irBytes, err := json.Marshal(wrapper.IR)
			if err != nil {
				t.Fatalf("marshal ir: %v", err)
			}

			probs := ir.ValidarCrudo(irBytes)
			if len(probs) == 0 {
				modo := ir.ModoBorrador
				if wrapper.Modo == "publicar" {
					modo = ir.ModoPublicar
				}
				s, err := ir.Decodificar(irBytes)
				if err == nil {
					probs = ir.Validar(s, nil, modo)
				}
			}

			// Comparar problemas obtenidos con problemas esperados
			normalizar := func(ps []ir.Problema) []ir.Problema {
				slices.SortFunc(ps, func(a, b ir.Problema) int {
					if c := strings.Compare(a.Path, b.Path); c != 0 {
						return c
					}
					if c := strings.Compare(a.Code, b.Code); c != 0 {
						return c
					}
					return strings.Compare(a.Severity, b.Severity)
				})
				return ps
			}

			esperaNorm := normalizar(wrapper.Espera)
			probsNorm := normalizar(probs)

			// Verificar que cada problema esperado está en probsNorm
			for _, exp := range esperaNorm {
				encontrado := false
				for _, got := range probsNorm {
					if got.Path == exp.Path && got.Code == exp.Code && got.Severity == exp.Severity {
						encontrado = true
						break
					}
				}
				if !encontrado {
					t.Errorf("en %s: esperaba problema %+v pero se obtuvo: %+v", arch, exp, probsNorm)
				}
			}
		})
	}
}

func TestCruzadoDeVerdad(t *testing.T) {
	archivos, err := filepath.Glob(filepath.Join("testdata", "validos", "*.json"))
	if err != nil || len(archivos) == 0 {
		t.Fatalf("no se encontraron fixtures válidas: %v", err)
	}

	emitidosDir := filepath.Join("testdata", "emitidos")
	_ = os.MkdirAll(emitidosDir, 0o755)

	for _, arch := range archivos {
		base := filepath.Base(arch)
		if strings.HasPrefix(base, "limite-") {
			// saltar ficheros de límite de 2MB para emitidos
			continue
		}
		dest := filepath.Join(emitidosDir, base)

		b, err := os.ReadFile(arch)
		if err != nil {
			t.Fatalf("leer %s: %v", arch, err)
		}
		s, err := ir.Decodificar(b)
		if err != nil {
			t.Fatalf("decodificar %s: %v", arch, err)
		}
		emitido, err := ir.Codificar(s)
		if err != nil {
			t.Fatalf("codificar %s: %v", arch, err)
		}

		if *update {
			var pretty bytes.Buffer
			_ = json.Indent(&pretty, emitido, "", "  ")
			pretty.WriteByte('\n')
			if err := os.WriteFile(dest, pretty.Bytes(), 0o644); err != nil {
				t.Fatalf("escribir emitido %s: %v", dest, err)
			}
		} else {
			disco, err := os.ReadFile(dest)
			if err != nil {
				t.Fatalf("falta fichero emitido %s (ejecuta go test -update): %v", dest, err)
			}
			var x1, x2 any
			_ = json.Unmarshal(emitido, &x1)
			_ = json.Unmarshal(disco, &x2)
			if !reflect.DeepEqual(x1, x2) {
				t.Errorf("emitido desactualizado para %s (ejecuta go test -update)", base)
			}
		}
	}
}
