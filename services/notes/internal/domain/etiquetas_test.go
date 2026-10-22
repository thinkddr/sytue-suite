// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// `TagInput.Normalize` es la puerta por la que entra TODO lo que el cliente dice
// de una etiqueta y sus estados, y no tenía ni un test. Lo que valida no es
// cosmético: los estados de una etiqueta se pintan en cada nota que la lleve, y
// un color que no es un color o un atajo de tres letras se propagan a la
// interfaz de todo el mundo.
//
// También fija lo que NORMALIZA, no solo lo que rechaza: el recorte de espacios
// y el color por defecto son parte del contrato: si desaparecieran, entrarían en
// la base de datos nombres con espacios al final que luego no casan al buscar.

func etiquetaValida() TagInput {
	return TagInput{
		Name:   "Prioridad",
		States: []TagStateInput{{Name: "Alta", Color: "#ff0000"}},
	}
}

func TestTagInputNormalizeAceptaLoBueno(t *testing.T) {
	in := etiquetaValida()
	if err := in.Normalize(); err != nil {
		t.Fatalf("rechazó una etiqueta válida: %v", err)
	}
}

func TestTagInputNormalizeRecortaYRellena(t *testing.T) {
	in := TagInput{
		Name:   "  Prioridad  ",
		States: []TagStateInput{{Name: "  Alta  ", Emoji: " 🔥 ", Shortcut: " a ", Color: ""}},
	}
	if err := in.Normalize(); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if in.Name != "Prioridad" {
		t.Errorf("nombre = %q; los espacios de los extremos tienen que irse", in.Name)
	}
	s := in.States[0]
	if s.Name != "Alta" || s.Emoji != "🔥" || s.Shortcut != "a" {
		t.Errorf("estado sin recortar: %+v", s)
	}
	// El color vacío NO es un error: se rellena. Es lo que permite crear un estado
	// desde la interfaz sin abrir el selector de color.
	if s.Color == "" {
		t.Error("el color vacío tenía que rellenarse con el de por defecto")
	}
	if !ValidHexColor(s.Color) {
		t.Errorf("el color por defecto no es válido: %q", s.Color)
	}
}

func TestTagInputNormalizeRechaza(t *testing.T) {
	casos := []struct {
		nombre string
		in     TagInput
	}{
		{"nombre vacío", TagInput{Name: "", States: etiquetaValida().States}},
		{"nombre solo espacios", TagInput{Name: "   ", States: etiquetaValida().States}},
		{
			"nombre de más de 60 caracteres",
			TagInput{Name: strings.Repeat("a", 61), States: etiquetaValida().States},
		},
		// SIN ESTADOS NO HAY ETIQUETA: una etiqueta sin valores no se puede aplicar
		// a nada, así que sería una fila muerta que además sale en el selector.
		{"sin estados", TagInput{Name: "Prioridad"}},
		{"más de 20 estados", TagInput{Name: "P", States: nEstados(21)}},
		{
			"estado sin nombre",
			TagInput{Name: "P", States: []TagStateInput{{Name: "  ", Color: "#ff0000"}}},
		},
		{
			"estado con nombre de más de 60",
			TagInput{Name: "P", States: []TagStateInput{{Name: strings.Repeat("b", 61), Color: "#ff0000"}}},
		},
		{
			"emoji demasiado largo",
			TagInput{Name: "P", States: []TagStateInput{{Name: "A", Emoji: strings.Repeat("🔥", 17), Color: "#ff0000"}}},
		},
		// El atajo es UNA tecla: es lo que se pulsa para aplicar el estado, y con
		// dos caracteres no hay pulsación que lo produzca.
		{
			"atajo de más de una tecla",
			TagInput{Name: "P", States: []TagStateInput{{Name: "A", Shortcut: "ab", Color: "#ff0000"}}},
		},
		{
			"color que no es un color",
			TagInput{Name: "P", States: []TagStateInput{{Name: "A", Color: "rojo"}}},
		},
		{
			"color sin almohadilla",
			TagInput{Name: "P", States: []TagStateInput{{Name: "A", Color: "ff0000"}}},
		},
		{
			"color de tres dígitos",
			TagInput{Name: "P", States: []TagStateInput{{Name: "A", Color: "#f00"}}},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			in := c.in
			err := in.Normalize()
			if err == nil {
				t.Fatal("lo aceptó")
			}
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("error = %v; quiero ErrInvalidInput para que el handler responda 400", err)
			}
		})
	}
}

// Justo en el límite: 60 caracteres y 20 estados SÍ entran. Sin esto, un cambio
// de `>` a `>=` pasaría los tests de arriba y estrecharía el producto en
// silencio.
func TestTagInputNormalizeEnElLimite(t *testing.T) {
	in := TagInput{Name: strings.Repeat("a", 60), States: nEstados(20)}
	if err := in.Normalize(); err != nil {
		t.Fatalf("rechazó el máximo permitido: %v", err)
	}
}

func nEstados(n int) []TagStateInput {
	out := make([]TagStateInput, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, TagStateInput{Name: "e", Color: "#ff0000"})
	}
	return out
}

// LinkKindFor traduce el tipo de una nota al tipo de destino de su enlace. Es
// una tabla, y lo que importa es que ningún tipo se quede fuera por descuido:
// una nota de tipo «evento» sin enlace es una nota que no lleva a ninguna parte.
func TestLinkKindFor(t *testing.T) {
	casos := []struct {
		kind   Kind
		quiero string
	}{
		{KindEvent, TargetCalendarEvent},
		{KindDoc, TargetDocsDocument},
		{KindDrive, TargetDriveFile},
		{KindRoom, TargetRoomChannel},
		{KindCrossref, TargetNote},
	}
	for _, c := range casos {
		t.Run(string(c.kind), func(t *testing.T) {
			got, ok := LinkKindFor(c.kind)
			if !ok || got != c.quiero {
				t.Errorf("LinkKindFor(%q) = (%q, %v); quiero (%q, true)", c.kind, got, ok, c.quiero)
			}
		})
	}
	// Un tipo que no enlaza a nada tiene que decirlo con el booleano, no con una
	// cadena vacía que alguien pueda guardar como si fuera un destino.
	for _, k := range []Kind{KindText, KindTodo, KindImage} {
		if got, ok := LinkKindFor(k); ok || got != "" {
			t.Errorf("LinkKindFor(%q) = (%q, %v); quiero (\"\", false)", k, got, ok)
		}
	}
}

// `LegalHold.Activo` decide si el recolector puede destruir lo que el hold
// cubre. Es una comparación de una línea y por eso es fácil no probarla; pero si
// se invirtiera, la purga se saltaría los holds sin dar ningún error.
func TestLegalHoldActivo(t *testing.T) {
	if !(LegalHold{}).Activo() {
		t.Error("un hold sin fecha de liberación tiene que estar ACTIVO")
	}
	liberado := time.Now()
	if (LegalHold{ReleasedAt: &liberado}).Activo() {
		t.Error("un hold con fecha de liberación ya NO preserva nada")
	}
}
