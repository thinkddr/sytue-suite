// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/fuentes"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// TestLosComandosDeFormatoSonLosDeLaWeb: cada comando de la cinta hace lo que su
// `SheetCommand`, en un solo paso de deshacer, y deshacer devuelve también el formato.
func TestLosComandosDeFormatoSonLosDeLaWeb(t *testing.T) {
	c := lienzoDePrueba()
	r := c.Rejilla
	c.Libro.Escribir(0, 0, "1234.5")
	c.Libro.Escribir(1, 0, "x")
	r.Seleccionar(0, 0, false)
	r.Seleccionar(1, 0, true) // A1:B1

	c.Mandar(CmdFormatoNum, "currency")
	if got := c.Libro.Texto(0, 0); got != "1.234,50\u00a0€" {
		t.Errorf("A1 en moneda se ve %q", got)
	}
	if got := c.Libro.Texto(1, 0); got != "x" {
		t.Errorf("un texto con formato de moneda se ve %q: el formato solo re-pinta números", got)
	}
	c.Mandar(CmdDecimales, "-1")
	if got := c.Libro.Texto(0, 0); got != "1.234,5\u00a0€" {
		t.Errorf("con un decimal menos se ve %q", got)
	}
	c.Mandar(CmdFormatoNum, "custom:0.00;[Rojo]-0.00")
	if f := c.Libro.FormatoNumero(0, 0); f.ID != "custom" || f.Mascara != "0.00;[Rojo]-0.00" {
		t.Errorf("la máscara propia se ha guardado como %+v", f)
	}

	c.Mandar(CmdEstilo, `{"fill":"#FEF08A","u":true}`)
	c.Mandar(CmdEstilo, `{"u":null,"size":14}`)
	if e := c.Libro.Estilo(1, 0); e.Relleno != "#FEF08A" || e.Subraya || e.Tam != 14 {
		t.Errorf("los dos parches dejan %+v: `null` tiene que quitar el subrayado y conservar lo demás", e)
	}
	c.Mandar(editor.Negrita, "")
	if b, _, _ := c.Libro.Formato(0, 0); !b {
		t.Error("la negrita no ha llegado al HTML de la celda")
	}
	c.Mandar(CmdBordes, "outer")
	if e := c.Libro.Estilo(0, 0); e.Bordes != (Bordes{T: true, B: true, L: true}) {
		t.Errorf("el contorno de A1:B1 deja en A1 %+v", e.Bordes)
	}
	c.Mandar(CmdFijar, "1,2")
	if got := c.Libro.Fijas(); got != (Fijas{Cols: 1, Rows: 2}) {
		t.Errorf("inmovilizar deja %+v", got)
	}

	// Deshacer paso a paso: el primero quita los paneles, y todos juntos dejan A1 como
	// estaba antes de tocar el formato.
	c.Deshacer(false)
	if got := c.Libro.Fijas(); got != (Fijas{}) {
		t.Errorf("deshacer no ha quitado los paneles: %+v", got)
	}
	for c.Libro.PuedeDeshacer() && c.Libro.FormatoNumero(0, 0).ID != "general" {
		c.Deshacer(false)
	}
	if got := c.Libro.Texto(0, 0); got != "1234.5" {
		t.Errorf("deshecho todo el formato, A1 se ve %q", got)
	}
	if e := c.Libro.Estilo(1, 0); e != (Estilo{}) {
		t.Errorf("deshecho todo el formato, B1 conserva %+v", e)
	}

	// Combinar junta el texto en la esquina de arriba a la izquierda.
	r.Seleccionar(0, 0, false)
	r.Seleccionar(1, 0, true)
	c.Mandar(CmdCombinar, "")
	if got := c.Libro.Crudo(0, 0); got != "1234.5 x" || c.Libro.Crudo(1, 0) != "" {
		t.Errorf("combinar A1:B1 deja %q y %q", got, c.Libro.Crudo(1, 0))
	}

	// Y en solo lectura no hace nada.
	c.SoloLectura(true)
	c.Mandar(CmdEstilo, `{"fill":"#BBF7D0"}`)
	if e := c.Libro.Estilo(0, 0); e.Relleno != "" {
		t.Error("un comando de formato ha escrito en un libro de solo lectura")
	}
}

