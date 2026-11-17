// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package cifrado

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"

	mailUI "git.sytue.eu/sytue/one/apps/fyne/apps/mail"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
)

type nativeDraftBlobACK struct {
	base      http.RoundTripper
	arm, lost atomic.Bool
}

func (t *nativeDraftBlobACK) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil || request.Method != http.MethodPut || !strings.HasPrefix(request.URL.Path, "/api/v1/cipher/blobs/mail/") || response.StatusCode < 200 || response.StatusCode >= 300 || !t.arm.CompareAndSwap(true, false) {
		return response, err
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	const body = "synthetic accepted private draft ciphertext ACK lost"
	response.StatusCode = http.StatusServiceUnavailable
	response.Status = "503 Service Unavailable"
	response.Body = io.NopCloser(strings.NewReader(body))
	response.ContentLength = int64(len(body))
	response.Header.Set("Content-Length", strconv.Itoa(len(body)))
	t.lost.Store(true)
	return response, nil
}

func nativeDraftSaveButtons(window fyne.Window, label string) []fyne.CanvasObject {
	found := make([]fyne.CanvasObject, 0, len(window.Canvas().Overlays().List()))
	for _, overlay := range window.Canvas().Overlays().List() {
		found = append(found, recorrido.Buscar(overlay, func(object fyne.CanvasObject) bool {
			button, ok := object.(*ui.Boton)
			return ok && button.Rotulo == label
		})...)
	}
	return found
}

// exerciseNativeDraftSave drives the real composer and Save button. Only window
// scheduling and the accepted-response loss are controlled by this harness.
func exerciseNativeDraftSave(ctx context.Context, t *testing.T, presenter *mailUI.Buzon, window fyne.Window, facade *api.Mail, holder *MailApplicationRepository, provider *storagetest.CipherGateway, transport *nativeDraftBlobACK, queue chan func()) (string, []byte) {
	t.Helper()
	drain := func() {
		for {
			select {
			case fn := <-queue:
				fn()
			default:
				return
			}
		}
	}
	t.Log("MAIL_DRAFT_UI_QUEUE_BEGIN")
	drain()
	before := provider.Snapshot()
	t.Log("MAIL_DRAFT_UI_OPEN_BEGIN")
	presenter.Redactar()
	t.Log("MAIL_DRAFT_UI_OPEN_READY")
	top := window.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("native composer did not open its actual dialog")
	}
	fields := recorrido.Buscar(top, func(object fyne.CanvasObject) bool {
		editor, ok := object.(*ui.EditorPlano)
		return ok && editor.PlaceHolder == presenter.P.T.T("compose.subjectPlaceholder", nil)
	})
	if len(fields) != 1 {
		t.Fatal("native composer did not expose its subject control", len(fields))
	}
	const subject = "PRIVATE-NATIVE-UI-DRAFT-MARKER"
	fynetest.Type(fields[0].(*ui.EditorPlano), subject)
	t.Log("MAIL_DRAFT_UI_SUBJECT_TYPED")
	saveLabel := presenter.P.T.T("compose.saveDraft", nil)
	buttons := nativeDraftSaveButtons(window, saveLabel)
	if len(buttons) != 1 {
		t.Fatal("native composer did not expose one Save button", len(buttons))
	}
	transport.arm.Store(true)
	fynetest.Tap(buttons[0].(*ui.Boton))
	drain()
	t.Log("MAIL_DRAFT_UI_LOST_ACK_RETURNED")
	if !transport.lost.Load() || len(nativeDraftSaveButtons(window, saveLabel)) != 1 {
		t.Fatal("accepted lost draft ACK closed or bypassed the actual composer")
	}
	port, private := facade.PrivateDraftRevisions()
	if !private || port == nil {
		t.Fatal("native presenter did not retain a private revision port")
	}
	pending, err := port.PendingDraftRevisions(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatal("native presenter lost its admitted original", len(pending), err)
	}
	attempt := pending[0]
	if work, err := holder.Pending(ctx); err != nil || work.Count == 0 {
		t.Fatal("uncertain draft ACK released retained ownership", work, err)
	}
	accepted := provider.Snapshot()
	if len(accepted.Data) != len(before.Data)+1 {
		t.Fatal("draft loss did not follow one actual accepted ciphertext upload")
	}
	buttons = nativeDraftSaveButtons(window, saveLabel)
	fynetest.Tap(buttons[0].(*ui.Boton))
	drain()
	t.Log("MAIL_DRAFT_UI_RETRY_ACK_RETURNED")
	if len(nativeDraftSaveButtons(window, saveLabel)) != 0 {
		t.Fatal("native composer did not close after acknowledging its exact revision")
	}
	after := provider.Snapshot()
	if len(after.Data) != len(accepted.Data) {
		t.Fatal("native Save retry uploaded a second ciphertext object")
	}
	for key, original := range accepted.Data {
		if !bytes.Equal(original, after.Data[key]) {
			t.Fatal("native Save retry changed accepted ciphertext")
		}
	}
	if pending, err := port.PendingDraftRevisions(ctx); err != nil || len(pending) != 0 {
		t.Fatal("native Save acknowledgement did not clean its retained revision", len(pending), err)
	}
	if work, err := holder.Pending(ctx); err != nil || work.Count != 0 {
		t.Fatal("native Save acknowledgement retained unfinished work", work, err)
	}
	body, err := facade.Raw(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	closeErr := body.Close()
	if err != nil || closeErr != nil {
		clear(raw)
		t.Fatal(err, closeErr)
	}
	parsed, err := eml.Leer(bytes.NewReader(raw))
	if err != nil || parsed.ID == "" || parsed.Asunto != subject {
		clear(raw)
		t.Fatal("native saved MIME lost the typed subject or stable Message-ID", err)
	}
	return attempt.ID, raw
}
