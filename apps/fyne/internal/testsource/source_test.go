// SPDX-License-Identifier: Apache-2.0
package testsource

import "testing"

func TestIntegrationOnlyNeverExcludesProductionOSOrFeatureAlternatives(t *testing.T) {
	for _, entry := range []struct {
		tag      string
		expected bool
	}{
		{"", false},
		{"integration && cgo", true},
		{"integration", true},
		{"integration && (ios || android)", true},
		{"integration || ios", false},
		{"ios", false},
		{"android && cgo", false},
		{"darwin && !tienda", false},
		{"!integration", false},
		{"(integration && ios) || (integration && cgo)", true},
		{"(integration && ios) || (ci && cgo)", false},
	} {
		t.Run(entry.tag, func(t *testing.T) {
			source := "// SPDX-License-Identifier: Apache-2.0\n"
			if entry.tag != "" {
				source += "//go:build " + entry.tag + "\n\n"
			}
			source += "package example\n"
			got, err := IntegrationOnly([]byte(source))
			if err != nil || got != entry.expected {
				t.Fatal("wrong production classification", got, err)
			}
		})
	}
	for _, header := range []string{"//go:build integration &&\n", "//go:build integration\n//go:build cgo\n"} {
		if only, err := IntegrationOnly([]byte(header + "package example\n")); err == nil || only {
			t.Fatal("invalid constraint hid a source file")
		}
	}
}
