// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package logofit encaja el logo que sube una organización en la barra superior de
// la suite sin destrozarle la composición.
//
// EL PROBLEMA. Lo que llega es lo que había en la carpeta de marca: casi siempre con
// margen muerto alrededor —a veces solo arriba y abajo, a veces solo a un lado—, casi
// nunca con la proporción de un hueco de 28 px de alto. Reescalar a secas deja el
// logo diminuto y descentrado, porque lo que se escala es el margen. Recortar a secas
// destroza las marcas cuyo fondo ES la marca: el propio isotipo de Sytue es un
// cuadrado negro con una cuña de latón; recortarle el negro lo deja en un monograma
// suelto.
//
// LA DECISIÓN. Primero se averigua cuál es el color del MARGEN; luego cada lado se
// come su margen por separado. Eso da gratis los casos que hay que cubrir —los cuatro
// bordes, solo los horizontales, solo los verticales, uno solo, o ninguno—: no son
// cinco modos que haya que elegir y programar por separado, son lo que sale de mirar
// cada lado con el mismo criterio.
//
// El margen se decide así, y el orden importa:
//
//  1. Si el borde tiene píxeles transparentes, el margen ES la transparencia. Un
//     alfa a cero es el diseñador diciendo «aquí no hay logo»; eso no se deduce, se
//     lee. Sin esta regla el recuento del punto 2 se equivoca justo cuando la marca
//     es grande: una que ocupa cuatro quintos del alto tiene más tinta que aire en el
//     perímetro, así que el recuento la toma a ELLA por margen y recorta la marca.
//  2. Si no hay transparencia —un JPEG, que es la mitad de lo que llega—, el margen
//     es el color que domine el perímetro.
//
// Y dos guardarraíles, cada uno por un caso real:
//
//   - Si ningún color domina el perímetro, no se recorta NADA. Es lo que salva al
//     logo con degradado de borde a borde y al que va sobre fotografía, donde un
//     recorte ingenuo se come media marca creyendo que el degradado es margen.
//   - Nunca se recorta más del 25 % de un lado. Es lo que salva a la marca diseñada
//     como placa, con aire intencionado: ese aire es parte del logo, no un descuido.
//
// LO QUE NO HACE, a propósito: no intenta adivinar el fondo para volverlo
// transparente (produce halo en el borde y no hay forma de acertar siempre), ni
// clasifica la marca por contraste, ni recolorea. Cuando no está seguro, no toca:
// devolver el logo intacto es un resultado que el administrador entiende y puede
// arreglar exportándolo mejor; devolverlo recortado por la mitad, no.
//
// Sin dependencias: solo la biblioteca estándar.
package logofit

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // registra el decodificador GIF en image.Decode/DecodeConfig
	_ "image/jpeg" // idem JPEG
	"image/png"
	"net/http"
)

// Errores de entrada. Todos son culpa del fichero, no del servidor: quien llame
// tiene que devolver 4xx con un mensaje que el administrador pueda accionar.
var (
	// ErrTipo: no es un PNG, JPEG o GIF. El tipo se decide mirando los BYTES, nunca
	// la extensión ni el content-type que manda el cliente.
	ErrTipo = errors.New("logofit: formato no admitido (PNG, JPEG o GIF)")
	// ErrEnorme: dimensiones fuera de rango. Se comprueba ANTES de decodificar.
	ErrEnorme = errors.New("logofit: imagen demasiado grande")
	// ErrIlegible: los bytes dicen ser una imagen pero no se dejan decodificar.
	ErrIlegible = errors.New("logofit: imagen ilegible")
	// ErrVacio: no queda nada después de recortar (una imagen de un solo color).
	ErrVacio = errors.New("logofit: la imagen no tiene contenido visible")
	// ErrAlargado: tan apaisada o tan alta que en la barra sería ilegible.
	ErrAlargado = errors.New("logofit: proporción demasiado alargada")
)

