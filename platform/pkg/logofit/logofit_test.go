// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package logofit

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// lienzo pinta una imagen de prueba: fondo, y dentro de `tinta` una marca con FORMA
// —un aspa gruesa—, no un rectángulo relleno.
//
// La forma no es un capricho del test: un rectángulo de color plano es una imagen
// degenerada (no tiene contenido que distinguir del fondo) y probar contra ella daba
// resultados que no se parecen en nada a los de un logo real. La marca toca los
// cuatro lados de su caja, así que el recorte tiene un objetivo bien definido, y deja
// fondo entre los brazos, que es lo que tiene cualquier marca de verdad.
func lienzo(w, h int, fondo color.NRGBA, tinta image.Rectangle) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, fondo)
		}
	}
	negro := color.NRGBA{R: 20, G: 20, B: 20, A: 255}
	tw, th := tinta.Dx(), tinta.Dy()
	if tw <= 0 || th <= 0 {
		return img
	}
	grosor := tw/6 + 1
	for y := tinta.Min.Y; y < tinta.Max.Y; y++ {
		// Dos diagonales: de esquina a esquina de la caja de tinta.
		t := (y - tinta.Min.Y) * tw / th
		for d := -grosor; d <= grosor; d++ {
			for _, x := range []int{tinta.Min.X + t + d, tinta.Max.X - 1 - t + d} {
				if x >= tinta.Min.X && x < tinta.Max.X {
					img.SetNRGBA(x, y, negro)
				}
			}
		}
	}
	return img
}

func aPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatalf("codificar png: %v", err)
	}
	return b.Bytes()
}

func aJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("codificar jpeg: %v", err)
	}
	return b.Bytes()
}

var (
	transparente = color.NRGBA{}
	blanco       = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
)

// El margen se quita lado a lado, que es de donde salen —sin programarlos por
// separado— los cuatro casos del encargo: los cuatro bordes, solo los
// horizontales, solo los verticales y uno solo.
func TestRecortePorLado(t *testing.T) {
	casos := []struct {
		nombre             string
		w, h               int
		fondo              color.NRGBA
		tinta              image.Rectangle
		arr, aba, izq, der int
	}{
		{
			"margen transparente en los cuatro lados", 100, 100, transparente,
			image.Rect(10, 10, 90, 90), 10, 10, 10, 10,
		},
		{
			"solo margen horizontal (arriba y abajo)", 100, 100, transparente,
			image.Rect(0, 15, 100, 85), 15, 15, 0, 0,
		},
		{
			"solo margen vertical (a los lados)", 100, 100, transparente,
			image.Rect(12, 0, 88, 100), 0, 0, 12, 12,
		},
		{
			"un solo borde", 100, 100, transparente,
			image.Rect(0, 0, 100, 80), 0, 20, 0, 0,
		},
		{
			"margen asimétrico", 100, 100, transparente,
			image.Rect(5, 20, 100, 90), 20, 10, 5, 0,
		},
		{
			"fondo blanco PINTADO, no transparente", 100, 100, blanco,
			image.Rect(10, 10, 90, 90), 10, 10, 10, 10,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, rep, err := Fit(aPNG(t, lienzo(c.w, c.h, c.fondo, c.tinta)))
			if err != nil {
				t.Fatalf("Fit: %v", err)
			}
			if rep.Arriba != c.arr || rep.Abajo != c.aba || rep.Izquierda != c.izq || rep.Derecha != c.der {
				t.Errorf("recorte = arriba %d, abajo %d, izq %d, der %d; se esperaba %d/%d/%d/%d",
					rep.Arriba, rep.Abajo, rep.Izquierda, rep.Derecha, c.arr, c.aba, c.izq, c.der)
			}
		})
	}
}

