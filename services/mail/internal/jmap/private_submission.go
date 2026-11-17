// SPDX-License-Identifier: Apache-2.0
package jmap

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

type privateSubmitter interface {
	SubmitPrivate(context.Context, string, model.Submission, []byte) (model.SubmissionReceipt, error)
	PrivateSubmissionReceipt(context.Context, string, string, string) (model.SubmissionReceipt, error)
}

func (a *API) privateSubmissionAuthority(w http.ResponseWriter, r *http.Request, epoch string) (storage.Scope, bool) {
	scope, ok := a.privateSourceScope(w, r)
	if !ok {
		return scope, false
	}
	state, err := a.privateSource.PrivateStorageState(r.Context(), scope.Subject)
	if err != nil {
		httpx.Fail(w, r, privateMailError(err))
		return scope, false
	}
	if epoch != state.Epoch || (state.Mode != "frozen" && state.Mode != "private") {
		httpx.Fail(w, r, httpx.ErrConflict.WithCode("private_storage_snapshot_changed"))
		return scope, false
	}
	if a.privateIngress == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return scope, false
	}
	ready, err := a.privateIngress.PrivateIngressReady(r.Context(), scope)
	if err != nil || !ready {
		httpx.Fail(w, r, httpx.ErrConflict.WithCode("private_mail_ingress_required"))
		return scope, false
	}
	return scope, true
}

func decodePrivateSubmission(r *http.Request) (model.Submission, error) {
	var input model.Submission
	header := r.Header.Get(model.SubmissionHeader)
	if header == "" || len(header) > 64<<10 {
		return input, model.ErrSubmission
	}
	raw, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil {
		return input, model.ErrSubmission
	}
	defer clear(raw)
	reader := json.NewDecoder(bytes.NewReader(raw))
	reader.DisallowUnknownFields()
	if reader.Decode(&input) != nil || reader.Decode(new(any)) != io.EOF || input.Validate() != nil || input.Generation <= 0 {
		return input, model.ErrSubmission
	}
	return input, nil
}

func (a *API) submitPrivateMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	input, err := decodePrivateSubmission(r)
	if err != nil || r.Header.Get("Content-Type") != "message/rfc822" {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	scope, ok := a.privateSubmissionAuthority(w, r, input.Epoch)
	if !ok {
		return
	}
	out, ok := a.outbound.(privateSubmitter)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	body := http.MaxBytesReader(w, r.Body, a.cfg.MaxMessageBytes)
	raw, err := io.ReadAll(body)
	if err != nil {
		clear(raw)
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("message_too_large"))
		return
	}
	defer clear(raw)
	if len(raw) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	receipt, err := out.SubmitPrivate(r.Context(), scope.Subject, input, raw)
	if err != nil {
		httpx.Fail(w, r, privateSubmissionError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, receipt)
}

func (a *API) privateMessageReceipt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	epoch, ok := privateMailEpoch(w, r)
	if !ok {
		return
	}
	operation := chi.URLParam(r, "operation")
	parsed, err := uuid.Parse(operation)
	if err != nil || parsed == uuid.Nil || parsed.String() != operation {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	scope, ok := a.privateSubmissionAuthority(w, r, epoch.String())
	if !ok {
		return
	}
	out, ok := a.outbound.(privateSubmitter)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	receipt, err := out.PrivateSubmissionReceipt(r.Context(), scope.Subject, epoch.String(), operation)
	if err != nil {
		httpx.Fail(w, r, privateSubmissionError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, receipt)
}

// Relay and parser failures may contain remote-supplied text. Keep those details
// out of HTTP errors and request logs; the durable receipt carries state only.
func privateSubmissionError(err error) error {
	switch {
	case errors.Is(err, model.ErrSubmissionControl):
		return httpx.ErrConflict.WithCode("private_mail_control_changed")
	case errors.Is(err, model.ErrSubmissionStarted):
		return httpx.ErrConflict.WithCode("private_mail_delivery_started")
	case errors.Is(err, model.ErrSubmissionConflict):
		return httpx.ErrConflict.WithCode("private_mail_submission_changed")
	case errors.Is(err, model.ErrSubmission):
		return httpx.ErrBadRequest
	case errors.Is(err, domain.ErrFromNoAutorizado):
		return httpx.ErrForbidden
	case errors.Is(err, domain.ErrMessageTooLarge):
		return httpx.ErrBadRequest.WithCode("message_too_large")
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, store.ErrPrivateStorageEpoch):
		return httpx.ErrConflict.WithCode("private_storage_snapshot_changed")
	default:
		return httpx.ErrUnavailable.WithCode("private_mail_submission_unavailable")
	}
}

type privateControlledSubmitter interface {
	RegisterPrivateSubmission(context.Context, string, model.SubmissionRegistration) (model.SubmissionReceipt, error)
	ChangePrivateSubmissionControl(context.Context, string, model.SubmissionControlChange) (model.SubmissionReceipt, error)
}

func (a *API) registerPrivateSubmission(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, ok := a.privateSourceScope(w, r); !ok {
		return
	}
	var registration model.SubmissionRegistration
	if httpx.DecodeJSONLimit(w, r, &registration, 64<<10) != nil || registration.Input.Validate() != nil || registration.Input.Generation != 1 {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	scope, ok := a.privateSubmissionAuthority(w, r, registration.Input.Epoch)
	if !ok {
		return
	}
	port, ok := a.outbound.(privateControlledSubmitter)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	receipt, err := port.RegisterPrivateSubmission(r.Context(), scope.Subject, registration)
	if err != nil {
		httpx.Fail(w, r, privateSubmissionError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, receipt)
}

func (a *API) changePrivateSubmissionControl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, ok := a.privateSourceScope(w, r); !ok {
		return
	}
	var change model.SubmissionControlChange
	if httpx.DecodeJSONLimit(w, r, &change, 4096) != nil || change.Validate() != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	scope, ok := a.privateSubmissionAuthority(w, r, change.Epoch)
	if !ok {
		return
	}
	port, ok := a.outbound.(privateControlledSubmitter)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	receipt, err := port.ChangePrivateSubmissionControl(r.Context(), scope.Subject, change)
	if err != nil {
		httpx.Fail(w, r, privateSubmissionError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, receipt)
}

// MountPrivateSubmissions is shared with the disposable integration harness.
// Every handler still requires authenticated tenancy and the exact owner scope.
func (a *API) MountPrivateSubmissions(router chi.Router) {
	router.Post("/private-storage/submissions", a.submitPrivateMessage)
	router.Post("/private-storage/submissions/register", a.registerPrivateSubmission)
	router.Post("/private-storage/submissions/control", a.changePrivateSubmissionControl)
	router.Get("/private-storage/submissions/{epoch}/{operation}", a.privateMessageReceipt)
}
