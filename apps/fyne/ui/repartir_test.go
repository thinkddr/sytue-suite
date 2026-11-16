// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"reflect"
	"testing"
)

// TestRepartirEnLineasEsElFlexWrap: cada elemento entra en la línea si cabe con su
// hueco; el que no, abre otra, y el primero de una línea va aunque no quepa él solo.
func TestRepartirEnLineasEsElFlexWrap(t *testing.T) {
	ocho := func(int) float32 { return 8 }
	casos := []struct {
		nombre string
		anchos []float32
		cabe   float32
		hueco  func(int) float32
		quiero [][]int
	}{
		{"todo cabe", []float32{100, 100}, 208, ocho, [][]int{{0, 1}}},
		{"el hueco no cabe", []float32{100, 100}, 207, ocho, [][]int{{0}, {1}}},
		{"uno más ancho que la línea", []float32{50, 400, 50}, 300, ocho, [][]int{{0}, {1}, {2}}},
		{"sin hueco", []float32{100, 100, 100}, 200, nil, [][]int{{0, 1}, {2}}},
		{"hueco por índice", []float32{10, 10, 10}, 40, func(i int) float32 { return float32(i) * 5 }, [][]int{{0, 1}, {2}}},
		{"nada", nil, 100, ocho, nil},
	}
	for _, c := range casos {
		if got := RepartirEnLineas(c.anchos, c.cabe, c.hueco); !reflect.DeepEqual(got, c.quiero) {
			t.Errorf("%s: %v y tienen que ser %v", c.nombre, got, c.quiero)
		}
	}
}
