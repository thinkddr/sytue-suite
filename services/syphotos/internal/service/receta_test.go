package service

import (
	"image"
	"image/color"
	"testing"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// Los MISMOS vectores que apps/frontend/apps/photos/src/editor/motor.test.ts:
// si un lado cambia una fórmula, el otro lo delata.

func pixel(t *testing.T, img image.Image, x, y int) (uint8, uint8, uint8) {
	t.Helper()
	c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	return c.R, c.G, c.B
}

// cerca admite ±1: la salida es de 8 bits y un valor que cae en x,5 redondea
// hacia un lado u otro según el ruido de coma flotante. Es la misma tolerancia
// que `toBeCloseTo` en el test del navegador.
func cerca(t *testing.T, que string, tengo, quiero uint8) {
	t.Helper()
	if d := int(tengo) - int(quiero); d < -1 || d > 1 {
		t.Fatalf("%s: %d, quería %d (±1)", que, tengo, quiero)
	}
}

func uno(c color.NRGBA) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	m.Set(0, 0, c)
	return m
}

func receta(pasos ...domain.Paso) domain.Receta {
	return domain.Receta{Version: domain.VersionReceta, Pasos: pasos}
}

func TestColor_exposicionDoblaLaLuz(t *testing.T) {
	out := aplicarReceta(uno(color.NRGBA{64, 64, 64, 255}),
		receta(domain.Paso{Tipo: domain.PasoExposicion, Params: map[string]float64{"valor": 1}}))
	r, _, _ := pixel(t, out, 0, 0)
	cerca(t, "exposición +1 sobre 64", r, 128)
}

func TestColor_contrasteNoMueveElGrisMedio(t *testing.T) {
	out := aplicarReceta(uno(color.NRGBA{191, 64, 128, 255}),
		receta(domain.Paso{Tipo: domain.PasoContraste, Params: map[string]float64{"valor": 1}}))
	r, g, b := pixel(t, out, 0, 0)
	cerca(t, "contraste +1, rojo", r, 255)
	cerca(t, "contraste +1, verde", g, 0)
	cerca(t, "contraste +1, azul (gris medio, no se mueve)", b, 128)
}

func TestColor_saturacionMenosUnoDejaGrisRec709(t *testing.T) {
	out := aplicarReceta(uno(color.NRGBA{230, 51, 26, 255}),
		receta(domain.Paso{Tipo: domain.PasoSaturacion, Params: map[string]float64{"valor": -1}}))
	r, g, b := pixel(t, out, 0, 0)
	quiero := a8(luminancia(230.0/255, 51.0/255, 26.0/255))
	if r != g || g != b {
		t.Fatalf("saturación −1 debía dar gris, dio (%d,%d,%d)", r, g, b)
	}
	cerca(t, "saturación −1, gris Rec.709", r, quiero)
}

func TestColor_temperaturaCalidaSubeRojoBajaAzul(t *testing.T) {
	out := aplicarReceta(uno(color.NRGBA{128, 128, 128, 255}),
		receta(domain.Paso{Tipo: domain.PasoTemperatura, Params: map[string]float64{"valor": 1}}))
	r, g, b := pixel(t, out, 0, 0)
	if r <= 128 || g != 128 || b >= 128 {
		t.Fatalf("temperatura +1 sobre gris: (%d,%d,%d)", r, g, b)
	}
}

func TestColor_seRecortaAlFinalNoEntrePasos(t *testing.T) {
	out := aplicarReceta(uno(color.NRGBA{128, 128, 128, 255}), receta(
		domain.Paso{Tipo: domain.PasoExposicion, Params: map[string]float64{"valor": 2}},
		domain.Paso{Tipo: domain.PasoExposicion, Params: map[string]float64{"valor": -2}},
	))
	r, _, _ := pixel(t, out, 0, 0)
	cerca(t, "+2 y −2 pasos vuelven al origen (¿se recortó entre pasos?)", r, 128)
}

func TestGeometria_recorteGiroYEspejoEnOrden(t *testing.T) {
	// 2×1: rojo | azul. Recortar la mitad derecha deja azul; girar 90 lo pone 1×1
	// igual; el espejo no cambia un píxel. Luego el orden: girar 90 el 2×1 entero
	// da 1×2 con rojo ARRIBA; espejar en vertical lo baja.
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	src.Set(1, 0, color.NRGBA{0, 0, 255, 255})

	out := aplicarReceta(src, receta(domain.Paso{
		Tipo:   domain.PasoRecorte,
		Params: map[string]float64{"x": 0.5, "y": 0, "ancho": 0.5, "alto": 1},
	}))
	if out.Bounds().Dx() != 1 || out.Bounds().Dy() != 1 {
		t.Fatalf("recorte de la mitad: %v", out.Bounds())
	}
	if _, _, b := pixel(t, out, 0, 0); b != 255 {
		t.Fatalf("la mitad derecha era azul")
	}

	out = aplicarReceta(src, receta(
		domain.Paso{Tipo: domain.PasoGiro, Params: map[string]float64{"grados": 90}},
		domain.Paso{Tipo: domain.PasoEspejo, Eje: "v"},
	))
	if out.Bounds().Dx() != 1 || out.Bounds().Dy() != 2 {
		t.Fatalf("giro de 90 sobre 2×1 debía dar 1×2, dio %v", out.Bounds())
	}
	if r, _, _ := pixel(t, out, 0, 1); r != 255 {
		t.Fatalf("tras girar (rojo arriba) y espejar en vertical, el rojo debía quedar abajo")
	}
}

func TestReceta_vaciaDevuelveLaMismaImagen(t *testing.T) {
	src := uno(color.NRGBA{1, 2, 3, 255})
	if aplicarReceta(src, domain.RecetaVacia()) != image.Image(src) {
		t.Fatal("la receta vacía debe devolver la imagen sin copiar")
	}
}

func TestEsPNG(t *testing.T) {
	casos := []struct {
		nombre string
		tipo   string
		esPng  bool
	}{
		{"foto.jpg", "image/jpeg", false},
		{"logo.png", "image/png", true},
		{"LOGO.PNG", "application/octet-stream", true},
		{"sin_extension", "image/png", true},
		{"sin_extension", "IMAGE/PNG", true},
		{"icono.svg", "image/svg+xml", false},
		{"playa.jpeg", "image/jpeg", false},
	}

	for _, c := range casos {
		a := domain.Activo{Nombre: c.nombre, Tipo: c.tipo}
		if got := EsPNG(a); got != c.esPng {
			t.Errorf("EsPNG(%+v) = %v; se esperaba %v", a, got, c.esPng)
		}
	}
}

func TestAplanarSobreBlanco(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.Set(0, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 0})
	src.Set(1, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 255})

	dst := aplanarSobreBlanco(src)

	r0, g0, b0, a0 := dst.At(0, 0).RGBA()
	if r0 != 0xffff || g0 != 0xffff || b0 != 0xffff || a0 != 0xffff {
		t.Errorf("píxel transparente aplanado = (%d, %d, %d, %d); se esperaba blanco", r0, g0, b0, a0)
	}

	r1, g1, b1, a1 := dst.At(1, 0).RGBA()
	if r1 != 0 || g1 != 0 || b1 != 0 || a1 != 0xffff {
		t.Errorf("píxel opaco aplanado = (%d, %d, %d, %d); se esperaba negro", r1, g1, b1, a1)
	}
}
