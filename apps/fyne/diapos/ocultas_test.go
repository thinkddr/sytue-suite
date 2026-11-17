// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

// La tabla de navegación con ocultas que exige también la web
// (`packages/office/test/slides-ocultas.test.ts`): presentar un mismo documento tiene que
// pasar por las mismas diapositivas en las dos.
const rutaVectoresOcultas = "../../frontend/packages/office/test/vectores/diapos-ocultas.json"

// barajaOcultas monta las diapositivas de un caso: `pasos[i]` clics de animación en la i,
// y la marca de oculta donde toca.
func barajaOcultas(ocultas []bool, pasos []int) []Diapo {
	ds := make([]Diapo, len(ocultas))
	for i, o := range ocultas {
		ds[i] = Diapo{ID: fmt.Sprintf("s%d", i)}
		if n := pasos[i]; n > 0 {
			c := Caja{ID: "b", W: 10, H: 10}
			for k := range n {
				c.Anims = append(c.Anims, Anim{Tipo: "fade", Orden: float64(k + 1), Disparo: "click"})
			}
			ds[i].Cajas = []Caja{c}
		}
		if o {
			si := true
			ds[i].Oculta = &si
		}
	}
	return ds
}

// TestLasOcultasSeSaltanComoEnLaWeb recorre los vectores compartidos: avanzar y
// retroceder, las teclas de salto, la «siguiente» del moderador y la posición de Cast.
//
// VISTO EN ROJO: con Avanzar sin `Saltables` (el `cur + 1` de antes), «en la primera,
// avanzar salta la oculta» acababa en la 1 y no en la 2.
func TestLasOcultasSeSaltanComoEnLaWeb(t *testing.T) {
	crudo, err := os.ReadFile(rutaVectoresOcultas)
	if err != nil {
		t.Fatalf("no se pueden leer los vectores: %v", err)
	}
	var v struct {
		Casos []struct {
			Nombre     string `json:"nombre"`
			Ocultas    []bool `json:"ocultas"`
			Pasos      []int  `json:"pasos"`
			Diapo      int    `json:"diapo"`
			Paso       int    `json:"paso"`
			Avanzar    [2]int `json:"avanzar"`
			Retroceder [2]int `json:"retroceder"`
			Teclas     struct {
				Siguiente int `json:"siguiente"`
				Anterior  int `json:"anterior"`
				Inicio    int `json:"inicio"`
				Fin       int `json:"fin"`
			} `json:"teclas"`
			Moderador int    `json:"moderador"`
			Cast      *[]int `json:"cast"`
		} `json:"casos"`
	}
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) == 0 {
		t.Fatal("los vectores no traen casos")
	}
	for _, c := range v.Casos {
		ds := barajaOcultas(c.Ocultas, c.Pasos)
		ultima := len(ds) - 1
		mask := Saltables(ds)
		if d, p := Avanzar(ds, c.Diapo, c.Paso, 1); [2]int{d, p} != c.Avanzar {
			t.Errorf("%s: avanzar da %v, quiero %v", c.Nombre, [2]int{d, p}, c.Avanzar)
		}
		if d, p := Avanzar(ds, c.Diapo, c.Paso, -1); [2]int{d, p} != c.Retroceder {
			t.Errorf("%s: retroceder da %v, quiero %v", c.Nombre, [2]int{d, p}, c.Retroceder)
		}
		for _, k := range []struct {
			tecla  fyne.KeyName
			quiero int
		}{
			{fyne.KeyRight, c.Teclas.Siguiente},
			{fyne.KeyPageDown, c.Teclas.Siguiente},
			{fyne.KeyLeft, c.Teclas.Anterior},
			{fyne.KeyPageUp, c.Teclas.Anterior},
			{fyne.KeyHome, c.Teclas.Inicio},
			{fyne.KeyEnd, c.Teclas.Fin},
		} {
			if got := TeclaPresentandoEn(k.tecla, c.Diapo, ultima, mask); got != k.quiero {
				t.Errorf("%s: %s lleva a %d, quiero %d", c.Nombre, k.tecla, got, k.quiero)
			}
		}
		if got := SiguienteVisible(mask, c.Diapo, 1); got != c.Moderador {
			t.Errorf("%s: el moderador enseña la %d como siguiente, quiero %d", c.Nombre, got, c.Moderador)
		}
		pos, ok := PosicionCast(&Modelo{Diapos: ds}, c.Diapo, "d1")
		switch {
		case c.Cast == nil && ok:
			t.Errorf("%s: Cast anuncia una oculta (%+v)", c.Nombre, pos)
		case c.Cast != nil && (!ok || pos.Index != (*c.Cast)[0] || pos.Total != (*c.Cast)[1]):
			t.Errorf("%s: Cast anuncia %d de %d (ok=%v), quiero %v", c.Nombre, pos.Index, pos.Total, ok, *c.Cast)
		}
	}
}

