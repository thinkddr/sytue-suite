// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// LO QUE EL EDITOR NATIVO GANÓ EL 2026-11-25 frente a la web: columnas, filas numeradas,
// números de línea y ajuste del código, subíndice y superíndice, Título 6, el `open` de
// un desplegable, emoji con «:», arrastrar un bloque por su asa, y —en la web y aquí—
// imágenes y ficheros en el texto, celdas combinadas y la barra pegada al teclado.

// TestLaIdaYVueltaConLaWebEsByteAByte: los ficheros que comparten el servicio, la web y
// Fyne (`apps/nucleo/notes/model/testdata/ida-y-vuelta`) se leen, son fieles —o sea, la
// nota se puede escribir aquí— y se vuelven a escribir IGUALES, byte a byte, también
// después de pasar por el editor.
//
// VISTO EN ROJO (2026-11-25): sin leer `open`, `data-lineas`, `data-ajuste`,
// `data-columnas`, `<sub>` ni `<sup>`, seis de estos diez salían no fieles (solo lectura)
// y perdían lo que no se leía al reescribirse.
func TestLaIdaYVueltaConLaWebEsByteAByte(t *testing.T) {
	ficheros, err := filepath.Glob(filepath.Join("..", "..", "..", "nucleo", "notes", "model", "testdata", "ida-y-vuelta", "*.html"))
	if err != nil || len(ficheros) < 10 {
		t.Fatalf("faltan los ficheros compartidos: %v %v", ficheros, err)
	}
	for _, f := range ficheros {
		t.Run(filepath.Base(f), func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			fuente := strings.TrimSpace(string(b))
			d := Leer(fuente)
			if !d.Fiel() {
				t.Fatalf("no es fiel (se abriría en solo lectura): vuelve\n  %s", d.HTML())
			}
			if got := d.HTML(); got != fuente {
				t.Fatalf("no vuelve byte a byte:\n  %s\n  %s", got, fuente)
			}
			if got := d.ParaEditar().ParaGuardar(); got != fuente {
				t.Fatalf("pasar por el editor lo cambia:\n  %s\n  %s", got, fuente)
			}
		})
	}
}

// TestElOrdenDeLosAtributosNoQuitaLaFidelidad: la web deja los atributos en el orden en
// que se pulsaron; la nota sigue siendo fiel y se escribe en el orden de siempre.
func TestElOrdenDeLosAtributosNoQuitaLaFidelidad(t *testing.T) {
	d := Leer(`<pre data-ajuste="" data-lineas="">a</pre><details open="" data-expandir=""><summary>T</summary><p>x</p></details>`)
	if !d.Fiel() {
		t.Fatal("el orden de los atributos quita la fidelidad")
	}
	if got := d.HTML(); got != `<pre data-lineas="" data-ajuste="">a</pre><details data-expandir="" open=""><summary>T</summary><p>x</p></details>` {
		t.Fatalf("se reescribe %s", got)
	}
}

// TestColumnasMalFormadasSeAbrenEnSoloLectura: lo que no es la forma de la web no se
// adivina.
func TestColumnasMalFormadasSeAbrenEnSoloLectura(t *testing.T) {
	for _, s := range []string{
		`<div data-columnas="2"><p>suelto</p><div data-columna=""><p>a</p></div></div>`,
		`<table><tbody><tr><td><div data-columnas="2"><div data-columna=""><p>a</p></div></div></td></tr></tbody></table>`,
	} {
		if Leer(s).Fiel() {
			t.Errorf("%s se tiene por fiel", s)
		}
	}
}