const (
	// Techo de píxeles ANTES de decodificar. Nadie exporta una marca a más de 2000 px
	// de lado para una barra de 28, y con 4 MP el búfer decodificado se queda en unos
	// 16 MB: importa porque quien decodifica es el servicio que emite todos los
	// tokens de la plataforma. Drive admite 40 MP porque ahí sí llegan fotos.
	MaxPixeles = 4_000_000

	// Alto de salida: 4× los 28 px de la barra, para que se vea nítido en pantallas
	// de densidad doble sin guardar cuatro derivadas.
	AltoSalida = 112

	// Tope de anchura. Un wordmark muy apaisado se limita por aquí en vez de por el
	// alto, o se comería la barra entera.
	MaxAncho = 640

	// Cuánto pueden diferir dos colores para considerarlos "el mismo". El antialias
	// y el JPEG mueven el borde unos pocos niveles; exigir igualdad exacta de bytes
	// haría que el recorte no encontrara NUNCA un margen uniforme en un JPEG.
	tolerancia = 8

	// Por debajo de este alfa, el píxel es margen transparente.
	alfaVacio = 8

	// Recorte máximo por lado, en tanto por ciento de esa dimensión.
	maxRecortePct = 25

	// Relación de aspecto máxima admitida (ancho/alto o alto/ancho).
	maxAspecto = 12

	// Cuánto del perímetro tiene que ser del mismo color para llamarlo margen.
	minPerimetroPct = 60

	// Tope de colores distintos en el perímetro antes de rendirse: por encima de
	// esto no es un margen, es una fotografía o un degradado.
	maxCubos = 64
)

// Report cuenta qué se hizo, para que la interfaz lo pueda explicar y el
// administrador no tenga que adivinar por qué su logo se ve como se ve.
type Report struct {
	// Ancho y alto finales, en píxeles.
	Ancho, Alto int
	// Píxeles recortados por lado.
	Arriba, Abajo, Izquierda, Derecha int
	// ¿Se recortó algo?
	Recortado bool
	// El perímetro es opaco: la marca lleva su propio fondo (una placa). No es un
	// error —el isotipo de Sytue es exactamente eso— pero SÍ es lo que hay que
	// avisar: un PNG con canal alfa pero fondo blanco pintado no se recorta (bien),
	// contrasta de sobra contra el grafito (así que ningún aviso de contraste salta)
	// y aparece en la barra como un ladrillo blanco.
	FondoOpaco bool
	// El recorte se abandonó: el borde no tiene un margen que quitar (un degradado,
	// una fotografía). El logo se devuelve entero.
	BordeSinMargen bool
	// Se tocó el tope del 25 % en algún lado: había más margen del que se quitó.
	TopeRecorte bool
}

// Fit normaliza el logo subido y devuelve un PNG listo para servir.
//
// El PNG se re-codifica SIEMPRE, aunque la imagen ya viniera perfecta: el byte que
// se sirve no es nunca el byte que subió el usuario, y eso es lo que hace que el
// endpoint público sea seguro sin tener que confiar en el decodificador de nadie.
func Fit(raw []byte) ([]byte, Report, error) {
	var rep Report

	// 1. El tipo, por los bytes. `http.DetectContentType` mira la firma del fichero.
	switch http.DetectContentType(raw) {
	case "image/png", "image/jpeg", "image/gif":
	default:
		return nil, rep, ErrTipo
	}

	// 2. Barrera anti-bomba: dimensiones declaradas fuera de rango → fuera SIN
	// decodificar, o sea sin reservar el búfer de píxeles.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, rep, ErrIlegible
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > MaxPixeles {
		return nil, rep, ErrEnorme
	}

	// 3. Ahora sí, la imagen entera.
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, rep, ErrIlegible
	}

	// Un solo color en toda la imagen no es un logo, y hay que descartarlo ANTES de
	// recortar: si no, el recorte se lleva su 25 % por lado y devuelve un rectángulo
	// de color plano con toda la pinta de ser un resultado correcto.
	if uniforme(src, src.Bounds()) {
		return nil, rep, ErrVacio
	}

	caja := recortar(src, &rep)
	if caja.Empty() {
		return nil, rep, ErrVacio
	}
	rep.FondoOpaco = perimetroOpaco(src, caja)

	w, h := caja.Dx(), caja.Dy()
	if w > h*maxAspecto || h > w*maxAspecto {
		return nil, rep, ErrAlargado
	}

	fin := escalar(src, caja)
	rep.Ancho, rep.Alto = fin.Bounds().Dx(), fin.Bounds().Dy()

	var buf bytes.Buffer
	if err := png.Encode(&buf, fin); err != nil {
		return nil, rep, fmt.Errorf("logofit: codificar png: %w", err)
	}
	return buf.Bytes(), rep, nil
}

