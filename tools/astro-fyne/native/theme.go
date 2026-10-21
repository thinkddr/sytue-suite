// SPDX-License-Identifier: Apache-2.0

package webui

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// CapturedTheme overlays captured CSS tokens on a public Fyne theme. Tokens are a
// copied snapshot; unrecognized custom properties remain available through CSS.
// Fonts, Colors and Sizes also accept explicit application overrides.
type CapturedTheme struct {
	Base   fyne.Theme
	Tokens map[string]string
	Colors map[fyne.ThemeColorName]color.Color
	Sizes  map[fyne.ThemeSizeName]float32
	Fonts  map[fyne.TextStyle]fyne.Resource
}

var _ fyne.Theme = (*CapturedTheme)(nil)

// NewCapturedTheme derives exact --fyne-color-<name> and --fyne-size-<name>
// overrides and the four conventional --color-* aliases. Unsupported values in
// mapped tokens are errors; arbitrary other tokens are retained verbatim.
// An explicit --fyne-color-* override takes precedence over a conventional alias.
func NewCapturedTheme(tokens map[string]string, base fyne.Theme) (*CapturedTheme, error) {
	t := &CapturedTheme{
		Base:   base,
		Tokens: make(map[string]string, len(tokens)),
		Colors: make(map[fyne.ThemeColorName]color.Color),
		Sizes:  make(map[fyne.ThemeSizeName]float32),
		Fonts:  make(map[fyne.TextStyle]fyne.Resource),
	}
	for name, value := range tokens {
		t.Tokens[name] = value
	}
	colors, sizes, err := capturedOverrides(t.Tokens)
	if err != nil {
		return nil, err
	}
	t.Colors, t.Sizes = colors, sizes
	return t, nil
}

// CSS returns the original custom property, including its exact spelling and
// unit. Absence is distinguishable from a present empty custom property.
func (t *CapturedTheme) CSS(name string) (string, bool) {
	if t == nil {
		return "", false
	}
	value, ok := t.Tokens[name]
	return value, ok
}

func (t *CapturedTheme) base() fyne.Theme {
	if t != nil && t.Base != nil {
		return t.Base
	}
	return theme.DefaultTheme()
}

// Color returns a captured color for either variant; captures represent one
// explicit browser theme state. Other colors preserve the base variant behavior.
func (t *CapturedTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if t != nil {
		if value, ok := t.Colors[name]; ok && value != nil {
			return value
		}
	}
	return t.base().Color(name, variant)
}

// Font preserves each captured/explicit font resource and otherwise delegates.
func (t *CapturedTheme) Font(style fyne.TextStyle) fyne.Resource {
	if t != nil {
		if value, ok := t.Fonts[style]; ok && value != nil {
			return value
		}
	}
	return t.base().Font(style)
}

// Icon delegates to the base; CSS color and size tokens do not define icons.
func (t *CapturedTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base().Icon(name)
}

// Size preserves fractional CSS pixels without substituting a Fyne scale.
func (t *CapturedTheme) Size(name fyne.ThemeSizeName) float32 {
	if t != nil {
		if value, ok := t.Sizes[name]; ok {
			return value
		}
	}
	return t.base().Size(name)
}

// Validate rejects unsupported mapped tokens and invalid manually set overrides.
// It does not reinterpret arbitrary tokens or mutate an application's overrides.
func (t *CapturedTheme) Validate() error {
	if t == nil {
		return fmt.Errorf("captured theme is nil")
	}
	if base, ok := t.Base.(*CapturedTheme); ok && base == t {
		return fmt.Errorf("captured theme cannot delegate to itself")
	}
	if _, _, err := capturedOverrides(t.Tokens); err != nil {
		return err
	}
	for name, value := range t.Colors {
		if name == "" || value == nil {
			return fmt.Errorf("invalid captured color override %q", name)
		}
	}
	for name, value := range t.Sizes {
		if name == "" || value < 0 || math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("invalid captured size override %q", name)
		}
	}
	for style, font := range t.Fonts {
		if font == nil {
			return fmt.Errorf("nil captured font override for %+v", style)
		}
	}
	return nil
}