func TestLasOrdenesDeLosBloquesNuevos(t *testing.T) {
	t.Run("números de línea y ajuste", func(t *testing.T) {
		d := Leer(`<pre>a</pre>`).ParaEditar()
		d.AlternarCodigo(0, "lineas")
		d.AlternarCodigo(0, "ajuste")
		if got := d.ParaGuardar(); got != `<pre data-lineas="" data-ajuste="">a</pre>` {
			t.Fatalf("deja %s", got)
		}
		d.Formatear(Pos{0, 0}, Pos{0, 0}, "p")
		if got := d.ParaGuardar(); got != `<p>a</p>` {
			t.Fatalf("convertir en párrafo deja %s", got)
		}
	})
	t.Run("filas numeradas", func(t *testing.T) {
		d := Leer(`<table><tbody><tr><td>a</td></tr></tbody></table>`).ParaEditar()
		d.FormatearTabla("numerada", "", Pos{0, 0})
		if got := d.ParaGuardar(); got != `<table data-numerada=""><tbody><tr><td>a</td></tr></tbody></table>` {
			t.Fatalf("numerar deja %s", got)
		}
		d.FormatearTabla("numerada", "", Pos{0, 0})
		if got := d.ParaGuardar(); got != `<table><tbody><tr><td>a</td></tr></tbody></table>` {
			t.Fatalf("quitar la numeración deja %s", got)
		}
	})
	t.Run("título 6", func(t *testing.T) {
		d := Leer(`<p>a</p>`).ParaEditar()
		d.Formatear(Pos{0, 0}, Pos{0, 0}, "h6")
		if got := d.ParaGuardar(); got != `<h6>a</h6>` {
			t.Fatalf("deja %s", got)
		}
	})
	t.Run("subíndice y superíndice se excluyen", func(t *testing.T) {
		d := Leer(`<p>x2</p>`).ParaEditar()
		d.AlternarMarca(Pos{0, 1}, Pos{0, 2}, "sup")
		if got := d.ParaGuardar(); got != `<p>x<sup>2</sup></p>` {
			t.Fatalf("superíndice deja %s", got)
		}
		d.AlternarMarca(Pos{0, 1}, Pos{0, 2}, "sub")
		if got := d.ParaGuardar(); got != `<p>x<sub>2</sub></p>` {
			t.Fatalf("subíndice encima deja %s", got)
		}
	})
	t.Run("un desplegable se abre y se cierra", func(t *testing.T) {
		d := Leer(`<details data-expandir="" open=""><summary>T</summary><p>a</p><p>b</p></details>`).ParaEditar()
		d.AbrirCaja(1, false)
		if got := d.ParaGuardar(); got != `<details data-expandir=""><summary>T</summary><p>a</p><p>b</p></details>` {
			t.Fatalf("cerrar deja %s", got)
		}
	})
	t.Run("columnas", func(t *testing.T) {
		d := Leer(`<p></p>`).ParaEditar()
		p := d.InsertarColumnas(Pos{0, 0}, 3)
		if p.B != 0 || !d.enColumnas(p.B) {
			t.Fatalf("el cursor queda en %v", p)
		}
		p = d.Insertar(p, "uno", Tramo{})
		if got := d.ParaGuardar(); got != `<div data-columnas="3"><div data-columna=""><p>uno</p></div><div data-columna=""><p><br/></p></div><div data-columna=""><p><br/></p></div></div><p><br/></p>` {
			t.Fatalf("deja %s", got)
		}
		if !Leer(d.ParaGuardar()).Fiel() {
			t.Fatal("lo insertado no se vuelve a leer")
		}
		if q := d.InsertarColumnas(p, 2); q != p {
			t.Fatal("unas columnas dentro de otras")
		}
	})
}

// TestCombinarYSepararCeldas: las tres órdenes, con el texto de la absorbida detrás del
// de la que se queda, y separar devuelve la rejilla tal como era (con celdas vacías).
func TestCombinarYSepararCeldas(t *testing.T) {
	const dos = `<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>`
	d := Leer(dos).ParaEditar()
	if !d.PuedeCombinar("combinarDerecha", Pos{0, 0}) || d.PuedeCombinar("combinarDerecha", Pos{1, 0}) || d.PuedeCombinar("separarCelda", Pos{0, 0}) {
		t.Fatal("lo que se puede hacer en cada celda no cuadra")
	}
	p := d.CombinarCeldas("combinarDerecha", Pos{0, 0})
	if got := d.ParaGuardar(); got != `<table><tbody><tr><td colspan="2">ab</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>` {
		t.Fatalf("combinar a la derecha deja %s", got)
	}
	if d.PuedeCombinar("combinarAbajo", p) {
		t.Fatal("una de dos columnas se combina con una de una")
	}
	p = d.CombinarCeldas("separarCelda", p)
	if got := d.ParaGuardar(); got != `<table><tbody><tr><td>ab</td><td><br/></td></tr><tr><td>c</td><td>d</td></tr></tbody></table>` {
		t.Fatalf("separar deja %s", got)
	}
	d.CombinarCeldas("combinarAbajo", p)
	if got := d.ParaGuardar(); got != `<table><tbody><tr><td rowspan="2">abc</td><td><br/></td></tr><tr><td>d</td></tr></tbody></table>` {
		t.Fatalf("combinar abajo deja %s", got)
	}
	if !Leer(d.ParaGuardar()).Fiel() {
		t.Fatal("la tabla combinada no se vuelve a leer")
	}
	d.CombinarCeldas("separarCelda", Pos{0, 0})
	if got := d.ParaGuardar(); got != `<table><tbody><tr><td>abc</td><td><br/></td></tr><tr><td><br/></td><td>d</td></tr></tbody></table>` {
		t.Fatalf("separar la de dos filas deja %s", got)
	}
}

