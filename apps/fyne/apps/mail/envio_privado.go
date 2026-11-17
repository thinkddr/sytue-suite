// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"time"

	"fyne.io/fyne/v2"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
)

type (
	privateOutgoingProvider interface {
		PrivateOutbound() (api.PrivateMailOutbound, bool)
	}
	privateOutgoingEditor struct {
		input    api.MailOutgoing
		attempt  api.MailOutgoingAttempt
		prepared bool
	}
)

func (b *Buzon) confirmarEnvioPrivado(rd *redaccion, from string, recipients []string, raw []byte, releaseAt time.Time) {
	if rd.privado == nil {
		clear(raw)
		return
	}
	port, ok := b.S.(privateOutgoingProvider)
	if !ok {
		clear(raw)
		b.avisarEnvio(api.ErrPrivateMailOperation)
		return
	}
	outbox, active := port.PrivateOutbound()
	if !active || outbox == nil {
		clear(raw)
		b.avisarEnvio(api.ErrPrivateMailOperation)
		return
	}
	t := b.P.T
	canvas := b.P.Lienzo()
	if canvas == nil {
		clear(raw)
		return
	}
	colors := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	body := ui.NuevoParrafo(t.T("privateMail.smtpConsentBody", nil), tema.TextoSM, ui.Normal, "text", colors).A(AnchoCuerpoDialogo(ui.DialogoSM, canvas.Size().Width))
	var dialog *ui.Dialogo
	cancel := func() { clear(raw); dialog.Cerrar() }
	dialog = ui.Confirmacion(t.T("privateMail.smtpConsentTitle", nil), t.T("common.cancel", nil), t.T("privateMail.smtpConsentConfirm", nil), false, body, cancel, func() {
		if !b.redaccionVigente(rd) {
			cancel()
			return
		}
		rd.privado.outgoing = &privateOutgoingEditor{input: api.MailOutgoing{Operation: uuid.NewString(), DraftID: rd.r.B.ID, From: from, Recipients: append([]string(nil), recipients...), MIME: bytes.Clone(raw), Consent: model.SMTPConsent, ReleaseAt: releaseAt}}
		clear(raw)
		dialog.Cerrar()
		b.reintentarEnvioPrivado(rd)
	})
	dialog.AlCerrar = cancel
	dialog.RotuloCerrar = t.T("ui.close", nil)
	dialog.Mostrar(canvas)
}