// TestElFormatoSePintaComoEnLaWeb: el relleno es el fondo de la celda, el borde del
// estilo es `border-strong`, la tinta es la del estilo y el rango se tiñe al 12 %
// TAMBIÉN sobre un relleno —`color-mix(ACCENT 12%, fill)`—.
//
// Visto en rojo pintando el relleno por debajo de la celda en vez de en su fondo (el
// píxel salía papel) y dejando el borde de arriba sin pintar.
func TestElFormatoSePintaComoEnLaWeb(t *testing.T) {
	r := rejillaDePrueba(500, 200)
	amarillo := color.NRGBA{R: 0xFE, G: 0xF0, B: 0x8A, A: 0xff}
	r.L.CambiarEstilo(conRelleno("#FEF08A"), 1, 1, 2, 1) // B2:C2
	r.L.CambiarEstilo(conBordes(&Bordes{T: true}), 3, 1, 3, 1)
	r.L.Escribir(4, 1, "rojo")
	r.L.CambiarEstilo(conTinta("#DC2626"), 4, 1, 4, 1)
	r.Seleccionar(0, 3, false) // la selección, lejos de lo que se mira
	img := pinta(t, 500, 200, r)
	guarda(t, img)

	x := func(c int) float32 { return AnchoCabecera + float32(c)*AnchoCelda }
	y := func(f int) float32 { return AltoCelda + float32(f)*AltoCelda }
	if got := px(img, x(1)+AnchoCelda/2, y(1)+4); !cerca(got, amarillo) {
		t.Errorf("B2 con relleno pinta %v y el relleno es %v", got, amarillo)
	}
	fuerte := tema.Color("border-strong", tema.Claro)
	if got := px(img, x(3)+AnchoCelda/2, y(1)+0.5); !cerca(got, fuerte) {
		t.Errorf("el borde de arriba de D2 pinta %v y `border-strong` es %v", got, fuerte)
	}
	// La línea de la cuadrícula SOBRE el relleno se mezcla con él, no con el papel.
	// superficie-hoja.css: marina #1a3541 al 17 % sobre el relleno #fef08a.
	quiero := color.NRGBA{R: 215, G: 209, B: 126, A: 255}
	if got := px(img, x(2)-0.5, y(1)+AltoCelda/2); !cerca(got, quiero) {
		t.Errorf("la línea entre B2 y C2 pinta %v y sobre el amarillo es %v", got, quiero)
	}
	if got, ok := tintaCelda(r, 4, 1); !ok || got != (color.NRGBA{R: 0xDC, G: 0x26, B: 0x26, A: 0xff}) {
		t.Errorf("la tinta del estilo no ha llegado al texto de E2: %v", got)
	}

	// El rango sobre el relleno: el 12 % del color de selección sobre el AMARILLO.
	r.Seleccionar(0, 1, false)
	r.Seleccionar(2, 1, true) // A2:C2
	img = pinta(t, 500, 200, r)
	// SheetCanvas: color-mix(in srgb, editor-select #1a9aa3 12%, #fef08a).
	tenido := color.NRGBA{R: 227, G: 230, B: 141, A: 255}
	if got := px(img, x(2)+AnchoCelda/2, y(1)+4); !cerca(got, tenido) {
		t.Errorf("C2 en el rango pinta %v y el 12 %% del color de selección sobre el relleno es %v", got, tenido)
	}
}

// tintaCelda es la tinta del texto pintado de una celda visible.
func tintaCelda(r *Rejilla, c, f int) (color.Color, bool) {
	x, y := AnchoCabecera+float32(c)*AnchoCelda-r.desX, AltoCelda+float32(f)*AltoCelda-r.desY
	for _, cp := range r.r.celdas {
		if cp.texto.Visible() && cp.texto.Text != "" && cp.fondo.Position() == fyne.NewPos(x, y) {
			return cp.texto.Color, true
		}
	}
	return nil, false
}

