// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math/rand/v2"
	"testing"

	"fyne.io/fyne/v2"
	xdraw "golang.org/x/image/draw"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Lo que se mide aquí, y por qué no vale lo que ya está medido de las listas
//
// La virtualización de `widget.List` está medida y es plana: 0,6 ms con 50 filas y con
// 5.000. Una REJILLA de miniaturas no es una lista, y por dos motivos:
//
//  1. Cada tesela es un widget con nueve primitivas y un raster, no una fila reciclada.
//  2. El raster no es gratis: la primera vez que se pinta a un tamaño hay que decodificar
//     el JPEG y remuestrear con Catmull-Rom.
//
// Estos bancos separan las dos cosas, que es lo único que deja decir a partir de
// cuántas fotos deja de ir fina.

// fotoDePrueba es una imagen de ruido de 512 px, que es el tamaño que sirve el servicio
// para la variante «miniatura». Ruido y no un color plano: un JPEG de color plano pesa
// diez veces menos y se decodifica más rápido, así que mediría otra cosa.
func fotoDePrueba() []byte {
	m := image.NewRGBA(image.Rect(0, 0, 512, 512))
	r := rand.New(rand.NewPCG(1, 2))
	for y := range 512 {
		for x := range 512 {
			m.Set(x, y, color.RGBA{
				R: uint8(r.UintN(256)), G: uint8(r.UintN(256)), B: uint8(r.UintN(256)), A: 255,
			})
		}
	}
	return jpegDe(m)
}

// BenchmarkTeselaPrimeraVez es el coste de una tesela que todavía no está en caché:
// decodificar el JPEG de 512 px, recortar al cuadrado y remuestrear a 402×402 (una
// tesela de 201 px de CSS a ×2), con las esquinas.
func BenchmarkTeselaPrimeraVez(b *testing.B) {
	datos := fotoDePrueba()
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		f := NuevaFototeca()
		f.Poner("x", datos)
		if f.TeselaYa("x", 402, 402, 8) == nil {
			b.Fatal("no salió la tesela")
		}
	}
}

// BenchmarkTeselaEnCache es el coste de la MISMA tesela una vez guardada, que es lo que
// paga cada repintado. Tiene que ser un acierto de mapa y nada más.
func BenchmarkTeselaEnCache(b *testing.B) {
	f := NuevaFototeca()
	f.Poner("x", fotoDePrueba())
	f.TeselaYa("x", 402, 402, 8)
	b.ReportAllocs()
	for b.Loop() {
		if f.Tesela("x", 402, 402, 8) == nil {
			b.Fatal("no salió la tesela")
		}
	}
}

// BenchmarkRejilla es el reparto de la cuadrícula con n teselas: lo que cuesta un
// Layout, sin pintar nada. Es la cuenta que corre en cada cambio de tamaño y en cada
// página nueva de la línea de tiempo.
func BenchmarkRejilla(b *testing.B) {
	for _, n := range []int{24, 100, 500, 2000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			objs := make([]fyne.CanvasObject, n)
			fotos := make([]api.Photo, n)
			teca := NuevaFototeca()
			for i := range objs {
				// Apaisadas y verticales alternas: la cuenta de las filas, con mezcla.
				fotos[i] = api.Photo{ID: fmt.Sprint(i), Width: 4000 - 1000*(i%2), Height: 3000 + 1000*(i%2)}
				objs[i] = NuevaMiniatura(fotos[i], teca)
			}
			r := filasDe(Vista{Ventana: 1280}, fotos)
			b.ReportAllocs()
			for b.Loop() {
				r.Layout(objs, fyne.NewSize(1216, r.AltoCon(objs, 1216)))
			}
		})
	}
}

// BenchmarkConstruirTeselas es lo que cuesta CREAR las teselas de una página de la
// línea de tiempo (la web pide de cien en cien). Es el coste que se paga al desplazarse
// y que no se recupera con caché.
func BenchmarkConstruirTeselas(b *testing.B) {
	teca := NuevaFototeca()
	b.ReportAllocs()
	for b.Loop() {
		for i := range 100 {
			m := NuevaMiniatura(api.Photo{ID: fmt.Sprint(i), Name: "f.jpg"}, teca)
			m.Rotulo = "Abrir f.jpg"
			_ = m.MinSize()
		}
	}
}

// BenchmarkPorEtapas parte el coste de una tesela en sus tres trozos. Es lo que dijo
// dónde estaba el cuello de verdad: no en la rejilla ni en construir los widgets, sino
// en decodificar y remuestrear, y encima en el hilo que pinta.
//
// Medido en el piloto (Ryzen 5600X), fuente de 512 px de RUIDO —el peor caso: un JPEG
// de ruido a calidad 92 no comprime y se decodifica más despacio que una foto real—:
//
//	decodificar        5,17 ms
//	CatmullRom         8,67 ms   6,7 MB por llamada
//	BiLinear           5,04 ms   6,7 MB
//	ApproxBiLinear     3,11 ms   0 B
//	esquinas           0,005 ms
//
// De ahí salen las dos decisiones de `fototeca.go`: preparar en segundo plano, y elegir
// el núcleo por la razón de reducción.
func BenchmarkPorEtapas(b *testing.B) {
	datos := fotoDePrueba()
	b.Run("decodificar", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, _, err := image.Decode(bytes.NewReader(datos)); err != nil {
				b.Fatal(err)
			}
		}
	})
	src, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		b.Fatal(err)
	}
	rec := recorteCover(src.Bounds(), 402, 402)
	for _, k := range []struct {
		nombre string
		cual   xdraw.Interpolator
	}{
		{"CatmullRom", xdraw.CatmullRom},
		{"BiLinear", xdraw.BiLinear},
		{"ApproxBiLinear", xdraw.ApproxBiLinear},
	} {
		b.Run("escalar/"+k.nombre, func(b *testing.B) {
			dst := image.NewRGBA(image.Rect(0, 0, 402, 402))
			b.ReportAllocs()
			for b.Loop() {
				k.cual.Scale(dst, dst.Bounds(), src, rec, draw.Src, nil)
			}
		})
	}
	b.Run("esquinas", func(b *testing.B) {
		dst := image.NewRGBA(image.Rect(0, 0, 402, 402))
		b.ReportAllocs()
		for b.Loop() {
			redondear(dst, 8)
		}
	})
}

// TestElEscaladorSeReutilizaPorTamano: el mismo par de tamaños tiene que devolver el
// MISMO `Scaler`. Sin eso, cada tesela reconstruye los pesos del núcleo y deja 6,7 MB
// de basura — 160 MB por pantallazo de veinticuatro.
func TestElEscaladorSeReutilizaPorTamano(t *testing.T) {
	r := image.Rect(0, 0, 512, 512)
	a, b := escalador(r, 402, 402), escalador(r, 402, 402)
	if a != b {
		t.Error("dos llamadas con el mismo tamaño dan escaladores distintos: no se está reutilizando")
	}
	if c := escalador(r, 201, 201); c == a {
		t.Error("dos tamaños distintos comparten escalador: los pesos del núcleo no valdrían")
	}
}
