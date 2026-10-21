// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"encoding/json"
	"fmt"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
)

// Pistas: para cada cargo del banco sin factura, dónde conseguirla. Un servicio
// online o un pago con tarjeta en internet tiene una página de facturas; una
// ferretería o un taller, un correo al que pedirla.

// MovimientoPista es lo que el modelo ve de un cargo: nada más que su texto, su
// fecha y su importe.
type MovimientoPista struct {
	ID, Fecha, Descripcion string
	Centimos               int64
}

// Pista es la respuesta del modelo para un movimiento, aún sin comprobar.
type Pista struct {
	ID         string `json:"id"`
	Empresa    string `json:"empresa"`
	Tipo       string `json:"tipo"`
	Web        string `json:"web"`
	URLFactura string `json:"url_factura"`
	Email      string `json:"email"`
}

// InstruccionPistas es la orden: dónde pedir o descargar la factura de cada cargo.
func InstruccionPistas() string {
	return strings.Join([]string{
		"Para cada cargo del banco del contexto, di qué empresa lo cobró y dónde conseguir su factura. Devuelve SOLO un objeto JSON así:",
		`{"movimientos":[{"id":"…","empresa":"…","tipo":"online|presencial|desconocido","web":"https://…","url_factura":"https://…","email":"…"}]}`,
		`"empresa": el nombre comercial que se reconoce en el texto del banco (p. ej. «CLAUDE.AI SUBSCRIPTION» es Anthropic; «AMZN Mktp ES» es Amazon).`,
		`"tipo": online si es un servicio o una compra por internet (suscripciones, software, publicidad, marketplaces, telefonía, luz); presencial si es un comercio o un profesional; desconocido si no se sabe.`,
		`"url_factura": la página https donde un cliente descarga sus facturas de esa empresa (el área de cliente o de facturación), solo si la conoces con seguridad.`,
		`"email": para un negocio sin descarga de facturas, el correo al que pedirla, solo si lo conoces con seguridad. No inventes direcciones: mejor vacío.`,
		"Deja vacío lo que no sepas. Un movimiento por cada id del contexto.",
	}, "\n")
}

// ContextoPistas lista los cargos: id | fecha | importe | texto del banco.
func ContextoPistas(movs []MovimientoPista) string {
	var b strings.Builder
	b.WriteString("CARGOS (id | fecha | importe | texto del banco)\n")
	for _, m := range movs {
		importe := m.Centimos
		if importe < 0 {
			importe = -importe
		}
		fmt.Fprintf(&b, "%s | %s | %d.%02d | %s\n", m.ID, m.Fecha, importe/100, importe%100, strings.ReplaceAll(m.Descripcion, "\n", " "))
	}
	return b.String()
}

// PistasDe lee la respuesta y se queda con las de los ids pedidos.
func PistasDe(texto string, ids map[string]bool) []Pista {
	var r struct {
		Movimientos []Pista `json:"movimientos"`
	}
	if json.Unmarshal([]byte(asistente.JSON(texto)), &r) != nil {
		return nil
	}
	out := make([]Pista, 0, len(r.Movimientos))
	vistas := map[string]bool{}
	for _, p := range r.Movimientos {
		p.ID = strings.TrimSpace(p.ID)
		if !ids[p.ID] || vistas[p.ID] {
			continue
		}
		vistas[p.ID] = true
		p.Empresa = strings.TrimSpace(p.Empresa)
		p.Tipo = strings.TrimSpace(p.Tipo)
		if p.Tipo != "online" && p.Tipo != "presencial" {
			p.Tipo = "desconocido"
		}
		p.Web, p.URLFactura, p.Email = strings.TrimSpace(p.Web), strings.TrimSpace(p.URLFactura), strings.TrimSpace(p.Email)
		out = append(out, p)
	}
	return out
}
