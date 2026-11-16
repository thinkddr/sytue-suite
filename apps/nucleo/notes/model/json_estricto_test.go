// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestJSONUnicoPreservesNumbersAndDistinguishesStructsFromMaps(t *testing.T) {
	structs := func(path []string) bool {
		return len(path) == 0 || len(path) == 1 && strings.EqualFold(path[0], "input") ||
			len(path) == 2 && strings.EqualFold(path[0], "items") && path[1] == "*"
	}
	for _, raw := range []string{
		`{"input":{"data":{"A":900719925474099312345,"a":1.234567890123456789e+99}}}`,
		`{"items":[{"title":"one"},{"title":"two"}]}`,
	} {
		before := raw
		if err := ValidarJSONSinClavesDuplicadas([]byte(raw), structs); err != nil || raw != before {
			t.Fatalf("valid JSON rejected or changed: %v", err)
		}
	}
	for _, raw := range []string{
		`{"input":{"data":{"a":1,"a":2}}}`,
		`{"input":{"title":"a","Title":"b"}}`,
		`{"items":[{"state":1,"ſtate":2}]}`,
		`{"x":1} {"x":2}`,
		`{"x":`,
	} {
		if err := ValidarJSONSinClavesDuplicadas([]byte(raw), structs); !errors.Is(err, ErrJSONAmbiguo) {
			t.Fatalf("ambiguous JSON accepted %q: %v", raw, err)
		}
	}
}

func TestJSONUnicoBoundsDepthKeysAndInput(t *testing.T) {
	for _, raw := range []string{
		strings.Repeat("[", maxStrictJSONDepth+2) + "0" + strings.Repeat("]", maxStrictJSONDepth+2),
		`{"` + strings.Repeat("k", maxStrictJSONKeyBytes+1) + `":0}`,
		strings.Repeat(" ", maxStrictJSONBytes+1),
		`[` + strings.Repeat(`0,`, maxStrictJSONTokens) + `0]`,
	} {
		if err := ValidarJSONSinClavesDuplicadas([]byte(raw), nil); !errors.Is(err, ErrJSONAmbiguo) {
			t.Fatal("resource limit not enforced", err)
		}
	}
}

func TestJSONUnicoRejectsInvalidUTF8WithoutNormalizing(t *testing.T) {
	raw := append([]byte(`{"data":"`), 0xff)
	raw = append(raw, []byte(`"}`)...)
	before := append([]byte(nil), raw...)
	if err := ValidarJSONSinClavesDuplicadas(raw, nil); !errors.Is(err, ErrJSONAmbiguo) {
		t.Fatal("invalid UTF-8 accepted", err)
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("source was modified")
	}
}

func TestJSONUnicoRejectsUnpairedSurrogateEscapes(t *testing.T) {
	for _, raw := range []string{`{"s":"\ud800"}`, `{"s":"\udc00"}`, `{"s":"\ud800\u0041"}`} {
		if err := ValidarJSONSinClavesDuplicadas([]byte(raw), nil); !errors.Is(err, ErrJSONAmbiguo) {
			t.Fatalf("accepted %q: %v", raw, err)
		}
	}
	for _, raw := range []string{`{"s":"\ud83d\ude00"}`, `{"s":"\\ud800"}`} {
		if err := ValidarJSONSinClavesDuplicadas([]byte(raw), nil); err != nil {
			t.Fatalf("rejected %q: %v", raw, err)
		}
	}
}
