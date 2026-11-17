// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"math"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

/*
DÓNDE VA LA FIRMA VISIBLE: los MISMOS casos que `firmaPosicion.test.tsx`. Que quien no
toca nada firma donde siempre ([56 56 296 130] en un A4); que cada posición cae dentro
del papel en un A4, un A5, una apaisada, un tique y una MediaBox que no empieza en 0,0;
que «firmar en todas» se calcula sobre la zona común; y que lo que se ENVÍA —las tres
vías que dejan marca y la del certificado del sistema— lleva el recuadro elegido.
*/

var (
	cajaA5         = Rect{0, 0, 420, 595}
	cajaApaisada   = Rect{0, 0, 842, 595}
	cajaTique      = Rect{0, 0, 160, 400}
	cajaDesplazada = Rect{100, 200, 712, 992}
)

func dentroDe(r, caja Rect) bool {
	return r[0] >= caja[0] && r[1] >= caja[1] && r[2] <= caja[2] && r[3] <= caja[3]
}

func TestPosicionFirmaPorDefectoEsLaDeSiempre(t *testing.T) {
	if posicionesFirma[0] != FirmaAbajoIzquierda {
		t.Errorf("la primera posición es %q", posicionesFirma[0])
	}
	siempre := Rect{56, 56, 296, 130}
	// En un A5 o una apaisada cabe con el margen entero: no se mueve.
	for _, caja := range []Rect{cajaA4, cajaA5, cajaApaisada} {
		if got := posicionFirma(caja, FirmaAbajoIzquierda); got != siempre {
			t.Errorf("abajo a la izquierda en %v = %v", caja, got)
		}
	}
	// Sin saber aún cómo es la página, lo mismo que antes.
	if got := posicionFirma(cajaParaFirmar(nil, 1, false), FirmaAbajoIzquierda); got != siempre {
		t.Errorf("sin páginas = %v", got)
	}
}

func TestPosicionFirmaCaeDondeDice(t *testing.T) {
	for _, c := range []struct {
		p    PosicionFirma
		caja Rect
		r    Rect
	}{
		{FirmaAbajoCentro, cajaA4, Rect{177.5, 56, 417.5, 130}},
		{FirmaAbajoDerecha, cajaA4, Rect{299, 56, 539, 130}},
		{FirmaArribaDerecha, cajaA4, Rect{299, 712, 539, 786}},
		{FirmaAbajoDerecha, cajaApaisada, Rect{546, 56, 786, 130}},
		{FirmaArribaDerecha, cajaA5, Rect{124, 465, 364, 539}},
		{FirmaArribaDerecha, cajaApaisada, Rect{546, 465, 786, 539}},
		// Una MediaBox que no empieza en 0,0 se respeta.
		{FirmaAbajoIzquierda, cajaDesplazada, Rect{156, 256, 396, 330}},
		{FirmaArribaDerecha, cajaDesplazada, Rect{416, 862, 656, 936}},
	} {
		if got := posicionFirma(c.caja, c.p); got != c.r {
			t.Errorf("%s en %v = %v; la web da %v", c.p, c.caja, got, c.r)
		}
	}
}

func TestNingunaPosicionSeSaleDelPapel(t *testing.T) {
	for _, caja := range []Rect{cajaA4, cajaA5, cajaApaisada, cajaTique, cajaDesplazada} {
		for _, p := range posicionesFirma {
			r := posicionFirma(caja, p)
			if !dentroDe(r, caja) || r[2]-r[0] <= 0 {
				t.Errorf("%s en %v = %v se sale", p, caja, r)
			}
		}
	}
	// En una hoja estrecha se escala sin deformar la rúbrica.
	r := posicionFirma(cajaTique, FirmaAbajoDerecha)
	if w, h := r[2]-r[0], r[3]-r[1]; w >= anchoFirma || math.Abs(w/h-anchoFirma/altoFirma) > 0.05 {
		t.Errorf("en un tique el recuadro es %v: %v×%v", r, w, h)
	}
}

