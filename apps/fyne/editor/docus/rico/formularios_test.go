// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

// vectoresFormularios son los de testdata/formularios.json, que recorre también
// `packages/office/test/formfields.test.ts`.
type vectoresFormularios struct {
	Opciones []struct {
		Crudo    string   `json:"crudo"`
		Opciones []string `json:"opciones"`
	} `json:"opciones"`
	Valores []struct {
		Tipo     string   `json:"tipo"`
		Crudo    string   `json:"crudo"`
		Opciones []string `json:"opciones"`
		Valor    *string  `json:"valor"`
	} `json:"valores"`
	Respuestas []struct {
		Nombre     string      `json:"nombre"`
		HTML       string      `json:"html"`
		Respuestas []Respuesta `json:"respuestas"`
		Texto      string      `json:"texto"`
	} `json:"respuestas"`
}

func leerVectoresFormularios(t *testing.T) vectoresFormularios {
	t.Helper()
	crudo, err := os.ReadFile("testdata/formularios.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectoresFormularios
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Opciones) < 3 || len(v.Valores) < 10 || len(v.Respuestas) < 8 {
		t.Fatalf("faltan vectores: %d opciones, %d valores, %d respuestas", len(v.Opciones), len(v.Valores), len(v.Respuestas))
	}
	return v
}

// TestLosFormulariosDanLoMismoQueLaWeb: `parseOptions`, `coerceFieldValue`,
// `extractFormAnswers` y `answersToText` con los vectores compartidos.
func TestLosFormulariosDanLoMismoQueLaWeb(t *testing.T) {
	v := leerVectoresFormularios(t)
	for _, c := range v.Opciones {
		if got := ParsearOpciones(c.Crudo); !slices.Equal(got, c.Opciones) {
			t.Errorf("ParsearOpciones(%q) = %q, quería %q", c.Crudo, got, c.Opciones)
		}
	}
	for _, c := range v.Valores {
		got, ok := ValorCampo(c.Tipo, c.Crudo, c.Opciones)
		if c.Valor == nil && ok || c.Valor != nil && (!ok || got != *c.Valor) {
			t.Errorf("ValorCampo(%s, %q) = %q, %v; quería %v", c.Tipo, c.Crudo, got, ok, c.Valor)
		}
	}
	for _, c := range v.Respuestas {
		got := ExtraerRespuestas(c.HTML)
		if !slices.Equal(got, c.Respuestas) {
			t.Errorf("%s: respuestas %+v, quería %+v", c.Nombre, got, c.Respuestas)
		}
		if txt := RespuestasATexto(got, "(sin nombre)"); txt != c.Texto {
			t.Errorf("%s: texto %q, quería %q", c.Nombre, txt, c.Texto)
		}
	}
}

// documentoFormulario es el de `buildFormFieldHtml` con los tres tipos.
const documentoFormulario = `<p>Nombre: <span data-formfield="text" data-name="Nombre" data-placeholder="Tu nombre"></span></p>` +
	`<p>Acepto <span data-formfield="checkbox" contenteditable="false" data-name="Acepto" data-checked="0"></span> las bases</p>` +
	`<p>Talla <span data-formfield="select" contenteditable="false" data-name="Talla" data-options="S|M|L" data-value="" data-placeholder="Elegir…"></span></p>`

