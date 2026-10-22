// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tag trees y lector de bits de las cabeceras de paquete (ISO 15444-1 §B.10.2).
//
// Un tag tree es cómo JPEG 2000 dice, para una rejilla de code-blocks, dos cosas
// que se repiten muchísimo: en qué capa aparece cada bloque por primera vez, y
// cuántos planos de bits suyos están vacíos. En vez de escribir un número por
// bloque, escribe un árbol donde cada nodo guarda el MÍNIMO de sus hijos, así que
// los valores parecidos —que es lo normal en una imagen— casi no ocupan.
//
// Va aparte del resto del códec porque no depende de nada: se lee y se prueba
// solo, y es donde un error se manifiesta como una imagen de ruido sin dar
// ningún fallo.
package domain

import "fmt"

// lectorBitsJPX lee bit a bit una cabecera de paquete, con el desrelleno que
// pide la norma.
//
// El relleno no es un detalle: dentro de una cabecera, un byte 0xFF va seguido
// de un bit de relleno, porque 0xFF seguido de un valor alto sería un marcador y
// partiría el codestream. Leer sin saltarlo desplaza TODO lo que viene detrás.
type lectorBitsJPX struct {
	datos []byte
	pos   int
	buf   uint32
	n     int
	// ultimoFF recuerda si el byte anterior fue 0xFF, que es lo que hace que el
	// siguiente traiga solo 7 bits.
	ultimoFF bool
}

func nuevoLectorBitsJPX(datos []byte) *lectorBitsJPX {
	return &lectorBitsJPX{datos: datos}
}

// bit devuelve el siguiente bit. Más allá del final devuelve 0, que es lo que
// hace que un paquete truncado se lea como «no hay más datos» en vez de romper.
func (l *lectorBitsJPX) bit() uint32 {
	if l.n == 0 {
		if l.pos >= len(l.datos) {
			return 0
		}
		b := l.datos[l.pos]
		l.pos++
		if l.ultimoFF {
			// Tras un 0xFF solo vienen 7 bits válidos.
			l.buf, l.n = uint32(b&0x7F), 7
			l.ultimoFF = false
		} else {
			l.buf, l.n = uint32(b), 8
			l.ultimoFF = b == 0xFF
		}
	}
	l.n--
	return (l.buf >> uint(l.n)) & 1
}

// bits lee n bits, el más significativo primero.
func (l *lectorBitsJPX) bits(n int) uint32 {
	var v uint32
	for i := 0; i < n; i++ {
		v = v<<1 | l.bit()
	}
	return v
}

// alinear termina el byte en curso, que es como acaba una cabecera de paquete.
func (l *lectorBitsJPX) alinear() {
	l.n = 0
	// Si el último byte leído fue 0xFF, el relleno se come el byte siguiente
	// entero: saltarlo aquí evita empezar los datos un byte antes de donde están.
	if l.ultimoFF {
		l.pos++
		l.ultimoFF = false
	}
}

// nodoTag es un nodo del árbol: su valor y si ya está decidido.
type nodoTag struct {
	valor int
	bajo  int  // cota inferior conocida hasta ahora
	listo bool // el valor es definitivo
}

// tagTree es el árbol de una rejilla de ancho×alto hojas.
type tagTree struct {
	ancho, alto int
	// niveles va de las HOJAS (0) a la raíz. Cada nivel es la mitad del anterior,
	// redondeando hacia arriba.
	niveles [][]nodoTag
	anchos  []int
}

// maxHojasTag acota el árbol. Una rejilla de code-blocks de un tile real no pasa
// de unos pocos miles; sin tope, una cabecera mentirosa reserva memoria a placer.
const maxHojasTag = 1 << 20

func nuevoTagTree(ancho, alto int) (*tagTree, error) {
	if ancho <= 0 || alto <= 0 {
		return nil, fmt.Errorf("jpx: tag tree de %d×%d", ancho, alto)
	}
	if ancho*alto > maxHojasTag {
		return nil, fmt.Errorf("jpx: tag tree de %d hojas, demasiado", ancho*alto)
	}
	t := &tagTree{ancho: ancho, alto: alto}
	w, h := ancho, alto
	for {
		t.niveles = append(t.niveles, make([]nodoTag, w*h))
		t.anchos = append(t.anchos, w)
		if w == 1 && h == 1 {
			break
		}
		w, h = (w+1)/2, (h+1)/2
	}
	return t, nil
}

// leer decodifica el valor de la hoja (x, y), parando en cuanto sepa que pasa de
// `tope`.
//
// Devuelve el valor y si es DEFINITIVO. Esa distinción es la clave de cómo
// funciona la cabecera de paquete: preguntar «¿este bloque aparece ya en la capa
// 3?» solo necesita saber si el valor es ≤ 3, no cuál es exactamente, y el
// codificador escribe justo los bits que hacen falta para responder eso.
func (t *tagTree) leer(l *lectorBitsJPX, x, y, tope int) (int, bool) {
	if x < 0 || y < 0 || x >= t.ancho || y >= t.alto {
		return 0, false
	}
	// De la raíz hacia la hoja: cada nodo hereda la cota de su padre.
	bajo := 0
	for nivel := len(t.niveles) - 1; nivel >= 0; nivel-- {
		w := t.anchos[nivel]
		desp := uint(nivel)
		idx := (y>>desp)*w + (x >> desp)
		n := &t.niveles[nivel][idx]
		if n.bajo < bajo {
			n.bajo = bajo
		}
		for !n.listo && n.bajo < tope {
			if l.bit() == 1 {
				// Un 1 cierra el nodo: su valor es la cota actual.
				n.valor, n.listo = n.bajo, true
			} else {
				n.bajo++
			}
		}
		if !n.listo {
			// Se agotó el tope sin decidir: lo único que se sabe es que el valor
			// es al menos `tope`.
			return n.bajo, false
		}
		bajo = n.valor
	}
	return bajo, true
}
