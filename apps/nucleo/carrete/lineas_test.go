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
