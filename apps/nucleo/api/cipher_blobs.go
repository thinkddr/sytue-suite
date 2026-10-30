// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func cipherBlobPath(product, id string) (string, error) {
	if !storage.Product(product) || !storage.BlobID(id) {
		return "", storage.ErrRecord
	}
	return "/cipher/blobs/" + url.PathEscape(product) + "/" + url.PathEscape(id), nil
}

// PutCipherBlob accepts already-encrypted bytes only. Applications use the
// maintained crypto adapter; keys/manifests are never arguments to this API.
func (d *Drive) PutCipherBlob(ctx context.Context, product, id string, body io.Reader, size int64) (storage.BlobReceipt, error) {
	path, err := cipherBlobPath(product, id)
	if err != nil {
		return storage.BlobReceipt{}, err
	}
	if size < 0 || size > storage.MaxCipherBlockBytes {
		return storage.BlobReceipt{}, storage.ErrRecord
	}
	response, err := d.c.Stream(ctx, http.MethodPut, path, nil, http.Header{"Content-Type": {"application/octet-stream"}}, body, size)
	if err != nil {
		return storage.BlobReceipt{}, err
	}
	defer func() { _ = response.Body.Close() }()
	var receipt storage.BlobReceipt
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&receipt); err != nil {
		return receipt, err
	}
	if !receipt.Valid(product, id) || receipt.SizeBytes != size {
		return storage.BlobReceipt{}, storage.ErrRecord
	}
	return receipt, nil
}

func (d *Drive) CipherBlob(ctx context.Context, product, id string) (storage.BlobReceipt, error) {
	path, err := cipherBlobPath(product, id)
	if err != nil {
		return storage.BlobReceipt{}, err
	}
	receipt, err := JSON[storage.BlobReceipt](ctx, d.c, Request{Path: path})
	if err == nil && !receipt.Valid(product, id) {
		return storage.BlobReceipt{}, storage.ErrRecord
	}
	return receipt, err
}

func (d *Drive) ReadCipherBlob(ctx context.Context, product, id string) (io.ReadCloser, error) {
	path, err := cipherBlobPath(product, id)
	if err != nil {
		return nil, err
	}
	response, err := d.c.Do(ctx, Request{Path: path + "/content"})
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

func (d *Drive) DeleteCipherBlob(ctx context.Context, product, id string) error {
	path, err := cipherBlobPath(product, id)
	if err != nil {
		return err
	}
	response, err := d.c.Do(ctx, Request{Method: http.MethodDelete, Path: path})
	if err != nil {
		return err
	}
	return response.Body.Close()
}
