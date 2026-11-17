// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"encoding/json"
	"os"
	"testing"
)

// rutaVectoresMayusculas son los vectores COMPARTIDOS con la web: los recorre también
// `packages/office/test/docs-mayusculas.test.ts` contra `mayusculas.ts`.
const rutaVectoresMayusculas = "../../../../frontend/packages/office/test/vectores/docus-mayusculas.json"

// TestLasMayusculasSonLasDeLaWeb: cada caso transforma igual aquí que en la web, trozo a
// trozo y párrafo a párrafo.
func TestLasMayusculasSonLasDeLaWeb(t *testing.T) {
	crudo, err := os.ReadFile(rutaVectoresMayusculas)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Casos []struct {
			Nombre   string
			Modo     string
			Previo   string
			Parrafos [][]string
			Esperado [][]string
		}
	}
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) < 8 {
		t.Fatalf("solo %d casos: el JSON ha dejado de leerse", len(v.Casos))
	}
	for _, caso := range v.Casos {
		modo, ok := LeerModoMayusculas(caso.Modo)
		if !ok {
			t.Errorf("%s: modo %q desconocido", caso.Nombre, caso.Modo)
			continue
		}
		c := NuevoCambioMayusculas(modo)
		for i, trozos := range caso.Parrafos {
			c.Parrafo()
			if i == 0 {
				c.Previo(caso.Previo)
			}
			for j, tr := range trozos {
				if got := c.Tramo(tr); got != caso.Esperado[i][j] {
					t.Errorf("%s: párrafo %d trozo %d = %q, la web da %q", caso.Nombre, i, j, got, caso.Esperado[i][j])
				}
			}
		}
	}
	if _, ok := LeerModoMayusculas("alternar"); ok {
		t.Error("un modo inventado no se acepta")
	}
}

// TestCambiarMayusculasNoMueveElFormato: las letras cambian y la negrita sigue sobre las
// mismas; lo de antes de la selección decide si empieza palabra. Se vio en ROJO sin el
// `Previo` de CambiarMayusculas: «la» salía «La».
func TestCambiarMayusculasNoMueveElFormato(t *testing.T) {
	d := Parse(`<p>hola <b>mUNDO</b> cruel</p>`)
	e := abrir(t, d, 0)
	e.CambiarMayusculas(2, 16, NuevoCambioMayusculas(CadaPalabra))
	despues(t, "cada palabra", d, `<p>hola <b>Mundo</b> Cruel</p>`)

	img := `<img src="` + png3x2 + `" data-sytue-image="" style="width:30px">`
	d = Parse(`<p>ab` + img + `cd</p>`)
	e = abrir(t, d, 0)
	e.CambiarMayusculas(0, e.Largo(), NuevoCambioMayusculas(Mayusculas))
	despues(t, "una imagen en medio se queda como estaba", d, `<p>AB`+img+`CD</p>`)
}

// TestPegarFormato es la mitad que pinta de la brocha: quita el formato propio, pone solo
// lo que difiere y deja lo que no es formato. Se vio en ROJO creando el envoltorio dentro
// del cambio de cada runa: salía un <b> por letra.
func TestPegarFormato(t *testing.T) {
	estiloDel := func(fuente string, desde int) Estilo {
		e := abrir(t, Parse(fuente), 0)
		return e.Formato(desde, desde+1)
	}
	negritaRoja := estiloDel(`<p><b><span style="color: rgb(220, 38, 38);">x</span></b></p>`, 0)
	props := [][2]string{{"color", "rgb(220, 38, 38)"}, {"font-size", "x-small"}}

	d := Parse(`<p>uno <i>dos</i> <a href="x">tres</a></p>`)
	e := abrir(t, d, 0)
	e.PegarFormato(0, e.Largo(), negritaRoja, props)
	// La cursiva se va, el enlace se queda y entran el color y el tamaño, que difieren.
	despues(t, "negrita roja", d,
		`<p><b><span style="color: rgb(220, 38, 38);"><span style="font-size: x-small;">uno dos </span></span></b>`+
			`<a href="x"><b><span style="color: rgb(220, 38, 38);"><span style="font-size: x-small;">tres</span></span></b></a></p>`)

	// Copiar texto normal sobre un título: se le niega la negrita que trae el bloque.
	normal := estiloDel(`<p>x</p>`, 0)
	d = Parse(`<h2>título</h2>`)
	e = abrir(t, d, 0)
	e.PegarFormato(0, 3, normal, [][2]string{{"color", "rgb(10, 10, 10)"}})
	// El color es el del folio en los dos: no se escribe.
	despues(t, "texto normal sobre un título", d, `<h2><span style="font-weight: normal;">tít</span>ulo</h2>`)
}
