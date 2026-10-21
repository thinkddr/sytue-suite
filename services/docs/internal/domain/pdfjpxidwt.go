// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Wavelet inversa y transformada de color (ISO 15444-1 §F y §G).
//
// Lo que sale de EBCOT son coeficientes de subbanda, no píxeles. Esto los
// devuelve a la imagen: primero deshaciendo la descomposición nivel a nivel, y
// después la mezcla de componentes, si la hubo.
//
// Hay dos wavelets y no son intercambiables:
//
//   - 5/3, reversible y con aritmética entera. Es la del modo SIN PÉRDIDA, que
//     es como se digitaliza un expediente o un plano.
//   - 9/7, irreversible y en coma flotante. Es la del modo con pérdida.
//
// Aplicar una donde toca la otra no da error: da una imagen borrosa o con un
// cuadriculado fino, que es de las cosas que más cuesta atribuir mirando.
package domain

// Coeficientes del lifting 9/7 (tabla F.4).
const (
	jpxAlfa  = -1.586134342059924
	jpxBeta  = -0.052980118572961
	jpxGama  = 0.882911075530934
	jpxDelta = 0.443506852043971
	jpxK     = 1.230174104914001
)

/*
idwt53Linea deshace una línea con la wavelet 5/3 entera.

`y` llega ENTRELAZADO: en las posiciones pares los coeficientes de baja
frecuencia y en las impares los de alta, que es como los deja la descomposición.
`i0` es el índice de la primera muestra en la rejilla de la imagen, y no siempre
es par: una subbanda puede empezar en impar, y ahí se invierten los papeles de
par e impar. Darlo por hecho descoloca la línea media píxel.
*/
func idwt53Linea(y []int32, i0 int) {
	n := len(y)
	if n == 0 {
		return
	}
	if n == 1 {
		// Una sola muestra: si es de alta frecuencia se divide, si es de baja se
		// queda igual (§F.3.7).
		if i0%2 != 0 {
			y[0] /= 2
		}
		return
	}
	en := func(i int) int32 { return y[reflejar(i, n)] }

	// Paso 1: las pares (baja) se corrigen con sus impares vecinas.
	pares := make([]int32, n)
	copy(pares, y)
	for i := 0; i < n; i++ {
		if (i+i0)%2 != 0 {
			continue
		}
		pares[i] = y[i] - pisoDiv(en(i-1)+en(i+1)+2, 4)
	}
	// Paso 2: las impares (alta) se corrigen con las pares YA corregidas.
	enP := func(i int) int32 { return pares[reflejar(i, n)] }
	for i := 0; i < n; i++ {
		if (i+i0)%2 == 0 {
			continue
		}
		pares[i] = y[i] + pisoDiv(enP(i-1)+enP(i+1), 2)
	}
	copy(y, pares)
}

/*
pisoDiv divide redondeando SIEMPRE hacia abajo.

Go trunca hacia cero: `-5/4` da -1 y la norma pide -2. Los coeficientes de una
wavelet son negativos la mitad de las veces, así que usar la división de Go
rompe la reversibilidad de la 5/3 —el modo SIN PÉRDIDA deja de serlo— y siembra
la imagen de píxeles desviados en uno. Es la clase de fallo que no se ve en una
foto y sí en un plano.
*/
func pisoDiv(a, b int32) int32 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// idwt97Linea deshace una línea con la wavelet 9/7 en coma flotante.
func idwt97Linea(y []float64, i0 int) {
	n := len(y)
	if n == 0 {
		return
	}
	if n == 1 {
		if i0%2 != 0 {
			y[0] /= 2
		}
		return
	}
	// Escalado (F-11): las bajas por 1/K y las altas por K.
	for i := 0; i < n; i++ {
		if (i+i0)%2 == 0 {
			y[i] /= jpxK
		} else {
			y[i] *= jpxK
		}
	}
	// Cuatro pasos de lifting, en el orden inverso al de la descomposición y con
	// el signo cambiado. Invertir el orden es lo que lo hace exacto.
	liftingInverso(y, i0, 0, -jpxDelta)
	liftingInverso(y, i0, 1, -jpxGama)
	liftingInverso(y, i0, 0, -jpxBeta)
	liftingInverso(y, i0, 1, -jpxAlfa)
}