// TestNumeroEIntroVaAUnaOcultaYDeAhiSeSigue: la presentación en marcha salta la oculta
// con →, llega a ella con «2 + Intro», y los enlaces relativos de una caja la saltan.
func TestNumeroEIntroVaAUnaOcultaYDeAhiSeSigue(t *testing.T) {
	m := &Modelo{Diapos: barajaOcultas([]bool{false, true, false}, []int{0, 0, 0})}
	p := NuevaPresentacion(m, 0, nil)
	p.Tecla(fyne.KeyRight)
	if p.Diapo != 2 {
		t.Fatalf("→ desde la 1 deja la %d y tenía que saltar la oculta hasta la 3", p.Diapo+1)
	}
	p.Tecla(fyne.KeyHome)
	p.Runa('2')
	p.Tecla(fyne.KeyReturn)
	if p.Diapo != 1 {
		t.Fatalf("«2 + Intro» deja la %d: a una oculta sí se llega por su número", p.Diapo+1)
	}
	if got := DestinoAccionEn(json.RawMessage(`"next"`), 0, 2, Saltables(m.Diapos)); got != 2 {
		t.Errorf("el enlace «siguiente» lleva a la %d y tenía que saltar la oculta", got+1)
	}
	if got := DestinoAccionEn(json.RawMessage(`2`), 0, 2, Saltables(m.Diapos)); got != 1 {
		t.Errorf("el enlace a la diapositiva 2 lleva a la %d: un número va aunque esté oculta", got+1)
	}
}

// TestUnaPresentacionConOcultasEsEditable: el campo `hidden` de la web se conserva —el
// `true` y también un `false` explícito—, así que el documento se abre editable; ocultar
// desde el carril escribe `true` y volver a enseñarla quita el campo.
//
// VISTO EN ROJO sin el campo `Oculta` en Diapo: el editor lo abría en solo lectura.
func TestUnaPresentacionConOcultasEsEditable(t *testing.T) {
	doc := `{"slides":[{"id":"a","boxes":[],"bg":"#1A2541"},{"id":"b","boxes":[],"bg":"#1A2541","hidden":true},{"id":"c","boxes":[],"bg":"#1A2541","hidden":false}]}`
	e, ctx, w := montar(t, doc)
	if ctx.SoloLectura {
		t.Fatalf("una presentación con ocultas se abre en solo lectura: %v", e.Modelo().NoFiel([]byte(doc)))
	}
	minis := miniaturas(w)
	if len(minis) != 3 {
		t.Fatalf("el carril enseña %d miniaturas y son 3", len(minis))
	}
	if got := minis[1].AccessibilityLabel(); !strings.Contains(got, "2") || got == minis[0].AccessibilityLabel() {
		t.Errorf("la miniatura oculta se anuncia «%s», igual que una que no lo está", got)
	}
	// Ctrl+Mayús+H sobre la primera la oculta; otra vez, la vuelve a enseñar.
	atajo := &desktop.CustomShortcut{KeyName: fyne.KeyH, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}
	fyne.DoAndWait(func() { minis[0].TypedShortcut(atajo) })
	if !e.Modelo().Diapos[0].EstaOculta() || !strings.Contains(e.Serializar(), `"hidden":true`) {
		t.Fatal("Ctrl+Mayús+H en la miniatura no la ha ocultado")
	}
	fyne.DoAndWait(func() { miniaturas(w)[0].TypedShortcut(atajo) })
	if e.Modelo().Diapos[0].Oculta != nil {
		t.Error("volver a enseñarla deja el campo puesto, y la web solo escribe el `true`")
	}
	// Con Alt no es el atajo.
	conAlt := &desktop.CustomShortcut{KeyName: fyne.KeyH, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift | fyne.KeyModifierAlt}
	if EsAtajoOcultar(conAlt) {
		t.Error("Ctrl+Alt+Mayús+H no es el atajo de ocultar")
	}
	guarda(t, "carril-ocultas.png", w.Canvas().Capture())
}

// TestLaMiniaturaOcultaSeAtenuaYTachaSuNumero: el velo y el tachón solo salen en la oculta.
func TestLaMiniaturaOcultaSeAtenuaYTachaSuNumero(t *testing.T) {
	m := &Modelo{Diapos: barajaOcultas([]bool{false, true}, []int{0, 0})}
	for i, quiero := range []bool{false, true} {
		mini := NuevaMiniatura(m, i, nil)
		w := test.NewTempWindow(t, mini)
		w.Resize(MedidaMiniatura(m))
		r := test.WidgetRenderer(mini).(*miniaturaRenderer)
		if visto := !r.velo.Hidden && !r.tachon.Hidden; visto != quiero {
			t.Errorf("miniatura %d: velo y tachón visibles = %v, quiero %v", i+1, visto, quiero)
		}
	}
}

func miniaturas(w fyne.Window) []*Miniatura {
	var out []*Miniatura
	for _, o := range test.LaidOutObjects(w.Content()) {
		if m, ok := o.(*Miniatura); ok && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}