// TestLaBarraDeTablaCombinaYNumera: los botones de la barra de tabla hacen su orden y se
// apagan donde no valen.
func TestLaBarraDeTablaCombinaYNumera(t *testing.T) {
	ed := editorNotas(t, `<table><tbody><tr><td>a</td><td>b</td></tr></tbody></table>`)
	ed.cur, ed.ancla = Pos{1, 0}, Pos{1, 0}
	ed.refrescarBarra()
	if !ed.botones["combinarDerecha"].Inerte || !ed.botones["separarCelda"].Inerte {
		t.Fatal("en la última celda se ofrece combinar a la derecha o separar")
	}
	ed.cur, ed.ancla = Pos{0, 0}, Pos{0, 0}
	ed.Orden("numerada", "")
	ed.Orden("combinarDerecha", "")
	if got := ed.HTML(); got != `<table data-numerada=""><tbody><tr><td colspan="2">ab</td></tr></tbody></table>` {
		t.Fatalf("deja %s", got)
	}
	ed.refrescarBarra()
	if b := ed.botones["numerada"]; !b.ConEstado || !b.Activo {
		t.Fatal("«Filas numeradas» no se ve pulsado")
	}
	ed.Orden("undo", "")
	ed.Orden("undo", "")
	if got := ed.HTML(); got != `<table><tbody><tr><td>a</td><td>b</td></tr></tbody></table>` {
		t.Fatalf("deshacer deja %s", got)
	}
}

// TestLaBarraDelCodigoNumeraYAjusta: los dos botones de la barra del código.
func TestLaBarraDelCodigoNumeraYAjusta(t *testing.T) {
	ed := editorNotas(t, `<pre>a</pre>`)
	if !ed.enCodigo() {
		t.Fatal("sin barra de código")
	}
	ed.Orden("lineas", "")
	ed.Orden("ajuste", "")
	if got := ed.HTML(); got != `<pre data-lineas="" data-ajuste="">a</pre>` {
		t.Fatalf("deja %s", got)
	}
	ed.refrescarBarra()
	if !ed.botones["lineas"].Activo || !ed.botones["ajuste"].Activo {
		t.Fatal("los botones no se ven pulsados")
	}
}

