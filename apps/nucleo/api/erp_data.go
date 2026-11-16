// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ERPData is the device-owned ERP surface. Missing capabilities must return an
// error; the private facade never falls back to the legacy transport.
type ERPData interface {
	PrivateStorageScope() storage.Scope
	Catalogo(context.Context) (ERPCatalogo, error)
	Listar(context.Context, string, string, string, int) (ERPPagina, error)
	ListarCon(context.Context, string, ERPFiltro) (ERPPagina, error)
	Estadisticas(context.Context, string) (ERPEstadisticas, error)
	MisPermisos(context.Context) (ERPPermisos, error)
	Obtener(context.Context, string, string) (ERPRegistro, error)
	Crear(context.Context, string, map[string]any) (ERPRegistro, error)
	Actualizar(context.Context, string, string, map[string]any) (ERPRegistro, error)
	CambiarEstado(context.Context, string, string, string) (ERPRegistro, error)
	Borrar(context.Context, string, string) error
	Restaurar(context.Context, string, string) error
	Duplicar(context.Context, string, string) (ERPRegistro, error)
	Asentar(context.Context, string, string) (string, error)
	Exportar(context.Context, string, string, string, []string) (io.ReadCloser, string, error)
	Imprimir(context.Context, string, string) (io.ReadCloser, error)
	Facturae(context.Context, string, string) (io.ReadCloser, string, error)
	UBL(context.Context, string, string) (io.ReadCloser, string, error)
	RegistrosVerifactu(context.Context, string, string) (ERPFacturaVerifactu, error)
	SubsanarVerifactu(context.Context, string, string) (ERPFacturaVerifactu, error)
}

var ErrPrivateERPOperation = errors.New("erp: operation unavailable through private data")

// WithPrivateData keeps the typed API used by existing screens. The caller owns
// the repository and drains its work before closing the SDK or changing account.
func (e *ERP) WithPrivateData(data ERPData) (*ERP, error) {
	if e == nil || e.c == nil || data == nil || e.private != nil {
		return nil, ErrPrivateERPOperation
	}
	scope := data.PrivateStorageScope()
	if !scope.Valid() || scope.Product != "erp" {
		return nil, ErrPrivateERPOperation
	}
	return &ERP{c: privateDataClient(e.c, ErrPrivateERPOperation), idioma: e.idioma, private: data}, nil
}
