// SPDX-License-Identifier: Apache-2.0
package resource

import "git.sytue.eu/sytue/one/apps/nucleo/storage"

// OriginServiceID selects the backend that owns the ORIGINAL resource SQL.
// This is a closed product catalog, never a URL or service name from a request.
func OriginServiceID(product string) string {
	if !storage.Product(product) {
		return ""
	}
	switch product {
	case "docs", "sheets", "slides":
		return "docs"
	default:
		return product
	}
}