// TestElMaquetadorPintaLosBloquesNuevos: los números de línea del código (uno por línea
// del código, no por fila partida), los de las filas de una tabla numerada (solo las del
// cuerpo), las columnas una al lado de otra (y una debajo de otra en el teléfono), un
// desplegable cerrado al leer y abierto al escribir, y el subíndice bajo la línea base.
func TestElMaquetadorPintaLosBloquesNuevos(t *testing.T) {
	test.NewTempApp(t)
	adornos := func(m *Maquetado, tipo tipoAdorno) []string {
		var out []string
		for _, a := range m.Adornos {
			if a.Tipo == tipo {
				out = append(out, a.Texto)
			}
		}
		return out
	}
	t.Run("código numerado", func(t *testing.T) {
		m := Maquetar(Leer("<pre data-lineas=\"\">uno\n"+strings.Repeat("larga ", 40)+"\ntres</pre>"), 400, tema.Claro)
		if got := adornos(m, adornoLineas); !slices.Equal(got, []string{"1", "2", "3"}) {
			t.Fatalf("números %v en %d líneas", got, len(m.Lineas))
		}
		if len(m.Lineas) <= 3 {
			t.Fatal("la línea larga no se parte")
		}
		if x := m.Lineas[0].X0; x < sangriaCodigoLineas {
			t.Fatalf("el código empieza en %v, encima de los números", x)
		}
	})
	t.Run("tabla numerada", func(t *testing.T) {
		m := Maquetar(Leer(`<table data-numerada=""><thead><tr><th>A</th></tr></thead><tbody><tr><td>x</td></tr><tr><td>y</td></tr></tbody></table>`), 400, tema.Claro)
		if got := adornos(m, adornoNumeroFila); !slices.Equal(got, []string{"1", "2"}) {
			t.Fatalf("números de fila %v", got)
		}
	})
	t.Run("columnas", func(t *testing.T) {
		const dos = `<div data-columnas="2"><div data-columna=""><p>izq</p></div><div data-columna=""><p>der</p></div></div>`
		m := Maquetar(Leer(dos), 700, tema.Claro)
		if len(m.Lineas) != 2 || m.Lineas[0].Y != m.Lineas[1].Y || m.Lineas[1].X0 < 330 {
			t.Fatalf("no van una al lado de otra: %+v", m.Lineas)
		}
		if n := len(adornos(m, adornoGuia)); n != 0 {
			t.Fatalf("al leer se pintan %d guías", n)
		}
		if n := len(adornos(Maquetar(Leer(dos).ParaEditar(), 700, tema.Claro), adornoGuia)); n != 2 {
			t.Fatalf("al escribir se pintan %d guías", n)
		}
		if m = Maquetar(Leer(dos), 360, tema.Claro); m.Lineas[1].Y != m.Lineas[0].Y {
			t.Fatal("en una tarjeta estrecha del escritorio no van lado a lado")
		}
		defer ui.TactilDePrueba(true)()
		m = Maquetar(Leer(dos), 360, tema.Claro)
		if m.Lineas[1].Y <= m.Lineas[0].Y || m.Lineas[1].X0 != m.Lineas[0].X0 {
			t.Fatal("en el teléfono no van una debajo de otra")
		}
	})
	t.Run("desplegable cerrado", func(t *testing.T) {
		const cerrado = `<details data-expandir=""><summary>T</summary><p>dentro</p></details><p>fuera</p>`
		d := Leer(cerrado)
		if n := len(Maquetar(d, 400, tema.Claro).Lineas); n != 1 {
			t.Fatalf("al leer se ven %d líneas", n)
		}
		d.alternarAlLeer(d.Bloques[0].Caja)
		if n := len(Maquetar(d, 400, tema.Claro).Lineas); n != 2 {
			t.Fatalf("abierto al leer se ven %d líneas", n)
		}
		if d.HTML() != cerrado {
			t.Fatal("abrirlo al leer cambia la nota")
		}
		if n := len(Maquetar(Leer(cerrado).ParaEditar(), 400, tema.Claro).Lineas); n != 2 {
			t.Fatalf("al escribir se ven %d líneas", n)
		}
	})
	t.Run("subíndice y superíndice", func(t *testing.T) {
		m := Maquetar(Leer(`<p>a<sub>b</sub><sup>c</sup></p>`), 400, tema.Claro)
		var sub, sup Trozo
		for _, tr := range m.Lineas[0].Trozos {
			switch tr.Texto {
			case "b":
				sub = tr
			case "c":
				sup = tr
			}
		}
		if sub.estilo.bajar <= 0 || sup.estilo.bajar >= 0 || sub.estilo.escalon.Tam >= escalonCuerpo.Tam {
			t.Fatalf("sub %+v sup %+v", sub.estilo, sup.estilo)
		}
	})
	t.Run("emoji en su cara", func(t *testing.T) {
		m := Maquetar(Leer(`<p>listo ✅ y 🇪🇺</p>`), 400, tema.Claro)
		var emoji []string
		for _, tr := range m.Lineas[0].Trozos {
			if tr.estilo.emoji {
				emoji = append(emoji, tr.Texto)
			}
		}
		if !slices.Equal(emoji, []string{"✅", "🇪🇺"}) {
			t.Fatalf("emoji %q", emoji)
		}
	})
}

// TestLosEmojiConDosPuntos: «:son» abre el menú con los emoji cuyas palabras empiezan
// así; elegir cambia «:son» por el emoji. «10:30» no abre nada.
func TestLosEmojiConDosPuntos(t *testing.T) {
	if q, ok := consultaDeEmoji("a las 10:30"); ok {
		t.Fatalf("«10:30» busca %q", q)
	}
	if q, ok := consultaDeEmoji("listo :cohe"); !ok || q != "cohe" {
		t.Fatalf("«:cohe» busca %q, %v", q, ok)
	}
	if got := buscarEmoji("corazon roto"); len(got) != 1 || got[0].s != "💔" {
		t.Fatalf("«cora» encuentra %v", got)
	}
	ed := editorNotas(t, "")
	teclear(ed.zona, "Hecho :rock")
	if ed.menu == nil || ed.menu.tipo != menuEmoji {
		t.Fatal("«:rock» no abre los emoji")
	}
	tecla(ed.zona, fyne.KeyReturn)
	if got := ed.HTML(); got != "Hecho 🚀" || ed.menu != nil {
		t.Fatalf("elegir deja %q", got)
	}
	teclear(ed.zona, " :zzzz")
	if ed.menu != nil {
		t.Fatal("sin emoji que case, el menú sigue abierto")
	}
}

