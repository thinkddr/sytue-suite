// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package interpretar

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

var ahora = time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)

// El caso que justifica todo: una frase con origen, tipo y rango.
func TestDe_FraseCompleta(t *testing.T) {
	in, err := De(`{"texto":"factura Acme","fuentes":["drive"],"tipos":["file"],
		"desde":"2026-08-01T00:00:00Z","hasta":"2026-08-31T23:59:59Z"}`,
		"facturas de Acme del mes pasado en Drive", ahora)
	if err != nil {
		t.Fatal(err)
	}
	if in.Texto != "factura Acme" {
		t.Fatalf("texto: %q", in.Texto)
	}
	if len(in.Fuentes) != 1 || in.Fuentes[0] != domain.SourceDrive {
		t.Fatalf("fuentes: %v", in.Fuentes)
	}
	if len(in.Tipos) != 1 || in.Tipos[0] != "file" {
		t.Fatalf("tipos: %v", in.Tipos)
	}
	if in.Desde == nil || in.Hasta == nil {
		t.Fatal("el rango debería estar puesto")
	}
	if in.Consulta.Text != in.Texto || in.Consulta.From != in.Desde {
		t.Fatal("la consulta no refleja lo interpretado")
	}
}

// El vallado ```json y el «Aquí tienes:» los pone el modelo aunque se le diga
// que no.
func TestDe_ConVallado(t *testing.T) {
	in, err := De("Aquí tienes:\n```json\n{\"texto\":\"acta\"}\n```", "acta", ahora)
	if err != nil {
		t.Fatal(err)
	}
	if in.Texto != "acta" {
		t.Fatalf("texto: %q", in.Texto)
	}
}

// Un origen inventado no puede filtrar: los conectores no lo conocen y la
// búsqueda se quedaría sin resultados sin decir por qué.
func TestDe_DescartaLoDesconocido(t *testing.T) {
	in, err := De(`{"texto":"x","fuentes":["Sytue Drive","mail","mail"],"tipos":["fichero","message"]}`, "x", ahora)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Fuentes) != 1 || in.Fuentes[0] != domain.SourceMail {
		t.Fatalf("fuentes: %v", in.Fuentes)
	}
	if len(in.Tipos) != 1 || in.Tipos[0] != "message" {
		t.Fatalf("tipos: %v", in.Tipos)
	}
}

// Una fecha en prosa NO es una fecha. Sin esto, «el jueves» se convertiría en
// un rango inventado que esconde resultados.
func TestDe_FechaEnProsaNoFiltra(t *testing.T) {
	in, err := De(`{"texto":"acta","desde":"el jueves pasado"}`, "acta del jueves pasado", ahora)
	if err != nil {
		t.Fatal(err)
	}
	if in.Desde != nil {
		t.Fatalf("no debería haber rango: %v", in.Desde)
	}
}

// Un rango al revés acotaría a la nada.
func TestDe_RangoAlReves(t *testing.T) {
	in, err := De(`{"texto":"x","desde":"2026-09-01","hasta":"2026-08-01"}`, "x", ahora)
	if err != nil {
		t.Fatal(err)
	}
	if in.Hasta != nil {
		t.Fatal("el extremo que sobra debería caerse")
	}
	if in.Desde == nil {
		t.Fatal("el otro extremo debería quedarse")
	}
}

// Interpretar no puede empeorar lo que ya funcionaba.
func TestDe_SinNadaEntendidoDevuelveLoOriginal(t *testing.T) {
	in, err := De(`{}`, "  memoria anual  ", ahora)
	if err != nil {
		t.Fatal(err)
	}
	if in.Texto != "memoria anual" {
		t.Fatalf("texto: %q", in.Texto)
	}
}

func TestDe_NoEsJSON(t *testing.T) {
	if _, err := De("lo siento, no lo he entendido", "x", ahora); err == nil {
		t.Fatal("debería fallar")
	}
}

// El filtro es de COSTE: una llamada al modelo por cada tecla de «fact» sería
// pagar seis veces por nada.
func TestMereceInterpretarse(t *testing.T) {
	if MereceInterpretarse("factura") || MereceInterpretarse("factura acme") {
		t.Fatal("una o dos palabras no dan para interpretar")
	}
	if !MereceInterpretarse("facturas de Acme del mes pasado") {
		t.Fatal("una frase sí")
	}
	if MereceInterpretarse(string(make([]rune, MaxConsulta+1))) {
		t.Fatal("un pegote no es una consulta")
	}
}

func TestInstruccion_LlevaLaHora(t *testing.T) {
	if got := Instruccion(ahora); !contieneSub(got, "2026-09-10T09:00:00Z") {
		t.Fatalf("sin la hora, «el mes pasado» no significa nada: %s", got)
	}
}

func contieneSub(s, sub string) bool { return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0) }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
