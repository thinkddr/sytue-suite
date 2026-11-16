// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privateDraftService struct {
	*servicioFalso
	port api.PrivateMailDraftRevisions
}

func (s *privateDraftService) PrivateDraftRevisions() (api.PrivateMailDraftRevisions, bool) {
	return s.port, true
}

type draftRevisionUIProbe struct {
	inputs       []api.MailDraftRevision
	commits      []api.MailDraftAttempt
	lose         int
	beforeCommit func()
}

func (p *draftRevisionUIProbe) PrepareDraftRevision(_ context.Context, input api.MailDraftRevision) (api.MailDraftAttempt, error) {
	input.MIME = bytes.Clone(input.MIME)
	p.inputs = append(p.inputs, input)
	return api.MailDraftAttempt{Scope: storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}, Epoch: "11111111-1111-4111-8111-111111111111", Editor: input.Editor, Revision: input.Revision, ID: "private-draft"}, nil
}

func (p *draftRevisionUIProbe) CommitDraftRevision(_ context.Context, attempt api.MailDraftAttempt) (api.MailMessage, error) {
	p.commits = append(p.commits, attempt)
	if p.beforeCommit != nil {
		fn := p.beforeCommit
		p.beforeCommit = nil
		fn()
	}
	if p.lose > 0 {
		p.lose--
		return api.MailMessage{}, errors.New("synthetic accepted draft ACK was lost")
	}
	return api.MailMessage{ID: attempt.ID}, nil
}

func (p *draftRevisionUIProbe) PendingDraftRevisions(context.Context) ([]api.MailDraftAttempt, error) {
	return nil, nil
}