// TestArrastrarUnBloquePorSuAsa: llevar un bloque, una tabla entera, meterlo en un panel
// y sacarlo; soltar dentro de una tabla o sobre sí mismo no hace nada.
func TestArrastrarUnBloquePorSuAsa(t *testing.T) {
	d := Leer(`<p>a</p><p>b</p><p>c</p>`).ParaEditar()
	if p := d.LlevarBloque(0, 3); p.B != 2 || d.ParaGuardar() != `<p>b</p><p>c</p><p>a</p>` {
		t.Fatalf("llevar al final deja %s (%v)", d.ParaGuardar(), p)
	}
	d.LlevarBloque(2, 2)
	if d.ParaGuardar() != `<p>b</p><p>c</p><p>a</p>` {
		t.Fatal("soltar sobre sí mismo mueve")
	}
	d = Leer(`<p>a</p><div data-panel="info"><p>x</p><p>y</p></div>`).ParaEditar()
	d.LlevarBloque(0, 2)
	if got := d.ParaGuardar(); got != `<div data-panel="info"><p>x</p><p>a</p><p>y</p></div>` {
		t.Fatalf("soltar dentro del panel deja %s", got)
	}
	d.LlevarBloque(1, 3)
	if got := d.ParaGuardar(); got != `<div data-panel="info"><p>x</p><p>y</p></div><p>a</p>` {
		t.Fatalf("sacarlo del panel deja %s", got)
	}
	d = Leer(`<p>a</p><table><tbody><tr><td>1</td><td>2</td></tr></tbody></table><p>b</p>`).ParaEditar()
	d.LlevarBloque(3, 0)
	if got := d.ParaGuardar(); got != `<p>b</p><p>a</p><table><tbody><tr><td>1</td><td>2</td></tr></tbody></table>` {
		t.Fatalf("llevar delante deja %s", got)
	}
	d.LlevarBloque(0, 3)
	if got := d.ParaGuardar(); got != `<p>b</p><p>a</p><table><tbody><tr><td>1</td><td>2</td></tr></tbody></table>` {
		t.Fatalf("soltar entre dos celdas deja %s", got)
	}
	d.LlevarBloque(2, 0)
	if got := d.ParaGuardar(); got != `<table><tbody><tr><td>1</td><td>2</td></tr></tbody></table><p>b</p><p>a</p>` {
		t.Fatalf("llevar la tabla entera deja %s", got)
	}
}

// TestArrastrarElAsaEnElEditor: con el ratón sobre un bloque, arrastrar desde su asa
// pinta la raya de dónde caerá y, al soltar, lo lleva allí en un paso de deshacer.
func TestArrastrarElAsaEnElEditor(t *testing.T) {
	test.NewTempApp(t)
	restaurar := ui.TactilDePrueba(false)
	defer restaurar()
	ed := editorNotas(t, `<p>uno</p><p>dos</p><p>tres</p>`)
	w := test.NewWindow(ed)
	defer w.Close()
	w.Resize(fyne.NewSize(500, 400))
	ed.zona.encima = 0
	asa, ok := ed.zona.cajaAsa(0)
	if !ok {
		t.Fatal("sin asa")
	}
	inicio := asa.Add(fyne.NewPos(4, 4))
	ultima := ed.zona.maquetado().Bloques[2]
	fin := fyne.NewPos(inicio.X, PYZonaEd+ultima.Y+ultima.Alto-1)
	ed.zona.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fin}, Dragged: fyne.NewDelta(fin.X-inicio.X, fin.Y-inicio.Y)})
	if ed.zona.llevando != 0 || ed.zona.soltar != 3 || ed.zona.rayaSoltar(500, tema.Claro) == nil {
		t.Fatalf("arrastrando: lleva %d, cae en %d", ed.zona.llevando, ed.zona.soltar)
	}
	ed.zona.DragEnd()
	if got := ed.HTML(); got != `<p>dos</p><p>tres</p><p>uno</p>` {
		t.Fatalf("soltar deja %s", got)
	}
	ed.Orden("undo", "")
	if got := ed.HTML(); got != `<p>uno</p><p>dos</p><p>tres</p>` {
		t.Fatalf("deshacer deja %s", got)
	}
}