func capturedOverrides(tokens map[string]string) (map[fyne.ThemeColorName]color.Color, map[fyne.ThemeSizeName]float32, error) {
	colors := make(map[fyne.ThemeColorName]color.Color)
	sizes := make(map[fyne.ThemeSizeName]float32)
	aliases := []struct {
		css  string
		fyne fyne.ThemeColorName
	}{
		{"--color-background", theme.ColorNameBackground},
		{"--color-primary", theme.ColorNamePrimary},
		{"--color-text", theme.ColorNameForeground},
		{"--color-border", theme.ColorNameSeparator},
	}
	for _, alias := range aliases {
		if value, ok := tokens[alias.css]; ok {
			parsed, err := capturedColor(value)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", alias.css, err)
			}
			colors[alias.fyne] = parsed
		}
	}
	names := make([]string, 0, len(tokens))
	for name := range tokens {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		switch {
		case strings.HasPrefix(name, "--fyne-color-"):
			key := strings.TrimPrefix(name, "--fyne-color-")
			if key == "" {
				return nil, nil, fmt.Errorf("%s: color name is empty", name)
			}
			parsed, err := capturedColor(tokens[name])
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", name, err)
			}
			colors[fyne.ThemeColorName(key)] = parsed
		case strings.HasPrefix(name, "--fyne-size-"):
			key := strings.TrimPrefix(name, "--fyne-size-")
			if key == "" {
				return nil, nil, fmt.Errorf("%s: size name is empty", name)
			}
			parsed, err := capturedPixels(tokens[name])
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", name, err)
			}
			sizes[fyne.ThemeSizeName(key)] = parsed
		}
	}
	return colors, sizes, nil
}

func capturedPixels(value string) (float32, error) {
	value = strings.TrimSpace(value)
	if value == "0" {
		return 0, nil
	}
	if !strings.HasSuffix(value, "px") {
		return 0, fmt.Errorf("size %q must be a computed CSS pixel value", value)
	}
	number, err := strconv.ParseFloat(strings.TrimSuffix(value, "px"), 32)
	if err != nil || number < 0 || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("size %q must be finite, non-negative CSS pixels", value)
	}
	return float32(number), nil
}

func capturedColor(value string) (color.NRGBA, error) {
	value = strings.TrimSpace(value)
	if value == "transparent" {
		return color.NRGBA{}, nil
	}
	if strings.HasPrefix(value, "#") {
		hex := strings.TrimPrefix(value, "#")
		if len(hex) == 3 || len(hex) == 4 {
			var expanded strings.Builder
			for _, digit := range hex {
				expanded.WriteRune(digit)
				expanded.WriteRune(digit)
			}
			hex = expanded.String()
		}
		if len(hex) != 6 && len(hex) != 8 {
			return color.NRGBA{}, fmt.Errorf("color %q needs #rgb, #rgba, #rrggbb or #rrggbbaa", value)
		}
		parsed, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return color.NRGBA{}, fmt.Errorf("invalid hexadecimal color %q", value)
		}
		if len(hex) == 6 {
			return color.NRGBA{R: uint8(parsed >> 16), G: uint8(parsed >> 8), B: uint8(parsed), A: 255}, nil
		}
		return color.NRGBA{R: uint8(parsed >> 24), G: uint8(parsed >> 16), B: uint8(parsed >> 8), A: uint8(parsed)}, nil
	}
	open := strings.IndexByte(value, '(')
	if open < 0 || !strings.HasSuffix(value, ")") || (value[:open] != "rgb" && value[:open] != "rgba") {
		return color.NRGBA{}, fmt.Errorf("unsupported color %q; capture computed rgb/rgba or hexadecimal", value)
	}
	body := strings.TrimSpace(value[open+1 : len(value)-1])
	var channels []string
	alpha := "1"
	if strings.Contains(body, ",") {
		if strings.Contains(body, "/") {
			return color.NRGBA{}, fmt.Errorf("color %q mixes comma and slash syntax", value)
		}
		parts := strings.Split(body, ",")
		if len(parts) != 3 && len(parts) != 4 {
			return color.NRGBA{}, fmt.Errorf("color %q needs three channels and optional alpha", value)
		}
		channels = parts[:3]
		if len(parts) == 4 {
			alpha = parts[3]
		}
	} else {
		parts := strings.Split(body, "/")
		if len(parts) > 2 {
			return color.NRGBA{}, fmt.Errorf("invalid alpha syntax in %q", value)
		}
		channels = strings.Fields(parts[0])
		if len(parts) == 2 {
			alpha = parts[1]
		}
		if len(channels) != 3 {
			return color.NRGBA{}, fmt.Errorf("color %q needs three channels", value)
		}
	}
	result := color.NRGBA{}
	destinations := []*uint8{&result.R, &result.G, &result.B}
	for index, channel := range channels {
		parsed, err := capturedChannel(channel, 255)
		if err != nil {
			return color.NRGBA{}, fmt.Errorf("color %q: %w", value, err)
		}
		*destinations[index] = uint8(math.Round(parsed))
	}
	parsed, err := capturedChannel(alpha, 1)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("color %q: %w", value, err)
	}
	result.A = uint8(math.Round(parsed * 255))
	return result, nil
}

func capturedChannel(value string, maximum float64) (float64, error) {
	value = strings.TrimSpace(value)
	percentage := strings.HasSuffix(value, "%")
	number, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
	if percentage {
		number = number * maximum / 100
	}
	if err != nil || number < 0 || number > maximum || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("channel %q must lie between 0 and %g", value, maximum)
	}
	return number, nil
}
