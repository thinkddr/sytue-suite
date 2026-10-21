// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"archive/zip"
	"bytes"
	"math/rand/v2"
	"strings"
	"testing"
)

// TestContarZipSumaLoDescomprimido: el ZIP que escribe el servicio (`archive/zip`, cada
// fichero comprimido y con su descriptor detrás) se cuenta en streaming y la suma es
// exactamente lo que ocupan los ficheros; también con uno vacío y uno incompresible.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `saltarDescriptor`, el segundo fichero
// no se encuentra y la suma se queda en el primero.
func TestContarZipSumaLoDescomprimido(t *testing.T) {
	var zipado bytes.Buffer
	zw := zip.NewWriter(&zipado)
	azar := make([]byte, 300_000)
	for i := range azar {
		azar[i] = byte(rand.IntN(256))
	}
	contenidos := [][]byte{[]byte(strings.Repeat("acta de la junta ", 5000)), {}, azar, []byte("fin")}
	var total int64
	for i, c := range contenidos {
		w, err := zw.Create("carpeta/fichero-" + string(rune('a'+i)) + ".txt")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(c)
		total += int64(len(c))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	var contado int64
	if err := ContarZip(bytes.NewReader(zipado.Bytes()), func(n int64) { contado += n }); err != nil {
		t.Fatal(err)
	}
	if contado != total {
		t.Errorf("se han contado %d bytes y los ficheros suman %d", contado, total)
	}
}

// TestLaBarraCuentaLoCompleto: la barra es lo COMPLETO más lo que lleva el de ahora, como
// el `done` de la web; el texto sí cuenta desde 1. Con uno solo a medias, media barra.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con la cuenta de antes (el número que se
// enseña, que empieza en 1), una descarga a medias pintaba la barra llena.
func TestLaBarraCuentaLoCompleto(t *testing.T) {
	p := NuevoProgreso(1)
	p.Empezar(0, "copia.zip")
	p.Avanzar(0.5)
	if f := p.Instantanea().Fraccion(); f != 0.5 {
		t.Errorf("a medias la barra va a %v", f)
	}
	q := NuevoProgreso(3)
	q.Empezar(1, "b")
	if i := q.Instantanea(); i.Actual != 2 || i.Fraccion() != float32(1)/3 {
		t.Errorf("el segundo de tres: %+v, barra %v", i, i.Fraccion())
	}
}
