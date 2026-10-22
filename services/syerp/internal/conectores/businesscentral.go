// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package conectores

// MapeosBusinessCentral son las entidades de Microsoft Dynamics 365 Business
// Central (API v2.0) que SyERP sabe importar. Sale por el driver OData de la
// pasarela, con el token del consentimiento de Microsoft: no hay driver propio
// porque Business Central ES OData v4, paginación por @odata.nextLink incluida.
//
// `Filtro` va como $filter: una misma entidad alimenta dos cosas (clientes que
// son empresa y los que son persona; artículos que son bien y los que son
// servicio), igual que `res.partner` en Odoo.
var MapeosBusinessCentral = []Mapeo{
	{
		Nombre: "customers:empresas", Objeto: "customers", Modelo: "contacto", Filtro: "type eq 'Company'",
		Campos: camposContactoBC(),
		Fijos:  map[string]any{"tipo": "organizacion"},
	},
	{
		Nombre: "customers:personas", Objeto: "customers", Modelo: "contacto", Filtro: "type eq 'Person'",
		Campos: camposContactoBC(),
		Fijos:  map[string]any{"tipo": "persona"},
	},
	{
		Nombre: "vendors", Objeto: "vendors", Modelo: "contacto",
		Campos: camposContactoBC(),
		Fijos:  map[string]any{"tipo": "organizacion"},
	},
	{
		Nombre: "items:bienes", Objeto: "items", Modelo: "producto", Filtro: "type ne 'Service'",
		Campos: map[string]string{"id": "externo_id", "number": "codigo", "displayName": "nombre", "unitPrice": "precio", "unitCost": "coste"},
		Dinero: map[string]bool{"precio": true, "coste": true},
		Fijos:  map[string]any{"tipo": "bien"},
	},
	{
		Nombre: "items:servicios", Objeto: "items", Modelo: "producto", Filtro: "type eq 'Service'",
		Campos: map[string]string{"id": "externo_id", "number": "codigo", "displayName": "nombre", "unitPrice": "precio", "unitCost": "coste"},
		Dinero: map[string]bool{"precio": true, "coste": true},
		Fijos:  map[string]any{"tipo": "servicio"},
	},
	{
		Nombre: "salesInvoices", Objeto: "salesInvoices", Modelo: "factura_venta",
		Campos: camposFacturaBC(),
		Dinero: map[string]bool{"base": true, "iva": true, "total": true},
		Etapa:  "status",
		// Draft/In Review aún no son factura; Corrective es la rectificativa que
		// anula la original.
		Estados: map[string]string{"Draft": "borrador", "In Review": "borrador", "Open": "confirmado", "Paid": "pagado", "Canceled": "anulado", "Corrective": "anulado"},
	},
	{
		Nombre: "purchaseInvoices", Objeto: "purchaseInvoices", Modelo: "factura_compra",
		Campos:  camposFacturaBC(),
		Dinero:  map[string]bool{"base": true, "iva": true, "total": true},
		Etapa:   "status",
		Estados: map[string]string{"Draft": "borrador", "In Review": "borrador", "Open": "confirmado", "Paid": "pagado"},
	},
}

func camposContactoBC() map[string]string {
	return map[string]string{
		"id": "externo_id", "displayName": "nombre", "email": "correo", "phoneNumber": "telefono",
		"website": "web", "addressLine1": "direccion", "city": "ciudad", "postalCode": "cp",
		"country": "pais", "taxRegistrationNumber": "nif",
	}
}

func camposFacturaBC() map[string]string {
	return map[string]string{
		"id": "externo_id", "invoiceDate": "fecha", "dueDate": "vencimiento",
		"totalAmountExcludingTax": "base", "totalTaxAmount": "iva", "totalAmountIncludingTax": "total",
	}
}
