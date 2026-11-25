// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"bytes"
	"encoding/binary"
	"image"
	"math"
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// # La caché de sombras del proceso
//
// Cada `Sombra` recordaba solo su última imagen, y las pantallas rehacen el árbol entero
// en cada cambio: cada tarjeta de Notes, cada hoja, cada botón flotante y cada panel
// nuevo volvía a desenfocar en la CPU la MISMA sombra que el árbol anterior. Medido el
// 2026-11-24 con el driver de pruebas: repintar Notes con cien notas eran 480 ms y 289 MB,
// casi todo desenfoque.
//
// La imagen es una función pura de (capas, radio, ancho, alto, recorte del interior) —el
// margen sale de las capas—, así que se guarda por esa clave y se comparte. Las imágenes
// guardadas NO se tocan nunca después de pintadas: varias `Sombra` las llevan a la vez.
//
// La caché es un LRU con un presupuesto de BYTES y no de entradas: una sombra de diálogo
// pesa cien veces lo que la de una tarjeta, y contar entradas dejaría crecer la memoria
// sin cota con las grandes.
//
// # Nueve partes
//
// Una sombra grande sale de la de un panel pequeño con las mismas capas: las esquinas
// van tal cual y el centro de cada borde se repite, porque el desenfoque de un borde recto
// es el mismo a lo largo de todo él. Así lo que cuesta pintar ya no depende del tamaño:
// la plantilla es del tamaño de las esquinas, y el resto es copiar memoria.
//
// No es una aproximación: el resultado es el MISMO, byte a byte, que pintar la sombra
// entera (lo vigila TestLaSombraDeNuevePartesEsLaMismaQueLaEntera). Lo es porque el
// desenfoque de caja suma en orden y, donde la fila es constante, suma y resta el mismo
// número —cero exacto—, y porque las cuentas de posición son exactas cuando las medidas
// son múltiplos de 1/64. Con una medida que no lo es (un ancho de 1020/7), o si la
// plantilla no sale constante donde tiene que estirarse, se pinta entera.

// presupuestoSombras son los bytes que la caché puede retener: unas doscientas sombras
// de tarjeta o una docena de diálogos de escritorio. Las que siguen en pantalla las
// retiene además su `Sombra`, así que esto acota solo lo que se guarda para la próxima.
const presupuestoSombras = 32 << 20

// sombras es la caché del proceso.
var sombras = nuevaCacheSombras(presupuestoSombras)

// cacheSombras es un LRU de imágenes de sombra con presupuesto de bytes. Es segura entre
// gorutinas: Ajustar corre en el hilo de la interfaz, pero no solo ahí.
type cacheSombras struct {
	mu          sync.Mutex
	presupuesto int
	usados      int
	// raiz es el centinela de la lista: raiz.sig es la más reciente y raiz.ant la más
	// vieja, la primera que sale.
	raiz     entradaSombra
	entradas map[string]*entradaSombra
	// aciertos y fallos son para los guardianes y para medir.
	aciertos, fallos int
}

type entradaSombra struct {
	clave    string
	img      *image.NRGBA
	ant, sig *entradaSombra
}

func nuevaCacheSombras(presupuesto int) *cacheSombras {
	c := &cacheSombras{presupuesto: presupuesto, entradas: map[string]*entradaSombra{}}
	c.raiz.ant, c.raiz.sig = &c.raiz, &c.raiz
	return c
}

func bytesSombra(img *image.NRGBA) int { return len(img.Pix) }

// quitar saca una entrada de la lista, sin tocar el mapa.
func (c *cacheSombras) quitar(e *entradaSombra) {
	e.ant.sig, e.sig.ant = e.sig, e.ant
	e.ant, e.sig = nil, nil
}

// delante pone la primera una entrada que no está en la lista.
func (c *cacheSombras) delante(e *entradaSombra) {
	e.ant, e.sig = &c.raiz, c.raiz.sig
	c.raiz.sig.ant = e
	c.raiz.sig = e
}

// buscar devuelve la imagen de esa clave, o nil, y la pone la primera del LRU.
func (c *cacheSombras) buscar(clave []byte) *image.NRGBA {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entradas[string(clave)]
	if !ok {
		c.fallos++
		return nil
	}
	c.aciertos++
	c.quitar(e)
	c.delante(e)
	return e.img
}

