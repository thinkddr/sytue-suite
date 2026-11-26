// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mediafoundation

import (
	"encoding/binary"
	"math"
	"runtime"
	"testing"
	"time"
)

// fotogramaNV12 hace un búfer NV12 de paso × filas: la luma de cada píxel es su fila y
// columna mezcladas, y el croma de cada pareja, U = 100+columna y V = 200−fila.
func fotogramaNV12(paso, filas int) []byte {
	b := make([]byte, tamano(formatoNV12, paso, filas))
	for f := range filas {
		for c := range paso {
			b[f*paso+c] = byte(f*7 + c)
		}
	}
	uv := b[paso*filas:]
	for f := range (filas + 1) / 2 {
		for c := range paso / 2 {
			uv[f*paso+2*c] = byte(100 + c)
			uv[f*paso+2*c+1] = byte(200 - f)
		}
	}
	return b
}

func TestNV12AI420(t *testing.T) {
	// Un H.264 de 6×4 descodificado en un búfer de paso 8 y 6 filas (como 1920×1080 en
	// 1920×1088), con la apertura arriba a la izquierda.
	m := medida{formato: formatoNV12, anchoCod: 8, altoCod: 6, paso: 8, ancho: 6, alto: 4}
	datos := fotogramaNV12(8, 6)
	pl, err := distribuir(len(datos), m)
	if err != nil {
		t.Fatal(err)
	}
	if pl.paso != 8 || pl.filas != 6 || pl.ancho != 6 || pl.alto != 4 {
		t.Fatalf("planos: %+v", pl)
	}
	img, err := aI420(datos, pl)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 6 || b.Dy() != 4 {
		t.Fatalf("tamaño: %v", b)
	}
	for f := range 4 {
		for c := range 6 {
			if got, want := img.Y[f*img.YStride+c], byte(f*7+c); got != want {
				t.Fatalf("Y(%d,%d) = %d, quiero %d", c, f, got, want)
			}
		}
	}
	for f := range 2 {
		for c := range 3 {
			if got := img.Cb[f*img.CStride+c]; got != byte(100+c) {
				t.Fatalf("U(%d,%d) = %d", c, f, got)
			}
			if got := img.Cr[f*img.CStride+c]; got != byte(200-f) {
				t.Fatalf("V(%d,%d) = %d", c, f, got)
			}
		}
	}
}

func TestI420ConAperturaYTamanoImpar(t *testing.T) {
	// I420 de paso 10 y 8 filas; se ve 5×3 desde (2, 2).
	paso, filas := 10, 8
	pasoC, filasC := 5, 4
	datos := make([]byte, tamano(formatoI420, paso, filas))
	for i := range paso * filas {
		datos[i] = byte(i)
	}
	u := datos[paso*filas:]
	v := u[pasoC*filasC:]
	for i := range pasoC * filasC {
		u[i], v[i] = byte(50+i), byte(150+i)
	}
	m := medida{formato: formatoI420, anchoCod: paso, altoCod: filas, x: 2, y: 2, ancho: 5, alto: 3}
	pl, err := distribuir(len(datos), m)
	if err != nil {
		t.Fatal(err)
	}
	img, err := aI420(datos, pl)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 5 || b.Dy() != 3 {
		t.Fatalf("tamaño: %v", b)
	}
	if got, want := img.Y[0], datos[2*paso+2]; got != want {
		t.Fatalf("Y(0,0) = %d, quiero %d", got, want)
	}
	if got, want := img.Y[2*img.YStride+4], datos[4*paso+6]; got != want {
		t.Fatalf("Y(4,2) = %d, quiero %d", got, want)
	}
	// El croma de (0,0) es el de (1,1) del plano: 1·5+1.
	if img.Cb[0] != byte(50+6) || img.Cr[0] != byte(150+6) {
		t.Fatalf("croma (0,0): %d %d", img.Cb[0], img.Cr[0])
	}
	if img.Cb[1*img.CStride+2] != byte(50+2*pasoC+3) {
		t.Fatalf("U(2,1) = %d", img.Cb[1*img.CStride+2])
	}
}

func TestDistribuirSinPasoNiFilasEnElTipo(t *testing.T) {
	// El tipo solo dice 100×50; el búfer viene con el paso alineado a 128 (64 no llega).
	m := medida{formato: formatoNV12, anchoCod: 100, altoCod: 50}
	pl, err := distribuir(tamano(formatoNV12, 128, 50), m)
	if err != nil {
		t.Fatal(err)
	}
	if pl.paso != 128 || pl.filas != 50 {
		t.Fatalf("planos: %+v", pl)
	}
	// Con el paso alineado y también las filas, a 16 (64).
	pl, err = distribuir(tamano(formatoNV12, 128, 64), medida{formato: formatoNV12, anchoCod: 100, altoCod: 50})
	if err != nil || pl.paso != 128 || pl.filas != 64 {
		t.Fatalf("planos: %+v %v", pl, err)
	}
	// Relleno al final: vale lo que dice el tipo.
	pl, err = distribuir(tamano(formatoI420, 100, 50)+4096, medida{formato: formatoI420, anchoCod: 100, altoCod: 50})
	if err != nil || pl.paso != 100 || pl.filas != 50 {
		t.Fatalf("con relleno: %+v %v", pl, err)
	}
}

