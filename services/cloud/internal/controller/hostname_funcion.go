// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"fmt"

	"github.com/google/uuid"
)

// HostnameDeFuncion construye el nombre público de una función desplegada.
//
// POR QUÉ ES PLANO Y NO `<función>.<proyecto>.fn.<dominio>`
// ----------------------------------------------------------
// Cada hostname de cliente pide su propio certificado a Let's Encrypt, y TODO
// certificado emitido por una CA pública se publica en los registros de
// Certificate Transparency: son públicos, permanentes y consultables por
// cualquiera. Con la forma anterior, ahí quedaba el nombre de cada función y el
// identificador de cada proyecto de cada cliente —su arquitectura interna—, sin
// que el cliente lo supiera ni pudiera evitarlo.
//
// Un comodín no lo resolvía: ACME NO emite comodines de varios niveles. `*.fn.<d>`
// cubre un segmento, y la forma vieja tenía dos por debajo de `fn`. Al aplanarla
// a UNO, un único certificado `*.fn.<dominio>` cubre todas las funciones de todos
// los clientes: deja de publicarse nada, y de paso desaparecen los cientos de
// certificados individuales y su renovación.
//
// EL ORDEN NO ES ESTÉTICO. El UUID va PRIMERO porque mide siempre 36 caracteres,
// así que el punto de corte entre proyecto y función es fijo y dos pares
// (proyecto, función) distintos no pueden producir el mismo hostname. Con la
// función delante —`<función>-<proyecto>`— sí podrían: tanto los nombres de
// función como los UUID contienen guiones, y `a-b-<uuid>` sería ambiguo. Nadie
// parsea este nombre hoy, pero una colisión enrutaría a la función equivocada,
// que es un fallo de aislamiento entre clientes, no una molestia.
func HostnameDeFuncion(proyecto uuid.UUID, funcion, dominioApps string) string {
	return fmt.Sprintf("%s-%s.fn.%s", proyecto, dnsSafe(funcion), dominioApps)
}