// guardar mete una imagen y saca las más viejas hasta caber en el presupuesto. Una
// imagen que sola ya no cabe no se guarda: echaría todo lo demás para nada.
func (c *cacheSombras) guardar(clave []byte, img *image.NRGBA) {
	n := bytesSombra(img)
	c.mu.Lock()
	defer c.mu.Unlock()
	if n > c.presupuesto {
		return
	}
	if e, ok := c.entradas[string(clave)]; ok {
		// Dos gorutinas la pintaron a la vez: vale la que ya estaba.
		c.quitar(e)
		c.delante(e)
		return
	}
	e := &entradaSombra{clave: string(clave), img: img}
	c.entradas[e.clave] = e
	c.delante(e)
	c.usados += n
	for c.usados > c.presupuesto {
		vieja := c.raiz.ant
		c.quitar(vieja)
		delete(c.entradas, vieja.clave)
		c.usados -= bytesSombra(vieja.img)
	}
}

// vaciar la deja como recién creada.
func (c *cacheSombras) vaciar() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.raiz.ant, c.raiz.sig = &c.raiz, &c.raiz
	clear(c.entradas)
	c.usados, c.aciertos, c.fallos = 0, 0, 0
}

// claveSombra escribe en `buf` lo que define la imagen. El número de capas no hace falta:
// lo dice la longitud, porque cada capa ocupa lo mismo.
func claveSombra(buf []byte, capas []tema.CapaSombra, radio float32, tam fyne.Size, sinInterior bool) []byte {
	f := func(v float32) { buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(v)) }
	for _, c := range capas {
		f(c.DesplX)
		f(c.DesplY)
		f(c.Desenfoque)
		buf = append(buf, c.Color.R, c.Color.G, c.Color.B, c.Color.A)
	}
	f(radio)
	f(tam.Width)
	f(tam.Height)
	if sinInterior {
		return append(buf, 1)
	}
	return append(buf, 0)
}

// imagenSombra es la imagen de la sombra, de la caché o recién pintada (y guardada).
func imagenSombra(capas []tema.CapaSombra, radio float32, tam fyne.Size, sinInterior bool) *image.NRGBA {
	var buf [128]byte
	clave := claveSombra(buf[:0], capas, radio, tam, sinInterior)
	if img := sombras.buscar(clave); img != nil {
		return img
	}
	img := pintarONueve(capas, radio, tam, sinInterior)
	sombras.guardar(clave, img)
	return img
}

// pintarONueve saca la sombra estirando la de una plantilla cuando se puede, y si no la
// pinta entera.
func pintarONueve(capas []tema.CapaSombra, radio float32, tam fyne.Size, sinInterior bool) *image.NRGBA {
	if pl, ok := tamPlantilla(capas, radio, tam); ok {
		// La plantilla también pasa por la caché: la comparten todos los tamaños.
		plantilla := imagenSombra(capas, radio, pl, sinInterior)
		margen := margenSombra(capas)
		w := int(math.Ceil(float64(tam.Width + 2*margen)))
		h := int(math.Ceil(float64(tam.Height + 2*margen)))
		if img := estirarSombra(plantilla, w, h); img != nil {
			return img
		}
	}
	return pintarSombra(capas, radio, tam, sinInterior)
}

// diadica dice si una medida es múltiplo de 1/64 y cabe holgada en un float32: con esas,
// las cuentas de posición de planoRedondeado son exactas y la plantilla da los mismos
// bytes que la sombra entera.
func diadica(v float32) bool {
	x := float64(v) * 64
	return math.Abs(x) < 1<<19 && x == math.Trunc(x)
}

// radiosCajas es lo que el desenfoque de σ arrastra un valor hacia dentro: la suma de los
// radios de sus tres cajas.
func radiosCajas(sigma float64) int {
	if sigma <= 0 {
		return 0
	}
	n := 0
	for _, c := range cajasPara(sigma, 3) {
		n += (c - 1) / 2
	}
	return n
}