func TestDistribuirNuncaSeSaleDelBufer(t *testing.T) {
	m := medida{formato: formatoNV12, anchoCod: 64, altoCod: 64, paso: 64}
	if _, err := distribuir(100, m); err == nil {
		t.Fatal("un búfer corto no tiene reparto")
	}
	if _, err := distribuir(1<<20, medida{formato: formatoNV12}); err == nil {
		t.Fatal("sin tamaño no hay reparto")
	}
	pl := planos{formato: formatoNV12, paso: 8, filas: 4, ancho: 8, alto: 4}
	if _, err := aI420(make([]byte, 10), pl); err == nil {
		t.Fatal("aI420 con un búfer corto tiene que fallar, no salirse")
	}
	pl.x = 2 // la apertura se sale del paso
	if _, err := aI420(make([]byte, 1000), pl); err == nil {
		t.Fatal("una apertura fuera del paso tiene que fallar")
	}
}

func TestInstantes(t *testing.T) {
	if got := deCien(10_000_000); got != time.Second {
		t.Fatalf("1 s en unidades de 100 ns: %v", got)
	}
	if got := deCien(333_667); got != 33366700*time.Nanosecond {
		t.Fatalf("un fotograma a 29,97: %v", got)
	}
	if aCien(1500*time.Millisecond) != 15_000_000 {
		t.Fatal("aCien")
	}
	if deCien(aCien(42*time.Second)) != 42*time.Second {
		t.Fatal("ida y vuelta")
	}
	if instanteImagen(-210_000) != 0 {
		t.Fatal("un fotograma antes de cero se enseña en cero")
	}
	if deCien(-210_000) != -21*time.Millisecond {
		t.Fatal("el audio antes de cero conserva su instante (el Reproductor lo descarta)")
	}
	if deCien(math.MaxInt64) != time.Duration(math.MaxInt64) {
		t.Fatal("sin desbordar")
	}
	// Tras buscar a 10 s: lo que acaba antes no se entrega; lo que lo cubre, sí.
	obj := 10 * time.Second
	if !pasado(9*time.Second, 40*time.Millisecond, obj) {
		t.Fatal("un fotograma de las 9 s queda antes")
	}
	if pasado(9980*time.Millisecond, 40*time.Millisecond, obj) {
		t.Fatal("el fotograma que se ve a las 10 s no se salta")
	}
	if pasado(9*time.Second, 0, obj) {
		t.Fatal("sin duración no se salta")
	}
	if pasado(time.Second, time.Second/25, 0) {
		t.Fatal("sin búsqueda no se salta nada")
	}
}

func TestElegirFlujo(t *testing.T) {
	v := func(ms int) pendiente { return pendiente{hay: true, t: time.Duration(ms) * time.Millisecond} }
	casos := []struct {
		v, a pendiente
		want flujo
	}{
		{v(0), v(0), deVideo},         // a la par, la imagen
		{v(40), v(20), deAudio},       // el audio va antes
		{v(40), v(60), deVideo},       // la imagen va antes
		{v(40), pendiente{}, deVideo}, // sin audio
		{pendiente{}, v(20), deAudio}, // sin vídeo
		{pendiente{}, pendiente{}, ninguno},
	}
	for _, c := range casos {
		if got := elegir(c.v, c.a); got != c.want {
			t.Errorf("elegir(%+v, %+v) = %d, quiero %d", c.v, c.a, got, c.want)
		}
	}
}

func pcm16(muestras ...int16) []byte {
	b := make([]byte, 2*len(muestras))
	for i, m := range muestras {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(m)) //nolint:gosec // G115: PCM con signo
	}
	return b
}

