// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Un «no lo leo» del sistema (MediaCodec sin recursos un momento) se vuelve a probar al
// rato; un «sí» se recuerda.
func TestElLectorDelSistemaReintentaLoQueNoLeyo(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "v.mp4")
	if err := os.WriteFile(ruta, []byte("vídeo"), 0o600); err != nil {
		t.Fatal(err)
	}
	ahora := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	intentos, puede := 0, false
	l := &lectorSistema{
		ahora: func() time.Time { return ahora },
		abrir: func(string) error {
			intentos++
			if !puede {
				return errors.New("sin recursos de códec")
			}
			return nil
		},
	}
	if l.lee(ruta) || intentos != 1 {
		t.Fatal("primer intento", intentos)
	}
	puede = true
	if l.lee(ruta) || intentos != 1 {
		t.Fatal("al momento debería recordar el «no» sin volver a abrir", intentos)
	}
	ahora = ahora.Add(reintentoSistema + time.Second)
	if !l.lee(ruta) || intentos != 2 {
		t.Fatal("pasado el rato debería volver a probar", intentos)
	}
	ahora = ahora.Add(time.Hour)
	if !l.lee(ruta) || intentos != 2 {
		t.Fatal("el «sí» se recuerda", intentos)
	}
}
