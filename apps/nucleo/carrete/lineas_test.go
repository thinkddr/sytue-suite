// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package carrete

import (
	"context"
	"testing"
	"time"
)

// TestDeLineas: lo que devuelven los puentes de Android y PhotoKit. Sin tipo, se saca del
// nombre; sin tamaño, -1; y una línea rota no se lleva a las demás.
func TestDeLineas(t *testing.T) {
	texto := "41\tIMG_1.HEIC\t\t1714564800000\t-1\n" +
		"roto\n" +
		"42\tIMG_2.jpg\timage/jpeg\t1714568400000\t2048\n" +
		"v43\tVID_3.MOV\t\t1714568400000\t-1\tv\n\n"
	l, err := deLineas(context.Background(), texto)
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 3 {
		t.Fatalf("quiero 2 fotos y un vídeo, hay %d: %+v", len(l), l)
	}
	if !l[2].Video || l[2].Tipo != "video/quicktime" || l[0].Video || l[1].Video {
		t.Errorf("el vídeo no se distingue: %+v", l)
	}
	if l[0].ID != "41" || l[0].Tipo != "image/heic" || l[0].Tamano != -1 ||
		!l[0].Tomada.Equal(time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("primera mal: %+v", l[0])
	}
	if l[1].Tipo != "image/jpeg" || l[1].Tamano != 2048 {
		t.Errorf("segunda mal: %+v", l[1])
	}
}

// TestDeLineasConLaDuracionDeLosVideos: los puentes ponen detrás de la «v» la duración en
// milisegundos y los lados del vídeo (MediaStore, PhotoKit). Lo que no traen o no se lee
// se queda a cero, y una foto no los toma aunque los lleve.
func TestDeLineasConLaDuracionDeLosVideos(t *testing.T) {
	texto := "v7\tVID_1.mp4\tvideo/mp4\t1714568400000\t9876543\tv\t83500\t1920\t1080\n" +
		"v8\tVID_2.mov\t\t1714568400000\t-1\tv\t1200\n" +
		"v9\tVID_3.mkv\t\t1714568400000\t-1\tv\tno\t-5\t720\n" +
		"10\tIMG_4.jpg\timage/jpeg\t1714568400000\t10\t\t500\t3\t4\n" +
		"11\tdemasiadas\t\t1\t1\tv\t1\t1\t1\textra\n"
	l, err := deLineas(context.Background(), texto)
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 4 {
		t.Fatalf("quiero cuatro, hay %d: %+v", len(l), l)
	}
	if v := l[0]; !v.Video || v.Duracion != 83500*time.Millisecond || v.Ancho != 1920 || v.Alto != 1080 || v.Tamano != 9876543 {
		t.Errorf("el MP4: %+v", v)
	}
	if v := l[1]; !v.Video || v.Tipo != "video/quicktime" || v.Duracion != 1200*time.Millisecond || v.Ancho != 0 || v.Alto != 0 {
		t.Errorf("el MOV sin lados: %+v", v)
	}
	if v := l[2]; !v.Video || v.Duracion != 0 || v.Ancho != 0 || v.Alto != 720 {
		t.Errorf("lo que no se lee es cero: %+v", v)
	}
	if f := l[3]; f.Video || f.Duracion != 0 || f.Ancho != 0 {
		t.Errorf("una foto no tiene duración: %+v", f)
	}
}