func TestCajaParaFirmar(t *testing.T) {
	paginas := []Pagina{{Numero: 1, Caja: cajaA4}, {Numero: 2, Caja: cajaA5}}
	if got := cajaParaFirmar(paginas, 2, false); got != cajaA5 {
		t.Errorf("la de la página 2 = %v", got)
	}
	if got := cajaParaFirmar(paginas, 9, false); got != cajaA4 {
		t.Errorf("una página que no existe cae en la primera: %v", got)
	}
	if got, _ := cajaComun(paginas); got != (Rect{0, 0, 420, 595}) {
		t.Errorf("la zona común = %v", got)
	}
	// En todas, la zona que comparten: no se sale de la más pequeña.
	if r := posicionFirma(cajaParaFirmar(paginas, 1, true), FirmaArribaDerecha); !dentroDe(r, cajaA5) {
		t.Errorf("en todas, arriba a la derecha = %v se sale del A5", r)
	}
}

// TestRedondearFirmaComoLaWeb: `Math.round` sube los medios también en negativo.
func TestRedondearFirmaComoLaWeb(t *testing.T) {
	for _, c := range [][2]float64{{1.24, 1}, {1.25, 1.5}, {-1.25, -1}, {-1.3, -1.5}} {
		if got := redondearFirma(c[0]); got != c[1] {
			t.Errorf("redondearFirma(%v) = %v; quiero %v", c[0], got, c[1])
		}
	}
}

// posicionesDe son las cuatro opciones del selector, por su rótulo.
func posicionesDe(objs []fyne.CanvasObject) []*Accion {
	acc := accionesDe(objs)
	var r []*Accion
	for _, n := range []string{"Abajo a la izquierda", "Abajo en el centro", "Abajo a la derecha", "Arriba a la derecha"} {
		if len(acc[n]) == 1 {
			r = append(r, acc[n][0])
		}
	}
	return r
}

// firmarDePrueba abre «Firmar» con un diálogo cuyo estado se puede tocar, y devuelve
// cómo recoger lo que pinta tras cada cambio.
func firmarDePrueba(t *testing.T, tam fyne.Size) (*dialogoFirmar, fyne.Window, func() []fyne.CanvasObject) {
	t.Helper()
	p, w, _ := dialogoA(t, DialogoFirmar, tam)
	d := nuevoDialogoFirmar(p.dialogo)
	objetos := func() []fyne.CanvasObject {
		var objs []fyne.CanvasObject
		todos(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) { objs = append(objs, o) })
		return objs
	}
	fyne.DoAndWait(func() {
		p.dialogo.pintar = d.vista
		p.dialogo.rehacer()
	})
	return d, w, objetos
}

