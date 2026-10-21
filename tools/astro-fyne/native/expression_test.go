// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package webui

import (
	"math"
	"strings"
	"testing"
)

func TestNumberRadixStringsRoundBeyondUint64LikeJavaScript(t *testing.T) {
	for _, value := range []string{
		"0x10000000000000000",
		"0X10000000000000000",
		"0b1" + strings.Repeat("0", 64),
		"0B1" + strings.Repeat("0", 64),
		"0o2" + strings.Repeat("0", 21),
		"0O2" + strings.Repeat("0", 21),
	} {
		if got := Number(value); got != math.Ldexp(1, 64) {
			t.Fatalf("Number(%q)=%g, expected2^64", value, got)
		}
	}
	// Exactly halfway between binary64 integers above 2^53 rounds to even.
	if got := Number("0x20000000000001"); got != math.Ldexp(1, 53) {
		t.Fatalf("large radix integer did not round once to nearest even: %g", got)
	}
	for _, value := range []string{
		"0x1" + strings.Repeat("0", 256),
		"0b1" + strings.Repeat("0", 1024),
		"0o1" + strings.Repeat("0", 342),
	} {
		if !math.IsInf(Number(value), 1) {
			t.Fatalf("Number(%q) should overflow to +Infinity", value)
		}
	}
}

func TestNumberDecimalOverflowAndUnderflowRemainNumeric(t *testing.T) {
	for _, value := range []string{"1e309", "+1e309", "Infinity", "+Infinity"} {
		if !math.IsInf(Number(value), 1) {
			t.Fatalf("Number(%q)=%g, expected+Infinity", value, Number(value))
		}
	}
	for _, value := range []string{"-1e309", "-Infinity"} {
		if !math.IsInf(Number(value), -1) {
			t.Fatalf("Number(%q)=%g, expected-Infinity", value, Number(value))
		}
	}
	if got := Number("-1e-4000"); got != 0 || !math.Signbit(got) {
		t.Fatalf("decimal underflow lost signed zero: %v", got)
	}
	for value, want := range map[string]float64{
		"": 0, "00012": 12, "+.5": 0.5, "1.": 1, "1.e2": 100,
		"\ufeff\u00a0\t12\u2028\u2029": 12,
	} {
		if got := Number(value); got != want {
			t.Fatalf("Number(%q)=%g, expected%g", value, got, want)
		}
	}
}

func TestNumberRejectsGoOnlyLiteralSpellingsAndSignedRadix(t *testing.T) {
	for _, value := range []string{
		"Inf", "+Inf", "-Inf", "inf", "infinity", "INFINITY", "0x1p2",
		"0x+1", "0x-1", "+0x1", "-0x1", "0b+1", "0o-1", "0b2", "0o8",
		"0x", "0b", "0o", "1_000", "1e", ".", "+", "12n", "\u008512\u0085",
	} {
		if got := Number(value); !math.IsNaN(got) {
			t.Fatalf("Number(%q)=%g, expectedNaN", value, got)
		}
	}
}

func TestStringRelationalOperatorsUseUTF16CodeUnitOrdering(t *testing.T) {
	for _, pair := range [][2]string{
		{"\U00010000", "\ue000"},
		{"\U00020000", "\ufffd"},
		{"prefix\U00010000", "prefix\ue000"},
		{"\U00010000", "\U00010001"},
		{"\U00010000", "\U00010000a"},
		{"", "a"},
	} {
		a, b := pair[0], pair[1]
		for operator, want := range map[string]bool{"<": true, "<=": true, ">": false, ">=": false} {
			if got := Binary(operator, a, b); got != want {
				t.Fatalf("%q %s %q=%v, expected%v", a, operator, b, got, want)
			}
		}
		if Binary(">", b, a) != true || Binary("<=", b, a) != false {
			t.Fatalf("reverse Unicode comparison did not reverse the order: %q,%q", a, b)
		}
	}
	for operator, want := range map[string]bool{"<": false, "<=": true, ">": false, ">=": true} {
		if got := Binary(operator, "\U00010000", "\U00010000"); got != want {
			t.Fatalf("equal strings %s = %v, expected%v", operator, got, want)
		}
	}
}

func TestStringNumberProjectionKeepsJavaScriptNotationThresholds(t *testing.T) {
	for _, value := range []struct {
		number float64
		want   string
	}{
		{math.Copysign(0, -1), "0"},
		{1e-6, "0.000001"},
		{-1e-6, "-0.000001"},
		{1e-7, "1e-7"},
		{-1e-7, "-1e-7"},
		{1e20, "100000000000000000000"},
		{-1e20, "-100000000000000000000"},
		{1e21, "1e+21"},
		{-1e21, "-1e+21"},
		{1.25e22, "1.25e+22"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
		{math.NaN(), "NaN"},
	} {
		if got := String(value.number); got != value.want {
			t.Fatalf("String(%g)=%q, expected%q", value.number, got, value.want)
		}
	}
}
