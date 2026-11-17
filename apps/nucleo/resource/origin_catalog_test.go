// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestResourceOriginServiceUsesClosedProductCatalog(t *testing.T) {
	for _, product := range storage.Products() {
		want := product
		if product == "docs" || product == "sheets" || product == "slides" {
			want = "docs"
		}
		actual := resource.OriginServiceID(product)
		if actual != want || strings.ContainsAny(actual, ":/") {
			t.Fatal("product selected another service", product, actual)
		}
	}
	for _, input := range []string{"", "unknown", "https://attacker.invalid", "docs/../mail", "DOCS"} {
		if resource.OriginServiceID(input) != "" {
			t.Fatal("request URL or unknown product selected source", input)
		}
	}
}
