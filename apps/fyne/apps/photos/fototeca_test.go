// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestMain arranca la aplicación de pruebas con NUESTRO tema: sin él, `canvas.Text`
// resolvería la letra de serie y las medidas de aquí serían las de otra fuente. Corre
// sin CGO y sin servidor gráfico, que es lo que deja meter estos guardianes en el
// pre-push.
func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	enLaInterfaz = func(func()) {} // ver enLaInterfaz
	m.Run()
}

// escala es a la que se comprueban los píxeles: DOS, que es donde los medios píxeles
// de la escala de la suite salen exactos (ver el paquete `ui`).
const escala float32 = 2

// bandas construye una imagen de w×h en cuatro franjas verticales de colores distintos.
// Sirve para ver DÓNDE recorta `object-fit: cover`: si el recorte no fuera centrado, el
// color del centro de la tesela no sería el de la franja del centro de la fuente.
func bandas(w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	colores := []color.RGBA{
		{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}, {R: 255, G: 255, A: 255},
	}
	for x := range w {
		c := colores[x*len(colores)/w]
		for y := range h {
			m.Set(x, y, c)
		}
	}
	return m
}

func jpegDe(m image.Image) []byte {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, m, &jpeg.Options{Quality: 92}); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func pngDe(m image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// tecaCon devuelve una fototeca ya cargada, sin red de por medio.
func tecaCon(datos map[string][]byte) *Fototeca {
	f := NuevaFototeca()
	for id, b := range datos {
		f.Poner(id, b)
	}
	return f
}

// TestElRecorteEsCoverYCentrado comprueba que una foto apaisada entra en una tesela
// cuadrada RECORTADA por los lados y no aplastada, que es lo que hace `object-cover` y
// lo que `canvas.Image` no sabe hacer.
//
// La fuente son 512×256 con cuatro franjas; en un cuadrado solo caben las dos del
// centro, así que la mitad izquierda de la tesela tiene que ser VERDE (la segunda
// franja) y la derecha AZUL (la tercera). Si el recorte no fuera centrado saldría roja.
func TestElRecorteEsCoverYCentrado(t *testing.T) {
	m := recortarYEscalar(pngDe(bandas(512, 256)), 200, 200, 0)
	if m == nil {
		t.Fatal("no se pudo decodificar la fuente")
	}
	casos := []struct {
		x, y int
		que  color.RGBA
		cual string
	}{
		{50, 100, color.RGBA{G: 255, A: 255}, "la franja verde"},
		{150, 100, color.RGBA{B: 255, A: 255}, "la franja azul"},
	}
	for _, c := range casos {
		r, g, b, _ := m.At(c.x, c.y).RGBA()
		q := color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
		if !cercaRGB(q, c.que) {
			t.Errorf("en (%d,%d) hay %v y con object-cover centrado toca %s (%v)", c.x, c.y, q, c.cual, c.que)
		}
	}
}

// TestElRecorteConservaLaProporcion: la fuente apaisada NO se deforma. Se mide con la
// anchura de la franja: en la fuente cada franja ocupa 1/4 del ancho, y en la tesela
// —que enseña solo la mitad central— cada una de las dos visibles ocupa la MITAD. Si
// hubiera estirado en vez de recortar, se verían las cuatro.
func TestElRecorteConservaLaProporcion(t *testing.T) {
	m := recortarYEscalar(pngDe(bandas(512, 256)), 200, 200, 0)
	distintos := map[color.RGBA]bool{}
	for x := range 200 {
		r, g, b, _ := m.At(x, 100).RGBA()
		distintos[color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}] = true
	}
	// Dos franjas más los píxeles de la transición que deja Catmull-Rom.
	if len(distintos) > 8 {
		t.Errorf("la fila del centro tiene %d colores: con el recorte cover solo caben dos franjas, "+
			"así que la imagen se ha estirado en vez de recortarse", len(distintos))
	}
}

// TestLaTeselaTieneLasEsquinasRedondas es el `rounded-sm overflow-hidden` del `<li>`.
// Fyne no lo sabe hacer —su recorte es RECTANGULAR, está en los techos del toolkit—, así
// que lo hace el generador poniendo el alfa a cero fuera del arco.
func TestLaTeselaTieneLasEsquinasRedondas(t *testing.T) {
	m := recortarYEscalar(pngDe(bandas(200, 200)), 200, 200, 8)
	if _, _, _, a := m.At(0, 0).RGBA(); a != 0 {
		t.Errorf("la esquina (0,0) tiene alfa %d y con radio 8 tiene que estar fuera del arco", a>>8)
	}
	if _, _, _, a := m.At(100, 100).RGBA(); a>>8 != 255 {
		t.Errorf("el centro tiene alfa %d y tiene que ser opaco", a>>8)
	}
	// Y el antialias: el píxel que el arco parte por la mitad no es ni 0 ni 255.
	if _, _, _, a := m.At(2, 2).RGBA(); a>>8 == 0 || a>>8 == 255 {
		t.Errorf("el píxel (2,2) del arco tiene alfa %d: sin antialias la esquina sale con escalones", a>>8)
	}
}

