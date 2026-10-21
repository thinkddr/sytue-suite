// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"errors"
	"image"
	"image/color"
	"net/http"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestSoloSinPermisoEsElCandado: un 403 con `sin_permiso` es el candado de «No tienes
// acceso»; otro 403 es un fallo con el mensaje del servicio en la banda, que es
// `esSinPermiso` de la web.
//
// VISTO EN ROJO (frontal, 2026-09-23): leyendo todo 403 como `Prohibida`, el de una
// organización personal salía con el candado y sin decir por qué.
func TestSoloSinPermisoEsElCandado(t *testing.T) {
	tr := traductor(t)
	sin := &api.Error{Status: http.StatusForbidden, Body: `{"error":{"code":"sin_permiso","message":"You do not have access to this module."}}`}
	if f, aviso := FaseDeLista(sin, true, 0, tr); f != Prohibida || aviso != "" {
		t.Errorf("sin_permiso: %v, %q; quiero el candado y sin banda", f, aviso)
	}
	otro := &api.Error{Status: http.StatusForbidden, Body: `{"error":{"code":"org_personal","message":"Personal accounts do not have an ERP."}}`}
	if f, aviso := FaseDeLista(otro, true, 0, tr); f != Fallo || aviso != "Personal accounts do not have an ERP." {
		t.Errorf("otro 403: %v, %q; quiero un fallo con el mensaje del servicio", f, aviso)
	}
	if f, _ := FaseDeLista(errors.New("corte"), true, 0, tr); f != Fallo {
		t.Errorf("un corte: %v", f)
	}
}

// TestElSelectorDelLoteMideLoDeLaWeb: el `<select>` de «Cambiar estado» con los estados
// de una factura. area-apagada-278.json, metrics.lote (Figtree12, raíz16): 100×24.
func TestElSelectorDelLoteMideLoDeLaWeb(t *testing.T) {
	opciones := make([]string, 0, 1+len(facturaArnes().Estados))
	opciones = append(opciones, traductor(t).T("relation.choose", nil))
	for _, e := range facturaArnes().Estados {
		opciones = append(opciones, e.Etiqueta)
	}
	if a := AnchoSelectLote(opciones); a < 99 || a > 101 {
		t.Errorf("el selector del lote mide %.2f de ancho; la web 100", a)
	}
	if AltoSelectLote != 24 {
		t.Errorf("el selector del lote mide %.2f de alto; la web 24", AltoSelectLote)
	}
}

// TestLasTarjetasDeLaPortadaCambianDeBordeAlPasar: `hover:border-border-strong` en las
// tarjetas del resumen y de los recientes.
//
// VISTO EN ROJO (frontal, 2026-09-23): con el `Refresh` vacío de antes, el borde se
// queda en `border` con el puntero encima.
func TestLasTarjetasDeLaPortadaCambianDeBordeAlPasar(t *testing.T) {
	o, _ := tarjetaKPI(tema.Claro, "Ventas", "4406,96 €", "Facturas", "", ui.InsigniaNeutra, 300, func() {})
	p, ok := o.(*PulsableCon)
	if !ok || p.caja == nil {
		t.Fatal("la tarjeta del resumen no lleva la caja de su fondo")
	}
	w := test.NewWindow(p)
	defer w.Close()
	w.Resize(fyne.NewSize(300, 100))
	borde := func() color.NRGBA {
		c, _ := p.caja.Objetos()[0].(*canvas.Rectangle).FillColor.(color.NRGBA)
		return c
	}
	if borde() != tema.Color("border", tema.Claro) {
		t.Errorf("en reposo el borde es %v y no `border`", borde())
	}
	p.MouseIn(&desktop.MouseEvent{})
	if borde() != tema.Color("border-strong", tema.Claro) {
		t.Errorf("con el puntero encima el borde es %v y no `border-strong`", borde())
	}
	p.MouseOut()
	if borde() != tema.Color("border", tema.Claro) {
		t.Errorf("al salir el borde es %v y no vuelve a `border`", borde())
	}
}

// TestLasCifrasVanASuAvanceFijo: `tabular-nums` como la variante `tnum` de la fuente,
// cada cifra centrada en su celda de 0,625 em. «2026» a text-sm13/500: Chromium
// medidas-vistas-extra-final.json, obligaciones-guia-calculo854, devuelve
// width40.5625 con padding0px4px: texto interior32.5625 y «1111» mide lo mismo que «8888».
//
// VISTO EN ROJO (frontal, 2026-09-23): con cada cifra a su avance propio, «1111» mide
// menos que «8888» y los importes del banco no quedan en columna.
func TestLasCifrasVanASuAvanceFijo(t *testing.T) {
	negro := color.NRGBA{A: 255}
	o, w := textoTabular("2026", tema.TextoSM, ui.Medio, negro)
	cerca(t, "el año con cifras tabulares", w, 32.5625)
	_, unos := textoTabular("1111", tema.TextoSM, ui.Medio, negro)
	_, ochos := textoTabular("8888", tema.TextoSM, ui.Medio, negro)
	if unos != ochos {
		t.Errorf("«1111» mide %.2f y «8888» %.2f: las cifras no van a su avance fijo", unos, ochos)
	}
	c, ok := o.(*fyne.Container)
	if !ok || len(c.Objects) != 4 {
		t.Fatalf("«2026» tendría que ir en cuatro cifras colocadas: %T", o)
	}
	c.Resize(fyne.NewSize(w, tema.TextoSM.Linea))
	celda := CifraTabular * tema.TextoSM.Tam
	for i, x := range c.Objects {
		d := x.(*canvas.Text)
		quiero := float32(i)*celda + (celda-ui.Medir(d.Text, tema.TextoSM, ui.Medio).Width)/2
		cerca(t, "la cifra "+d.Text, d.Position().X, quiero)
	}
}