// TestFirmarElegirPosicionMandaEseRect: pulsar una posición la marca, mueve el recuadro
// de la miniatura y es lo que viaja —a mano en `rect` con los trazos dentro, con
// certificado en `Rect`—, con el tamaño de la página elegida. Sin firma visible y con
// la aplicación oficial no se ofrece.
func TestFirmarElegirPosicionMandaEseRect(t *testing.T) {
	d, w, objetos := firmarDePrueba(t, fyne.NewSize(1280, 800))
	defer w.Close()
	ops := posicionesDe(objetos())
	if len(ops) != 4 {
		t.Fatalf("hay %d posiciones", len(ops))
	}
	if ops[0].Peso != ui.Medio || ops[0].Borde != "primary" || ops[3].Borde != "border" {
		t.Error("de entrada va marcada «Abajo a la izquierda» y solo ella")
	}
	m, ok := primero[*miniaturaFirma](objetos())
	if !ok {
		t.Fatal("no está la miniatura")
	}
	antes, _ := m.cajaRecuadro()

	fyne.DoAndWait(func() { ops[3].AlPulsar() }) // «Arriba a la derecha»
	if d.posicion != FirmaArribaDerecha {
		t.Fatalf("la posición es %q", d.posicion)
	}
	ops = posicionesDe(objetos())
	if ops[3].Borde != "primary" || ops[0].Borde != "border" {
		t.Error("la marcada no ha pasado a «Arriba a la derecha»")
	}
	m, _ = primero[*miniaturaFirma](objetos())
	despues, _ := m.cajaRecuadro()
	// Arriba a la derecha en pantalla: más a la derecha y más arriba que antes.
	if despues.X <= antes.X || despues.Y >= antes.Y {
		t.Errorf("el recuadro de la miniatura no se ha movido: %v → %v", antes, despues)
	}

	d.lienzo.trazos = [][][2]float64{{{0, 0}, {480, 148}}}
	quiero := Rect{299, 712, 539, 786}
	c := d.cambio()
	datos, _ := c.Datos.(map[string]any)
	if datos["rect"] != quiero {
		t.Errorf("a mano se manda rect %v; quiero %v", datos["rect"], quiero)
	}
	trazos, _ := datos["trazos"].([][][2]float64)
	if len(trazos) != 1 || trazos[0][0] != [2]float64{299, 786} || trazos[0][1] != [2]float64{539, 712} {
		t.Errorf("los trazos no caen en el recuadro elegido: %v", trazos)
	}
	// La página 2 del documento de prueba es otro A4; con una apaisada cambia el sitio.
	// Escribir otra página mueve también la miniatura, sin quitarle el foco al campo.
	d.a.p.Contenido.Paginas[1].Caja = cajaApaisada
	d.a.p.Contenido.Paginas[1].Ancho, d.a.p.Contenido.Paginas[1].Alto = 842, 595
	fyne.DoAndWait(func() { w.Canvas().Focus(d.pagina); d.pagina.PonerTexto("2") })
	if datos, _ = d.cambio().Datos.(map[string]any); datos["rect"] != (Rect{546, 465, 786, 539}) {
		t.Errorf("en la apaisada se manda %v", datos["rect"])
	}
	m, _ = primero[*miniaturaFirma](objetos())
	if m.pagina.Numero != 2 || m.rect != (Rect{546, 465, 786, 539}) || m.MinSize().Height >= m.MinSize().Width {
		t.Errorf("la miniatura no sigue a la página 2: %+v %v %v", m.pagina, m.rect, m.MinSize())
	}
	if w.Canvas().Focused() != d.pagina {
		t.Error("al escribir la página, el campo pierde el foco")
	}
	fyne.DoAndWait(func() { d.pagina.PonerTexto("1") })
	d.modo = "certificado"
	d.pkcs12 = ficheroElegido{nombre: "yo.p12", datos: []byte("P12")}
	if datos, _ = d.cambio().Datos.(map[string]any); datos["Rect"] != quiero {
		t.Errorf("con certificado se manda Rect %v", datos["Rect"])
	}
	if got := d.opcionesDeFirma().Rect; got != quiero {
		t.Errorf("con el certificado del sistema se manda %v", got)
	}

	fyne.DoAndWait(func() { d.modo = "oficial"; d.a.rehacer() })
	if len(posicionesDe(objetos())) != 0 {
		t.Error("con la aplicación oficial no se ofrece dónde poner la firma")
	}
	fyne.DoAndWait(func() { d.modo, d.visible = "mano", false; d.a.rehacer() })
	if len(posicionesDe(objetos())) != 0 {
		t.Error("sin firma visible no se ofrece dónde ponerla")
	}
	if datos, _ = d.cambio().Datos.(map[string]any); datos["rect"] != (Rect{}) {
		t.Errorf("sin firma visible el recuadro es %v", datos["rect"])
	}
}

// TestRecuadroDibujadoComoLaWeb: los mismos casos que `describe('recuadro dibujado')`.
func TestRecuadroDibujadoComoLaWeb(t *testing.T) {
	a4 := cajaA4
	if r := recuadroArrastrado(a4, [2]float64{100, 400}, [2]float64{340, 420}); r != (Rect{100, 400, 340, 474}) {
		t.Errorf("arrastrar deja %v", r)
	}
	if r := recuadroArrastrado(a4, [2]float64{340, 474}, [2]float64{100, 470}); r != (Rect{100, 400, 340, 474}) {
		t.Errorf("hacia atrás deja %v", r)
	}
	if r := recuadroArrastrado(a4, [2]float64{200, 200}, [2]float64{200, 200}); r[2]-r[0] != anchoMinimoFirma {
		t.Errorf("un toque suelto deja %v", r)
	}
	for _, c := range []struct {
		r, caja Rect
	}{
		{recuadroArrastrado(a4, [2]float64{500, 800}, [2]float64{900, 900}), a4},
		{recuadroArrastrado(cajaTique, [2]float64{0, 0}, [2]float64{1000, 0}), cajaTique},
		{recuadroArrastrado(cajaDesplazada, [2]float64{110, 210}, [2]float64{0, 0}), cajaDesplazada},
		{moverRecuadro(a4, Rect{56, 56, 296, 130}, -500, 9000), a4},
		{escalarRecuadro(a4, Rect{300, 700, 540, 774}, 10), a4},
	} {
		if !dentroDe(c.r, c.caja) {
			t.Errorf("%v se sale de %v", c.r, c.caja)
		}
	}
	if r := encajarFirma(a4, Rect{-10, -10, 230, 64}); r != (Rect{0, 0, 240, 74}) {
		t.Errorf("encajar deja %v", r)
	}
	if r := escalarRecuadro(a4, Rect{100, 100, 340, 174}, 0.01); r[2]-r[0] != anchoMinimoFirma || math.Abs((r[0]+r[2])/2-220) > 0.5 {
		t.Errorf("encoger deja %v", r)
	}
	r := Rect{297.5, 421, 417.5, 458}
	f := aFracciones(a4, r)
	if got := deFracciones(a4, f); got != r {
		t.Errorf("ida y vuelta: %v", got)
	}
	if enA5 := deFracciones(cajaA5, f); math.Abs(enA5[0]-210) > 0.5 || !dentroDe(enA5, cajaA5) {
		t.Errorf("en un A5: %v", enA5)
	}
	if got := rectElegido(FirmaDibujada, a4, nil); got != (Rect{56, 56, 296, 130}) {
		t.Errorf("sin dibujar sale en %v", got)
	}
}

