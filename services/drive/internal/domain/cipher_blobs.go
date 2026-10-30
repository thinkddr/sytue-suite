// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "git.sytue.eu/sytue/one/apps/nucleo/storage"

type CipherBlob struct {
	storage.BlobReceipt
	StorageKey string `json:"-"`
	OwnerSub   string `json:"-"`
}