// TestLaTeselaSePideEnPixelesDeDispositivo es LA medición que justifica usar
// `canvas.Raster` y no `canvas.Image`.
//
// A ×2, una tesela de 201,078 px de CSS son 402 filas de dispositivo. Fyne le pide al
// generador de un raster EXACTAMENTE esos píxeles, así que la foto se remuestrea una
// vez, a la resolución final. `canvas.Image` decodificaría a los 512 px de la variante
// y luego estiraría el mapa de bits con `ApproxBiLinear` hasta la caja — dos
// remuestreos, el segundo malo.
func TestLaTeselaSePideEnPixelesDeDispositivo(t *testing.T) {
	ladoCSS := float32(201.078)
	pedido := make(chan [2]int, 8)
	r := fcanvas.NewRaster(func(w, h int) image.Image {
		select {
		case pedido <- [2]int{w, h}:
		default:
		}
		return image.NewRGBA(image.Rect(0, 0, w, h))
	})
	r.ScaleMode = fcanvas.ImageScalePixels
	r.Resize(fyne.NewSize(ladoCSS, ladoCSS))
	pinta(t, ladoCSS, ladoCSS, r)

	select {
	case p := <-pedido:
		quiero := int(ladoCSS * escala)
		if p[0] < quiero-1 || p[0] > quiero+1 {
			t.Errorf("Fyne pidió %d×%d px y a ×%v una tesela de %v px de CSS son %d: "+
				"si pidiera los de CSS, la foto se estaría ampliando al pintar",
				p[0], p[1], escala, ladoCSS, quiero)
		}
		t.Logf("MEDIDO: tesela de %v px de CSS a ×%v → el raster recibe %d×%d px de dispositivo",
			ladoCSS, escala, p[0], p[1])
	default:
		t.Fatal("el generador del raster no llegó a llamarse: la tesela no se pintó")
	}
}

// TestLaFototecaNoPasaDeSuTope: una galería son miles de fotos y un mapa sin tope se
// come la memoria de la máquina. Se meten bytes hasta pasarse del tope y se comprueba
// que la caché los suelta.
func TestLaFototecaNoPasaDeSuTope(t *testing.T) {
	f := NuevaFototeca()
	uno := make([]byte, 1<<20) // 1 MB por foto
	for i := range TopeBytes/len(uno) + 8 {
		f.Poner(fmt.Sprintf("f%d", i), uno)
	}
	crudas, _ := f.Memoria()
	if crudas > TopeBytes {
		t.Errorf("las crudas ocupan %d B y el tope son %d: la caché no expulsa", crudas, TopeBytes)
	}
	// Y lo pintado, que es lo caro: 4 bytes por píxel.
	fuente := pngDe(bandas(512, 512))
	g := tecaCon(nil)
	for i := range TopePixeles/(402*402) + 6 {
		id := fmt.Sprintf("p%d", i)
		g.Poner(id, fuente)
		g.TeselaYa(id, 402, 402, 8)
	}
	_, pintadas := g.Memoria()
	if pintadas > TopePixeles*bytesPorPixe {
		t.Errorf("lo pintado ocupa %d B y el tope son %d", pintadas, TopePixeles*bytesPorPixe)
	}
	t.Logf("MEDIDO: con teselas de 402×402 (×2), el tope de lo pintado se alcanza en %d teselas (%.1f MB)",
		TopePixeles/(402*402), float64(pintadas)/(1<<20))
}

// TestLaMiniaturaPintaLaFotoEnSuCaja: la tesela declara lo que la rejilla le da y la
// foto lo llena. Se comprueba con el color, no con la descripción: la fuente es roja
// entera, así que el centro de la tesela tiene que salir rojo y las esquinas no.
func TestLaMiniaturaPintaLaFotoEnSuCaja(t *testing.T) {
	lado := float32(201.078)
	rojo := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for x := range 512 {
		for y := range 512 {
			rojo.Set(x, y, color.RGBA{R: 220, G: 30, B: 40, A: 255})
		}
	}
	teca := tecaCon(map[string][]byte{"a": pngDe(rojo)})
	m := NuevaMiniatura(api.Photo{ID: "a", Name: "a.jpg", Width: 512, Height: 512}, teca)
	m.Rotulo = "Abrir a.jpg"
	m.Resize(fyne.NewSize(lado, lado))
	// Dos pasadas: la primera pone la tesela en cola —prepararla NO ocurre en el hilo
	// que pinta— y la segunda ya la encuentra. Sin la espera, este guardián compararía
	// el hueco reservado y pasaría siempre.
	pinta(t, lado+8, lado+8, m)
	teca.Esperar()
	img := pinta(t, lado+8, lado+8, m)

	centro := px(img, int(lado*escala/2), int(lado*escala/2))
	if !cerca(centro, color.NRGBA{R: 220, G: 30, B: 40, A: 255}) {
		t.Errorf("el centro de la tesela pinta %v y la foto es (220,30,40)", centro)
	}
	// Fuera de la tesela, el lienzo blanco: la foto no se sale de su caja.
	if got := px(img, int((lado+4)*escala), int(lado*escala/2)); !cerca(got, color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("a 4 px a la derecha de la tesela hay %v y tenía que haber lienzo blanco", got)
	}
}

