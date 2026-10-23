// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"math"
	"testing"
)

func TestConfigInt32Boundaries(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		def, lo, hi int32
		want        int32
	}{
		{name: "missing uses default", def: 3, lo: 0, hi: 20, want: 3},
		{name: "malformed uses default", input: "invalid", def: 3, lo: 0, hi: 20, want: 3},
		{name: "fraction uses default", input: "1.5", def: 3, lo: 0, hi: 20, want: 3},
		{name: "whitespace uses default", input: " 2 ", def: 3, lo: 0, hi: 20, want: 3},
		{name: "in range", input: "7", def: 3, lo: 0, hi: 20, want: 7},
		{name: "parallelism cannot be zero", input: "0", def: 1, lo: 1, hi: 50, want: 1},
		{name: "worker can be scaled to zero", input: "0", def: 1, lo: 0, hi: 20, want: 0},
		{name: "negative clamps to lower bound", input: "-1", def: 3, lo: 0, hi: 20, want: 0},
		{name: "excess clamps to upper bound", input: "51", def: 1, lo: 1, hi: 50, want: 50},
		{name: "positive int32 overflow clamps", input: "2147483648", def: 1, lo: 1, hi: 50, want: 50},
		{name: "negative int32 overflow clamps", input: "-2147483649", def: 1, lo: 0, hi: 20, want: 0},
		{name: "maximum int64 clamps", input: "9223372036854775807", def: 1, lo: 1, hi: 50, want: 50},
		{name: "minimum int64 clamps", input: "-9223372036854775808", def: 1, lo: 0, hi: 20, want: 0},
		{name: "positive int64 overflow uses default", input: "9223372036854775808", def: 3, lo: 0, hi: 20, want: 3},
		{name: "negative int64 overflow uses default", input: "-9223372036854775809", def: 3, lo: 0, hi: 20, want: 3},
		{name: "maximum int32 remains exact", input: "2147483647", lo: math.MinInt32, hi: math.MaxInt32, want: math.MaxInt32},
		{name: "minimum int32 remains exact", input: "-2147483648", lo: math.MinInt32, hi: math.MaxInt32, want: math.MinInt32},
		{name: "full range upper overflow", input: "2147483648", lo: math.MinInt32, hi: math.MaxInt32, want: math.MaxInt32},
		{name: "full range lower overflow", input: "-2147483649", lo: math.MinInt32, hi: math.MaxInt32, want: math.MinInt32},
		{name: "default below range clamps", def: -1, lo: 0, hi: 20, want: 0},
		{name: "default above range clamps", def: 51, lo: 1, hi: 50, want: 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := configInt32(tt.input, tt.def, tt.lo, tt.hi); got != tt.want {
				t.Errorf("configInt32(%q, %d, %d, %d) = %d; want %d", tt.input, tt.def, tt.lo, tt.hi, got, tt.want)
			}
		})
	}
}
