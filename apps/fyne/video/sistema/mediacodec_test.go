// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package sistema

import (
	"encoding/binary"
	"errors"
	"image"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// muestraJava empaqueta como VideoSistema.java (cabecera() y ponerImagen/sacarSonido).
func muestraJava(tipo int32, us int64, a, b int32, datos []byte) []byte {
	out := make([]byte, javaCabecera, javaCabecera+len(datos))
	binary.BigEndian.PutUint32(out, uint32(tipo))   //nolint:gosec // G115: como el putInt de Java
	binary.BigEndian.PutUint64(out[4:], uint64(us)) //nolint:gosec // G115: como el putLong de Java
	binary.BigEndian.PutUint32(out[12:], uint32(a)) //nolint:gosec // G115: como el putInt de Java
	binary.BigEndian.PutUint32(out[16:], uint32(b)) //nolint:gosec // G115: como el putInt de Java
	return append(out, datos...)
}

func TestLaImagenDeMediaCodecSeLeeSinCopiar(t *testing.T) {
	// 5×3: luma 15, croma 3×2 cada una.
	datos := make([]byte, 15+6+6)
	for i := range datos {
		datos[i] = byte(i)
	}
	b := muestraJava(javaImagen, 1_040_000, 5, 3, datos)
	m, err := muestraDeJava(b)
	if err != nil {
		t.Fatal(err)
	}
	if m.Tiempo != 1040*time.Millisecond || m.PCM != nil {
		t.Fatalf("instante %v, PCM %v", m.Tiempo, m.PCM)
	}
	img, ok := m.Imagen.(*image.YCbCr)
	if !ok || img.Rect != image.Rect(0, 0, 5, 3) || img.SubsampleRatio != image.YCbCrSubsampleRatio420 {
		t.Fatalf("imagen %T %v", m.Imagen, m.Imagen.Bounds())
	}
	// (4, 2) en luma es 2·5+4; su croma (2, 1) es 1·3+2 tras la luma, y Cr tras Cb.
	c := img.YCbCrAt(4, 2)
	if c.Y != 14 || c.Cb != 15+5 || c.Cr != 21+5 {
		t.Errorf("(4,2) = %+v", c)
	}
	// Los planos son los bytes que llegaron, sin otra copia (y sin poder crecer uno sobre otro).
	if &img.Y[0] != &b[javaCabecera] || cap(img.Y) != 15 || cap(img.Cb) != 6 {
		t.Error("la imagen copia los bytes o sus planos se pisan")
	}
}

func TestElSonidoDeMediaCodecEsMonoConSigno(t *testing.T) {
	m, err := muestraDeJava(muestraJava(javaSonido, 20_000, 3, 0, []byte{0x00, 0x01, 0xff, 0xff, 0x80, 0x00}))
	if err != nil {
		t.Fatal(err)
	}
	if m.Imagen != nil || m.Tiempo != 20*time.Millisecond || len(m.PCM) != 3 || m.PCM[0] != 1 || m.PCM[1] != -1 || m.PCM[2] != -32768 {
		t.Errorf("sonido: %+v", m)
	}
}

func TestUnaMuestraRotaDeMediaCodecEsUnError(t *testing.T) {
	for nombre, b := range map[string][]byte{
		"corta":           {0, 0, 0, 1},
		"tipo raro":       muestraJava(7, 0, 0, 0, nil),
		"imagen sin lado": muestraJava(javaImagen, 0, 0, 2, nil),
		"imagen corta":    muestraJava(javaImagen, 0, 4, 2, make([]byte, 11)),
		"imagen larga":    muestraJava(javaImagen, 0, 4, 2, make([]byte, 13)),
		"imagen enorme":   muestraJava(javaImagen, 0, 1<<20, 1<<20, nil),
		"sonido impar":    muestraJava(javaSonido, 0, 2, 0, make([]byte, 3)),
		"sonido negativo": muestraJava(javaSonido, 0, -1, 0, nil),
	} {
		if _, err := muestraDeJava(b); err == nil {
			t.Errorf("%s: sin error", nombre)
		}
	}
}

func TestLaInformacionDeMediaCodec(t *testing.T) {
	in, err := infoDeJava("2000000\t1080\t1920\t1\t1")
	if err != nil {
		t.Fatal(err)
	}
	if in.Duracion != 2*time.Second || in.Ancho != 1080 || in.Alto != 1920 || !in.ConVideo || !in.ConAudio {
		t.Errorf("info: %+v", in)
	}
	if in, err := infoDeJava("1000000\t0\t0\t0\t1"); err != nil || in.ConVideo || !in.ConAudio {
		t.Errorf("solo sonido: %+v %v", in, err)
	}
	for _, s := range []string{"", "1\t2\t3", "x\t1\t1\t1\t1", "0\t0\t0\t1\t0", "0\t0\t0\t0\t0", "0\t99999\t1\t1\t0"} {
		if _, err := infoDeJava(s); err == nil {
			t.Errorf("%q: sin error", s)
		}
	}
	if !errors.Is(errorDeAbrir(javaSinCodec), ErrSinDescodificador) {
		t.Error("SIN_CODEC no es ErrSinDescodificador")
	}
	for _, c := range []int{javaNoLee, javaSinPistas, javaFallo, -99} {
		if errorDeAbrir(c) == nil || errors.Is(errorDeAbrir(c), ErrSinDescodificador) {
			t.Errorf("%d: %v", c, errorDeAbrir(c))
		}
	}
}

// Las constantes de aquí son las de VideoSistema.java: si una cambia allí, aquí se lee mal
// sin que nada falle al compilar.
func TestLasConstantesDeMediaCodecSonLasDelJava(t *testing.T) {
	j, err := os.ReadFile("../../android/java/eu/sytue/nativo/VideoSistema.java")
	if err != nil {
		t.Fatal(err)
	}
	for nombre, quiero := range map[string]int{
		"NO_LEE": javaNoLee, "SIN_PISTAS": javaSinPistas, "SIN_CODEC": javaSinCodec, "FALLO": javaFallo,
		"IMAGEN": javaImagen, "SONIDO": javaSonido, "CABECERA": javaCabecera,
	} {
		m := regexp.MustCompile(`static final int ` + nombre + ` = (-?\d+);`).FindSubmatch(j)
		if m == nil {
			t.Errorf("VideoSistema.java no declara %s", nombre)
			continue
		}
		if n, _ := strconv.Atoi(string(m[1])); n != quiero {
			t.Errorf("%s: %d en Java y %d en Go", nombre, n, quiero)
		}
	}
}
