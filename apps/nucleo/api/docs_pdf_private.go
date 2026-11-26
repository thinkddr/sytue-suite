// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
)

// ErrPrivatePDFOperation prevents an explicitly private PDF from sending its
// contents to the former HTTP transformation service when the requested
// operation does not yet have an authorized local implementation.
var ErrPrivatePDFOperation = errors.New("pdf: private document requires an authorized local operation")

// WithPrivateDrive uses the caller's already authorized private Drive repository
// for PDF storage, local viewing, text, pages and forms. It borrows that repository; it neither
// opens another app's WAL nor grants access to another product's SDK identity.
// The original client remains available for explicitly legacy accounts. This
// facade does not enable legacy HTTP transformations of private PDF contents.
func (p *PDF) WithPrivateDrive(data DriveData) (*PDF, error) {
	if p == nil || p.drive == nil || p.motor == nil {
		return nil, ErrPrivatePDFOperation
	}
	drive, err := p.drive.WithPrivateData(data)
	if err != nil {
		return nil, errors.Join(ErrPrivatePDFOperation, err)
	}
	return &PDF{drive: drive, motor: privateDataClient(p.motor, ErrPrivatePDFOperation), signatures: newLocalSignatures(), local: &pdfLocal{}}, nil
}

// CheckRemoteProcessing rejects content processing outside the authorized
// device for a private document. It never checks availability by sending data.
// Local assistants must use their own authorized device executor instead of
// the legacy service client passed to the native PDF assistant.
func (p *PDF) CheckRemoteProcessing(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p == nil || p.drive == nil || p.motor == nil || p.drive.data != nil {
		return ErrPrivatePDFOperation
	}
	return nil
}

// ConAviso es una copia que avisa con `avisar` tras cada escritura con éxito en el Drive
// privado (Drive.ConAviso): guardar, subir, renombrar, copiar, mandar a la papelera o
// firmar dejan un fichero nuevo o cambiado en la carpeta «PDF», y el widget de CertiPDF
// se vuelve a publicar. Sin Drive privado, el mismo cliente: el de HTTP avisa con su
// transporte.
func (p *PDF) ConAviso(avisar func()) *PDF {
	if p == nil || p.drive == nil || !p.drive.Privado() || avisar == nil {
		return p
	}
	copia := *p
	copia.drive = p.drive.ConAviso(avisar)
	return &copia
}