// TestLoInmovilizadoNoSeMueve: con una columna y dos filas inmovilizadas, al desplazar
// A1 y A2 siguen en su sitio, lo que se desplaza pasa POR DEBAJO, y el filo de lo
// inmovilizado lleva `border-strong`.
//
// Visto en rojo quitando el `if c >= v.fc` de `vista.x`: la columna A se iba con las
// demás y en su sitio aparecía la F.
func TestLoInmovilizadoNoSeMueve(t *testing.T) {
	r := rejillaDePrueba(500, 200)
	r.L.Fijar(1, 2)
	r.L.CambiarEstilo(conRelleno("#BBF7D0"), 0, 0, 0, 0) // A1 en verde
	r.L.CambiarEstilo(conRelleno("#FEF08A"), 1, 5, 9, 5) // la fila 6 en amarillo
	r.Desplazar(AnchoCelda*4+10, AltoCelda*3)
	img := pinta(t, 500, 200, r)
	guarda(t, img)

	verde := color.NRGBA{R: 0xBB, G: 0xF7, B: 0xD0, A: 0xff}
	if got := px(img, AnchoCabecera+AnchoCelda/2, AltoCelda+AltoCelda/2); !cerca(got, verde) {
		t.Errorf("A1 inmovilizada pinta %v en su sitio y es verde (%v): se ha desplazado", got, verde)
	}
	// La fila 6 (índice 5) ha subido tres filas y cae en la tercera fila visible, pero
	// las dos primeras son las inmovilizadas: ahí está, justo debajo.
	amarillo := color.NRGBA{R: 0xFE, G: 0xF0, B: 0x8A, A: 0xff}
	yFila6 := AltoCelda + 5*AltoCelda - 3*AltoCelda
	if got := px(img, 300, yFila6+AltoCelda/2); !cerca(got, amarillo) {
		t.Errorf("la fila 6 desplazada pinta %v donde debería estar (%v)", got, amarillo)
	}
	// Y la columna A tapa a la B desplazada: en la franja de A de la fila 6 no hay
	// amarillo, porque la A no lo lleva y está encima.
	if got := px(img, AnchoCabecera+AnchoCelda/2, yFila6+AltoCelda/2); cerca(got, amarillo) {
		t.Error("lo desplazado se pinta por ENCIMA de la columna inmovilizada")
	}
	fuerte := tema.Color("border-strong", tema.Claro)
	// En A2 y no en A1: A1 es la celda ancla y su marco tapa los dos últimos píxeles.
	if got := px(img, AnchoCabecera+AnchoCelda-0.5, 2*AltoCelda+AltoCelda/2); !cerca(got, fuerte) {
		t.Errorf("el filo de la columna inmovilizada pinta %v y es `border-strong` (%v)", got, fuerte)
	}
	if got := px(img, 300, AltoCelda+2*AltoCelda-0.5); !cerca(got, fuerte) {
		t.Errorf("el filo de las filas inmovilizadas pinta %v y es `border-strong` (%v)", got, fuerte)
	}
	// Pulsar sobre lo inmovilizado da la inmovilizada; al lado, la desplazada.
	if c, ok := r.celdaEn(fyne.NewPos(AnchoCabecera+10, AltoCelda+10)); !ok || c != (Celda{}) {
		t.Errorf("pulsar sobre A1 inmovilizada da %v", c)
	}
	if c, ok := r.celdaEn(fyne.NewPos(AnchoCabecera+AnchoCelda+1, yFila6+1)); !ok || c.F != 5 || c.C < 4 {
		t.Errorf("pulsar a la derecha de lo inmovilizado da %v", c)
	}
}

// TestLosAtajosDeLaWeb: Ctrl+Inicio y Ctrl+Fin, Mayús+Tab, Ctrl+B, y el tabulador que
// se queda en la rejilla (sin esto, Fyne lo usa para pasar el foco al siguiente
// control y Tab no movería nunca de celda).
func TestLosAtajosDeLaWeb(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	if !r.AcceptsTab() || !r.ed.AcceptsTab() {
		t.Fatal("la rejilla o su celda abierta dejan escapar el tabulador")
	}
	r.L.Escribir(3, 7, "x")
	r.L.Escribir(5, 2, "y")
	ctrl := fyne.KeyModifierShortcutDefault
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEnd, Modifier: ctrl})
	if got := r.Seleccion(); got != (Celda{C: 5, F: 7}) {
		t.Errorf("Ctrl+Fin lleva a %v y la última columna y fila con algo son F y 8", got)
	}
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyHome, Modifier: ctrl})
	if got := r.Seleccion(); got != (Celda{}) {
		t.Errorf("Ctrl+Inicio lleva a %v", got)
	}
	r.Seleccionar(3, 3, false)
	r.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	r.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	r.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	if got := r.Seleccion(); got != (Celda{C: 2, F: 3}) {
		t.Errorf("Mayús+Tab lleva a %v y retrocede una columna", got)
	}
	r.Seleccionar(3, 7, false)
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyB, Modifier: ctrl})
	if b, _, _ := r.L.Formato(3, 7); !b {
		t.Error("Ctrl+B no pone la negrita")
	}
}

