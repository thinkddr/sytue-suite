// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del tag tree y del lector de bits de las cabeceras de paquete.
//
// Estas dos piezas no dan error cuando fallan: dan una IMAGEN DE RUIDO. Un bit
// de relleno que no se salta desplaza todo lo que viene detrás, y un tag tree
// mal recorrido devuelve números plausibles. Por eso se prueban con los bits
// escritos a mano, y la codificación de cada caso va DERIVADA en el comentario:
// una secuencia copiada de memoria es justo lo que no se puede comprobar.
package domain

import "testing"

// bitsDe convierte «10110» en los bytes que leería el decodificador, rellenando
// con ceros hasta completar el último byte.
func bitsDe(t *testing.T, s string) []byte {
	t.Helper()
	var out []byte
	var b byte
	n := 0
	for _, c := range s {
		if c != '0' && c != '1' {
			continue
		}
		b = b<<1 | byte(c-'0')
		n++
		if n == 8 {
			out = append(out, b)
			b, n = 0, 0
		}
	}
	if n > 0 {
		out = append(out, b<<uint(8-n))
	}
	return out
}

// TestElTagTreeLeeUnaRejillaConocida usa la rejilla 2×3 del ejemplo de la norma
// —valores 1,3,2,2,2,1— con la codificación derivada paso a paso abajo.
//
// Si el recorrido de padres a hijos estuviera mal, saldrían números plausibles
// pero equivocados, y la imagen se llenaría de basura repartida sin dar error.
func TestElTagTreeLeeUnaRejillaConocida(t *testing.T) {
	// Valores, en orden (x, y):
	//   w(0,0)=1  w(1,0)=3
	//   w(0,1)=2  w(1,1)=2
	//   w(0,2)=2  w(1,2)=1
	//
	// El árbol tiene tres niveles: 2×3 hojas, 1×2 intermedios y la raíz. Cada
	// nodo guarda el MÍNIMO de sus hijos, así que los intermedios valen 1 y 1, y
	// la raíz 1. Un nodo se escribe emitiendo un 0 por cada paso que su valor
	// está por encima de la cota heredada, y un 1 al llegar:
	//
	//   (0,0): raíz 0→1 «01», intermedio ya en 1 «1», hoja 1 «1»      → 0111
	//   (1,0): raíz e intermedio ya cerrados; hoja 1→3 «001»          → 001
	//   (0,1): hoja 1→2 «01»                                          → 01
	//   (1,1): hoja 1→2 «01»                                          → 01
	//   (0,2): intermedio de abajo «1», hoja 1→2 «01»                 → 101
	//   (1,2): hoja 1 «1»                                             → 1
	arbol, err := nuevoTagTree(2, 3)
	if err != nil {
		t.Fatal(err)
	}
	l := nuevoLectorBitsJPX(bitsDe(t, "0111 001 01 01 101 1"))

	quiero := [][3]int{
		{0, 0, 1},
		{1, 0, 3},
		{0, 1, 2},
		{1, 1, 2},
		{0, 2, 2},
		{1, 2, 1},
	}
	for _, c := range quiero {
		v, listo := arbol.leer(l, c[0], c[1], 99)
		if !listo {
			t.Fatalf("la hoja (%d,%d) quedó sin decidir", c[0], c[1])
		}
		if v != c[2] {
			t.Errorf("hoja (%d,%d) = %d, quería %d", c[0], c[1], v, c[2])
		}
	}
}

// TestElTopeParaCuandoYaSabeLoQueLePreguntan: la cabecera de paquete no pregunta
// «¿cuánto vale?» sino «¿vale menos que esto?», y el codificador escribe solo los
// bits para responder eso. Leer de más se come bits del siguiente campo.
func TestElTopeParaCuandoYaSabeLoQueLePreguntan(t *testing.T) {
	arbol, err := nuevoTagTree(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	// «0 0 0» = el valor es al menos 3, sin llegar a cerrarse.
	l := nuevoLectorBitsJPX(bitsDe(t, "000"))
	v, listo := arbol.leer(l, 0, 0, 3)
	if listo {
		t.Fatal("con tope 3 y tres ceros el valor NO puede estar decidido")
	}
	if v != 3 {
		t.Errorf("cota = %d, quería 3", v)
	}
	// Y al seguir leyendo con más margen, continúa donde lo dejó en vez de
	// empezar de cero: si reiniciara, el valor saldría demasiado pequeño.
	l2 := nuevoLectorBitsJPX(bitsDe(t, "1"))
	v, listo = arbol.leer(l2, 0, 0, 99)
	if !listo || v != 3 {
		t.Errorf("tras reanudar: valor %d, listo %v; quería 3 y true", v, listo)
	}
}

// TestTrasUnFFSoloVienenSieteBits es el check del relleno. Es la trampa clásica
// de esta parte: sin saltarlo, todo lo que viene detrás se lee corrido un bit y
// la imagen sale con ruido sin que nada falle.
func TestTrasUnFFSoloVienenSieteBits(t *testing.T) {
	l := nuevoLectorBitsJPX([]byte{0xFF, 0xFF})
	// Los ocho primeros bits son el 0xFF entero.
	for i := 0; i < 8; i++ {
		if l.bit() != 1 {
			t.Fatalf("bit %d del primer byte debería ser 1", i)
		}
	}
	// El segundo byte es 0xFF pero SOLO aporta 7 bits: el primero es relleno.
	if got := l.bits(7); got != 0x7F {
		t.Errorf("los siete bits tras el 0xFF = %#x, quería 0x7F", got)
	}
	// Y no queda nada más de ese byte.
	if l.pos != 2 {
		t.Errorf("posición = %d, quería 2: el byte tras el 0xFF se agota en 7 bits", l.pos)
	}
}

// TestUnaCabeceraTruncadaNoRompe: un PDF con el flujo cortado es corriente, y
// leer más allá del final tiene que comportarse como «no hay más», no reventar.
func TestUnaCabeceraTruncadaNoRompe(t *testing.T) {
	l := nuevoLectorBitsJPX([]byte{0x80})
	if l.bit() != 1 {
		t.Fatal("el primer bit de 0x80 es 1")
	}
	for i := 0; i < 100; i++ {
		if l.bit() != 0 {
			t.Fatalf("más allá del final se debe leer 0 (bit %d)", i)
		}
	}
	arbol, err := nuevoTagTree(4, 4)
	if err != nil {
		t.Fatal(err)
	}
	// Con ceros infinitos el valor nunca se cierra: tiene que salir por el tope.
	if _, listo := arbol.leer(l, 3, 3, 5); listo {
		t.Error("sin bits que lo cierren, el valor no puede darse por definitivo")
	}
}

// TestElArbolRechazaTamanosImposibles: el ancho sale de la cabecera, o sea de
// fuera, y de ahí llega un cero o un número enorme.
func TestElArbolRechazaTamanosImposibles(t *testing.T) {
	for _, c := range [][2]int{{0, 4}, {4, 0}, {-1, 4}, {1 << 20, 1 << 20}} {
		if _, err := nuevoTagTree(c[0], c[1]); err == nil {
			t.Errorf("un tag tree de %d×%d debería rechazarse", c[0], c[1])
		}
	}
}
