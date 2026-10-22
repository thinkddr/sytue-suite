// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del decodificador de fax. Los flujos se escriben BIT A BIT con los
// códigos de la especificación (T.4/T.6) en vez de meter un binario en el repo:
// así el test dice de dónde sale cada byte y se puede leer al lado de la tabla.
package domain

import (
	"strings"
	"testing"
)

// bitsAByte convierte «0011011011» en los bytes que lo contienen, rellenando
// con ceros hasta completar el último.
func bitsAByte(t *testing.T, bits string) []byte {
	t.Helper()
	var out []byte
	acc, n := byte(0), 0
	for _, c := range bits {
		if c != '0' && c != '1' {
			continue
		}
		acc <<= 1
		if c == '1' {
			acc |= 1
		}
		n++
		if n == 8 {
			out = append(out, acc)
			acc, n = 0, 0
		}
	}
	if n > 0 {
		out = append(out, acc<<uint(8-n))
	}
	return out
}

func TestCCITTG4LineaEnBlanco(t *testing.T) {
	// Una línea toda blanca en G4 es un solo modo vertical V0: «el cambio está
	// donde dice la línea de arriba», y arriba no hay ninguno.
	out, err := descomprimirCCITT(bitsAByte(t, "1"), parametrosCCITT{
		K: -1, Columnas: 8, Filas: 1, MaxFilas: 1,
	})
	if err != nil {
		t.Fatalf("descomprimirCCITT: %v", err)
	}
	if len(out) != 1 || out[0] != 0xFF {
		t.Errorf("fila = %08b, quería 11111111 (todo blanco)", out)
	}
}

// TestCCITTG4ModoHorizontal es el caso que de verdad ejercita las tablas de
// rachas: cuatro blancos y cuatro negros escritos a mano.
func TestCCITTG4ModoHorizontal(t *testing.T) {
	// 001 = horizontal · 1011 = 4 blancos · 011 = 4 negros
	out, err := descomprimirCCITT(bitsAByte(t, "001"+"1011"+"011"), parametrosCCITT{
		K: -1, Columnas: 8, Filas: 1, MaxFilas: 1,
	})
	if err != nil {
		t.Fatalf("descomprimirCCITT: %v", err)
	}
	if len(out) != 1 || out[0] != 0xF0 {
		t.Errorf("fila = %08b, quería 11110000 (cuatro blancos y cuatro negros)", out)
	}
}

// TestCCITTG4SegundaLineaSeApoyaEnLaPrimera comprueba lo que HACE que G4
// comprima: la segunda línea no describe sus rachas, dice que sus cambios están
// donde los de la anterior. Si la línea de referencia no se propagara, esta
// saldría en blanco y el escaneo se vería a medias.
func TestCCITTG4SegundaLineaSeApoyaEnLaPrimera(t *testing.T) {
	flujo := "001" + "1011" + "011" + // primera línea, a mano
		"1" + "1" // segunda: dos V0, «igual que arriba»
	out, err := descomprimirCCITT(bitsAByte(t, flujo), parametrosCCITT{
		K: -1, Columnas: 8, Filas: 2, MaxFilas: 2,
	})
	if err != nil {
		t.Fatalf("descomprimirCCITT: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("salieron %d filas, quería 2", len(out))
	}
	if out[0] != 0xF0 || out[1] != 0xF0 {
		t.Errorf("filas = %08b, quería las dos 11110000", out)
	}
}

func TestCCITTBlackIs1Invierte(t *testing.T) {
	out, err := descomprimirCCITT(bitsAByte(t, "001"+"1011"+"011"), parametrosCCITT{
		K: -1, Columnas: 8, Filas: 1, MaxFilas: 1, NegroEs1: true,
	})
	if err != nil {
		t.Fatalf("descomprimirCCITT: %v", err)
	}
	// Con /BlackIs1 el 1 es negro: la misma imagen sale con los bits al revés.
	if out[0] != 0x0F {
		t.Errorf("fila = %08b, quería 00001111", out)
	}
}

func TestCCITTG3UnidimensionalLeeRachas(t *testing.T) {
	// K = 0: cada línea son rachas alternas empezando por blanco, sin referencia.
	out, err := descomprimirCCITT(bitsAByte(t, "1011"+"011"), parametrosCCITT{
		K: 0, Columnas: 8, Filas: 1, MaxFilas: 1,
	})
	if err != nil {
		t.Fatalf("descomprimirCCITT: %v", err)
	}
	if out[0] != 0xF0 {
		t.Errorf("fila = %08b, quería 11110000", out)
	}
}

func TestCCITTRellenaLasFilasQueFaltan(t *testing.T) {
	// Un fax cortado se enseña entero con lo que falta en blanco, que es como lo
	// ve un aparato de fax; devolver menos filas descuadraría la imagen.
	out, err := descomprimirCCITT(bitsAByte(t, "1"), parametrosCCITT{
		K: -1, Columnas: 8, Filas: 4, MaxFilas: 4,
	})
	if err != nil {
		t.Fatalf("descomprimirCCITT: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("salieron %d filas, quería 4", len(out))
	}
	for i, b := range out {
		if b != 0xFF {
			t.Errorf("fila %d = %08b, quería blanca", i, b)
		}
	}
}

func TestCCITTFlujoVacioFalla(t *testing.T) {
	if _, err := descomprimirCCITT(nil, parametrosCCITT{K: -1, Columnas: 8}); err == nil {
		t.Error("un flujo vacío no puede dar una imagen")
	}
}

func TestCCITTNoSeAtascaConBasura(t *testing.T) {
	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		for _, basura := range [][]byte{
			{0x00, 0x00, 0x00, 0x00}, {0xFF, 0xFF, 0xFF}, {0x01, 0x02, 0x03, 0x04, 0x05},
		} {
			_, _ = descomprimirCCITT(basura, parametrosCCITT{K: -1, Columnas: 64, MaxFilas: 32})
			_, _ = descomprimirCCITT(basura, parametrosCCITT{K: 0, Columnas: 64, MaxFilas: 32})
			_, _ = descomprimirCCITT(basura, parametrosCCITT{K: 4, Columnas: 64, MaxFilas: 32})
		}
	}()
	select {
	case <-hecho:
	case <-timeoutCorto():
		t.Fatal("el decodificador de fax se colgó con basura")
	}
}

// TestRenderImagenCCITTSeVe es el cierre: una página escaneada tiene que salir
// como imagen y no como el recuadro gris de «no sé dibujar esto».
func TestRenderImagenCCITTSeVe(t *testing.T) {
	fax := string(bitsAByte(t, "001"+"1011"+"011"))
	svg := renderizar(t, pdfConContenido(t,
		"q 50 0 0 50 10 10 cm /Im0 Do Q",
		`/Resources << /XObject << /Im0 5 0 R >> >>`,
		"<< /Type /XObject /Subtype /Image /Width 8 /Height 1 /Filter /CCITTFaxDecode "+
			"/DecodeParms << /K -1 /Columns 8 /Rows 1 >> /ColorSpace /DeviceGray "+
			"/BitsPerComponent 1 /Length "+itoa(len(fax))+" >>\nstream\n"+fax+"\nendstream"))
	if strings.Contains(svg, "#e5e5e5") {
		t.Fatalf("el escaneo salió como recuadro gris: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, "data:image/png;base64,") {
		t.Errorf("no se emitió la imagen decodificada: %s", primerasLineas(svg))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
