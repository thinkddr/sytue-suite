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

func cipherBlobRequest(scope storage.Scope, id string) (string, http.Header, error) {
	value, err := storage.EncodeScopeHeader(scope)
	if err != nil || !storage.BlobID(id) {
		return "", nil, storage.ErrRecord
	}
	return "/cipher/blobs/" + url.PathEscape(scope.Product) + "/" + url.PathEscape(id), http.Header{storage.ScopeHeader: {value}}, nil
}

// PutCipherBlob accepts already-encrypted bytes only. Applications use the
// maintained crypto adapter; keys/manifests are never arguments to this API.
func (d *Drive) PutCipherBlob(ctx context.Context, scope storage.Scope, id string, body io.Reader, size int64) (storage.BlobReceipt, error) {
	path, headers, err := cipherBlobRequest(scope, id)
	if err != nil {
		return storage.BlobReceipt{}, err
	}
	if size < 0 || size > storage.MaxCipherBlockBytes {
		return storage.BlobReceipt{}, storage.ErrRecord
	}
	headers.Set("Content-Type", "application/octet-stream")
	response, err := d.c.Stream(ctx, http.MethodPut, path, nil, headers, body, size)
	if err != nil {
		return storage.BlobReceipt{}, err
	}
	defer func() { _ = response.Body.Close() }()
	var receipt storage.BlobReceipt
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&receipt); err != nil {
		return receipt, err
	}
	if !receipt.Valid(scope.Product, id) || receipt.SizeBytes != size {
		return storage.BlobReceipt{}, storage.ErrRecord
	}
	return receipt, nil
}

func (d *Drive) CipherBlob(ctx context.Context, scope storage.Scope, id string) (storage.BlobReceipt, error) {
	path, headers, err := cipherBlobRequest(scope, id)
	if err != nil {
		return storage.BlobReceipt{}, err
	}
	receipt, err := JSON[storage.BlobReceipt](ctx, d.c, Request{Path: path, Header: headers})
	if err == nil && !receipt.Valid(scope.Product, id) {
		return storage.BlobReceipt{}, storage.ErrRecord
	}
	return receipt, err
}

func (d *Drive) ReadCipherBlob(ctx context.Context, scope storage.Scope, id string) (io.ReadCloser, error) {
	path, headers, err := cipherBlobRequest(scope, id)
	if err != nil {
		return nil, err
	}
	response, err := d.c.Do(ctx, Request{Path: path + "/content", Header: headers})
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

func (d *Drive) DeleteCipherBlob(ctx context.Context, scope storage.Scope, id string) error {
	path, headers, err := cipherBlobRequest(scope, id)
	if err != nil {
		return err
	}
	response, err := d.c.Do(ctx, Request{Method: http.MethodDelete, Path: path, Header: headers})
	if err != nil {
		return err
	}
	return response.Body.Close()
}