func (b *Buzon) reintentarEnvioPrivado(rd *redaccion) {
	if !b.redaccionVigente(rd) || rd.privado == nil || rd.privado.outgoing == nil || rd.enviando || rd.guardando {
		return
	}
	provider, ok := b.S.(privateOutgoingProvider)
	if !ok {
		b.avisarEnvio(api.ErrPrivateMailOperation)
		return
	}
	port, active := provider.PrivateOutbound()
	if !active || port == nil {
		b.avisarEnvio(api.ErrPrivateMailOperation)
		return
	}
	state := rd.privado.outgoing
	input := state.input
	input.MIME = bytes.Clone(input.MIME)
	attempt, prepared := state.attempt, state.prepared
	if rd.pararAuto != nil {
		rd.pararAuto()
	}
	rd.enviando = true
	b.pintarBotonesRedactor(rd)
	type result struct {
		status   api.MailOutgoingStatus
		attempt  api.MailOutgoingAttempt
		prepared bool
	}
	pedir(b, func(ctx context.Context) (result, error) {
		value := result{attempt: attempt, prepared: prepared}
		defer clear(input.MIME)
		if !value.prepared {
			var err error
			value.attempt, err = port.PrepareOutbound(ctx, input)
			if err != nil {
				return value, err
			}
			value.prepared = true
		}
		var err error
		value.status, err = port.CommitOutbound(ctx, value.attempt)
		return value, err
	}, func(value result, err error) {
		rd.enviando = false
		if value.prepared {
			state.prepared = true
			state.attempt = value.attempt
			clear(state.input.MIME)
			state.input.MIME = nil
		}
		b.pintarBotonesRedactor(rd)
		if err != nil {
			b.avisarEnvio(err)
			return
		}
		if value.status.Receipt.Canceled {
			rd.privado.outgoing = nil
			b.alGuardarBorrador()
			b.avisar(ui.AvisoExito, "undo.done", nil)
			return
		}
		if value.status.Receipt.Failed() {
			b.recuperarEnvioRechazado(rd, port, value.attempt)
			return
		}
		if !value.status.Queued && !value.status.Receipt.Accepted() {
			b.P.Avisar(ui.AvisoPeligro, b.P.T.T("privateMail.deliveryUncertain", nil), "")
			return
		}
		original := rd.r.B
		b.cerrar(rd)
		b.alGuardarBorrador()
		notice := ui.NuevoAviso(ui.AvisoExito, b.P.T.T("compose.sent", nil), "")
		notice.RotuloCerrar = b.P.T.T("ui.dismissNotification", nil)
		if value.status.Queued {
			notice.Descripcion = b.P.T.T("privateMail.queuedBody", nil)
			notice.Duracion = time.Until(value.status.ReleaseAt)
			notice.RotuloAccion = b.P.T.T("compose.undo", nil)
			notice.AlPulsarAccion = func() { b.cancelarEnvioPrivado(port, value.attempt, original) }
		}
		b.P.Notificar(notice)
	})
}

func (b *Buzon) cancelarEnvioPrivado(port api.PrivateMailOutbound, attempt api.MailOutgoingAttempt, original Borrador) {
	pedir(b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, port.CancelPrivateOutbound(ctx, attempt)
	}, func(_ struct{}, err error) {
		if err != nil {
			b.avisarEnvio(err)
			return
		}
		b.abrirRedactor(original)
		b.alGuardarBorrador()
		b.avisar(ui.AvisoExito, "undo.done", nil)
	})
}

func (b *Buzon) descartarEnvioPrivado(rd *redaccion) {
	if !b.redaccionVigente(rd) || rd.enviando || rd.guardando {
		return
	}
	provider, ok := b.S.(privateOutgoingProvider)
	if !ok {
		return
	}
	port, active := provider.PrivateOutbound()
	if !active || port == nil {
		return
	}
	state := rd.privado.outgoing
	input := state.input
	input.MIME = bytes.Clone(input.MIME)
	attempt, prepared := state.attempt, state.prepared
	rd.guardando = true
	b.pintarBotonesRedactor(rd)
	pedir(b, func(ctx context.Context) (api.MailOutgoingAttempt, error) {
		defer clear(input.MIME)
		if !prepared {
			var err error
			attempt, err = port.PrepareOutbound(ctx, input)
			if err != nil {
				return attempt, err
			}
		}
		return attempt, port.CancelPrivateOutbound(ctx, attempt)
	}, func(attempt api.MailOutgoingAttempt, err error) {
		rd.guardando = false
		b.pintarBotonesRedactor(rd)
		if err != nil {
			b.avisarEnvio(err)
			return
		}
		clear(state.input.MIME)
		rd.privado.outgoing = nil
		b.cerrar(rd)
		b.alGuardarBorrador()
	})
}

func (b *Buzon) recuperarEnvioRechazado(rd *redaccion, port api.PrivateMailOutbound, attempt api.MailOutgoingAttempt) {
	pedir(b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, port.CancelPrivateOutbound(ctx, attempt)
	}, func(_ struct{}, err error) {
		if err != nil {
			b.avisarEnvio(err)
			return
		}
		if !b.redaccionVigente(rd) || rd.privado == nil {
			return
		}
		rd.privado.outgoing = nil
		b.avisarEnvio(api.ErrPrivateMailOperation)
		b.alGuardarBorrador()
	})
}