func TestAMono(t *testing.T) {
	// Mono de 16 bits: tal cual, también los extremos.
	got := aMono(pcm16(0, 1, -1, 32767, -32768), formatoSonido{bits: 16, canales: 1, muestreo: 48000})
	want := []int16{0, 1, -1, 32767, -32768}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mono: %v, quiero %v", got, want)
		}
	}
	// Estéreo: la media; lo que sobra al final (media muestra) no cuenta.
	b := append(pcm16(1000, 3000, -2000, 0), 0x01)
	got = aMono(b, formatoSonido{bits: 16, canales: 2, muestreo: 44100})
	if len(got) != 2 || got[0] != 2000 || got[1] != -1000 {
		t.Fatalf("estéreo: %v", got)
	}
	// Coma flotante, con lo que se pasa de 1 recortado.
	fl := make([]byte, 12)
	binary.LittleEndian.PutUint32(fl[0:], math.Float32bits(0.5))
	binary.LittleEndian.PutUint32(fl[4:], math.Float32bits(-1))
	binary.LittleEndian.PutUint32(fl[8:], math.Float32bits(1.5))
	got = aMono(fl, formatoSonido{flotante: true, bits: 32, canales: 1, muestreo: 48000})
	if len(got) != 3 || got[0] != 16384 || got[1] != -32768 || got[2] != 32767 {
		t.Fatalf("flotante: %v", got)
	}
	// 24 bits con signo.
	got = aMono([]byte{0x00, 0x00, 0x80, 0xff, 0xff, 0x7f}, formatoSonido{bits: 24, canales: 1, muestreo: 48000})
	if len(got) != 2 || got[0] != -32768 || got[1] != 32767 {
		t.Fatalf("24 bits: %v", got)
	}
	if aMono(pcm16(1, 2), formatoSonido{bits: 8, canales: 1, muestreo: 48000}) != nil {
		t.Fatal("un formato que no se sabe leer no da nada")
	}
}

func TestRemuestreoSinCosturas(t *testing.T) {
	// Una rampa a 24 kHz pasada a 48 kHz en trozos de tamaños distintos tiene que dar
	// lo mismo que de una vez, y ser la rampa con los puntos intermedios.
	rampa := make([]int16, 1000)
	for i := range rampa {
		rampa[i] = int16(i * 10) //nolint:gosec // G115: cabe
	}
	entero := (&remuestreo{de: 24000, a: 48000}).pasar(rampa)
	r := &remuestreo{de: 24000, a: 48000}
	var trozos []int16
	for i := 0; i < len(rampa); {
		n := min(1+i%37, len(rampa)-i)
		trozos = append(trozos, r.pasar(rampa[i:i+n])...)
		i += n
	}
	if len(entero) != len(trozos) {
		t.Fatalf("de una vez %d muestras, a trozos %d", len(entero), len(trozos))
	}
	for i := range entero {
		if entero[i] != trozos[i] {
			t.Fatalf("muestra %d: %d de una vez, %d a trozos", i, entero[i], trozos[i])
		}
	}
	// Doble de muestras (menos la última, que espera a la siguiente) y la rampa.
	if n := len(entero); n < 1996 || n > 2000 {
		t.Fatalf("%d muestras de 1000 a doble muestreo", n)
	}
	for i := range len(entero) - 1 {
		if d := int(entero[i+1]) - int(entero[i]); d != 5 {
			t.Fatalf("la rampa sube de 5 en 5 y en %d sube %d", i, d)
		}
	}
	// 44,1 → 48 kHz: cuenta de muestras proporcional.
	r = &remuestreo{de: 44100, a: 48000}
	total := 0
	for range 100 {
		total += len(r.pasar(make([]int16, 441)))
	}
	if total < 47990 || total > 48010 {
		t.Fatalf("un segundo a 44,1 kHz da %d muestras a 48 kHz", total)
	}
	// Igual muestreo: tal cual; tras reiniciar se empieza de cero.
	r = &remuestreo{de: 48000, a: 48000}
	if got := r.pasar([]int16{1, 2, 3}); len(got) != 3 {
		t.Fatal("sin remuestrear")
	}
	r = &remuestreo{de: 24000, a: 48000}
	r.pasar([]int16{100, 200})
	r.reiniciar()
	if got := r.pasar([]int16{7, 9}); len(got) == 0 || got[0] != 7 {
		t.Fatalf("tras reiniciar: %v", got)
	}
}

func TestFormatoSonidoValido(t *testing.T) {
	if !(formatoSonido{bits: 16, canales: 6, muestreo: 48000}).valido() {
		t.Fatal("5.1 de 16 bits vale")
	}
	if (formatoSonido{flotante: true, bits: 64, canales: 2, muestreo: 48000}).valido() {
		t.Fatal("double no se lee")
	}
	if (formatoSonido{bits: 16, canales: 0, muestreo: 48000}).valido() {
		t.Fatal("sin canales no")
	}
}

func TestFueraDeWindowsNoHay(t *testing.T) {
	if Disponible() && runtime.GOOS != "windows" {
		t.Fatal("Media Foundation solo en Windows")
	}
	if _, err := Abrir("nada.mp4"); err == nil {
		t.Fatal("un fichero que no existe no se abre")
	}
}