// liftingInverso aplica un paso a las muestras de una paridad, usando las de la
// otra. `paridad` 0 = pares, 1 = impares.
func liftingInverso(y []float64, i0, paridad int, k float64) {
	n := len(y)
	en := func(i int) float64 { return y[reflejar(i, n)] }
	nuevo := make([]float64, n)
	copy(nuevo, y)
	for i := 0; i < n; i++ {
		if (i+i0)%2 != paridad {
			continue
		}
		nuevo[i] = y[i] + k*(en(i-1)+en(i+1))
	}
	copy(y, nuevo)
}

/*
reflejar devuelve el índice dentro del array usando extensión SIMÉTRICA.

Los bordes de una wavelet necesitan muestras que no existen. Extenderlos con
ceros deja una línea oscura en cada borde de la imagen —y en cada borde de cada
tile, o sea una cuadrícula—; la norma manda reflejar, que es continuar la señal
como si estuviera espejada.
*/
func reflejar(i, n int) int {
	if n == 1 {
		return 0
	}
	periodo := 2 * (n - 1)
	i %= periodo
	if i < 0 {
		i += periodo
	}
	if i >= n {
		i = periodo - i
	}
	return i
}

// idwt53 deshace UN nivel de descomposición sobre una rejilla entera.
//
// Primero las columnas y luego las filas —o al revés, da lo mismo mientras sean
// los dos—: la transformada 2D es separable, y hacer solo uno de los dos ejes
// deja la imagen con rayas.
func idwt53(datos []int32, ancho, alto, x0, y0 int) {
	col := make([]int32, alto)
	for x := 0; x < ancho; x++ {
		for y := 0; y < alto; y++ {
			col[y] = datos[y*ancho+x]
		}
		idwt53Linea(col, y0)
		for y := 0; y < alto; y++ {
			datos[y*ancho+x] = col[y]
		}
	}
	for y := 0; y < alto; y++ {
		idwt53Linea(datos[y*ancho:(y+1)*ancho], x0)
	}
}

// idwt97 hace lo mismo con la wavelet irreversible.
func idwt97(datos []float64, ancho, alto, x0, y0 int) {
	col := make([]float64, alto)
	for x := 0; x < ancho; x++ {
		for y := 0; y < alto; y++ {
			col[y] = datos[y*ancho+x]
		}
		idwt97Linea(col, y0)
		for y := 0; y < alto; y++ {
			datos[y*ancho+x] = col[y]
		}
	}
	for y := 0; y < alto; y++ {
		idwt97Linea(datos[y*ancho:(y+1)*ancho], x0)
	}
}

/*
rctInversa deshace la transformada reversible de componentes (§G.2).

Es la que acompaña a la wavelet 5/3, y es entera y exacta: se usa cuando el
documento no puede perder ni un bit. Convierte el YUV reversible del códec de
vuelta a RGB.
*/
func rctInversa(y, u, v []int32) {
	for i := range y {
		// Piso, no truncado: las crominancias son negativas la mitad del tiempo y
		// con la división de Go la transformada deja de ser exacta.
		g := y[i] - pisoDiv(u[i]+v[i], 4)
		r := v[i] + g
		b := u[i] + g
		y[i], u[i], v[i] = r, g, b
	}
}

/*
ictInversa deshace la transformada irreversible (§G.3): el YCbCr de toda la
vida, el mismo que usa JPEG.
*/
func ictInversa(y, cb, cr []float64) {
	for i := range y {
		r := y[i] + 1.402*cr[i]
		g := y[i] - 0.34413*cb[i] - 0.71414*cr[i]
		b := y[i] + 1.772*cb[i]
		y[i], cb[i], cr[i] = r, g, b
	}
}
