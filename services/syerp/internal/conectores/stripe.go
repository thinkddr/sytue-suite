// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package conectores conecta SyERP con Stripe y Salesforce.
//
// REPARTO. Lo que SALE hacia un tercero sale por el Sytue Integration Gateway
// (:8093): ahí está el único cliente de egress de la plataforma, con SSRF,
// límite, auditoría y secretos sellados. Este paquete NO abre conexiones a
// Stripe ni a Salesforce por su cuenta; pide por la pasarela.
//
// Lo que ENTRA es otra cosa: un webhook de Stripe llega sin sesión y sin
// pasarela, así que su verificación vive aquí y es la única autenticación que
// tiene. Por eso se hace bien y no «cuando haya tiempo».
package conectores

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	stripepagos "git.sytue.eu/sytue/one/platform/pkg/pagos/stripe"
)

// ToleranciaFirma es la ventana admitida entre la marca de tiempo firmada y
// ahora. Cinco minutos es lo que recomienda Stripe: sin ella, quien capture una
// petición válida puede reproducirla mañana y cobrar otra vez.
const ToleranciaFirma = stripepagos.ToleranciaFirma

// MaxCuerpoWebhook acota lo que se lee de un extremo sin autenticar. La firma se
// calcula sobre el cuerpo ENTERO, así que leerlo completo es obligatorio; lo que
// no puede es ser ilimitado.
const MaxCuerpoWebhook = stripepagos.MaxCuerpoWebhook

var (
	// ErrFirmaInvalida es el único error que se le cuenta a quien llama sin
	// firma válida: distinguir «falta la cabecera» de «el HMAC no cuadra» le
	// dice a quien prueba dónde está fallando.
	ErrFirmaInvalida = stripepagos.ErrFirmaInvalida
	// ErrFirmaCaducada separa el reintento legítimo tardío del ataque de
	// repetición. No se devuelve al exterior; sirve para registrarlo.
	ErrFirmaCaducada = stripepagos.ErrFirmaCaducada
	// ErrEventoIgnorado marca un evento VERIFICADO que no mueve dinero. No es un
	// fallo —se responde 200— pero tampoco un cobro, y devolverlo con nombre
	// evita el par (nil, nil) que obliga a quien llama a adivinar cuál es cuál.
	ErrEventoIgnorado = errors.New("evento de Stripe sin efecto contable")
)

// VerificarFirmaStripe comprueba la cabecera `Stripe-Signature` contra el cuerpo
// crudo y el secreto de firma de la organización.
//
// Cabecera: `t=<unix>,v1=<hmac hex>[,v1=<otro>]`. Se firma `<t>.<cuerpo>` con
// HMAC-SHA256. Puede venir MÁS DE UN v1 durante una rotación de secreto, y basta
// con que cuadre uno: no admitirlo deja de aceptar cobros justo mientras se rota.
//
// El cuerpo tiene que ser el CRUDO, byte a byte. Reserializar el JSON cambia
// espacios y orden de claves, y entonces no cuadra ninguna firma legítima.
func VerificarFirmaStripe(cuerpo []byte, cabecera, secreto string, ahora time.Time) error {
	return stripepagos.VerificarFirma(cuerpo, cabecera, secreto, ahora)
}

// EventoStripe es lo que SyERP mira de un evento. El resto del cuerpo se ignora
// a propósito: cuanto menos se interprete de un mensaje de fuera, menos hay que
// revisar cuando el emisor cambia algo.
type EventoStripe struct {
	ID   string `json:"id"`
	Tipo string `json:"type"`
	Data struct {
		Objeto struct {
			ID string `json:"id"`
			// Invoice y Checkout Session nombran el importe distinto; se leen los
			// dos y gana el que venga.
			AmountPaid  *int64 `json:"amount_paid"`
			AmountTotal *int64 `json:"amount_total"`
			Moneda      string `json:"currency"`
			// Referencia al documento de SyERP. `metadata.syerp_ref` la pone quien
			// crea el cobro; si no está, se busca la factura por este `id` en su
			// campo `externo_id`.
			Metadata map[string]string `json:"metadata"`
		} `json:"object"`
	} `json:"data"`
}

// Cobro es lo que un evento significa para el ERP: qué documento se ha pagado y
// por cuánto, en céntimos.
type Cobro struct {
	EventoID string
	Tipo     string
	// RefDocumento es la referencia del documento (FV-2026-00001) si el evento la
	// trae en metadata.
	RefDocumento string
	// ExternoID es el identificador del objeto en Stripe, con el que se busca la
	// factura cuando no hay metadata.
	ExternoID string
	Centimos  int64
	Moneda    string
}

// tiposDeCobro son los eventos que mueven dinero de verdad. Los demás se aceptan
// con un 200 y no se procesan: devolver error a un evento que no interesa hace
// que Stripe reintente eternamente algo que nunca va a cambiar.
var tiposDeCobro = map[string]bool{
	"invoice.paid":               true,
	"invoice.payment_succeeded":  true,
	"checkout.session.completed": true,
	"payment_intent.succeeded":   true,
}

// LeerEventoStripe interpreta el cuerpo YA VERIFICADO. Devuelve ErrEventoIgnorado
// si el evento no es de los que mueven dinero.
func LeerEventoStripe(cuerpo []byte) (Cobro, error) {
	var ev EventoStripe
	if err := json.Unmarshal(cuerpo, &ev); err != nil {
		return Cobro{}, fmt.Errorf("evento de Stripe ilegible: %w", err)
	}
	if ev.ID == "" {
		return Cobro{}, fmt.Errorf("evento de Stripe sin identificador")
	}
	if !tiposDeCobro[ev.Tipo] {
		return Cobro{}, ErrEventoIgnorado
	}
	c := Cobro{
		EventoID:     ev.ID,
		Tipo:         ev.Tipo,
		ExternoID:    ev.Data.Objeto.ID,
		RefDocumento: ev.Data.Objeto.Metadata["syerp_ref"],
		Moneda:       strings.ToUpper(ev.Data.Objeto.Moneda),
	}
	switch {
	case ev.Data.Objeto.AmountPaid != nil:
		c.Centimos = *ev.Data.Objeto.AmountPaid
	case ev.Data.Objeto.AmountTotal != nil:
		c.Centimos = *ev.Data.Objeto.AmountTotal
	}
	if c.Centimos <= 0 {
		return Cobro{}, fmt.Errorf("evento %s sin importe cobrado", ev.ID)
	}
	// Stripe trabaja en la unidad mínima de la divisa, igual que este ERP: no hay
	// conversión que hacer, y por eso no se hace ninguna. Las divisas sin
	// decimales (JPY) usan la unidad entera y tampoco necesitan tocarse.
	if c.Moneda == "" {
		c.Moneda = "EUR"
	}
	return c, nil
}
