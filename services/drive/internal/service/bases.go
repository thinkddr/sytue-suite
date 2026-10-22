// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/basesconn"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// Exportar una VISTA de Sytue Bases como CSV a una carpeta (ADR 0068, bases-2).
//
// Drive TIRA de Bases con el token de una conexión que el propietario de la base concedió
// para Sytue Drive; no es Bases quien empuja. Así la procedencia la dice Bases en su
// respuesta (no un campo que el llamante pueda inventar) y cada lectura queda en la
// auditoría de la base. La persona que pide aporta lo demás, como en cualquier subida: su
// organización, su cuota y la carpeta. El token se usa UNA vez y no se guarda: refrescar el
// fichero es volver a exportar, y revocar la conexión impide hacerlo.

// ErrConexionBases: el token no es de conexión, o Bases lo rechaza (revocado, caducado, de
// otro producto o de otra organización).
var ErrConexionBases = errors.New("drive: la conexión de Sytue Bases no es válida para Sytue Drive")

// Topes de UNA exportación.
const (
	basesMaxBytes = 32 << 20
	basesMaxFilas = 10_000
	basesMaxCols  = 100
)

// clienteBases habla con Sytue Bases por dentro del clúster.
var clienteBases = &http.Client{Timeout: 30 * time.Second}

// nombreDeExportacion es el nombre del CSV: el de la vista, saneado, con `.csv`. El nombre
// de una vista lo escribe quien administra la base: aquí pierde separadores de ruta y
// caracteres de control y se acorta para caber en los 255 bytes de un nombre de Drive.
func nombreDeExportacion(p basesconn.Procedencia) string {
	n := strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, p.TableName+" - "+p.ViewName))
	n = strings.Trim(n, ". -")
	for len(n) > 240 {
		_, t := utf8.DecodeLastRuneInString(n)
		n = n[:len(n)-t]
	}
	if n == "" {
		n = "Sytue Bases"
	}
	return n + ".csv"
}

// ExportarVistaDeBases lee la vista que abarca `token` y la deja como CSV en la carpeta.
// Si ya hay un fichero con ese nombre, es una versión nueva de él (como cualquier subida).
func (s *Service) ExportarVistaDeBases(ctx context.Context, folderID uuid.UUID, token, ownerSub string) (domain.File, store.Procedencia, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return domain.File{}, store.Procedencia{}, err
	}
	l, err := basesconn.Leer(ctx, clienteBases, basesconn.Peticion{
		Raiz: s.cfg.BasesURL, Token: token, Consumidor: basesconn.Drive, Tenant: t.Slug,
		MaxBytes: basesMaxBytes, MaxFilas: basesMaxFilas, MaxCols: basesMaxCols,
	})
	if err != nil {
		if errors.Is(err, basesconn.ErrToken) || errors.Is(err, basesconn.ErrRevocada) {
			return domain.File{}, store.Procedencia{}, ErrConexionBases
		}
		return domain.File{}, store.Procedencia{}, err
	}
	csv, err := basesconn.CSV(l)
	if err != nil {
		return domain.File{}, store.Procedencia{}, err
	}
	f, err := s.UploadFile(ctx, folderID, nombreDeExportacion(l.Provenance), "text/csv; charset=utf-8", ownerSub, bytes.NewReader(csv))
	if err != nil {
		return domain.File{}, store.Procedencia{}, err
	}
	detalle, err := json.Marshal(l.Provenance)
	if err != nil {
		return domain.File{}, store.Procedencia{}, err
	}
	p := store.Procedencia{Version: f.Version, Source: l.Provenance.Source, Detail: detalle, CreatedBy: ownerSub, CreatedAt: time.Now().UTC()}
	// ponytail: el fichero y su procedencia van en dos transacciones (UploadFile ya cerró la
	// suya). Si esta falla, el fichero queda sin procedencia y se dice: no se da por buena
	// una exportación que no sabe de dónde viene. Una sola transacción pide pasar el `tx`
	// por createFileTx.
	if err := s.team.GuardarProcedencia(ctx, f.ID, f.Version, p.Source, detalle, ownerSub); err != nil {
		log.From(ctx).Error("drive: exportación de Sytue Bases sin procedencia", "file", f.ID, "error", err)
		return domain.File{}, store.Procedencia{}, err
	}
	return f, p, nil
}

// ProcedenciaDeFichero devuelve de dónde salió cada versión exportada de un fichero.
func (s *Service) ProcedenciaDeFichero(ctx context.Context, id uuid.UUID) ([]store.Procedencia, error) {
	// GetFile primero: lo que no existe para quien pide no tiene procedencia que enseñar.
	if _, err := s.GetFile(ctx, id); err != nil {
		return nil, err
	}
	return s.team.ProcedenciaDe(ctx, id)
}