// TestLasImagenesYLosFicherosDelTexto: un adjunto de imagen entra como `<img>` y uno
// que no lo es como enlace; la imagen se pinta con la del adjunto, y un id que no es de
// adjunto no se resuelve.
func TestLasImagenesYLosFicherosDelTexto(t *testing.T) {
	test.NewTempApp(t)
	const png1x1 = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	img := Adjunto{ID: "1732540000000-ab12cd", Name: "plano.png", Type: "image/png", DataURL: png1x1}
	pdf := Adjunto{ID: "1732540000000-ef34gh", Name: "informe.pdf", Type: "application/pdf", DataURL: "data:application/pdf;base64,JVBERg=="}
	ed := editorNotas(t, "<p>Plano: </p>")
	ed.InsertarAdjunto(img)
	ed.escribir(" y ")
	ed.InsertarAdjunto(pdf)
	quiero := `<p>Plano: <img src="adjunto:1732540000000-ab12cd" alt="plano.png"/> y <a href="adjunto:1732540000000-ef34gh" target="_blank" rel="noopener noreferrer">informe.pdf</a>` + " </p>"
	if got := ed.HTML(); got != quiero {
		t.Fatalf("deja\n  %s\nquiero\n  %s", got, quiero)
	}
	resolver := imagenesDeAdjuntos(func() []Adjunto { return []Adjunto{img, pdf} })
	if im := resolver("adjunto:" + img.ID); im == nil || im.Bounds().Dx() != 1 {
		t.Fatal("la imagen del adjunto no se descodifica")
	}
	for _, src := range []string{"adjunto:" + pdf.ID, "adjunto:../x", "https://espia.example/x.png", png1x1} {
		if resolver(src) != nil {
			t.Errorf("%.40s se resuelve", src)
		}
	}
	// Una recién puesta cuyo adjunto aún no ha llegado se ve en cuanto llega.
	var lista []Adjunto
	tarde := imagenesDeAdjuntos(func() []Adjunto { return lista })
	if tarde("adjunto:"+img.ID) != nil {
		t.Fatal("se resuelve un adjunto que no está")
	}
	lista = []Adjunto{img}
	if tarde("adjunto:"+img.ID) == nil {
		t.Fatal("la imagen no se ve al llegar su adjunto")
	}
	ed.InsertarAdjunto(Adjunto{ID: `x" onerror="y`, Name: "x", Type: "image/png"})
	if strings.Contains(ed.HTML(), "onerror") {
		t.Fatal("un id de adjunto con comillas entra en el texto")
	}
}

// TestLaBarraVaPegadaAlTeclado: en el teléfono, escribiendo con el teclado fuera, la barra
// de formato deja el editor y va al portal justo encima del teclado; al irse el teclado,
// vuelve arriba.
func TestLaBarraVaPegadaAlTeclado(t *testing.T) {
	test.NewTempApp(t)
	defer ui.MovilDePrueba(true)()
	defer ui.TactilDePrueba(true)()
	ed := editorNotas(t, "<p>hola</p>")
	ed.portal = nuevoPortal()
	w := test.NewWindow(container.NewStack(container.NewVBox(ed), ed.portal.c))
	defer w.Close()
	w.Resize(fyne.NewSize(390, 800))
	ed.zona.enfocado = true
	ed.ponerTeclado(300)
	if !slices.Contains(ed.portal.c.Objects, fyne.CanvasObject(ed.barra)) || slices.Contains(ed.barras(), fyne.CanvasObject(ed.barra)) {
		t.Fatal("la barra no ha ido al portal")
	}
	alto := w.Canvas().Size().Height
	abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(ed.portal.c)
	if abajo := abs.Y + ed.barra.Position().Y + ed.barra.Size().Height; abajo < alto-300-1 || abajo > alto-300+1 {
		t.Fatalf("la barra acaba en %v y el teclado empieza en %v", abajo, alto-300)
	}
	ed.zona.dejarTeclado()
	if slices.Contains(ed.portal.c.Objects, fyne.CanvasObject(ed.barra)) || !slices.Contains(ed.barras(), fyne.CanvasObject(ed.barra)) {
		t.Fatal("sin teclado, la barra no vuelve arriba")
	}
}
