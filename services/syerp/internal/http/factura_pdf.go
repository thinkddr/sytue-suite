// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

// La factura en PDF: lo que se adjunta al correo que va al cliente, y lo que se
// descarga para archivarla. Sale del MISMO impreso que el HTML
// (efactura.Imprimible.PDF, con apps/nucleo/pdfdoc), así que dice lo mismo.

// facturaPDF escribe el PDF de la factura.
func facturaPDF(f facturaImprimible) ([]byte, error) { return f.PDF() }

// nombrePDF es el nombre del fichero de la factura.
func nombrePDF(f facturaImprimible) string { return f.NombrePDF() }
