// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Límites del lector ante un PDF hostil.
//
// Los dos fallos que cubre se midieron antes de arreglarlos, y por eso estas
// pruebas llevan el número: un PDF de ~4 MB de «[» reventaba la pila de Go con
// `fatal error: stack overflow` —no es un pánico, no se puede recuperar, y se
// llevaba el servicio entero—, y un flujo de 2 MB de Flate se hinchaba a 1 GiB
// (8,6 s, 2,6 GiB asignados) antes de que nadie mirara qué había dentro.
package domain

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"math"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLectorNoSeCaeConArraysAnidadosSinFin(t *testing.T) {
	// 4 000 000 de niveles: antes, `fatal error: stack overflow`.
	for _, abre := range []string{"[", "<</A "} {
		cuerpo := strings.Repeat(abre, 4_000_000)
		pdf := pdfMinimo(t, "<< /Size 4 /Root 1 0 R >>", []string{
			`<< /Type /Catalog /Pages 2 0 R /X ` + cuerpo + ` >>`,
			`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
			`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>`,
		})
		inicio := time.Now()
		_, _ = LeerPDF(pdf) // abre o no abre: lo que no puede es matar el proceso
		if d := time.Since(inicio); d > 5*time.Second {
			t.Errorf("%q anidado: tardó %v", abre, d)
		}
	}
}

func TestIntDesdeInt64RespetaLimitesDeInt(t *testing.T) {
	for _, v := range []int64{int64(math.MinInt), int64(math.MaxInt)} {
		got, ok := intDesdeInt64(v)
		if !ok || int64(got) != v {
			t.Errorf("intDesdeInt64(%d) = %d, %t", v, got, ok)
		}
	}
	if math.MaxInt < math.MaxInt64 {
		maxInt, minInt := int64(math.MaxInt), int64(math.MinInt)
		if _, ok := intDesdeInt64(maxInt + 1); ok {
			t.Error("se aceptó un valor por encima de MaxInt")
		}
		if _, ok := intDesdeInt64(minInt - 1); ok {
			t.Error("se aceptó un valor por debajo de MinInt")
		}
	}
}

func TestCargarSeccionXrefRechazaOffsetsFueraDelFichero(t *testing.T) {
	d := &PDFDoc{buf: []byte("%PDF-")}
	for _, off := range []int64{-1, math.MaxInt64} {
		if _, err := d.cargarSeccionXref(off); !errors.Is(err, ErrPDFCorrupto) {
			t.Errorf("offset %d: se esperaba ErrPDFCorrupto, recibido %v", off, err)
		}
	}
}

func TestAnchosDeXrefRechazaValoresInvalidos(t *testing.T) {
	for _, n := range []int64{-1, 9, math.MaxInt64} {
		_, err := anchosDeXref(pdfDict{"W": pdfArray{int64(1), n, int64(1)}})
		if !errors.Is(err, ErrPDFCorrupto) {
			t.Errorf("ancho %d: se esperaba ErrPDFCorrupto, recibido %v", n, err)
		}
	}
	if got, err := anchosDeXref(pdfDict{"W": pdfArray{int64(1), int64(2), int64(1)}}); err != nil || got != [3]int{1, 2, 1} {
		t.Errorf("anchosDeXref válidos = %v, %v", got, err)
	}
}

func TestImagenDentroDelLimiteNoDesbordaElProducto(t *testing.T) {
	if imagenDentroDelLimite(maxPixelesImagen/2+1, 2) {
		t.Error("se aceptó una imagen que supera el límite")
	}
	if !imagenDentroDelLimite(maxPixelesImagen/2, 2) {
		t.Error("se rechazó una imagen justo dentro del límite")
	}
	if imagenDentroDelLimite(math.MaxInt, 2) {
		t.Error("se aceptó una dimensión cuyo producto desborda")
	}
}

// El límite de seguridad debe salir hasta el llamador, también al saltar una
// clave inválida; tratarlo como truncado reiniciaba el análisis del PDF hostil.
func TestElLimiteDeAnidadoNoSeTrataComoTruncado(t *testing.T) {
	for nombre, cuerpo := range map[string]string{
		"array":          strings.Repeat("[", maxAnidadoObjeto+2),
		"diccionario":    strings.Repeat("<</A ", maxAnidadoObjeto+2),
		"clave inválida": "<< " + strings.Repeat("[", maxAnidadoObjeto+2),
	} {
		t.Run(nombre, func(t *testing.T) {
			d := &PDFDoc{}
			_, err := d.parsearObjeto(&pdfLexer{buf: []byte(cuerpo)})
			if !errors.Is(err, errAnidadoDemasiado) {
				t.Fatalf("el límite de seguridad debe propagarse: %v", err)
			}
		})
	}
}

func TestEnterosPDFSeAjustanAlRangoDeInt(t *testing.T) {
	casos := []struct {
		texto string
		ok    bool
	}{
		{"2147483647", true},
		{"-2147483648", true},
		{"2147483648", strconv.IntSize == 64},
		{"-2147483649", strconv.IntSize == 64},
		{"9223372036854775807", strconv.IntSize == 64},
		{"-9223372036854775808", strconv.IntSize == 64},
	}
	for _, caso := range casos {
		t.Run(caso.texto, func(t *testing.T) {
			entero, ok := (&pdfLexer{buf: []byte(caso.texto)}).entero()
			if ok != caso.ok {
				t.Errorf("entero(): valor=%d ok=%t; quería ok=%t", entero, ok, caso.ok)
			}
			numero, _, ok := (&pdfLexer{buf: []byte(caso.texto)}).numero()
			if ok != caso.ok {
				t.Errorf("numero(): valor=%v ok=%t; quería ok=%t", numero, ok, caso.ok)
			}
		})
	}
}

func TestLectorSigueLeyendoLoAnidadoDeVerdad(t *testing.T) {
	// Cien niveles caben de sobra: un PDF real no pasa de una docena.
	cuerpo := strings.Repeat("[", 100) + "7" + strings.Repeat("]", 100)
	d, err := LeerPDF(pdfMinimo(t, "<< /Size 4 /Root 1 0 R >>", []string{
		`<< /Type /Catalog /Pages 2 0 R /X ` + cuerpo + ` >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>`,
	}))
	if err != nil {
		t.Fatalf("un anidado razonable debe abrir: %v", err)
	}
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	v := raiz["X"]
	for i := 0; i < 100; i++ {
		a, ok := v.(pdfArray)
		if !ok || len(a) != 1 {
			t.Fatalf("nivel %d: %T", i, v)
		}
		v = a[0]
	}
	if n, _ := comoEntero(v); n != 7 {
		t.Errorf("el valor del fondo = %v, quería 7", v)
	}
}

func TestInflarSeNiegaAHincharseSinFin(t *testing.T) {
	var zb bytes.Buffer
	zw := zlib.NewWriter(&zb)
	bloque := make([]byte, 1<<20)
	for i := 0; i < maxFlujoInflado>>20+72; i++ { // ~200 MiB de ceros
		_, _ = zw.Write(bloque)
	}
	_ = zw.Close()

	var antes, despues runtime.MemStats
	runtime.ReadMemStats(&antes)
	inicio := time.Now()
	out, err := inflar(zb.Bytes())
	duracion := time.Since(inicio)
	runtime.ReadMemStats(&despues)
	if !errors.Is(err, errFlujoDemasiadoGrande) || out != nil {
		t.Fatalf("debería negarse con errFlujoDemasiadoGrande: len=%d err=%v", len(out), err)
	}
	// La duración se observa, pero no decide la seguridad: bajo -race y con
	// otros paquetes compilando, el reloj de pared mide también la contención.
	// El contador de TestLeerAcotadoSeDetieneEnElPrimerByteDeExceso demuestra
	// exactamente cuánto trabajo admite el lector antes de rechazar la bomba.
	t.Logf("rechazo Flate: %v; asignaciones: %d MiB", duracion, (despues.TotalAlloc-antes.TotalAlloc)>>20)
	// Tope de memoria: lo asignado no pasa de unas pocas veces el límite.
	if mib := (despues.TotalAlloc - antes.TotalAlloc) >> 20; mib > 4*maxFlujoInflado>>20 {
		t.Errorf("asignó %d MiB para rechazar un flujo de %d MiB", mib, maxFlujoInflado>>20)
	}
}

// contadorFlujoPDF produce un flujo finito de ceros sin reservarlo entero.
// Cuenta bytes entregados al lector: el tiempo de un runner no cambia ese límite.
type contadorFlujoPDF struct {
	restantes int
	leidos    int
}

func (c *contadorFlujoPDF) Read(p []byte) (int, error) {
	if c.restantes == 0 {
		return 0, io.EOF
	}
	n := min(len(p), c.restantes)
	clear(p[:n])
	c.restantes -= n
	c.leidos += n
	return n, nil
}

func TestLeerAcotadoSeDetieneEnElPrimerByteDeExceso(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		bytes  int
	}{
		{"EOF justo en el límite", maxFlujoInflado},
		{"un byte de exceso", maxFlujoInflado + 1},
		{"cola sin consumir", maxFlujoInflado + 32<<10},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			r := &contadorFlujoPDF{restantes: caso.bytes}
			out, err := leerAcotado(r)
			if caso.bytes == maxFlujoInflado {
				if err != nil || len(out) != maxFlujoInflado {
					t.Fatalf("EOF en el límite: len=%d err=%v", len(out), err)
				}
			} else if !errors.Is(err, errFlujoDemasiadoGrande) || out != nil {
				t.Fatalf("el byte de exceso debe rechazar el flujo: len=%d err=%v", len(out), err)
			}
			if esperado := min(caso.bytes, maxFlujoInflado+1); r.leidos != esperado {
				t.Errorf("leyó %d bytes; debía detenerse en %d", r.leidos, esperado)
			}
		})
	}
}

func TestInflarSigueAbriendoLoNormal(t *testing.T) {
	var zb bytes.Buffer
	zw := zlib.NewWriter(&zb)
	_, _ = zw.Write([]byte("hola, PDF"))
	_ = zw.Close()
	out, err := inflar(zb.Bytes())
	if err != nil || string(out) != "hola, PDF" {
		t.Fatalf("inflar = %q, %v", out, err)
	}
}

// Con la suma Adler-32 del final equivocada los datos ya salieron enteros; antes
// se descartaban y se reintentaba como deflate crudo, que fallaba en la cabecera.
func TestInflarAceptaSumaDeComprobacionErronea(t *testing.T) {
	var zb bytes.Buffer
	zw := zlib.NewWriter(&zb)
	_, _ = zw.Write([]byte("hola, PDF"))
	_ = zw.Close()
	b := zb.Bytes()
	b[len(b)-1] ^= 0xFF
	out, err := inflar(b)
	if err != nil || string(out) != "hola, PDF" {
		t.Fatalf("inflar = %q, %v", out, err)
	}
}

func TestRunLengthNoSeHinchaSinFin(t *testing.T) {
	// Cada par de bytes (129, x) escribe 128 repeticiones: 64 a 1.
	entrada := bytes.Repeat([]byte{129, 'x'}, maxFlujoInflado/64+1_000_000)
	out := decodificarRunLength(entrada)
	if len(out) > maxFlujoInflado {
		t.Errorf("RunLength produjo %d bytes, el tope es %d", len(out), maxFlujoInflado)
	}
}