// tamPlantilla es el panel más pequeño cuya sombra, estirada, da la de `tam`: con el
// MISMO resto fraccionario en cada eje (así el borde cae igual en el píxel) y lo bastante
// grande para que, tras las tres cajas, quede en medio una columna y una fila que no ve
// ninguna esquina. Devuelve false si no se puede o no vale la pena.
func tamPlantilla(capas []tema.CapaSombra, radio float32, tam fyne.Size) (fyne.Size, bool) {
	if !diadica(tam.Width) || !diadica(tam.Height) || !diadica(radio) {
		return tam, false
	}
	var alcX, alcY float32
	for _, c := range capas {
		if !diadica(c.DesplX) || !diadica(c.DesplY) {
			return tam, false
		}
		r := float32(radiosCajas(float64(c.Desviacion())))
		alcX = max(alcX, r+abs(c.DesplX))
		alcY = max(alcY, r+abs(c.DesplY))
	}
	// El radio que de verdad se pinta (planoRedondeado lo recorta a medio lado). La
	// plantilla mide al menos dos radios, así que recorta igual.
	radio = min(radio, min(tam.Width, tam.Height)/2)
	techo := func(v float32) float32 { return float32(math.Ceil(float64(v))) }
	minX := 2*techo(radio) + 2*techo(alcX) + 8
	minY := 2*techo(radio) + 2*techo(alcY) + 8
	pl := tam
	// Por debajo de un par de píxeles de más, estirar no ahorra nada.
	if tam.Width >= minX+2 {
		pl.Width = tam.Width - float32(math.Floor(float64(tam.Width-minX)))
	}
	if tam.Height >= minY+2 {
		pl.Height = tam.Height - float32(math.Floor(float64(tam.Height-minY)))
	}
	return pl, pl != tam
}

// estirarSombra lleva la plantilla a w×h: corta por su columna y su fila del medio,
// repite cada una lo que haga falta y pega las esquinas y los bordes tal cual. Si esa
// columna o esa fila no son constantes —no debería pasar, pero de eso depende que el
// resultado sea exacto— devuelve nil y quien llama la pinta entera.
func estirarSombra(pl *image.NRGBA, w, h int) *image.NRGBA {
	wt, ht := pl.Rect.Dx(), pl.Rect.Dy()
	n, m := w-wt, h-ht
	if n < 0 || m < 0 || wt < 3 || ht < 3 {
		return nil
	}
	sx, sy := wt/2, ht/2
	if !columnaConstante(pl, sx) || !filaConstante(pl, sy) {
		return nil
	}
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	estirarFila := func(dst, src []byte) {
		copy(dst, src[:4*sx])
		medio := dst[4*sx : 4*(sx+n)]
		if n > 0 {
			// El píxel del medio, duplicando lo copiado hasta llenar el hueco.
			copy(medio, src[4*sx:4*sx+4])
			for hecho := 4; hecho < len(medio); hecho *= 2 {
				copy(medio[hecho:], medio[:hecho])
			}
		}
		copy(dst[4*(sx+n):], src[4*sx:])
	}
	filaDe := func(img *image.NRGBA, y int) []byte {
		return img.Pix[y*img.Stride : y*img.Stride+4*img.Rect.Dx()]
	}
	for y := range sy {
		estirarFila(filaDe(out, y), filaDe(pl, y))
	}
	medio := filaDe(out, sy)
	estirarFila(medio, filaDe(pl, sy))
	for y := sy + 1; y < sy+m; y++ {
		copy(filaDe(out, y), medio)
	}
	for y := sy + m; y < h; y++ {
		estirarFila(filaDe(out, y), filaDe(pl, y-m))
	}
	return out
}

// columnaConstante dice si la columna x es igual que sus dos vecinas en todas las filas.
func columnaConstante(img *image.NRGBA, x int) bool {
	for y := range img.Rect.Dy() {
		p := img.Pix[y*img.Stride+4*(x-1):]
		if !bytes.Equal(p[0:4], p[4:8]) || !bytes.Equal(p[4:8], p[8:12]) {
			return false
		}
	}
	return true
}

// filaConstante dice si la fila y es igual que sus dos vecinas.
func filaConstante(img *image.NRGBA, y int) bool {
	ancho := 4 * img.Rect.Dx()
	a := img.Pix[(y-1)*img.Stride:][:ancho]
	b := img.Pix[y*img.Stride:][:ancho]
	c := img.Pix[(y+1)*img.Stride:][:ancho]
	return bytes.Equal(a, b) && bytes.Equal(b, c)
}
