// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package efactura es la factura que sale y la que entra, sin almacén: Facturae 3.2.2
// y UBL 2.1 de salida, la factura para el cliente en HTML y en PDF, y la lectura de
// las facturas electrónicas recibidas (UBL, CII y Facturae).
//
// Trabaja con los tipos MÍNIMOS de esta página (Factura, Parte), que cada almacén
// rellena desde los suyos: services/syerp desde `store.Registro` y
// `store.Organizacion`, y el ERP de Sytue Office (apps/nucleo/local/erp) desde
// `api.ERPRegistro` y `api.ERPOrganizacion`. Así las dos ediciones escriben el mismo
// fichero para la misma factura (PLAN-EDICION-LOCAL §6.13, R-NO-COPIAR).
//
// VERI*FACTU no vive aquí: quien tenga un registro pasa la URL de su QR tributario
// (Factura.QR) y, para el impreso, su imagen (Imprimible.PonerQR).
package efactura

import (
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
)

// Parte es el emisor o el receptor de una factura emitida.
type Parte struct {
	// Nombre es la razón social, o nombre y apellidos de una persona física.
	Nombre, NIF, Direccion, CP, Ciudad string
	// Pais es el código ISO de dos letras; el del receptor, tal como esté en la ficha.
	Pais   string
	Correo string
	// Fisica: persona física (Facturae la distingue de la jurídica).
	Fisica bool
	// DIR3 son los códigos contable, gestor y tramitador de una administración
	// receptora; vacíos fuera de FACe.
	DIR3 [3]string
}

// Factura es lo que la generación necesita de una factura emitida.
type Factura struct {
	// ID es el identificador interno: el número cuando no hay referencia.
	ID string
	// Numero es la referencia de la factura (`FV-2026-00001`).
	Numero      string
	Fecha       string // AAAA-MM-DD
	Vencimiento string
	Notas       string
	// Estado es el del flujo: borrador, confirmado, pagado, anulado.
	Estado string
	// Tipo es el `tipo_factura` (F1 completa, F2 simplificada…).
	Tipo string
	// RegimenIVA es el `regimen_iva`, que da la leyenda de exención.
	RegimenIVA string
	// Base e IVA, en céntimos.
	Base, IVA int64
	Lineas    []dominio.LineaFactura
	// Rectificativa: la factura rectifica otra; Rectifica es el número de esa otra,
	// si se conoce.
	Rectificativa bool
	Rectifica     string
	// QR es la URL del QR tributario (VERI*FACTU); vacía, sin él.
	QR string
}

// Simplificada dice si es una factura simplificada (F2): no identifica al comprador.
func (f Factura) Simplificada() bool { return f.Tipo == "F2" }

// numero es la referencia o, sin ella, el identificador.
func (f Factura) numero() string {
	if f.Numero != "" {
		return f.Numero
	}
	return f.ID
}

// CodigoPais deja pasar un ISO de dos letras y, si el contacto trae el país escrito
// de otra forma, cae al del emisor: un código inventado invalida el fichero entero en
// el receptor.
func CodigoPais(s, respaldo string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) == 2 {
		return s
	}
	return respaldo
}

// Primero es el primer valor no vacío, sin espacios alrededor.
func Primero(valores ...string) string {
	for _, v := range valores {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func importe(c int64) string { return dominio.Importe(c) }