func TestPrivateDraftSaveButtonRetriesOriginalBeforeAdmittingLaterEdits(t *testing.T) {
	base := servicioConTodas()
	probe := &draftRevisionUIProbe{lose: 1}
	b, _, _ := buzonConVentana(t, &privateDraftService{servicioFalso: base, port: probe}, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{Asunto: "Original private revision", Cuerpo: "Original body"})
	rd := b.redaccion
	probe.beforeCommit = func() { rd.r.asunto.SetText("Edits while acknowledgement is pending") }
	test.Tap(rd.guardar)
	if b.redaccion != rd || rd.privado.pending == nil || !rd.privado.pending.prepared || len(probe.inputs) != 1 {
		t.Fatal("lost ACK closed the editor or discarded its original revision")
	}
	first := bytes.Clone(probe.inputs[0].MIME)
	defer clear(first)
	test.Tap(rd.guardar)
	if b.redaccion != nil || len(probe.inputs) != 2 || len(probe.commits) != 3 || probe.commits[0] != probe.commits[1] {
		t.Fatal("save did not finish the original attempt before the new edit", len(probe.inputs), len(probe.commits))
	}
	if probe.inputs[0].Editor != probe.inputs[1].Editor || probe.inputs[0].Revision == probe.inputs[1].Revision || probe.inputs[1].ID != "private-draft" || !bytes.Equal(first, probe.inputs[0].MIME) {
		t.Fatal("editor identity or immutable revision changed during replay")
	}
	original, err := eml.Leer(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	newer, err := eml.Leer(bytes.NewReader(probe.inputs[1].MIME))
	if err != nil {
		t.Fatal(err)
	}
	if original.ID == newer.ID || original.Asunto != "Original private revision" || newer.Asunto != "Edits while acknowledgement is pending" {
		t.Fatal("later edit reused the earlier Message-ID or lost its text")
	}
	if base.pidio("SaveDraft") != 0 || base.pidio("Send") != 0 {
		t.Fatal("private revision fell through to a legacy write", base.llamadas)
	}
}

func TestPrivateDraftCloseKeepsDialogUntilAcknowledgement(t *testing.T) {
	base := servicioConTodas()
	probe := &draftRevisionUIProbe{lose: 1}
	b, _, _ := buzonConVentana(t, &privateDraftService{servicioFalso: base, port: probe}, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{Asunto: "Unacknowledged original", Cuerpo: "Body"})
	rd := b.redaccion
	b.cerrarGuardando(rd)
	if b.redaccion != rd || rd.privado.pending == nil {
		t.Fatal("close discarded an unacknowledged original")
	}
	b.cerrarGuardando(rd)
	if b.redaccion != nil || len(probe.inputs) != 1 || len(probe.commits) != 2 || probe.commits[0] != probe.commits[1] {
		t.Fatal("close did not join and acknowledge the same revision")
	}
}

func TestUnsupportedPrivateDraftPortCannotSaveThroughLegacyService(t *testing.T) {
	base := servicioConTodas()
	b, _, _ := buzonConVentana(t, &privateDraftService{servicioFalso: base}, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{Asunto: "Closed private revision"})
	rd := b.redaccion
	test.Tap(rd.guardar)
	if b.redaccion != rd || base.pidio("SaveDraft") != 0 {
		t.Fatal("unsupported private revision adopted legacy saving")
	}
}

func TestPrivateDraftCloseAdmitsAttachmentOnlyRevision(t *testing.T) {
	base := servicioConTodas()
	probe := &draftRevisionUIProbe{}
	b, _, _ := buzonConVentana(t, &privateDraftService{servicioFalso: base, port: probe}, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{Adjuntos: []eml.Adjunto{{Nombre: "private.txt", Tipo: "text/plain", Datos: []byte("PRIVATE-ATTACHMENT-ONLY")}}})
	b.cerrarGuardando(b.redaccion)
	if b.redaccion != nil || len(probe.inputs) != 1 || len(probe.commits) != 1 {
		t.Fatal("closing discarded an attachment-only private draft")
	}
	parsed, err := eml.Leer(bytes.NewReader(probe.inputs[0].MIME))
	if err != nil || len(parsed.Adjuntos) != 1 || string(parsed.Adjuntos[0].Datos) != "PRIVATE-ATTACHMENT-ONLY" {
		t.Fatal("attachment-only revision lost its original MIME", err)
	}
}

func TestPrivateDraftSessionRoundTripRetainsOriginalAndUsesFreshOwnerPort(t *testing.T) {
	originalPort := &draftRevisionUIProbe{lose: 1}
	otherPort := &draftRevisionUIProbe{}
	resumedPort := &draftRevisionUIProbe{}
	service := func(port *draftRevisionUIProbe) *privateDraftService {
		return &privateDraftService{servicioFalso: servicioConTodas(), port: port}
	}
	b, _, _ := buzonConVentana(t, service(originalPort), fyne.NewSize(1280, 744))
	const owner = "issuer\x00owner\x00org-a"
	const other = "issuer\x00other\x00org-b"
	b.CambiarSesion(t.Context(), owner)
	b.abrirRedactor(Borrador{Asunto: "Original account A revision", Cuerpo: "PRIVATE-ACCOUNT-A-ORIGINAL"})
	state := b.redaccion.privado
	originalPort.beforeCommit = func() { b.S = service(otherPort); b.CambiarSesion(t.Context(), other) }
	test.Tap(b.redaccion.guardar)
	if b.redaccion != nil || b.redacciones[owner].privado != state || state.pending == nil || len(originalPort.inputs) != 1 || len(originalPort.commits) != 1 {
		t.Fatal("account change lost the admitted original whose callback was cancelled")
	}
	if len(otherPort.inputs) != 0 || len(otherPort.commits) != 0 {
		t.Fatal("account B published account A's private draft")
	}
	original := bytes.Clone(originalPort.inputs[0].MIME)
	defer clear(original)
	b.S = service(resumedPort)
	b.CambiarSesion(t.Context(), owner)
	if b.redaccion == nil || b.redaccion.privado != state || len(resumedPort.commits) != 0 {
		t.Fatal("returning to A discarded its pending revision or retried without action")
	}
	test.Tap(b.redaccion.guardar)
	if b.redaccion != nil || len(resumedPort.inputs) != 1 || len(resumedPort.commits) != 1 || len(originalPort.commits) != 1 || originalPort.commits[0] != resumedPort.commits[0] {
		t.Fatal("returning to A did not retry its original revision through fresh owner resources")
	}
	if resumedPort.inputs[0].Editor != originalPort.inputs[0].Editor || resumedPort.inputs[0].Revision != originalPort.inputs[0].Revision || !bytes.Equal(resumedPort.inputs[0].MIME, original) {
		t.Fatal("account round trip recomposed the original MIME or Message-ID")
	}
	if len(otherPort.inputs) != 0 || len(otherPort.commits) != 0 {
		t.Fatal("account B acquired an attempt owned by A")
	}
}

func TestPrivateDraftLateDiscardConfirmationJoinsAdmittedOriginal(t *testing.T) {
	base := servicioConTodas()
	probe := &draftRevisionUIProbe{lose: 2}
	b, window, _ := buzonConVentana(t, &privateDraftService{servicioFalso: base, port: probe}, fyne.NewSize(1280, 744))
	b.abrirRedactor(Borrador{ID: "private-draft", Asunto: "Original admitted after confirmation opened", Cuerpo: "PRIVATE-LATE-CONFIRMATION"})
	rd := b.redaccion
	b.aspaRedactor(rd)
	confirm := confirmarEncolado(t, window.Canvas(), b.P.T.T("compose.discardConfirm", nil))
	b.guardarBorrador(rd, true, false)
	if rd.privado.pending == nil || len(probe.commits) != 1 {
		t.Fatal("autosave did not admit the original behind its earlier confirmation")
	}
	confirm()
	if b.redaccion != rd || rd.privado.pending == nil || base.pidio("Delete") != 0 || len(probe.commits) != 2 {
		t.Fatal("discard confirmation closed or deleted an admitted revision")
	}
	test.Tap(rd.guardar)
	if b.redaccion != nil || len(probe.inputs) != 1 || len(probe.commits) != 3 || probe.commits[0] != probe.commits[1] || probe.commits[1] != probe.commits[2] {
		t.Fatal("late discard did not finish acknowledgement of the original revision")
	}
}
