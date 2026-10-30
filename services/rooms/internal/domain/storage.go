// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "git.sytue.eu/sytue/one/apps/nucleo/storage"

// StorageProduct is a routing namespace, never an object title or filename.
func StorageProduct(product string) bool {
	return storage.Product(product)
}
