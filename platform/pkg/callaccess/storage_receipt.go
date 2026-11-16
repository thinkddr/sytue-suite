// SPDX-License-Identifier: Apache-2.0
package callaccess

import (
	"context"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type (
	StorageReceiptVerifier interface {
		VerifyStorageReceipt(context.Context, storage.Scope, storage.PublicationTarget, storage.Reference) error
	}
	PrivateIngressAuthority interface {
		StorageAuthorityReader
		StorageReceiptVerifier
	}
)

func (c *Client) VerifyStorageReceipt(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, receipt storage.Reference) error {
	if !scope.Valid() || !target.Valid() || !receipt.Valid() || receipt.RoomID != target.RoomID {
		return ErrUnavailable
	}
	return c.request(ctx, http.MethodPost, "/internal/storage-authority/receipt", struct {
		Scope   storage.Scope             `json:"scope"`
		Target  storage.PublicationTarget `json:"target"`
		Receipt storage.Reference         `json:"receipt"`
	}{scope, target, receipt}, nil)
}