// TestLaTeselaReservaElHuecoAntesDeLaFoto: sin foto todavía, el sitio ya está pintado
// con `bg-bg-subtle`. Sin esto la cuadrícula da saltos mientras se desplaza, que es el
// defecto más visible de una galería.
func TestLaTeselaReservaElHuecoAntesDeLaFoto(t *testing.T) {
	lado := float32(201.078)
	m := NuevaMiniatura(api.Photo{ID: "sinfoto"}, NuevaFototeca())
	m.Resize(fyne.NewSize(lado, lado))
	img := pinta(t, lado+8, lado+8, m)
	got := px(img, int(lado*escala/2), int(lado*escala/2))
	if quiero := tema.Color("bg-subtle", tema.Claro); !cerca(got, quiero) {
		t.Errorf("la tesela sin foto pinta %v y el hueco reservado es bg-subtle %v", got, quiero)
	}
}

// ---- utilidades de píxel, las mismas que usa el paquete ui ----

func pinta(t *testing.T, w, h float32, objs ...fyne.CanvasObject) image.Image {
	t.Helper()
	return pintaA(t, escala, w, h, objs...)
}

// pintaA es pinta a una escala concreta. El barrido de las diez pantallas va a ×1
// —ahí no se mide un medio píxel— y los guardianes de píxel a ×2.
func pintaA(t *testing.T, esc, w, h float32, objs ...fyne.CanvasObject) image.Image {
	t.Helper()
	fondo := fcanvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	fondo.Resize(fyne.NewSize(w, h))
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewWithoutLayout(append([]fyne.CanvasObject{fondo}, objs...)...))
	c.SetScale(esc)
	c.Resize(fyne.NewSize(w, h))
	return c.Capture()
}

func px(img image.Image, x, y int) color.NRGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// cerca admite ±2 niveles de 255: el alfa se cuantiza al componer y ningún gate puede
// exigir byte a byte (está medido en el paquete ui).
func cerca(a, b color.NRGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= 2 && d(a.G, b.G) <= 2 && d(a.B, b.B) <= 2 && d(a.A, b.A) <= 2
}

// cercaRGB admite lo que mete el JPEG y el remuestreo: ±12 niveles. Se usa solo para
// distinguir franjas de colores primarios, no para comparar con un token.
func cercaRGB(a, b color.RGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= 12 && d(a.G, b.G) <= 12 && d(a.B, b.B) <= 12
}

// TestElVisorEnsenyaLaDeOtroTamanyoMientrasPrepara: al estirar la ficha la foto no se
// queda en blanco —lo que hay es la última que se preparó—, y tras girarla (Tirar) no
// sale la de antes del giro.
func TestElVisorEnsenyaLaDeOtroTamanyoMientrasPrepara(t *testing.T) {
	g := tecaCon(nil)
	if g.Pintada("f") != nil {
		t.Fatal("sin nada preparado no puede haber tesela")
	}
	g.Poner("f", pngDe(bandas(512, 512)))
	g.TeselaYa("f", 300, 200, 0)
	if m := g.Pintada("f"); m == nil || m.Bounds().Dx() != 300 {
		t.Fatalf("la última preparada es la de 300 de ancho y sale %v", m)
	}
	g.Tirar("f")
	if g.Pintada("f") != nil {
		t.Error("tras Tirar sigue saliendo la tesela de antes")
	}
}

// TestLaFotoQueLlegaRepintaSoloLoQueLaEnsenya: `avisar` llama a los repintados de ESA
// foto y no a los de otra, y tras `Olvidar` ya no llama a nadie.
func TestLaFotoQueLlegaRepintaSoloLoQueLaEnsenya(t *testing.T) {
	antes := enLaInterfaz
	enLaInterfaz = func(f func()) { f() }
	defer func() { enLaInterfaz = antes }()
	g := NuevaFototeca()
	a, b := 0, 0
	g.apuntar("a", func() { a++ })
	g.apuntar("b", func() { b++ })
	g.avisar("a")
	if a != 1 || b != 0 {
		t.Fatalf("al llegar «a» se repintan a=%d y b=%d; toca 1 y 0", a, b)
	}
	g.Olvidar()
	g.avisar("a")
	if a != 1 {
		t.Error("tras Olvidar se sigue repintando lo de la pantalla vieja")
	}
}
