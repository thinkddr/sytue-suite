// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package conectores

// MapeosOdoo son los modelos de Odoo que SyERP sabe importar, por la misma
// pasarela que Salesforce: el driver JSON-RPC vive en el Integration Gateway y
// aquí solo se declara qué columna va a qué campo. Es el camino de quien deja
// Odoo: contactos, productos, facturas emitidas y oportunidades el primer día.
//
// Solo campos ESCALARES. Un many2one de Odoo llega como `[id, "nombre"]` y la
// tabla de la pasarela lo trae como tal; traducirlo exigiría resolver el id del
// tercero contra lo ya importado, que es un segundo pase. Por eso el cliente de
// la factura no viene: se completa en el ERP, y la referencia externa permite
// volver a importar sin duplicar.
var MapeosOdoo = []Mapeo{
	{
		Nombre: "res.partner:empresas", Objeto: "res.partner", Modelo: "contacto",
		Filtro: [][]any{{"is_company", "=", true}},
		Campos: map[string]string{
			"id": "externo_id", "name": "nombre", "email": "correo", "phone": "telefono",
			"website": "web", "street": "direccion", "city": "ciudad", "zip": "cp", "vat": "nif",
		},
		Fijos: map[string]any{"tipo": "organizacion"},
	},
	{
		Nombre: "res.partner:personas", Objeto: "res.partner", Modelo: "contacto",
		Filtro: [][]any{{"is_company", "=", false}},
		Campos: map[string]string{
			"id": "externo_id", "name": "nombre", "email": "correo", "phone": "telefono",
			"street": "direccion", "city": "ciudad", "zip": "cp", "vat": "nif",
		},
		Fijos: map[string]any{"tipo": "persona"},
	},
	{
		Nombre: "product.product", Objeto: "product.product", Modelo: "producto",
		Campos: map[string]string{
			"id": "externo_id", "name": "nombre", "default_code": "codigo",
			"list_price": "precio", "standard_price": "coste", "description_sale": "descripcion",
		},
		Dinero: map[string]bool{"precio": true, "coste": true},
		Fijos:  map[string]any{"tipo": "bien"},
	},
	{
		Nombre: "account.move:ventas", Objeto: "account.move", Modelo: "factura_venta",
		Filtro: [][]any{{"move_type", "=", "out_invoice"}},
		Campos: map[string]string{
			"id": "externo_id", "invoice_date": "fecha", "invoice_date_due": "vencimiento",
			"amount_untaxed": "base", "amount_tax": "iva", "amount_total": "total", "narration": "notas",
		},
		Dinero: map[string]bool{"base": true, "iva": true, "total": true},
		// Odoo: draft → posted → (payment_state paid) → cancel. Lo pagado no se
		// distingue sin otra columna; entra como confirmado y se cobra aquí.
		Estados: map[string]string{"draft": "borrador", "posted": "confirmado", "cancel": "anulado"},
		Etapa:   "state",
	},
	{
		Nombre: "crm.lead", Objeto: "crm.lead", Modelo: "oportunidad",
		Campos: map[string]string{
			"id": "externo_id", "name": "nombre", "expected_revenue": "importe", "date_deadline": "vencimiento",
		},
		Dinero: map[string]bool{"importe": true},
	},
}