// TestFirmarDibujarElRecuadroSinPerderElTrazo: elegir «Dibujar en la página» enseña la
// hoja en grande; arrastrar en ella pone el recuadro que se manda, y la rúbrica del
// lienzo sigue ahí, metida en él. Con el foco, las flechas lo mueven.
func TestFirmarDibujarElRecuadroSinPerderElTrazo(t *testing.T) {
	d, w, objetos := firmarDePrueba(t, fyne.NewSize(1280, 800))
	defer w.Close()
	if _, ok := primero[*dibujoFirma](objetos()); ok {
		t.Fatal("sin elegirlo, no se ofrece dibujar")
	}
	d.lienzo.trazos = [][][2]float64{{{0, 0}, {480, 148}}}
	op := accionesDe(objetos())["Dibujar en la página"]
	if len(op) != 1 {
		t.Fatal("no está «Dibujar en la página»")
	}
	fyne.DoAndWait(func() { op[0].AlPulsar() })
	if d.posicion != FirmaDibujada {
		t.Fatalf("la posición es %q", d.posicion)
	}
	area, ok := primero[*dibujoFirma](objetos())
	if !ok {
		t.Fatal("no está el área de dibujar")
	}
	if got := area.MinSize().Width; got != anchoDibujoFirma {
		t.Errorf("el área mide %v de ancho; quiero %v (`max-w-64`)", got, anchoDibujoFirma)
	}
	tam := area.Size()
	escala := float64(tam.Width) / 595
	en := func(x, y float64) fyne.Position { return fyne.NewPos(float32(x*escala), float32(y*escala)) }
	fyne.DoAndWait(func() {
		area.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: en(340, 320)}, Dragged: fyne.NewDelta(float32(240*escala), float32(20*escala))})
		area.DragEnd()
	})
	r := d.rectVisible()
	if r[0] < 95 || r[0] > 105 || r[3] < 537 || r[3] > 547 || math.Abs((r[2]-r[0])/(r[3]-r[1])-anchoFirma/altoFirma) > 0.05 {
		t.Errorf("el recuadro dibujado es %v", r)
	}
	if d.miniatura == nil || d.miniatura.rect != r {
		t.Errorf("la miniatura no sigue al recuadro dibujado: %v", d.miniatura)
	}
	datos, _ := d.cambio().Datos.(map[string]any)
	if datos["rect"] != r {
		t.Errorf("se manda %v; quiero %v", datos["rect"], r)
	}
	trazos, _ := datos["trazos"].([][][2]float64)
	if len(trazos) != 1 || trazos[0][0] != [2]float64{r[0], r[3]} || trazos[0][1] != [2]float64{r[2], r[1]} {
		t.Errorf("la rúbrica no cae en el recuadro dibujado: %v", trazos)
	}
	// Rehacer el diálogo (otra pestaña y vuelta) no lo mueve ni borra la rúbrica.
	fyne.DoAndWait(func() { d.modo = "certificado"; d.a.rehacer(); d.modo = "mano"; d.a.rehacer() })
	if d.rectVisible() != r || len(d.lienzo.trazos) != 1 {
		t.Errorf("tras rehacer: %v, %d trazos", d.rectVisible(), len(d.lienzo.trazos))
	}
	area, _ = primero[*dibujoFirma](objetos())
	fyne.DoAndWait(func() { area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp}) })
	if got := d.rectVisible(); got[1] <= r[1] {
		t.Errorf("la flecha arriba no lo sube: %v → %v", r, got)
	}
}