// TestLasCabecerasSeleccionanLaColumnaYLaFila: clicar la cabecera de una columna la
// selecciona ENTERA, la de una fila también, y la esquina la hoja; arrastrar desde una
// celda marca el rango desde ESA celda y no desde la que estaba seleccionada.
func TestLasCabecerasSeleccionanLaColumnaYLaFila(t *testing.T) {
	r := rejillaDePrueba(500, 200)
	r.Tapped(&fyne.PointEvent{Position: fyne.NewPos(AnchoCabecera+AnchoCelda*2+10, 10)}) // cabecera C
	if c1, f1, c2, f2 := r.Rango(); c1 != 2 || c2 != 2 || f1 != 0 || f2 != Filas-1 {
		t.Errorf("clicar la cabecera C selecciona %d,%d..%d,%d", c1, f1, c2, f2)
	}
	r.Tapped(&fyne.PointEvent{Position: fyne.NewPos(10, AltoCelda*4+10)}) // cabecera de la fila 4
	if c1, f1, c2, f2 := r.Rango(); c1 != 0 || c2 != Columnas-1 || f1 != 3 || f2 != 3 {
		t.Errorf("clicar la cabecera 4 selecciona %d,%d..%d,%d", c1, f1, c2, f2)
	}
	r.Tapped(&fyne.PointEvent{Position: fyne.NewPos(10, 10)})
	if c1, f1, c2, f2 := r.Rango(); c1 != 0 || f1 != 0 || c2 != Columnas-1 || f2 != Filas-1 {
		t.Errorf("clicar la esquina selecciona %d,%d..%d,%d", c1, f1, c2, f2)
	}
	// Arrastre de B2 a C3, con la selección en otra parte (a la vista: si la rejilla se
	// desplazara, los puntos de abajo caerían en otras celdas).
	r.Seleccionar(3, 5, false)
	desde := fyne.NewPos(AnchoCabecera+AnchoCelda+10, AltoCelda*2+10)
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: desde.AddXY(AnchoCelda, AltoCelda)}, Dragged: fyne.NewDelta(AnchoCelda, AltoCelda)})
	r.DragEnd()
	if c1, f1, c2, f2 := r.Rango(); c1 != 1 || f1 != 1 || c2 != 2 || f2 != 2 {
		t.Errorf("arrastrar de B2 a C3 marca %d,%d..%d,%d", c1, f1, c2, f2)
	}
}

// TestLaCaraDeLaCeldaEsLaDeLaWeb: `<b><i>` es la negrita cursiva DE VERDAD (antes salía
// en cursiva sin negrita), y una familia que el equipo no tiene cae en la reserva de su
// pila: la monoespaciada para Courier, Figtree para Georgia.
func TestLaCaraDeLaCeldaEsLaDeLaWeb(t *testing.T) {
	if r, _ := caraCelda("", true, true); r != tema.NegraCursiva {
		t.Errorf("negrita y cursiva salen con %v", r)
	}
	if r, _ := caraCelda("", true, false); r != tema.Negra {
		t.Errorf("la negrita sale con %v", r)
	}
	if _, ok := fuentes.DelSistema("Courier New", false, false); !ok {
		if r, st := caraCelda("Courier New", false, false); r != nil || !st.Monospace {
			t.Error("Courier New sin instalar no cae en la monoespaciada")
		}
	}
	if _, ok := fuentes.DelSistema("Georgia", true, true); !ok {
		if r, _ := caraCelda("Georgia", true, true); r != tema.NegraCursiva {
			t.Error("Georgia sin instalar no cae en Figtree")
		}
	}
}