// TestUnFormularioSeEditaYSusCamposSonPiezasAtomicas: el documento con los tres tipos se
// abre editable, vuelve byte a byte, y cada campo es UNA runa del texto con su campo leído.
func TestUnFormularioSeEditaYSusCamposSonPiezasAtomicas(t *testing.T) {
	d := Parse(documentoFormulario)
	if !d.Editable() {
		t.Fatalf("un formulario de la web tiene que abrirse editable: %v", d.NoEntendido())
	}
	if d.HTML() != documentoFormulario {
		t.Fatalf("sin tocar tiene que volver igual:\n%s", d.HTML())
	}
	bs := d.Bloques()
	if got := bs[1].Texto(); got != "Acepto "+string(ObjetoRuna)+" las bases" {
		t.Fatalf("la casilla tiene que ser una runa: %q", got)
	}
	var campos []*CampoFormulario
	for _, b := range bs {
		for _, tr := range b.Tramos {
			if tr.Formulario != nil {
				campos = append(campos, tr.Formulario)
			}
		}
	}
	if len(campos) != 3 || campos[0].Tipo != CampoTexto || campos[0].Marcador != "Tu nombre" ||
		campos[1].Marcada() || !slices.Equal(campos[2].Opciones, []string{"S", "M", "L"}) || campos[2].Marcador != "Elegir…" {
		t.Fatalf("campos leídos mal: %+v", campos)
	}
	// Una edición del texto de alrededor conserva la ficha entera.
	abrir(t, d, 1).Sustituir(0, 0, "Sí, ")
	if !strings.Contains(d.HTML(), `<p>Sí, Acepto <span data-formfield="checkbox" contenteditable="false" data-name="Acepto" data-checked="0"></span> las bases</p>`) {
		t.Fatalf("la ficha se ha perdido al escribir al lado:\n%s", d.HTML())
	}
	// Otros atributos en un campo siguen sin entenderse.
	if Parse(`<p><span data-formfield="text" data-name="x" class="y"></span></p>`).Editable() {
		t.Fatal("un campo con un atributo desconocido no se puede dar por entendido")
	}
}

// TestRellenarCampoGuardaElValorEnElHTMLComoLaWeb: el texto va dentro del campo, la casilla
// en `data-checked` y el desplegable en `data-value`, validado contra sus opciones.
func TestRellenarCampoGuardaElValorEnElHTMLComoLaWeb(t *testing.T) {
	d := Parse(documentoFormulario)
	cs := d.CamposFormulario()
	if len(cs) != 3 {
		t.Fatalf("hay %d campos", len(cs))
	}
	if !d.RellenarCampo(cs[0], "  Ana\n  López ") || !d.RellenarCampo(cs[1], "1") || !d.RellenarCampo(cs[2], "M") {
		t.Fatal("rellenar tiene que cambiar los tres")
	}
	if d.RellenarCampo(cs[2], "XL") || d.RellenarCampo(cs[1], "sí") {
		t.Fatal("una opción que no está, o el mismo valor, no cambian nada")
	}
	quiero := `<p>Nombre: <span data-formfield="text" data-name="Nombre" data-placeholder="Tu nombre">Ana López</span></p>` +
		`<p>Acepto <span data-formfield="checkbox" contenteditable="false" data-name="Acepto" data-checked="1"></span> las bases</p>` +
		`<p>Talla <span data-formfield="select" contenteditable="false" data-name="Talla" data-options="S|M|L" data-value="M" data-placeholder="Elegir…"></span></p>`
	if d.HTML() != quiero || !d.Guardable() {
		t.Fatalf("el HTML rellenado no es el de la web:\n%s", d.HTML())
	}
	if got := RespuestasATexto(d.Respuestas(), ""); got != "Nombre: Ana López\nAcepto: Sí\nTalla: M" {
		t.Fatalf("respuestas: %q", got)
	}
	// Vaciar el texto deja el campo vacío (el `:empty` del marcador), sin el `<br>` de antes.
	d = Parse(`<p><span data-formfield="text" data-name="N"><br></span></p>`)
	c := d.CamposFormulario()[0]
	if !d.RellenarCampo(c, "") || d.HTML() != `<p><span data-formfield="text" data-name="N"></span></p>` {
		t.Fatalf("vaciar: %s", d.HTML())
	}
	// Un campo de una lectura vieja (tras rehacer el documento) no toca nada.
	viejo := Parse(documentoFormulario).CamposFormulario()[1]
	if d.RellenarCampo(viejo, "1") {
		t.Fatal("un campo de otro documento no se rellena")
	}
}
