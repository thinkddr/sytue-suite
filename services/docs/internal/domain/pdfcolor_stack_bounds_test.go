// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package domain

import (
	"reflect"
	"testing"
)

func TestPostScriptStackOperandsBounded(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   []float64
	}{
		{"1 2 3 3 1 roll", []float64{3, 1, 2}},
		{"1 2 3 3 -9223372036854775808 roll", []float64{3, 1, 2}},
		{"1 9223372036854775807 copy", []float64{1}},
		{"1 9223372036854775807 index", []float64{1}},
		{"1 2 3 9223372036854775807 1 roll", []float64{1, 2, 3}},
		{"1 2 3 3 NaN roll", []float64{1, 2, 3}},
		{"1 2 3 3 Inf roll", []float64{1, 2, 3}},
		{"1 9223372036854775807 bitshift", []float64{0}},
		{"8 -1 bitshift", []float64{4}},
		{"-1 -63 bitshift", []float64{1}},
		{"1 NaN bitshift", []float64{0}},
		{"1 Inf bitshift", []float64{0}},
		{"1 2 2 copy", []float64{1, 2, 1, 2}},
		{"1 2 0 index", []float64{1, 2, 2}},
	} {
		t.Run(tc.source, func(t *testing.T) {
			steps := 0
			stack := ejecutarPS(tokenizarPS([]byte(tc.source)), nil, &steps)
			got := make([]float64, len(stack))
			for i, value := range stack {
				got[i] = value.num
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("stack = %v; want %v", got, tc.want)
			}
		})
	}
}
