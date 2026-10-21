// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package office

import (
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Vectores es la tabla de casos que comparten la web y este paquete.
const Vectores = "apps/frontend/packages/office/test/vectores/macros.json"

type caso struct {
	Nombre      string  `json:"nombre"`
	Kind        string  `json:"kind"`
	Editor      string  `json:"editor"`
	Cmd         string  `json:"cmd"`
	Valor       *string `json:"valor"`
	SoloLectura bool    `json:"soloLectura"`
	Esperado    *string `json:"esperado"`
}

// TestValidacionDeMacrosCoincideConLaWeb: los mismos casos que `macros.test.ts` exige a la
// validación de TypeScript. Si uno cambia, cambia en los dos lados.
//
// POR VER EN ROJO: invirtiendo el `edita && e.SoloLectura` de ValidarPaso deberían fallar los
// tres casos de solo lectura. (El guardián equivalente de la web, `macros.test.ts`, sí se vio
// en rojo; este no se ha podido ejecutar todavía: el contenedor de pruebas estaba sin PID.)
func TestValidacionDeMacrosCoincideConLaWeb(t *testing.T) {
	var doc struct {
		Origenes struct {
			Propio string   `json:"propio"`
			Drive  []string `json:"drive"`
		} `json:"origenes"`
		Casos []caso `json:"casos"`
	}
	if err := json.Unmarshal([]byte(webref.Read(t, Vectores)), &doc); err != nil {
		t.Fatalf("%s: %v", Vectores, err)
	}
	// Sin esto, un parser que dejara de ver los casos pasaría en verde.
	if len(doc.Casos) < 60 {
		t.Fatalf("solo se leen %d casos", len(doc.Casos))
	}
	for _, c := range doc.Casos {
		t.Run(c.Nombre, func(t *testing.T) {
			p := Paso{Editor: c.Editor, Cmd: c.Cmd}
			if c.Valor != nil {
				p.Valor, p.ConValor = *c.Valor, true
			}
			e := Entorno{
				Kind: c.Kind, SoloLectura: c.SoloLectura,
				Origenes: Origenes{Propio: doc.Origenes.Propio, Drive: doc.Origenes.Drive},
			}
			quiero := Valido
			if c.Esperado != nil {
				quiero = Rechazo(*c.Esperado)
			}
			if got := ValidarPaso(p, e); got != quiero {
				t.Errorf("%s %q: %q, y la web dice %q", c.Cmd, p.Valor, got, quiero)
			}
		})
	}
}

func TestValidarMacroEsTodoONada(t *testing.T) {
	e := Entorno{Kind: Documento}
	buena := []Paso{{Editor: Documento, Cmd: "bold"}, {Editor: Documento, Cmd: "color", Valor: "#DC2626", ConValor: true}}
	if r, _ := ValidarMacro(buena, e); r != Valido {
		t.Fatalf("una macro buena valida: %q", r)
	}
	mala := append(append([]Paso{}, buena...), Paso{Editor: Documento, Cmd: "font", Valor: "Papyrus", ConValor: true}, Paso{Editor: Documento, Cmd: "italic"})
	if r, i := ValidarMacro(mala, e); r != ValorMalo || i != 2 {
		t.Fatalf("el primer paso malo la rechaza entera: %q en %d", r, i)
	}
	if r, _ := ValidarMacro(buena, Entorno{Kind: Documento, SoloLectura: true}); r != SoloLectura {
		t.Fatalf("en solo lectura no se reproduce: %q", r)
	}
}

func TestLosTechos(t *testing.T) {
	e := Entorno{Kind: Documento}
	larga := make([]Paso, MaxPasos+1)
	for i := range larga {
		larga[i] = Paso{Editor: Documento, Cmd: "bold"}
	}
	if r, _ := ValidarMacro(larga, e); r != Limite {
		t.Errorf("un paso de más pasa el techo: %q", r)
	}
	if r, _ := ValidarMacro(larga[:MaxPasos], e); r != Valido {
		t.Errorf("el techo exacto vale: %q", r)
	}
	grande := Paso{Editor: Hoja, Cmd: "style", Valor: `{"x":"` + strings.Repeat("a", MaxValor) + `"}`, ConValor: true}
	if r := ValidarPaso(grande, Entorno{Kind: Hoja}); r != ValorMalo {
		t.Errorf("un valor desmesurado se rechaza: %q", r)
	}
	// Todo lo no reproducible está en el registro: si no, la lista de exclusión sería ruido.
	for id := range NoReproducibles {
		if _, ok := BuscarComando(id); !ok {
			t.Errorf("%s está excluido pero no figura en el registro", id)
		}
	}
}