// Lo que NO se debe tocar. Cada uno de estos es una marca que un recorte ingenuo
// destroza, y son la razón de que existan los dos guardarraíles.
func TestLoQueNoSeToca(t *testing.T) {
	t.Run("logo ya ajustado: no se recorta nada", func(t *testing.T) {
		_, rep, err := Fit(aPNG(t, lienzo(80, 80, transparente, image.Rect(0, 0, 80, 80))))
		if err != nil {
			t.Fatalf("Fit: %v", err)
		}
		if rep.Recortado {
			t.Errorf("recortó un logo que ya venía ajustado: %+v", rep)
		}
	})

	t.Run("degradado de borde a borde: ningún color domina, no se recorta", func(t *testing.T) {
		img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
		for y := 0; y < 100; y++ {
			for x := 0; x < 100; x++ {
				img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 2), G: 40, B: uint8(y * 2), A: 255})
			}
		}
		_, rep, err := Fit(aPNG(t, img))
		if err != nil {
			t.Fatalf("Fit: %v", err)
		}
		if rep.Recortado || !rep.BordeSinMargen {
			t.Errorf("un degradado no tiene margen que quitar: %+v", rep)
		}
	})

	t.Run("placa con aire intencionado: el recorte se para en el 25%%", func(t *testing.T) {
		// Marca diminuta en el centro: el margen es el 40% por lado, pero ese aire
		// es parte del diseño y quitarlo entero cambiaría la marca.
		_, rep, err := Fit(aPNG(t, lienzo(100, 100, transparente, image.Rect(40, 40, 60, 60))))
		if err != nil {
			t.Fatalf("Fit: %v", err)
		}
		if rep.Arriba > 25 || rep.Izquierda > 25 {
			t.Errorf("se pasó del tope del 25%%: %+v", rep)
		}
		if !rep.TopeRecorte {
			t.Error("tocó el tope pero no lo dijo en el informe")
		}
	})

	t.Run("marco decorativo: la esquina es el marco, así que se respeta", func(t *testing.T) {
		// Borde de color distinto del interior: las esquinas coinciden entre sí, pero
		// la primera fila NO es uniforme (lleva marco y relleno), así que no avanza.
		img := lienzo(100, 100, color.NRGBA{R: 180, G: 140, B: 80, A: 255}, image.Rect(6, 6, 94, 94))
		_, rep, err := Fit(aPNG(t, img))
		if err != nil {
			t.Fatalf("Fit: %v", err)
		}
		if rep.Arriba > 6 || rep.Izquierda > 6 {
			t.Errorf("se comió el marco: %+v", rep)
		}
	})
}

// El caso que ninguna heurística de recorte detecta y el más frecuente: un PNG con
// canal alfa pero fondo blanco pintado. No hay nada que recortar y contrasta de
// sobra, así que ningún aviso salta; en la barra es un ladrillo blanco.
func TestAvisaFondoOpaco(t *testing.T) {
	_, rep, err := Fit(aPNG(t, lienzo(100, 100, blanco, image.Rect(10, 10, 90, 90))))
	if err != nil {
		t.Fatalf("Fit: %v", err)
	}
	if !rep.FondoOpaco {
		t.Error("un logo con placa blanca tiene que avisar de que trae su propio fondo")
	}
	_, rep, err = Fit(aPNG(t, lienzo(100, 100, transparente, image.Rect(10, 10, 90, 90))))
	if err != nil {
		t.Fatalf("Fit: %v", err)
	}
	if rep.FondoOpaco {
		t.Error("falso positivo: el margen era transparente")
	}
}

// Un JPEG no tiene canal alfa: la regla «recorta lo que tenga alfa 0» no haría nada
// con él, que es el formato más habitual en una carpeta de marca.
func TestJPEGSeRecortaAunqueNoTengaAlfa(t *testing.T) {
	_, rep, err := Fit(aJPEG(t, lienzo(200, 200, blanco, image.Rect(30, 30, 170, 170))))
	if err != nil {
		t.Fatalf("Fit: %v", err)
	}
	if !rep.Recortado {
		t.Fatalf("no recortó el margen blanco de un JPEG: %+v", rep)
	}
	// El JPEG mueve los bordes unos niveles; con tolerancia 0 no se recortaría nada.
	if rep.Arriba < 25 || rep.Arriba > 30 {
		t.Errorf("recorte superior = %d, se esperaba ~30 (con el ruido del JPEG)", rep.Arriba)
	}
}

