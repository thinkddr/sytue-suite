// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateOutgoingUIService struct {
	*privateDraftService
	outbox api.PrivateMailOutbound
}

func (s *privateOutgoingUIService) PrivateOutbound() (api.PrivateMailOutbound, bool) {
	return s.outbox, true
}

type outgoingUIProbe struct {
	inputs  []api.MailOutgoing
	commits []api.MailOutgoingAttempt
	loseACK bool
}

func (p *outgoingUIProbe) PrepareOutbound(_ context.Context, input api.MailOutgoing) (api.MailOutgoingAttempt, error) {
	input.MIME = bytes.Clone(input.MIME)
	p.inputs = append(p.inputs, input)
	return api.MailOutgoingAttempt{Scope: storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}, Epoch: "11111111-1111-4111-8111-111111111111", Operation: input.Operation}, nil
}

func (p *outgoingUIProbe) CommitOutbound(_ context.Context, attempt api.MailOutgoingAttempt) (api.MailOutgoingStatus, error) {
	p.commits = append(p.commits, attempt)
	if p.loseACK {
		p.loseACK = false
		return api.MailOutgoingStatus{}, errors.New("synthetic lost external response")
	}
	input := p.inputs[0]
	fingerprint, err := (model.Submission{Operation: input.Operation, Epoch: attempt.Epoch, From: input.From, Recipients: input.Recipients, Consent: input.Consent}).Fingerprint(input.MIME)
	return api.MailOutgoingStatus{Attempt: attempt, Receipt: model.SubmissionReceipt{Operation: attempt.Operation, Fingerprint: fingerprint, States: []string{"accepted"}}}, err
}

func (p *outgoingUIProbe) CancelPrivateOutbound(context.Context, api.MailOutgoingAttempt) error {
	return nil
}

func (p *outgoingUIProbe) PendingOutbound(context.Context) ([]api.MailOutgoingAttempt, error) {
	return nil, nil
}

func (p *outgoingUIProbe) Scheduled(context.Context) ([]api.MailScheduled, error) { return nil, nil }
func (p *outgoingUIProbe) Reschedule(context.Context, string, time.Time) error    { return nil }
func (p *outgoingUIProbe) CancelOutbound(context.Context, string) error           { return nil }

func tapPrivateConsent(t *testing.T, b *Buzon, window fyne.Window, key string) {
	t.Helper()
	top := window.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("SMTP consent was not shown")
	}
	label := b.P.T.T(key, nil)
	found := recorrido.Buscar(top, func(object fyne.CanvasObject) bool {
		button, ok := object.(*ui.Boton)
		return ok && button.AccessibilityLabel() == label
	})
	if len(found) != 1 {
		t.Fatal("SMTP consent action was not uniquely available", label, len(found))
	}
	test.Tap(found[0].(*ui.Boton))
}

func TestPrivateNativeSendRequiresConsentAndRetainsExactAttemptAcrossLostACK(t *testing.T) {
	base := servicioConTodas()
	probe := &outgoingUIProbe{loseACK: true}
	service := &privateOutgoingUIService{privateDraftService: &privateDraftService{servicioFalso: base, port: &draftRevisionUIProbe{}}, outbox: probe}
	b, window, _ := buzonConVentana(t, service, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{Para: "recipient@external.test", Asunto: "Original outgoing message", Cuerpo: "Original private body"})
	rd := b.redaccion
	b.enviar(rd, true)
	if len(probe.inputs) != 0 || len(probe.commits) != 0 {
		t.Fatal("message left before explicit SMTP consent")
	}
	tapPrivateConsent(t, b, window, "common.cancel")
	if b.redaccion != rd || len(probe.inputs) != 0 {
		t.Fatal("cancelled consent sent or discarded the message")
	}
	b.enviar(rd, true)
	tapPrivateConsent(t, b, window, "privateMail.smtpConsentConfirm")
	if b.redaccion != rd || len(probe.inputs) != 1 || len(probe.commits) != 1 || rd.privado.outgoing == nil || !rd.privado.outgoing.prepared {
		t.Fatal("lost ACK erased the original send attempt")
	}
	original := bytes.Clone(probe.inputs[0].MIME)
	defer clear(original)
	if probe.inputs[0].Consent != model.SMTPConsent {
		t.Fatal("SMTP operation was not explicitly consented")
	}
	rd.r.asunto.SetText("Later editor changes")
	b.enviar(rd, true)
	if b.redaccion != nil || len(probe.inputs) != 1 || len(probe.commits) != 2 || probe.commits[0] != probe.commits[1] || !bytes.Equal(original, probe.inputs[0].MIME) {
		t.Fatal("send retry recomposed or replaced the original operation")
	}
	for _, call := range base.llamadas {
		if strings.HasPrefix(call, "Send(") || strings.HasPrefix(call, "Delete(") {
			t.Fatal("private send escaped into legacy delivery or deleted a newer draft", call)
		}
	}
}