// TestUnAreaApagadaVaEnteraAlOchentaPorCiento: `opacity-80` de la tarjeta, que en la web
// atenúa también la insignia, los modelos y el botón. Los PUNTOS son los de Chromium,
// area-apagada-278.json (2026-10-04): Card real aislado a278×177 sobre el fondo
// de la página, CSS1Compat, raíz16 y Figtree; cada uno sale de su rectángulo DOM.
// Los COLORES son esa misma composición rehecha a mano con la paleta petróleo, sin
// pasar por el renderer nativo: cada capa (card-blue #e8efff; `bg/50` encima para el
// modelo; primary #0f7680 y su borde rgba(15,29,32,.6); bg-subtle #e8efef) al 80 % sobre
// el lienzo #f1f6f6. El borde va sobre primary antes de aplicar la opacidad del grupo.
func TestUnAreaApagadaVaEnteraAlOchentaPorCiento(t *testing.T) {
	b := portadaArnes(t, 1280)
	c := b.tarjeta(tema.Claro, b.Modulos[0], 278)
	w := test.NewWindow(c.o)
	defer w.Close()
	w.SetPadded(false)
	w.Resize(fyne.NewSize(278, c.alto))
	img := w.Canvas().Capture()
	for _, p := range []struct {
		que     string
		x, y    int
		r, g, b uint32
	}{
		{"el fondo de la tarjeta", 2, 88, 234, 240, 253},
		{"el fondo del modelo", 20, 85, 237, 243, 248},
		{"el borde del botón", 219, 159, 60, 101, 105},
		{"el fondo del botón", 182, 144, 60, 144, 152},
		// Cuatro píxeles antes del borde derecho, en el centro vertical y fuera del texto.
		{"el fondo de la insignia", 257, 27, 235, 240, 237},
	} {
		r, g, bl, _ := img.At(p.x, p.y).RGBA()
		if dif(r>>8, p.r)+dif(g>>8, p.g)+dif(bl>>8, p.b) > 4 {
			t.Errorf("%s pinta (%d,%d,%d); la web (%d,%d,%d)", p.que, r>>8, g>>8, bl>>8, p.r, p.g, p.b)
		}
	}
}

// TestLaCasillaDeLaTablaEsLaDelNavegador: la del ERP es el `<input type="checkbox">` de
// Chromium (13×13), no la `Checkbox` de la suite (14). Píxeles de la web, tabla de
// facturas del arnés en claro (2026-09-24), relativos a la esquina de la casilla.
//
// VISTO EN ROJO (frontal, 2026-09-24): con el primario de la suite en vez del azul del
// navegador, la marcada sale (26, 53, 65) y no (0, 117, 255).
func TestLaCasillaDeLaTablaEsLaDelNavegador(t *testing.T) {
	pintar := func(marcada bool) image.Image {
		c := NuevaCasillaNativa("Ferretería Lobo", marcada, func() {})
		w := test.NewWindow(c)
		defer w.Close()
		w.SetPadded(false)
		w.Resize(c.MinSize())
		if s := c.MinSize(); s.Width != 13 || s.Height != 13 {
			t.Errorf("la casilla mide %v; la del navegador 13×13", s)
		}
		return w.Canvas().Capture()
	}
	mirar := func(que string, img image.Image, x, y int, r, g, b uint32) {
		t.Helper()
		cr, cg, cb, _ := img.At(x, y).RGBA()
		if dif(cr>>8, r)+dif(cg>>8, g)+dif(cb>>8, b) > 6 {
			t.Errorf("%s en (%d, %d) pinta (%d,%d,%d); la web (%d,%d,%d)", que, x, y, cr>>8, cg>>8, cb>>8, r, g, b)
		}
	}
	m := pintar(true)
	mirar("el relleno de la marcada", m, 1, 5, 0, 117, 255)
	mirar("el relleno de la marcada", m, 11, 11, 0, 117, 255)
	for _, p := range [][2]int{{9, 3}, {3, 7}, {6, 7}, {4, 8}, {5, 8}} {
		mirar("el tic", m, p[0], p[1], 255, 255, 255)
	}
	vacia := pintar(false)
	mirar("el borde de la vacía", vacia, 0, 6, 0x76, 0x76, 0x76)
	mirar("el borde de la vacía", vacia, 6, 0, 0x76, 0x76, 0x76)
	mirar("el fondo de la vacía", vacia, 6, 6, 255, 255, 255)
}
