// SPDX-License-Identifier: Apache-2.0
package generated

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2/theme"
)

func TestThemeIsGeneratedFromCapturedAstroCustomProperties(t *testing.T) {
	generated, err := NewGeometryTheme(theme.DefaultTheme())
	if err != nil {
		t.Fatal(err)
	}
	want := color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 255}
	if got := generated.Color(theme.ColorNamePrimary, theme.VariantLight); got != want {
		t.Fatalf("captured primary color: got %v, want %v", got, want)
	}
	if got := generated.Size(theme.SizeNamePadding); got != 5 {
		t.Fatalf("captured padding: got %v, want 5", got)
	}
	if got, ok := generated.CSS("--component-density"); !ok || got != "compact" {
		t.Fatalf("CSS token inventory was lost: %q", got)
	}
}
