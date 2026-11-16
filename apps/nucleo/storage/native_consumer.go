// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

// NativeReturnApp keeps a native consumer separate from the data product. Only
// registered suite apps may consume Drive data; the normal product flow keeps
// its original empty consumer. This URL/account routing does not grant OS UID
// authority or change the private-product catalog.
func NativeReturnApp(product, consumer string) string {
	if consumer == "" {
		return NativeApp(product)
	}
	if product != "drive" || consumer == product {
		return ""
	}
	return NativeApp(consumer)
}
