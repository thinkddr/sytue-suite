// SPDX-License-Identifier: Apache-2.0

package webui

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type capturedThemeBase struct {
	font fyne.Resource
	icon fyne.Resource
}

func (b capturedThemeBase) Color(_ fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	return color.NRGBA{R: 17, G: 18, B: uint8(variant), A: 255}
}
func (b capturedThemeBase) Font(_ fyne.TextStyle) fyne.Resource     { return b.font }
func (b capturedThemeBase) Icon(_ fyne.ThemeIconName) fyne.Resource { return b.icon }
func (b capturedThemeBase) Size(_ fyne.ThemeSizeName) float32       { return 10.75 }

func TestCapturedThemeTokensAndOverrides(t *testing.T) {
	tokens := map[string]string{
		"--color-primary":         "#1234",
		"--color-background":      "rgb(12 24 48 / 50%)",
		"--color-text":            "rgba(10, 20, 30, 0.25)",
		"--color-border":          "#abc",
		"--fyne-color-primary":    "#11223380",
		"--fyne-color-customName": "rgb(100%, 0%, 50%)",
		"--fyne-size-padding":     "3.5px",
		"--fyne-size-text":        "12.25px",
		"--application-layout":    "calc(100vw - 13rem)",
		"--empty":                 "",
	}
	captured, err := NewCapturedTheme(tokens, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := captured.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name fyne.ThemeColorName
		want color.NRGBA
	}{
		{theme.ColorNamePrimary, color.NRGBA{R: 17, G: 34, B: 51, A: 128}},
		{theme.ColorNameBackground, color.NRGBA{R: 12, G: 24, B: 48, A: 128}},
		{theme.ColorNameForeground, color.NRGBA{R: 10, G: 20, B: 30, A: 64}},
		{theme.ColorNameSeparator, color.NRGBA{R: 170, G: 187, B: 204, A: 255}},
		{"customName", color.NRGBA{R: 255, G: 0, B: 128, A: 255}},
	}
	for _, test := range cases {
		if got := captured.Color(test.name, 0); got != test.want {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
	if got := captured.Size(theme.SizeNamePadding); got != 3.5 {
		t.Errorf("fractional padding was changed: %v", got)
	}
	if got := captured.Size(theme.SizeNameText); got != 12.25 {
		t.Errorf("fractional text size was changed: %v", got)
	}
	if value, ok := captured.CSS("--application-layout"); !ok || value != "calc(100vw - 13rem)" {
		t.Fatalf("uninterpreted CSS token was lost: %q, %v", value, ok)
	}
	if value, ok := captured.CSS("--empty"); !ok || value != "" {
		t.Fatalf("present empty token was lost: %q, %v", value, ok)
	}
	if _, ok := captured.CSS("--missing"); ok {
		t.Fatal("missing CSS token was fabricated")
	}
	tokens["--application-layout"] = "changed"
	if value, _ := captured.CSS("--application-layout"); value != "calc(100vw - 13rem)" {
		t.Fatal("theme tokens alias the caller's mutable map")
	}
}

func TestCapturedThemeFontAndBaseFallback(t *testing.T) {
	font := fyne.NewStaticResource("base-font", []byte("base"))
	icon := fyne.NewStaticResource("base-icon", []byte("icon"))
	base := capturedThemeBase{font: font, icon: icon}
	captured, err := NewCapturedTheme(nil, base)
	if err != nil {
		t.Fatal(err)
	}
	style := fyne.TextStyle{Bold: true, Italic: true}
	override := fyne.NewStaticResource("captured-font", []byte("captured"))
	captured.Fonts[style] = override
	if captured.Font(style) != override || captured.Font(fyne.TextStyle{}) != font {
		t.Fatal("font override or base fallback was lost")
	}
	if captured.Icon("confirm") != icon || captured.Size("unmapped") != 10.75 {
		t.Fatal("icon or size did not delegate to the supplied base")
	}
	if got := captured.Color("unmapped", 1); got != base.Color("unmapped", 1) {
		t.Fatalf("fallback did not retain theme variant: %v", got)
	}
	defaults, err := NewCapturedTheme(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := defaults.Size(theme.SizeNamePadding), theme.DefaultTheme().Size(theme.SizeNamePadding); got != want {
		t.Fatalf("missing base did not use the Fyne default: %v != %v", got, want)
	}
}

func TestCapturedThemeRejectsUnsupportedMappedTokens(t *testing.T) {
	cases := []struct{ name, value string }{
		{"--fyne-size-padding", "1rem"},
		{"--fyne-size-padding", "calc(2px + 1px)"},
		{"--fyne-size-padding", "NaNpx"},
		{"--fyne-size-padding", "-1px"},
		{"--fyne-size-padding", "1e99px"},
		{"--fyne-size-", "1px"},
		{"--fyne-color-primary", "oklch(50% 0.2 150)"},
		{"--fyne-color-primary", "var(--brand)"},
		{"--fyne-color-primary", "#12345"},
		{"--fyne-color-primary", "rgba(1, 2, 3, 1.1)"},
		{"--fyne-color-primary", "rgb(1 2 3 / NaN)"},
		{"--fyne-color-primary", "rgb(1, 2, 3 / 50%)"},
		{"--fyne-color-", "#000"},
		{"--color-primary", "not-a-color"},
	}
	for _, test := range cases {
		t.Run(test.value, func(t *testing.T) {
			captured, err := NewCapturedTheme(map[string]string{test.name: test.value}, nil)
			if err == nil || captured != nil || !strings.Contains(err.Error(), test.name) {
				t.Fatalf("unsupported mapped token accepted: %v, %v", captured, err)
			}
		})
	}
	captured, err := NewCapturedTheme(map[string]string{"--application-size": "anything"}, nil)
	if err != nil {
		t.Fatal("arbitrary non-Fyne token was interpreted:", err)
	}
	captured.Tokens["--fyne-size-padding"] = "invalid"
	if captured.Validate() == nil {
		t.Fatal("Validate ignored a newly invalid mapped token")
	}
	delete(captured.Tokens, "--fyne-size-padding")
	captured.Sizes["padding"] = float32(math.Inf(1))
	if captured.Validate() == nil {
		t.Fatal("Validate ignored a nonfinite manual override")
	}
}