// recortar devuelve la caja de contenido, mirando cada lado por separado.
//
// El ancla es el color de MARGEN, que se saca de la moda del perímetro y no de las
// cuatro esquinas. La diferencia importa justo en el caso «un solo borde»: una marca
// que llega hasta el borde de arriba y deja aire abajo tiene las esquinas de arriba
// llenas de tinta y las de abajo vacías, así que un ancla por esquinas se rendiría
// —y ese es precisamente uno de los reajustes que hay que hacer—. Con la moda, la
// tinta del borde superior es minoría, el fondo gana, y entonces la fila de arriba
// no es margen (no es del color de margen) mientras la de abajo sí. Cada lado sale
// solo.
//
// Si ningún color domina el perímetro, no se recorta nada: es el degradado de borde
// a borde y el logo sobre fotografía, donde un recorte ingenuo se come media marca.
func recortar(src image.Image, rep *Report) image.Rectangle {
	b := src.Bounds()
	fondo, ok := colorDeMargen(src, b)
	if !ok {
		rep.BordeSinMargen = true
		return b
	}

	maxX := b.Dx() * maxRecortePct / 100
	maxY := b.Dy() * maxRecortePct / 100
	caja := b

	// Cada lado avanza por su cuenta mientras la fila o columna ENTERA siga siendo
	// margen. De aquí salen los casos «solo arriba», «solo los lados», «ninguno»…
	for caja.Min.Y < caja.Max.Y && caja.Min.Y-b.Min.Y < maxY && filaVacia(src, caja, caja.Min.Y, fondo) {
		caja.Min.Y++
	}
	for caja.Max.Y > caja.Min.Y && b.Max.Y-caja.Max.Y < maxY && filaVacia(src, caja, caja.Max.Y-1, fondo) {
		caja.Max.Y--
	}
	for caja.Min.X < caja.Max.X && caja.Min.X-b.Min.X < maxX && columnaVacia(src, caja, caja.Min.X, fondo) {
		caja.Min.X++
	}
	for caja.Max.X > caja.Min.X && b.Max.X-caja.Max.X < maxX && columnaVacia(src, caja, caja.Max.X-1, fondo) {
		caja.Max.X--
	}

	rep.Arriba = caja.Min.Y - b.Min.Y
	rep.Abajo = b.Max.Y - caja.Max.Y
	rep.Izquierda = caja.Min.X - b.Min.X
	rep.Derecha = b.Max.X - caja.Max.X
	rep.Recortado = rep.Arriba+rep.Abajo+rep.Izquierda+rep.Derecha > 0
	rep.TopeRecorte = rep.Arriba >= maxY || rep.Abajo >= maxY || rep.Izquierda >= maxX || rep.Derecha >= maxX
	return caja
}