func TestEscalado(t *testing.T) {
	t.Run("se ajusta al alto de salida", func(t *testing.T) {
		_, rep, err := Fit(aPNG(t, lienzo(400, 400, transparente, image.Rect(0, 0, 400, 400))))
		if err != nil {
			t.Fatalf("Fit: %v", err)
		}
		if rep.Alto != AltoSalida {
			t.Errorf("alto = %d, se esperaba %d", rep.Alto, AltoSalida)
		}
	})

	t.Run("nunca amplía", func(t *testing.T) {
		_, rep, err := Fit(aPNG(t, lienzo(40, 30, transparente, image.Rect(0, 0, 40, 30))))
		if err != nil {
			t.Fatalf("Fit: %v", err)
		}
		if rep.Alto != 30 || rep.Ancho != 40 {
			t.Errorf("amplió un logo pequeño: %dx%d", rep.Ancho, rep.Alto)
		}
	})

	t.Run("un wordmark apaisado se limita por el ancho", func(t *testing.T) {
		_, rep, err := Fit(aPNG(t, lienzo(4000, 400, transparente, image.Rect(0, 0, 4000, 400))))
		if err != nil {
			t.Fatalf("Fit: %v", err)
		}
		if rep.Ancho > MaxAncho {
			t.Errorf("ancho = %d, por encima del tope %d", rep.Ancho, MaxAncho)
		}
	})
}

func TestRechazos(t *testing.T) {
	t.Run("no es una imagen", func(t *testing.T) {
		if _, _, err := Fit([]byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>")); !errors.Is(err, ErrTipo) {
			t.Errorf("err = %v, se esperaba ErrTipo", err)
		}
	})

	t.Run("bomba de descompresión: se rechaza SIN decodificar", func(t *testing.T) {
		// Cabecera PNG que declara 30000x30000 (900 MP). Son ~70 bytes: si la guarda
		// no estuviera antes del Decode, aquí se reservarían gigabytes.
		var b bytes.Buffer
		if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 4, 4))); err != nil {
			t.Fatalf("png: %v", err)
		}
		raw := b.Bytes()
		// Reescribe ancho y alto en el IHDR (bytes 16..24). El CRC queda mal, pero
		// DecodeConfig lee las dimensiones antes de comprobarlo.
		for i, v := range []byte{0, 0, 117, 48, 0, 0, 117, 48} {
			raw[16+i] = v
		}
		if _, _, err := Fit(raw); !errors.Is(err, ErrEnorme) && !errors.Is(err, ErrIlegible) {
			t.Errorf("err = %v, se esperaba ErrEnorme o ErrIlegible", err)
		}
	})

	t.Run("imagen de un solo color: no hay logo", func(t *testing.T) {
		if _, _, err := Fit(aPNG(t, lienzo(50, 50, blanco, image.Rect(0, 0, 0, 0)))); !errors.Is(err, ErrVacio) {
			t.Errorf("err = %v, se esperaba ErrVacio", err)
		}
	})

	t.Run("proporción imposible", func(t *testing.T) {
		if _, _, err := Fit(aPNG(t, lienzo(1000, 20, transparente, image.Rect(0, 0, 1000, 20)))); !errors.Is(err, ErrAlargado) {
			t.Errorf("err = %v, se esperaba ErrAlargado", err)
		}
	})
}

// La salida SIEMPRE se re-codifica: el byte que se sirve no es nunca el byte que
// subió el usuario. Es lo que permite servir el logo sin confiar en el fichero.
func TestLaSalidaEsSiemprePNGNuevo(t *testing.T) {
	entrada := aJPEG(t, lienzo(120, 120, blanco, image.Rect(20, 20, 100, 100)))
	salida, _, err := Fit(entrada)
	if err != nil {
		t.Fatalf("Fit: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(salida)); err != nil {
		t.Errorf("la salida no es un PNG válido: %v", err)
	}
	if bytes.Equal(entrada, salida) {
		t.Error("se devolvió el fichero de entrada tal cual")
	}
}

// La marca ocupa cuatro quintos del alto en un bloque sólido y llega al borde de
// arriba: hay MÁS tinta que aire en el perímetro. Contando colores, el ganador es la
// tinta, así que un recuento a secas la toma por margen y recorta la marca en vez del
// aire — justo al revés. Lo que lo salva es que el alfa del borde inferior dice
// dónde NO hay logo, y eso no hay que deducirlo.
func TestMarcaQueDominaElPerimetro(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 20, G: 20, B: 20, A: 255})
		}
	}
	_, rep, err := Fit(aPNG(t, img))
	if err != nil {
		t.Fatalf("Fit: %v", err)
	}
	if rep.Arriba != 0 {
		t.Errorf("recortó %d px de la MARCA por arriba; no debía tocarla", rep.Arriba)
	}
	if rep.Abajo != 20 {
		t.Errorf("recorte inferior = %d, se esperaba 20 (el aire de debajo)", rep.Abajo)
	}
}
