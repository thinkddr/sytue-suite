// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// LO QUE PIDE SYTUE MAIL AL MODELO: la fuente y el tamaño de la barra, las imágenes
// `cid:` de su cuerpo, soltar por donde cae y saber dónde está el cursor.

// TestFuenteYTamano: vuelven enteros en el `style` del span, la barra los pone sobre la
// selección o para lo que se escriba, y «Quitar formato» se los lleva.
func TestFuenteYTamano(t *testing.T) {
	crudo := `<p><span style="font-family:Georgia, serif;font-size:18px">a</span>b</p>`
	if d := Leer(crudo); !d.Fiel() {
		t.Fatalf("no vuelve entero: %s", d.HTML())
	}

	d := editar(t, "<p>hola</p>")
	d.PonerFuente(Pos{0, 0}, Pos{0, 2}, "Georgia, serif")
	d.PonerTamano(Pos{0, 1}, Pos{0, 2}, "24pt")
	quiero(t, d, `<p><span style="font-family:Georgia, serif">h</span>`+
		`<span style="font-family:Georgia, serif;font-size:24pt">o</span>la</p>`)
	d.QuitarFormato(Pos{0, 0}, Pos{0, 4})
	quiero(t, d, `<p>hola</p>`)

	ed := NuevoEditorNota(trInterno(t), Leer(""), "")
	ed.Orden("tamano", "18px")
	teclear(ed.zona, "grande")
	if got := ed.HTML(); got != `<span style="font-size:18px">grande</span>` {
		t.Errorf("el tamaño sin selección deja %q", got)
	}

	// Y se pinta a su tamaño: la línea crece con la letra (`leading-relaxed` en
	// proporción), y la de al lado no.
	m := Maquetar(Leer(`<p>a</p><p><span style="font-size:24px">b</span></p>`), 300, tema.Claro)
	if a, b := m.Lineas[0].Alto, m.Lineas[1].Alto; a > escalonCuerpo.Linea+0.01 || b < 24*1.625-0.01 {
		t.Errorf("las líneas miden %.2f y %.2f; quiero %.2f y %.2f", a, b, escalonCuerpo.Linea, 24*1.625)
	}
	if !monoespaciada(`"Courier New", Courier, monospace`) || monoespaciada("Georgia, serif") {
		t.Error("la de ancho fijo es Courier New, y solo ella")
	}
}

// TestImagenesEnElCuerpo: un `<img>` vuelve entero y es una letra (que no llega al
// portapapeles); la barra lo mete en el cursor y Retroceso lo quita; y se pinta a su
// tamaño, sin pasar del ancho y apoyado en la línea base.
//
// VISTO EN ROJO (2026-09-24, en el frontal): sin escribir el `<img>`, la nota ya no vuelve
// entera y se cae aquí.
func TestImagenesEnElCuerpo(t *testing.T) {
	crudo := `<p>a<img src="cid:foto1" alt="" style="max-width: 100%"/>b</p>`
	d := Leer(crudo)
	if !d.Fiel() {
		t.Fatalf("no vuelve entera: %s", d.HTML())
	}
	if got := d.Bloques[0].Texto(); got != "a"+string(Objeto)+"b" {
		t.Errorf("el texto es %q", got)
	}
	if got := d.TextoEntre(Pos{0, 0}, Pos{0, 3}); got != "ab" {
		t.Errorf("al portapapeles va %q", got)
	}

	e := editar(t, "<p>ab</p>")
	if p := e.InsertarImagen(Pos{0, 1}, "cid:x"); p != (Pos{0, 2}) {
		t.Errorf("el cursor queda en %v", p)
	}
	quiero(t, e, `<p>a<img src="cid:x" alt="" style="max-width: 100%"/>b</p>`)
	e.Retroceso(Pos{0, 2})
	quiero(t, e, `<p>ab</p>`)

	sola := Leer(`<p><img src="cid:foto1" alt="" style="max-width: 100%"/></p>`)
	sola.Imagenes = func(src string) image.Image {
		if src == "cid:foto1" {
			return image.NewRGBA(image.Rect(0, 0, 100, 50))
		}
		return nil
	}
	for _, c := range []struct {
		ancho, an, al float32
	}{{300, 100, 50}, {60, 60, 30}} {
		m := Maquetar(sola, c.ancho, tema.Claro)
		l := m.Lineas[0]
		var im *imagenLinea
		for _, tr := range l.Trozos {
			if tr.img != nil {
				im = tr.img
			}
		}
		if im == nil || im.an != c.an || im.al != c.al {
			t.Fatalf("a %v de ancho la imagen es %+v; quiero %v × %v", c.ancho, im, c.an, c.al)
		}
		if l.Base-l.Y < c.al || l.Alto < c.al {
			t.Errorf("la imagen no cabe en su línea: base a %.1f, alto %.1f", l.Base-l.Y, l.Alto)
		}
		if l.XDe(1) != l.XDe(0)+c.an || l.PosEn(l.XDe(0)+c.an*0.9) != 1 {
			t.Errorf("el cursor no salta la imagen: %.1f → %.1f", l.XDe(0), l.XDe(1))
		}
	}
	// Sin la imagen (una dirección que nadie da), un hueco de 16.
	if l := Maquetar(d, 300, tema.Claro).Lineas[0]; l.Trozos[1].img == nil || l.Trozos[1].Ancho != ladoImagenRota {
		t.Errorf("la imagen que no se tiene mide %+v", l.Trozos[1])
	}
}

// TestSoltarPorDondeCaeYAvisarDelCursor: `ui.SoltarEnCampo` pulsa (MouseDown) donde se
// suelta y pega: el cursor tiene que ir ahí antes de soltar. Y cada movimiento dice dónde
// queda la raya, que es lo que Mail usa para tenerla a la vista.
//
// VISTO EN ROJO (2026-09-24, en el frontal): con MouseDown vacío, el cursor se queda al
// principio y lo soltado entraba allí.
func TestSoltarPorDondeCaeYAvisarDelCursor(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer("<p>uno</p><p>dos</p>"), "")
	w := test.NewWindow(ed)
	defer w.Close()
	w.Resize(fyne.NewSize(400, 300))
	var arriba []float32
	ed.AlMoverCursor = func(y, alto float32) {
		if alto <= 0 {
			t.Errorf("el cursor mide %v", alto)
		}
		arriba = append(arriba, y)
	}
	ed.AlPrincipio()
	m := ed.zona.maquetado()
	dos := m.Lineas[1]
	ev := &desktop.MouseEvent{Button: desktop.MouseButtonPrimary}
	ev.Position = fyne.NewPos(PXZonaEd+dos.X1, PYZonaEd+dos.Y+dos.Alto/2)
	ed.zona.MouseDown(ev)
	ed.zona.MouseUp(ev)
	if ed.cur != (Pos{1, 3}) {
		t.Fatalf("el cursor queda en %v y el puntero estaba al final de «dos»", ed.cur)
	}
	ed.zona.TypedShortcut(&fyne.ShortcutPaste{Clipboard: &portapapelesDePrueba{"!"}})
	if got := ed.HTML(); !strings.Contains(got, "dos!") {
		t.Errorf("lo soltado no entra por donde cae: %q", got)
	}
	if len(arriba) < 2 || arriba[0] >= arriba[len(arriba)-1] {
		t.Errorf("el aviso del cursor no baja a la segunda línea: %v", arriba)
	}
}

type portapapelesDePrueba struct{ texto string }

func (p *portapapelesDePrueba) Content() string     { return p.texto }
func (p *portapapelesDePrueba) SetContent(s string) { p.texto = s }