// colorDeMargen: el color mayoritario del perímetro, si hay uno que domine.
//
// Se agrupa con la misma tolerancia que el recorte, así que el ruido del JPEG y el
// antialias caen en el mismo cubo. Dos cosas hacen que se rinda, y las dos quieren
// decir «este borde no es un margen»: que el perímetro tenga demasiados colores
// distintos (un degradado, una foto) o que el ganador no llegue a la mayoría
// exigida.
func colorDeMargen(src image.Image, b image.Rectangle) (color.NRGBA, bool) {
	// La transparencia manda sobre cualquier recuento: un píxel con alfa 0 es el
	// diseñador diciendo «aquí no hay logo», y eso no hay que deducirlo. Solo cuando
	// no hay canal alfa —un JPEG, que es la mitad de lo que llega— toca adivinar el
	// fondo por color. Sin esta regla el recuento se equivoca justo en el caso «un
	// solo borde»: una marca que ocupa cuatro quintos del alto tiene MÁS tinta que
	// aire en el perímetro, así que la moda elegía la tinta como margen y recortaba
	// la marca en vez del aire.
	if hayTransparencia(src, b) {
		return color.NRGBA{}, true
	}

	type cubo struct {
		c color.NRGBA
		n int
	}
	cubos := make([]cubo, 0, maxCubos)
	total := 0
	mete := func(x, y int) bool {
		c := nrgba(src, x, y)
		total++
		for i := range cubos {
			if igual(cubos[i].c, c) {
				cubos[i].n++
				return true
			}
		}
		if len(cubos) >= maxCubos {
			return false
		}
		cubos = append(cubos, cubo{c: c, n: 1})
		return true
	}
	for x := b.Min.X; x < b.Max.X; x++ {
		if !mete(x, b.Min.Y) || !mete(x, b.Max.Y-1) {
			return color.NRGBA{}, false
		}
	}
	for y := b.Min.Y + 1; y < b.Max.Y-1; y++ {
		if !mete(b.Min.X, y) || !mete(b.Max.X-1, y) {
			return color.NRGBA{}, false
		}
	}
	mejor := cubo{}
	for _, c := range cubos {
		if c.n > mejor.n {
			mejor = c
		}
	}
	if total == 0 || mejor.n*100 < total*minPerimetroPct {
		return color.NRGBA{}, false
	}
	return mejor.c, true
}

// hayTransparencia: ¿algún píxel del borde está vacío?
func hayTransparencia(src image.Image, b image.Rectangle) bool {
	for x := b.Min.X; x < b.Max.X; x++ {
		if nrgba(src, x, b.Min.Y).A <= alfaVacio || nrgba(src, x, b.Max.Y-1).A <= alfaVacio {
			return true
		}
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if nrgba(src, b.Min.X, y).A <= alfaVacio || nrgba(src, b.Max.X-1, y).A <= alfaVacio {
			return true
		}
	}
	return false
}

// uniforme: ¿toda la imagen es del mismo color? Entonces no hay logo que encajar, y
// hay que decirlo antes de recortar: si no, el recorte se lleva su 25 % por lado y
// devuelve un rectángulo de color plano que parece un resultado correcto.
func uniforme(src image.Image, b image.Rectangle) bool {
	c0 := nrgba(src, b.Min.X, b.Min.Y)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if !igual(nrgba(src, x, y), c0) {
				return false
			}
		}
	}
	return true
}

func filaVacia(src image.Image, caja image.Rectangle, y int, fondo color.NRGBA) bool {
	for x := caja.Min.X; x < caja.Max.X; x++ {
		if !igual(nrgba(src, x, y), fondo) {
			return false
		}
	}
	return true
}

func columnaVacia(src image.Image, caja image.Rectangle, x int, fondo color.NRGBA) bool {
	for y := caja.Min.Y; y < caja.Max.Y; y++ {
		if !igual(nrgba(src, x, y), fondo) {
			return false
		}
	}
	return true
}

// igual: dos píxeles cuentan como el mismo margen si los dos son casi
// transparentes, o si tienen la misma opacidad y un color parecido. Comparar el
// color de un píxel transparente no sirve de nada —lo que hay debajo del alfa 0 es
// basura arbitraria según quién exportara el fichero—, de ahí el primer caso.
func igual(a, b color.NRGBA) bool {
	if a.A <= alfaVacio && b.A <= alfaVacio {
		return true
	}
	if dif(a.A, b.A) > tolerancia {
		return false
	}
	return dif(a.R, b.R) <= tolerancia && dif(a.G, b.G) <= tolerancia && dif(a.B, b.B) <= tolerancia
}

