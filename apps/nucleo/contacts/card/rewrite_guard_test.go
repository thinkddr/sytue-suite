// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package card

import (
	"errors"
	"strings"
	"testing"
)

func TestGuardVCardRewrite(t *testing.T) {
	card := func(lines string) string {
		return "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada\r\n" + lines + "\r\nEND:VCARD\r\n"
	}
	accepted := map[string]string{
		"text quotes":           card(`NOTE:Name "Ada", Countess`),
		"simple quoted param":   card(`EMAIL;TYPE="work":ada@example.invalid`),
		"caret escapes":         card(`X-ENTRY;X-LABEL="a^'^^^nb":value`),
		"group":                 card(`item1.EMAIL;TYPE=work:ada@example.invalid`),
		"folded content":        card("NOTE:one\r\n two"),
		"folded utf8":           card("NOTE:Jos\xc3\r\n \xa9"),
		"lf version 3":          "BEGIN:VCARD\nVERSION:3.0\nFN:Ada\nEND:VCARD\n",
		"long unbroken photo":   card("PHOTO:data:image/png;base64," + strings.Repeat("A", 128<<10)),
		"long folded property":  card("NOTE:" + strings.Repeat("a", 64<<10) + "\r\n " + strings.Repeat("b", 64<<10)),
		"plain structured data": card(`N:Lovelace;Ada;;;`),
	}
	for name, raw := range accepted {
		t.Run(name, func(t *testing.T) {
			if err := guardVCardRewrite(raw, len(raw)); err != nil {
				t.Fatalf("unexpected rejection: %v", err)
			}
		})
	}

	rejected := map[string]string{
		"quoted colon":          card(`ADR;GEO="geo:12,34":;;Main;Madrid;;;ES`),
		"quoted semicolon":      card(`ADR;LABEL="floor;2":;;Main;Madrid;;;ES`),
		"quoted comma":          card(`X-ENTRY;X-LABEL="a,b":value`),
		"quoted backslash":      card(`X-ENTRY;X-PATH="C:\folder\":value`),
		"quoted values list":    card(`X-ENTRY;X-LABEL="a","b":value`),
		"unquoted values list":  card(`TEL;TYPE=work,voice:123`),
		"folded quoted colon":   card("ADR;GEO=\"geo\r\n :12\":;;Main;Madrid;;;ES"),
		"escaped name":          card(`N:Doe\;Smith;Ada;;;`),
		"escaped address":       card(`ADR:;;Calle\; Mayor;Madrid;;;ES`),
		"name component list":   card(`N:Doe,Smith;Ada;;;`),
		"categories list":       card(`CATEGORIES:one,two`),
		"unterminated quote":    card(`X-ENTRY;X-LABEL="a:value`),
		"no equals":             card(`X-ENTRY;X-LABEL:value`),
		"no colon":              card(`X-ENTRY;X-LABEL=value`),
		"discarded quoted list": card(`X-ENTRY;X-LABEL="a","b":value`),
		"missing end":           "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada\r\n",
		"unterminated fold":     "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada\r\nEND:V\r\n CARD",
		"duplicate version":     card(`VERSION:4.0`),
		"unsupported version":   "BEGIN:VCARD\r\nVERSION:2.1\r\nFN:Ada\r\nEND:VCARD\r\n",
		"two cards":             card(`NOTE:first`) + card(`NOTE:second`),
		"invalid utf8":          card("NOTE:\xff"),
		"nul control":           card("NOTE:a\x00b"),
		"delete control":        card("NOTE:a\x7fb"),
	}
	for name, raw := range rejected {
		t.Run(name, func(t *testing.T) {
			if err := guardVCardRewrite(raw, len(raw)); !errors.Is(err, errUnsafeVCardRewrite) {
				t.Fatalf("got %v, want unsafe rewrite error", err)
			}
		})
	}
	if err := guardVCardRewrite(card("NOTE:one"), 1); !errors.Is(err, errUnsafeVCardRewrite) {
		t.Fatalf("size bound: got %v, want unsafe rewrite error", err)
	}
}
