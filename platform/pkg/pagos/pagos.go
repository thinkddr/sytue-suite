// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package pagos es lo común a las pasarelas con las que una organización cobra en su
// propia cuenta (Mollie, Adyen, Redsys, Stripe): la interfaz, sus tipos y la ÚNICA
// comprobación de que una notificación habla del pago que esperamos.
//
// La plataforma nunca custodia fondos: cada pasarela se crea con la credencial de la
// organización y el dinero va a su cuenta. Cada subpaquete habla con un origen fijo,
// sin seguir redirecciones, con el cuerpo acotado y sin copiar en sus errores lo que
// diga el proveedor.
//
// Lecciones del cobro de Sytue ID (`docs/roadmap.md`, «El cobro con Mollie»): el
// importe sale siempre del servidor y se compara al recibir la notificación; la clave
// de idempotencia lleva el importe; y una firma válida sobre OTRO pago del mismo
// comercio no marca nada pagado. Lo último es Verificar.
package pagos

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Pasarela es lo que la tienda pide a cada proveedor.
type Pasarela interface {
	// CrearPago abre el pago en la pasarela: devuelve la URL a la que mandar al
	// comprador o, en Redsys, el formulario que hay que autoenviarle.
	CrearPago(ctx context.Context, s Solicitud) (Creado, error)
	// ConsultarPago pregunta a la pasarela, con la credencial de la organización,
	// cómo está un pago. Es la única fuente de verdad: el cuerpo de una
	// notificación solo sirve para saber qué preguntar.
	ConsultarPago(ctx context.Context, ref string) (Estado, error)
	// Caducar cierra en la pasarela un pago cuya reserva ha vencido, si esa
	// pasarela lo permite; si no, no hace nada.
	Caducar(ctx context.Context, ref string) error
	// Reembolsar devuelve `centimos` del pago con la clave de idempotencia dada:
	// dos llamadas con la misma clave son un solo reembolso.
	Reembolsar(ctx context.Context, ref string, centimos int64, clave string) error
}

// Solicitud es un pago que abrir. El importe lo calcula el servidor, nunca el
// navegador.
type Solicitud struct {
	Centimos          int64
	Divisa            string
	Descripcion       string // la que ve el comprador en su extracto
	Referencia        string // número del pedido, para quien lo muestre; Redsys y Adyen referencian el cobro por MetadatoPago
	ClaveIdempotencia string // obligatoria; lleva el importe (lección n.º 3 del cobro de ID)
	URLRetorno        string
	URLWebhook        string
	Idioma            string // «es», «en»…: el de la página de pago
	Metodo            string // vacío = los que tenga la pasarela
	Caduca            time.Time
	// Metadatos viajan con el pago y vuelven al consultarlo. Verificar exige en
	// MetadatoPago el id de la fila `payments`.
	Metadatos map[string]string
}

// Creado es lo que devuelve CrearPago: URL o formulario, nunca los dos.
type Creado struct {
	Ref        string // tr_… / pspReference / Ds_Order / cs_…
	URL        string
	Formulario *Formulario
}

// Formulario es la petición POST que el navegador tiene que mandar a la pasarela.
type Formulario struct {
	Accion string
	Campos map[string]string
}

// Estado es un pago tal como lo cuenta la pasarela.
type Estado struct {
	Estado        string // uno de los Estado* de abajo
	Importe       int64  // céntimos
	Reembolsado   int64
	Contracargado int64
	Divisa        string
	Modo          string // ModoPrueba o ModoReal
	Comercio      string // perfil, cuenta de comercio o FUC+terminal del cobro
	Metadatos     map[string]string
}

// Estados de un pago, comunes a las cuatro pasarelas. Lo reembolsado y lo
// contracargado van aparte, en importes.
const (
	EstadoAbierto    = "open"
	EstadoPendiente  = "pending"
	EstadoAutorizado = "authorized"
	EstadoPagado     = "paid"
	EstadoCancelado  = "canceled"
	EstadoCaducado   = "expired"
	EstadoFallido    = "failed"
)

// Modos de una pasarela: el de la credencial y el de cada pago.
const (
	ModoPrueba = "test"
	ModoReal   = "live"
)

// DivisaEUR es la única de la v1 (§10.3 del encargo de Pages).
const DivisaEUR = "EUR"

// MetadatoPago es la clave de Metadatos que lleva el id de la fila `payments`.
const MetadatoPago = "sytue_payment_id"

// Esperado es lo que la tienda sabe del pago ANTES de preguntar a la pasarela: la fila
// `payments` y la pasarela guardada.
type Esperado struct {
	Pago     string // id de la fila `payments`
	Importe  int64  // céntimos
	Modo     string // el de la pasarela guardada
	Comercio string // el de la pasarela guardada
}

// ErrNoCasa es un pago que no es el que esperamos.
var ErrNoCasa = errors.New("pagos: el pago no casa con el esperado")

// Verificar comprueba que el estado que da la pasarela es el del pago esperado:
// mismo importe, en euros, con nuestro id en los metadatos, en el modo y el comercio
// de la pasarela guardada. Se llama con cada notificación, de las cuatro pasarelas,
// antes de tocar nada. Lo esperado no puede tener huecos: un comercio vacío casaría
// con otro comercio vacío.
func Verificar(esperado Esperado, estado Estado) error {
	switch {
	case esperado.Pago == "" || esperado.Importe <= 0 || esperado.Comercio == "" ||
		(esperado.Modo != ModoPrueba && esperado.Modo != ModoReal):
		return fmt.Errorf("%w: lo esperado está incompleto", ErrNoCasa)
	case estado.Importe != esperado.Importe:
		return fmt.Errorf("%w: importe", ErrNoCasa)
	case estado.Divisa != DivisaEUR:
		return fmt.Errorf("%w: divisa", ErrNoCasa)
	case estado.Metadatos[MetadatoPago] != esperado.Pago:
		return fmt.Errorf("%w: metadatos", ErrNoCasa)
	case estado.Modo != esperado.Modo:
		return fmt.Errorf("%w: modo", ErrNoCasa)
	case estado.Comercio != esperado.Comercio:
		return fmt.Errorf("%w: comercio", ErrNoCasa)
	}
	return nil
}