func dif(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func nrgba(src image.Image, x, y int) color.NRGBA {
	c, _ := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
	return c
}

// perimetroOpaco: ¿el borde de la caja es sólido? Con un solo píxel translúcido no
// basta para decir que no —el antialias de una esquina redondeada deja unos
// cuantos—, así que se pide el 98 %.
func perimetroOpaco(src image.Image, caja image.Rectangle) bool {
	total, opacos := 0, 0
	mira := func(x, y int) {
		total++
		if nrgba(src, x, y).A == 255 {
			opacos++
		}
	}
	for x := caja.Min.X; x < caja.Max.X; x++ {
		mira(x, caja.Min.Y)
		mira(x, caja.Max.Y-1)
	}
	for y := caja.Min.Y + 1; y < caja.Max.Y-1; y++ {
		mira(caja.Min.X, y)
		mira(caja.Max.X-1, y)
	}
	return total > 0 && opacos*100 >= total*98
}

// escalar lleva la caja al alto de salida con un filtro de CAJA (media de área):
// cada píxel de destino es la media de la región de origen que cubre. Es stdlib
// pura y para reducir da mucha mejor calidad que el vecino más cercano, sin
// aliasing. Nunca amplía: un logo pequeño estirado se ve peor que pequeño.
//
// ponytail: media de caja, no Lanczos — de sobra para 112 px de alto. Si algún día
// entra golang.org/x/image en el módulo, se cambia aquí y en Drive.
func escalar(src image.Image, caja image.Rectangle) *image.NRGBA {
	sw, sh := caja.Dx(), caja.Dy()
	dw, dh := destino(sw, sh)
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for dy := 0; dy < dh; dy++ {
		sy0 := caja.Min.Y + dy*sh/dh
		sy1 := caja.Min.Y + (dy+1)*sh/dh
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for dx := 0; dx < dw; dx++ {
			sx0 := caja.Min.X + dx*sw/dw
			sx1 := caja.Min.X + (dx+1)*sw/dw
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, a, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					c := nrgba(src, sx, sy)
					r += uint64(c.R)
					g += uint64(c.G)
					bl += uint64(c.B)
					a += uint64(c.A)
					n++
				}
			}
			if n == 0 {
				continue
			}
			dst.SetNRGBA(dx, dy, color.NRGBA{
				R: media8(r, n), G: media8(g, n), B: media8(bl, n), A: media8(a, n),
			})
		}
	}
	return dst
}

// media8 promedia un acumulador de muestras de 8 bits. La suma va en uint64
// porque acumular cientos de píxeles desbordaría un uint8, pero el COCIENTE cabe
// siempre en 8 bits: ninguna media puede superar al mayor de sus sumandos, y
// todos venían de un canal de 8 bits.
//
// El `min` deja eso garantizado por construcción en vez de por un razonamiento
// sobre quién llena el acumulador, que es lo único que separa esto de un
// desbordamiento silencioso el día que alguien cambie de dónde salen las
// muestras. Está en una función y no repetido cuatro veces en la expresión para
// que la justificación viva en un solo sitio.
func media8(suma, n uint64) uint8 {
	//nolint:gosec // G115: el min de la línea acota el valor a 255; no hay conversión que pueda desbordar.
	return uint8(min(suma/n, 255))
}

// destino: alto fijo, ancho por proporción, y el tope de anchura manda sobre el alto
// para que un wordmark muy apaisado no ocupe la barra entera. Nunca amplía.
func destino(sw, sh int) (int, int) {
	if sh <= AltoSalida && sw <= MaxAncho {
		return sw, sh
	}
	dh := AltoSalida
	dw := sw * dh / sh
	if dw > MaxAncho {
		dw = MaxAncho
		dh = sh * dw / sw
	}
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	return dw, dh
}