// TestLaCeldaSeEditaConSuLetra: la celda abierta escribe con la negrita y la cursiva de
// la celda, como la web, que la edita en su sitio; una fórmula, sin ellas (la web la
// abre con su texto crudo, sin el `<b>`), y con la letra de su tinte.
func TestLaCeldaSeEditaConSuLetra(t *testing.T) {
	c := lienzoDePrueba()
	c.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A1":"<b><i>x</i></b>","A2":"<b>=1+1</b>"}}]}`)
	r := c.Rejilla
	r.Seleccionar(0, 0, false)
	r.Editar("")
	pinta(t, 560, 300, c)
	if r.ed.TextStyle != tema.EstiloDeCara(tema.NegraCursiva) {
		t.Errorf("una celda en negrita cursiva se edita con %+v", r.ed.TextStyle)
	}
	r.Cancelar()
	r.Seleccionar(0, 1, false)
	r.Editar("")
	pinta(t, 560, 300, c)
	if r.ed.TextStyle != tema.EstiloDeCara(tema.Regular) || c.Rejilla.r.tinteCelda.cara != tema.Regular {
		t.Errorf("una fórmula se edita con %+v y su tinte con %v", r.ed.TextStyle, c.Rejilla.r.tinteCelda.cara)
	}
}

// TestElBotonDePanelEsElDeLaWeb: POP_BTN de InicioSheets.tsx mide `h-6` (24 px con raíz de 16) y, pulsado,
// lleva `bg-selected` sobre el panel y el borde del mismo color (`border-transparent`);
// suelto, el borde `border` sobre `surface`.
func TestElBotonDePanelEsElDeLaWeb(t *testing.T) {
	suelto, pulsado := botonPop("Arriba", false, nil), botonPop("Medio", true, nil)
	if h := suelto.MinSize().Height; h != 24 {
		t.Errorf("POP_BTN mide %v de alto y `h-6` son 24", h)
	}
	for _, b := range []fyne.CanvasObject{suelto, pulsado} {
		b.Resize(fyne.NewSize(80, 24))
	}
	pulsado.Move(fyne.NewPos(0, 30))
	img := pinta(t, 80, 60, suelto, pulsado)
	v := tema.Claro
	seleccion := tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v))
	if got := px(img, 40, 30+2); !cerca(got, seleccion) {
		t.Errorf("el pulsado pinta %v por dentro y la web %v", got, seleccion)
	}
	if got := px(img, 40, 30); !cerca(got, seleccion) {
		t.Errorf("el borde del pulsado es %v y con `border-transparent` es su fondo %v", got, seleccion)
	}
	if got := px(img, 40, 0); cerca(got, tema.Color("surface", v)) {
		t.Errorf("el suelto no pinta su borde: %v", got)
	}
}

// TestLaMuestraDeColorEsLaDeSwatches: InicioSheets.tsx fija `h-5 w-5` (20 px con raíz
// de 16), y la elegida lleva el check
// en `mix-blend-difference` (el inverso del color de debajo), no con el aro de
// `ui.MuestraColor`.
func TestLaMuestraDeColorEsLaDeSwatches(t *testing.T) {
	amarillo := color.NRGBA{R: 0xFE, G: 0xF0, B: 0x8A, A: 0xff}
	elegida := nuevaMuestraRegla(amarillo, "Relleno #FEF08A", true, nil)
	suelta := nuevaMuestraRegla(amarillo, "Relleno #FEF08A", false, nil)
	if got := elegida.MinSize(); got != fyne.NewSize(20, 20) {
		t.Errorf("la muestra mide %v y `h-5 w-5` son 20", got)
	}
	cuenta := func(m *muestraRegla) int {
		m.Resize(m.MinSize())
		img := pinta(t, 20, 20, m)
		n := 0
		for y := range img.Bounds().Dy() {
			for x := range img.Bounds().Dx() {
				r, g, b, _ := img.At(x, y).RGBA()
				if cerca(color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)}, inverso(amarillo)) {
					n++
				}
			}
		}
		return n
	}
	if n := cuenta(elegida); n < 10 {
		t.Errorf("la elegida lleva %d píxeles del inverso del amarillo: el check no se ve", n)
	}
	if n := cuenta(suelta); n != 0 {
		t.Errorf("la suelta pinta %d píxeles de check", n)
	}
}

