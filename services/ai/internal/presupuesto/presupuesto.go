// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package presupuesto lleva la cuenta de lo que cada usuario gasta al mes en el
// modelo caro y decide, antes de cada petición, si todavía puede usarlo.
//
// POR QUÉ EXISTE. El modelo de calidad cuesta unas nueve veces más que el
// rápido. Sin un tope por usuario, una pestaña con un bucle o un puñado de
// usuarios entusiastas convierten una función en una factura, y el operador se
// entera a fin de mes. Con él, el gasto por usuario tiene techo conocido y lo
// que pasa al agotarlo no es un error: es el modelo rápido.
//
// ES UN LÍMITE BLANDO, y las tres consecuencias de serlo están asumidas:
//
//   - Se comprueba ANTES y se cobra DESPUÉS, porque hasta que el modelo no
//     responde no se sabe cuántos tokens costó. Así que la petición que cruza el
//     límite se sirve entera y lo deja pasado. El desvío máximo es una petición.
//
//   - El contador vive en la caché efímera (Dragonfly), no en la base de datos.
//     Un vaciado de la caché regala el presupuesto del mes en curso. Se acepta:
//     el contador NO es un registro contable —para eso está la factura del
//     proveedor— sino un freno.
//
//   - Sin caché configurada no hay cuenta y no hay tope. Se degrada abierto a
//     propósito: quedarse sin asistente porque falta una pieza opcional es peor
//     que gastar de más, y el operador tiene el aviso en el log de arranque.
package presupuesto

import (
	"context"
	"fmt"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// Precio es la tarifa de un modelo en euros por millón de tokens.
type Precio struct {
	Entrada float64
	Salida  float64
}

// Coste devuelve lo que cuestan esos tokens, en euros.
func (p Precio) Coste(entrada, salida int) float64 {
	return float64(entrada)/1e6*p.Entrada + float64(salida)/1e6*p.Salida
}

// Contador decide y acumula. Su cero es utilizable: sin caché, Permite siempre
// devuelve true y Cobrar no hace nada.
type Contador struct {
	c *cache.Client
	// TopeMensual en euros por usuario. <= 0 => sin tope.
	TopeMensual float64
}

// Nuevo construye el contador. `c` puede ser nil (sin caché => sin tope).
func Nuevo(c *cache.Client, topeMensual float64) *Contador {
	return &Contador{c: c, TopeMensual: topeMensual}
}

// clave es el contador de un usuario para un mes concreto.
//
// EL MES VA DENTRO DE LA CLAVE y no se guarda como dato: así el reinicio mensual
// no necesita ningún proceso que lo haga: el 1 de cada mes la clave cambia y la
// anterior caduca sola. Un cron que ponga contadores a cero es una pieza más que
// puede no ejecutarse, y su fallo se ve tarde y mal.
func clave(tenant, usuario string, ahora time.Time) string {
	return fmt.Sprintf("ai:presupuesto:%s:%s:%s", tenant, usuario, ahora.UTC().Format("2006-01"))
}

// ttl es lo que vive el contador: dos meses, no uno.
//
// Con exactamente un mes, el contador del 31 de enero caducaría a mitad del 28
// de febrero y el usuario recuperaría presupuesto a mitad de mes. Con dos, la
// clave del mes en curso siempre sobrevive al mes en curso y la del anterior se
// va sola sin que a nadie le importe.
const ttl = 62 * 24 * time.Hour

// Permite dice si el usuario puede usar todavía el modelo caro, y cuánto lleva
// gastado. Ante un fallo de la caché deja pasar: ver la nota del paquete.
func (p *Contador) Permite(ctx context.Context, tenant, usuario string) (bool, float64) {
	if p == nil || p.c == nil || p.TopeMensual <= 0 {
		return true, 0
	}
	raw, hay, err := p.c.Get(ctx, clave(tenant, usuario, time.Now()))
	if err != nil {
		log.From(ctx).Warn("presupuesto: no se pudo leer el contador; se deja pasar", "error", err)
		return true, 0
	}
	if !hay {
		return true, 0
	}
	var gastado float64
	if _, err := fmt.Sscanf(raw, "%g", &gastado); err != nil {
		// Un valor ilegible es un contador corrupto, no un usuario sin
		// presupuesto: se deja pasar y la siguiente suma lo reescribe.
		log.From(ctx).Warn("presupuesto: contador ilegible; se deja pasar", "valor", raw)
		return true, 0
	}
	return gastado < p.TopeMensual, gastado
}

// Cobrar suma el coste de una petición ya servida. Devuelve el total del mes.
//
// No devuelve error a propósito: quien llama ya ha respondido al usuario y no
// puede hacer nada con el fallo salvo registrarlo, que es lo que se hace aquí.
func (p *Contador) Cobrar(ctx context.Context, tenant, usuario string, euros float64) float64 {
	if p == nil || p.c == nil || euros <= 0 {
		return 0
	}
	total, err := p.c.SumarTTL(ctx, clave(tenant, usuario, time.Now()), euros, ttl)
	if err != nil {
		log.From(ctx).Warn("presupuesto: no se pudo cobrar", "error", err, "euros", euros)
		return 0
	}
	return total
}
