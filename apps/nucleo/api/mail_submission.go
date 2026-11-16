// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type PrivateMailSubmissionTransport interface {
	Submit(context.Context, model.Submission, io.Reader, int64) (model.SubmissionReceipt, error)
	Receipt(context.Context, string) (model.SubmissionReceipt, error)
	Register(context.Context, model.SubmissionRegistration) (model.SubmissionReceipt, error)
	Control(context.Context, model.SubmissionControlChange) (model.SubmissionReceipt, error)
}

type mailSubmissionTransport struct {
	client *Client
	scope  storage.Scope
	epoch  string
}

// PrivateSubmissions grants only the explicit transient SMTP port. It never
// reopens the private facade's legacy send, upload or account endpoints.
func (m *Mail) PrivateSubmissions(scope storage.Scope, epoch string) (PrivateMailSubmissionTransport, error) {
	if m == nil || m.c == nil || m.data != nil || !scope.Valid() || scope.Product != "mail" || !storage.MigrationEpoch(epoch) {
		return nil, ErrPrivateMailOperation
	}
	httpClient := *m.c.http
	httpClient.Jar = nil
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &mailSubmissionTransport{client: New(m.c.base, m.c.session, &httpClient), scope: scope, epoch: epoch}, nil
}

func (t *mailSubmissionTransport) Submit(ctx context.Context, input model.Submission, raw io.Reader, size int64) (model.SubmissionReceipt, error) {
	var out model.SubmissionReceipt
	if input.Validate() != nil || input.Generation <= 0 || input.Epoch != t.epoch || size <= 0 || raw == nil {
		return out, model.ErrSubmission
	}
	request, err := privateAppRequest(t.scope, "mail", "/private-storage/submissions")
	if err != nil {
		return out, err
	}
	metadata, err := json.Marshal(input)
	if err != nil {
		return out, err
	}
	defer clear(metadata)
	request.Header.Set(model.SubmissionHeader, base64.RawURLEncoding.EncodeToString(metadata))
	request.Header.Set("Content-Type", "message/rfc822")
	request.Header.Set("Accept", "application/json")
	response, err := t.client.Stream(ctx, http.MethodPost, request.Path, nil, request.Header, raw, size)
	if err != nil {
		return out, err
	}
	defer response.Body.Close()
	body := &io.LimitedReader{R: response.Body, N: 64 << 10}
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&out); err != nil {
		return out, err
	}
	if body.N <= 0 || decoder.Decode(new(any)) != io.EOF {
		return out, model.ErrSubmission
	}
	return out, nil
}

func (t *mailSubmissionTransport) Receipt(ctx context.Context, operation string) (model.SubmissionReceipt, error) {
	request, err := privateAppRequest(t.scope, "mail", "/private-storage/submissions/"+url.PathEscape(t.epoch)+"/"+url.PathEscape(operation))
	if err != nil {
		return model.SubmissionReceipt{}, err
	}
	return JSON[model.SubmissionReceipt](ctx, t.client, request)
}

func (t *mailSubmissionTransport) Register(ctx context.Context, registration model.SubmissionRegistration) (model.SubmissionReceipt, error) {
	if registration.Input.Validate() != nil || registration.Input.Epoch != t.epoch || registration.Input.Generation != 1 {
		return model.SubmissionReceipt{}, model.ErrSubmission
	}
	request, err := privateAppRequest(t.scope, "mail", "/private-storage/submissions/register")
	if err != nil {
		return model.SubmissionReceipt{}, err
	}
	request.Method = http.MethodPost
	request.Header.Set("Content-Type", "application/json")
	request.Body, err = json.Marshal(registration)
	if err != nil {
		return model.SubmissionReceipt{}, err
	}
	defer clear(request.Body)
	return JSON[model.SubmissionReceipt](ctx, t.client, request)
}

func (t *mailSubmissionTransport) Control(ctx context.Context, change model.SubmissionControlChange) (model.SubmissionReceipt, error) {
	if change.Validate() != nil || change.Epoch != t.epoch {
		return model.SubmissionReceipt{}, model.ErrSubmission
	}
	request, err := privateAppRequest(t.scope, "mail", "/private-storage/submissions/control")
	if err != nil {
		return model.SubmissionReceipt{}, err
	}
	request.Method = http.MethodPost
	request.Header.Set("Content-Type", "application/json")
	request.Body, err = json.Marshal(change)
	if err != nil {
		return model.SubmissionReceipt{}, err
	}
	defer clear(request.Body)
	return JSON[model.SubmissionReceipt](ctx, t.client, request)
}
