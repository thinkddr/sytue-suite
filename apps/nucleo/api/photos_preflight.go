// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (p *Photos) StartPrivateSourcePreflight(ctx context.Context, scope storage.Scope, epoch string) (model.SourcePreflightInfo, error) {
	request, err := privateAppRequest(scope, "photos", "/private-storage/preflight/paged")
	if err != nil || !storage.MigrationEpoch(epoch) {
		return model.SourcePreflightInfo{}, storage.ErrRecord
	}
	request.Method = http.MethodPost
	request.Header.Set("Content-Type", "application/json")
	request.Body, _ = json.Marshal(struct {
		Epoch string `json:"epoch"`
	}{Epoch: epoch})
	result, err := JSON[model.SourcePreflightInfo](ctx, p.c, request)
	if err == nil {
		err = result.Validate(scope, epoch)
	}
	return result, err
}

func photoPreflightRequest(info model.SourcePreflightInfo, path string) (Request, error) {
	if info.Validate(info.Scope, info.Epoch) != nil {
		return Request{}, storage.ErrRecord
	}
	return privateAppRequest(info.Scope, "photos", "/private-storage/preflight/"+info.Epoch+"/"+info.CAS+path)
}

func (p *Photos) PrivateSourcePreflightPage(ctx context.Context, info model.SourcePreflightInfo, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	request, err := photoPreflightRequest(info, "/rows")
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	cursor, err := storage.EncodeSourceCursor(after)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	request.Query = url.Values{}
	if cursor != "" {
		request.Query.Set("after", cursor)
	}
	result, err := JSON[storage.StreamSourcePage](ctx, p.c, request)
	if err == nil {
		err = result.Validate(info.Scope, info.Epoch, after)
	}
	return result, err
}

func (p *Photos) OpenPrivateSourcePreflightJSON(ctx context.Context, info model.SourcePreflightInfo, row storage.StreamSourceRow) (io.ReadCloser, error) {
	if row.Validate() != nil || !model.SourceKind(row.Kind) || row.Size > model.MaxRowBytes {
		return nil, storage.ErrRecord
	}
	request, err := photoPreflightRequest(info, "/source/"+row.Kind+"/"+url.PathEscape(row.ID))
	if err != nil {
		return nil, err
	}
	return openPrivateSourceResponse(ctx, p.c, request, row.Size, row.SHA256)
}
