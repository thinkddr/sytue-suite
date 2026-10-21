// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package conectores

// MapeosHolded son las colecciones de Holded que SyERP sabe importar, por el
// driver `holded` de la pasarela. Es el camino de quien deja Holded: contactos,
// productos y facturas emitidas y recibidas el primer día.
//
// El driver ya entrega las fechas como AAAA-MM-DD, las direcciones aplanadas
// (`billAddress.city`) y el estado como texto.
//
// ponytail: todos los contactos entran como organización. Holded no distingue
// empresa de persona en el listado de una forma que se pueda filtrar al pedir;
// quien sea persona se corrige en la ficha. Si molesta, un segundo pase por el
// campo del contacto.
var MapeosHolded = []Mapeo{
	{
		Nombre: "contacts", Objeto: "contacts", Modelo: "contacto",
		Campos: map[string]string{
			"id": "externo_id", "name": "nombre", "code": "nif", "email": "correo", "phone": "telefono",
			"billAddress.address": "direccion", "billAddress.city": "ciudad",
			"billAddress.postalCode": "cp", "billAddress.country": "pais",
		},
		Fijos: map[string]any{"tipo": "organizacion"},
	},
	{
		Nombre: "products", Objeto: "products", Modelo: "producto",
		Campos: map[string]string{"id": "externo_id", "name": "nombre", "sku": "codigo", "price": "precio", "cost": "coste", "desc": "descripcion"},
		Dinero: map[string]bool{"precio": true, "coste": true},
		Fijos:  map[string]any{"tipo": "bien"},
	},
	{
		Nombre: "invoices", Objeto: "documents/invoice", Modelo: "factura_venta",
		Campos: camposDocumentoHolded(),
		Dinero: map[string]bool{"base": true, "iva": true, "total": true},
		Etapa:  "status",
		// Estado de cobro de Holded: 0 pendiente, 1 pagada, 2 pagada en parte.
		// Lo pagado en parte sigue pendiente de cobro.
		Estados: map[string]string{"0": "confirmado", "1": "pagado", "2": "confirmado"},
	},
	{
		Nombre: "purchases", Objeto: "documents/purchase", Modelo: "factura_compra",
		Campos:  camposDocumentoHolded(),
		Dinero:  map[string]bool{"base": true, "iva": true, "total": true},
		Etapa:   "status",
		Estados: map[string]string{"0": "confirmado", "1": "pagado", "2": "confirmado"},
	},
}

func camposDocumentoHolded() map[string]string {
	return map[string]string{
		"id": "externo_id", "date": "fecha", "dueDate": "vencimiento",
		"subtotal": "base", "tax": "iva", "total": "total", "notes": "notas",
	}
}