// TestElBordeLlevaSuColorYSuTrazo: el color y el trazo de cada lado de un libro importado se
// leen, se pintan (el color, el grosor y los segmentos en discontinuo, de puntos y doble) y se
// vuelven a escribir; antes se perdían al sanear y, al guardar, el borde volvía a ser el de serie.
func TestElBordeLlevaSuColorYSuTrazo(t *testing.T) {
	var c estiloCrudo
	if err := json.Unmarshal([]byte(`{"border":{"t":true,"tc":"#FF0000","ts":"thick","l":true,"lc":"rojo","ls":"groove","b":false,"bc":"#00FF00"}}`), &c); err != nil {
		t.Fatal(err)
	}
	e, ok := c.sanear()
	if !ok || e.Bordes != (Bordes{T: true, TC: "#FF0000", TS: "thick", L: true}) {
		t.Fatalf("bordes = %+v", e.Bordes)
	}
	if b, _ := json.Marshal(e); !strings.Contains(string(b), `"border":{"t":true,"l":true,"tc":"#FF0000","ts":"thick"}`) {
		t.Errorf("al guardar: %s", b)
	}
	r := rejillaDePrueba(500, 200)
	r.L.CambiarEstilo(func(cr *estiloCrudo) { cr.Bordes = e.Bordes.crudo() }, 3, 1, 3, 1)
	r.L.CambiarEstilo(func(cr *estiloCrudo) {
		cr.Bordes = (&Bordes{
			T: true, TC: "#FF0000", TS: "dashed",
			L: true, LC: "#00AA00", LS: "dotted",
			B: true, BC: "#0000FF", BS: "double",
		}).crudo()
	}, 2, 1, 2, 1)
	r.Seleccionar(0, 3, false)
	img := pinta(t, 500, 200, r)
	x := AnchoCabecera + 3*AnchoCelda
	y := AltoCelda + AltoCelda
	rojo := color.NRGBA{R: 0xFF, A: 0xff}
	verde := color.NRGBA{G: 0xAA, A: 0xff}
	azul := color.NRGBA{B: 0xFF, A: 0xff}
	// Grueso: 3 px de rojo arriba, también en el tercero.
	if got := px(img, x+AnchoCelda/2, y+2.5); !cerca(got, rojo) {
		t.Errorf("el tercer px del borde grueso de D2 pinta %v y es rojo", got)
	}
	// C2: discontinuo arriba (trazo en x+1.5, hueco de papel en x+4), punteado a la izquierda
	// (punto en y+0.5, hueco de papel en y+1.5) y doble abajo (dos líneas en y+h-2.5 e y+h-0.5
	// con hueco de papel en y+h-1.5, sin la línea de la cuadrícula por el medio).
	xc := AnchoCabecera + 2*AnchoCelda
	if got := px(img, xc+1.5, y+0.5); !cerca(got, rojo) {
		t.Errorf("el primer trazo discontinuo de C2 pinta %v y es rojo", got)
	}
	if got := px(img, xc+4.0, y+0.5); !cerca(got, Papel) {
		t.Errorf("el primer hueco discontinuo de C2 pinta %v y debe ser papel", got)
	}
	if got := px(img, xc+0.5, y+0.5); !cerca(got, verde) {
		t.Errorf("el primer punto de C2 pinta %v y es verde", got)
	}
	if got := px(img, xc+0.5, y+1.5); !cerca(got, Papel) {
		t.Errorf("el primer hueco punteado de C2 pinta %v y debe ser papel", got)
	}
	if got := px(img, xc+AnchoCelda/2, y+AltoCelda-2.5); !cerca(got, azul) {
		t.Errorf("la línea interior del borde doble de C2 pinta %v y es azul", got)
	}
	if got := px(img, xc+AnchoCelda/2, y+AltoCelda-1.5); !cerca(got, Papel) {
		t.Errorf("el hueco del borde doble de C2 pinta %v y debe ser papel", got)
	}
	if got := px(img, xc+AnchoCelda/2, y+AltoCelda-0.5); !cerca(got, azul) {
		t.Errorf("la línea exterior del borde doble de C2 pinta %v y es azul", got)
	}
}
