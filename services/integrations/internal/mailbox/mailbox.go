// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package mailbox connects a person's external mailbox. The integration gateway
// owns its encrypted credentials and actor authorization; clients receive only
// mail content and connection metadata. No remote message is imported implicitly.
//
// El cliente vive en `apps/nucleo/cuentas/correo` (M-01, R-NO-COPIAR): este paquete
// lo reexporta tal cual y le pone el marcador anti-SSRF del servidor, que el núcleo no
// puede importar. El comportamiento es el de antes.
package mailbox

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/correo"
	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

const (
	MaxMessageBytes = correo.MaxMessageBytes
	PageSize        = correo.PageSize
)

// Errors are stable categories. Provider replies and credentials must never be
// returned in API error bodies or logs.
var (
	ErrInvalid     = correo.ErrInvalid
	ErrAuth        = correo.ErrAuth
	ErrTLS         = correo.ErrTLS
	ErrBlocked     = correo.ErrBlocked
	ErrUnavailable = correo.ErrUnavailable
	ErrUnsupported = correo.ErrUnsupported
	ErrTooLarge    = correo.ErrTooLarge
	ErrNotFound    = correo.ErrNotFound
	ErrSendUnknown = correo.ErrSendUnknown
)

type (
	Server     = correo.Server
	Account    = correo.Account
	Address    = correo.Address
	Profile    = correo.Profile
	Folder     = correo.Folder
	Attachment = correo.Attachment
	Message    = correo.Message
	Page       = correo.Page
	Outgoing   = correo.Outgoing
	SendResult = correo.SendResult
	Action     = correo.Action
	Client     = correo.Client
	HTTPDoer   = correo.HTTPDoer
)

// marcador es la barrera anti-SSRF del servidor: cada conexión TCP valida su destino
// y marca por `ssrfhttp`.
var marcador = correo.Marcador{
	Validar:   ssrfhttp.ValidateHost,
	Marcar:    ssrfhttp.SafeDialContext,
	Bloqueado: func(err error) bool { return errors.Is(err, ssrfhttp.ErrBlocked) },
}

// NewProtocolClient uses the suite's SSRF dialer for every TCP connection. No
// insecure transport, proxy, certificate override or automatic send retry exists.
func NewProtocolClient(a Account) Client { return correo.NewProtocolClient(a, marcador) }

// NewCloudClient talks to Gmail or Microsoft Graph through transport.
func NewCloudClient(account Account, transport HTTPDoer) Client {
	return correo.NewCloudClient(account, transport)
}

// OAuthProfile reads the identity of an OAuth account.
func OAuthProfile(ctx context.Context, provider string, transport HTTPDoer) (Profile, error) {
	return correo.OAuthProfile(ctx, provider, transport)
}

// DecodeMessage and EncodeMessage are the MIME bridge.
func DecodeMessage(raw []byte, id, folder string) (Message, error) {
	return correo.DecodeMessage(raw, id, folder)
}

func EncodeMessage(from string, out Outgoing) ([]byte, error) { return correo.EncodeMessage(from, out) }
