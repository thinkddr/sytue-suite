// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	m.Run()
}

// escala a la que se comprueban los píxeles: ×2, por lo mismo que en `ui` —a ×1 el
// pintor tiene que elegir una fila para un medio píxel y se mide el redondeo, no el
// widget—.
const escala float32 = 2

func pinta(t *testing.T, w, h float32, objs ...fyne.CanvasObject) image.Image {
	t.Helper()
	fondo := fcanvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	fondo.Resize(fyne.NewSize(w, h))
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewWithoutLayout(append([]fyne.CanvasObject{fondo}, objs...)...))
	c.SetScale(escala)
	c.Resize(fyne.NewSize(w, h))
	return c.Capture()
}

// guarda deja el PNG donde se pueda mirar con los ojos: la única forma de comprobar
// que lo pintado se PARECE a la captura de la web, que es lo que ningún número dice.
func guarda(t *testing.T, nombre string, img image.Image) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		return
	}
	f, err := os.Create(dir + "/" + nombre)
	if err != nil {
		t.Fatalf("no se puede guardar la captura: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func px(img image.Image, x, y int) color.NRGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func cerca(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > 1 || int(y)-int(x) > 1 }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

// caja devuelve el rectángulo de píxeles de DISPOSITIVO donde aparece un color.
func caja(img image.Image, c color.NRGBA) (x0, x1, y0, y1 int) {
	b := img.Bounds()
	x0, x1, y0, y1 = 1<<30, -1, 1<<30, -1
	for y := range b.Dy() {
		for x := range b.Dx() {
			if cerca(px(img, x, y), c) {
				x0, x1, y0, y1 = min(x0, x), max(x1, x), min(y0, y), max(y1, y)
			}
		}
	}
	return x0, x1, y0, y1
}

// tinta cuenta los píxeles que NO son del fondo: la medida de cuánto trazo hay.
func cuentaTinta(img image.Image, fondo color.NRGBA) int {
	b, n := img.Bounds(), 0
	for y := range b.Dy() {
		for x := range b.Dx() {
			if !cerca(px(img, x, y), fondo) {
				n++
			}
		}
	}
	return n
}

func leeModelo(t *testing.T) *Modelo {
	t.Helper()
	m, err := Leer(muestra(t))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestElEncajeEsElDeLaWeb son los números de `scale` de SlidesCanvas.tsx, incluida la
// regla que no es la obvia: el ancho manda, el alto solo si tiene hueco.
//
// El caso que importa es el del área SIN ALTO, que es como llegó a producción una
// diapositiva que se salía 372 px en un iPhone 12: tomar el mínimo de los dos ejes y
// caer a 0,5 cuando sale negativo es lo que lo causaba.
func TestElEncajeEsElDeLaWeb(t *testing.T) {
	m := &Modelo{} // 16:9 → escenario 960×540
	casos := []struct {
		nombre string
		area   fyne.Size
		zoom   float32
		quiero float32
	}{
		// 1024-64 = 960 justo, y de alto sobra: el ancho manda y sale exactamente 1.
		{"el ancho manda", fyne.NewSize(1024, 2000), 100, 1},
		// De alto solo hay 604-64 = 540: encaja por el alto.
		{"el alto aprieta", fyne.NewSize(2000, 604), 100, 1},
		// Sin alto medido NO se cae a un tamaño fijo: se usa el ancho.
		{"área sin alto", fyne.NewSize(1024, 0), 100, 1},
		// Ni ancho ni alto: el suelo, y no medio escenario.
		{"área sin medir", fyne.NewSize(0, 0), 100, 0.08},
		{"el zoom multiplica", fyne.NewSize(1024, 2000), 50, 0.5},
	}
	for _, c := range casos {
		if got := m.Encaje(c.area, HuecoEncaje, c.zoom); got != c.quiero {
			t.Errorf("%s: Encaje(%v) = %v, quiero %v", c.nombre, c.area, got, c.quiero)
		}
	}
}

// TestElEscenarioGuardaSuProporcion: el lienzo centra la caja de proporción fija más
// grande que quepa, y no se estira para llenar el widget.
//
// Se mide sobre los píxeles PINTADOS del fondo de la diapositiva, no sobre el número
// que calcula el propio widget: comparar una implementación consigo misma pasa siempre.
func TestElEscenarioGuardaSuProporcion(t *testing.T) {
	casos := []struct {
		nombre     string
		json       string
		proporcion float32
	}{
		{"16:9 por defecto", `{"slides":[{"id":"s","boxes":[]}]}`, 16.0 / 9.0},
		{"4:3", `{"slides":[{"id":"s","boxes":[]}],"size":{"preset":"4:3"}}`, 4.0 / 3.0},
	}
	for _, c := range casos {
		m, err := Leer([]byte(c.json))
		if err != nil {
			t.Fatal(err)
		}
		l := NuevoLienzo(m, 0)
		// Un hueco deliberadamente CUADRADO: si el escenario se estirase, saldría
		// cuadrado y la proporción se perdería sin que nadie lo viera.
		l.Resize(fyne.NewSize(400, 400))
		// Fondo gris para que el papel blanco de la diapositiva se distinga.
		gris := fcanvas.NewRectangle(color.NRGBA{R: 0xE9, G: 0xE6, B: 0xDD, A: 255})
		gris.Resize(fyne.NewSize(400, 400))
		img := pinta(t, 400, 400, gris, l)
		x0, x1, y0, y1 := caja(img, papel)
		w, h := float32(x1-x0+1), float32(y1-y0+1)
		if p := w / h; p < c.proporcion-0.01 || p > c.proporcion+0.01 {
			t.Errorf("%s: el escenario pintado mide %vx%v (proporción %v) y tiene que ser %v",
				c.nombre, w, h, p, c.proporcion)
		}
		// Y centrado: el mismo aire arriba que abajo. El margen son DOS píxeles de
		// dispositivo —uno por lado—: el origen del escenario cae en medio píxel de
		// CSS (87,5 a ×2 son 175 filas) y la fila de borde queda a medio cubrir, así
		// que el barrido por color no la cuenta.
		if arriba, abajo := y0, img.Bounds().Dy()-1-y1; arriba-abajo > 2 || abajo-arriba > 2 {
			t.Errorf("%s: %d px de aire arriba y %d abajo: el escenario no está centrado",
				c.nombre, arriba, abajo)
		}
	}
}

// TestElEscenarioEscalaSinRasterizar es LA medición de este encargo.
//
// Fyne no sabe escalar un subárbol ya compuesto, y una presentación es justo eso: un
// lienzo de tamaño fijo que se encoge para caber. Lo que se comprueba aquí es que la
// salida NO se degrada al encoger, porque el escenario no se compone y luego se
// escala: se compone YA a la escala final, y el texto se conforma al tamaño que toca.
//
// La medida: se pinta el MISMO título a dos escalas y se compara la cobertura de tinta
// por unidad de área. Si el escenario se rasterizara y luego se estirara —lo que hace
// `canvas.Image` con ApproxBiLinear, el motivo de que los iconos vayan con
// `canvas.Raster`—, al encoger se perderían trazos enteros y la cobertura caería.
func TestElEscenarioEscalaSinRasterizar(t *testing.T) {
	m, err := Leer([]byte(`{"slides":[{"id":"s","boxes":[` +
		`{"id":"b","x":0,"y":0,"w":100,"h":100,"html":"<h1>Presentación</h1>"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}

	cobertura := func(ancho float32) float64 {
		l := NuevoLienzo(m, 0)
		alto := ancho * 9 / 16
		l.Resize(fyne.NewSize(ancho, alto))
		img := pinta(t, ancho, alto, l)
		guarda(t, fmt.Sprintf("escenario-%.0f.png", ancho), img)
		return float64(cuentaTinta(img, papel)) / float64(ancho*alto)
	}

	// 960 es el escenario a tamaño natural; 128 es el ancho de la miniatura del
	// carril (THUMB_W), o sea ×0,1333: el caso más duro que hay en la pantalla.
	grande, chica := cobertura(960), cobertura(128)
	t.Logf("cobertura de tinta: %.5f a 960 px de ancho (×1) y %.5f a 128 (×0,133)", grande, chica)
	if grande == 0 || chica == 0 {
		t.Fatalf("no se ha pintado nada: cobertura %v y %v", grande, chica)
	}
	// El texto pequeño cubre proporcionalmente MENOS (el antialias reparte el mismo
	// trazo en menos píxeles), pero no puede desaparecer. Con estirado de mapa de
	// bits la caída es de más de un orden de magnitud.
	if chica < grande/3 {
		t.Errorf("a ×0,133 la cobertura de tinta cae de %.4f a %.4f (más de 3×): "+
			"el escenario se está rasterizando y estirando en vez de conformarse al tamaño final",
			grande, chica)
	}
}

// TestLoQueNoSePintaSaleDicho: nada se cuela como si estuviera dibujado.
func TestLoQueNoSePintaSaleDicho(t *testing.T) {
	m := leeModelo(t)
	// Formas, gráfico, diagrama, conectores y el marcador del vídeo se pintan con el
	// SVG de la web, y la Georgia del tema con la del equipo; queda la imagen por URL.
	quiero := []string{
		"slides.box.image", // la imagen es http(s), no un dataURL
	}
	got := m.NoPintado()
	if len(got) != len(quiero) {
		t.Fatalf("NoPintado = %v, quiero %v", got, quiero)
	}
	for _, q := range quiero {
		if !contiene(got, q) {
			t.Errorf("falta %q en %v", q, got)
		}
	}
	// Y una presentación que sí se entiende entera no dice nada.
	simple, err := Leer([]byte(`{"slides":[{"id":"s","boxes":[{"id":"b","x":0,"y":0,"w":50,"h":20,"html":"<p>hola</p>"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if n := simple.NoPintado(); len(n) != 0 {
		t.Errorf("una diapositiva de solo texto dice que no pinta %v", n)
	}
}

func contiene(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// TestUnTemaPropioSePintaConSusColoresYSuFuente es `slideVisuals` con un tema de la
// presentación: fondo, tinta y fuente salen de él; uno con un color roto se pinta como
// Papel; y el fondo propio de la diapositiva sigue mandando.
func TestUnTemaPropioSePintaConSusColoresYSuFuente(t *testing.T) {
	m := &Modelo{
		Diapos: []Diapo{{ID: "s1"}, {ID: "s2", Fondo: "#BBF7D0"}},
		Tema:   "custom-1",
		Temas: []TemaPropio{
			{ID: "custom-1", Nombre: "Cooperativa", Fondo: "#F4F1EA", Tinta: "#1B2A41", Fuente: "Georgia"},
			{ID: "custom-2", Nombre: "Roto", Fondo: "rojo", Tinta: "#000000"},
		},
	}
	v := m.Visuales(0)
	if v.Fondo != (color.NRGBA{R: 0xF4, G: 0xF1, B: 0xEA, A: 255}) || v.Tinta != (color.NRGBA{R: 0x1B, G: 0x2A, B: 0x41, A: 255}) || v.Familia != "Georgia" {
		t.Errorf("tema propio = %+v", v)
	}
	if f := m.Visuales(1).Fondo; f != (color.NRGBA{R: 0xBB, G: 0xF7, B: 0xD0, A: 255}) {
		t.Errorf("el fondo propio de la diapositiva debía mandar: %+v", f)
	}
	m.Tema = "custom-2"
	if v := m.Visuales(0); v.Fondo != papel || v.Tinta != tinta || v.Familia != "" {
		t.Errorf("un tema con un color roto debía pintarse como Papel: %+v", v)
	}
	m.PonerTema("custom-1")
	if m.Tema != "custom-1" {
		t.Errorf("PonerTema no acepta un tema propio: %q", m.Tema)
	}
	m.PonerTema("custom-9")
	if m.Tema != "" {
		t.Errorf("un tema que no existe debía dejar Papel: %q", m.Tema)
	}
}
