// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (p *Photos) PrivateSourcePlanRouting(ctx context.Context, scope storage.Scope) (model.SourcePlanRouting, error) {
	request, err := privateAppRequest(scope, "photos", "/private-storage/plan")
	if err != nil {
		return model.SourcePlanRouting{}, err
	}
	result, err := JSON[model.SourcePlanRouting](ctx, p.c, request)
	if err == nil {
		err = result.Validate(scope)
	}
	return result, err
}

func (p *Photos) PrivateSourcePreflight(ctx context.Context, scope storage.Scope, epoch string) (model.SourcePreflight, error) {
	request, err := privateAppRequest(scope, "photos", "/private-storage/preflight")
	if err != nil || !storage.MigrationEpoch(epoch) {
		return model.SourcePreflight{}, storage.ErrRecord
	}
	request.Method = http.MethodPost
	request.Header.Set("Content-Type", "application/json")
	request.Body, _ = json.Marshal(struct {
		Epoch string `json:"epoch"`
	}{Epoch: epoch})
	result, err := JSON[model.SourcePreflight](ctx, p.c, request)
	if err == nil {
		err = result.Validate(scope, epoch)
	}
	return result, err
}

func (p *Photos) AcceptSourcePlan(ctx context.Context, scope storage.Scope, epoch, cas string, link storage.Reference) (model.SourcePlanReference, error) {
	request, err := privateAppRequest(scope, "photos", "/private-storage/plan")
	if err != nil || !storage.MigrationEpoch(epoch) || cas == "" || len(cas) > 4096 || !utf8.ValidString(cas) || !link.Valid() {
		return model.SourcePlanReference{}, storage.ErrRecord
	}
	request.Method = http.MethodPost
	request.Header.Set("Content-Type", "application/json")
	request.Body, _ = json.Marshal(struct {
		Epoch string            `json:"epoch"`
		CAS   string            `json:"cas"`
		Link  storage.Reference `json:"link"`
	}{Epoch: epoch, CAS: cas, Link: link})
	result, err := JSON[model.SourcePlanReference](ctx, p.c, request)
	if err == nil && (!result.Valid() || result.Epoch != epoch || result.Link != link) {
		err = storage.ErrRecord
	}
	return result, err
}
