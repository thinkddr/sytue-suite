// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"regexp"
	"strconv"
)

// ---------------------------------------------------------------------------
// Tamaño de disco de un Sytue Postgres
// ---------------------------------------------------------------------------
//
// `pvc_size` viaja en el mapa de configuración LIBRE del servicio, así que hasta
// aquí llegaba tal cual lo escribiera el cliente. Un `"grande"` se persistía sin
// una queja y reventaba después en el reconciliador, al pasar por el parser de
// cantidades de Kubernetes — un pánico que, hasta que se contuvo, tumbaba el
// bucle entero y dejaba sin reconciliar a TODOS los inquilinos.
//
// La contención sigue estando (controller.guard), pero es la red, no el suelo: sin
// esta validación el servicio se queda en 'provisioning' para siempre y el cliente
// no se entera de por qué. El error tiene que salir al crear, donde hay alguien
// mirando.

// pvcSizeRe acepta un entero seguido de Mi, Gi o Ti.
//
// Es DELIBERADAMENTE más estrecho que Kubernetes, que además admite decimales,
// notación exponencial y unidades SI (M, G, T). Todo lo que pasa por aquí lo
// parsea Kubernetes seguro; al revés no, y esa asimetría es la que interesa. Un
// cliente que quiera «1.5Gi» escribe «1536Mi», que no admite dos lecturas.
var pvcSizeRe = regexp.MustCompile(`^([1-9]\d{0,4})(Mi|Gi|Ti)$`)

// Límites del disco de una instancia. El suelo evita un volumen donde Postgres no
// arranca; el techo no es una política de plan —eso lo decide la cuota— sino un
// tope de cordura para que un cero de más no reserve un almacenamiento absurdo
// que además se factura.
const (
	MinPVCSizeGiB = 1
	MaxPVCSizeGiB = 4096
)

// ValidatePVCSize acepta el tamaño de disco de un Postgres gestionado. Vacío es
// válido: significa «usa el de tu plan», y el valor lo rellena la capa service.
func ValidatePVCSize(size string) error {
	if size == "" {
		return nil
	}
	m := pvcSizeRe.FindStringSubmatch(size)
	if m == nil {
		return fmt.Errorf("%w: tamaño de disco inválido %q: usa un entero con Mi, Gi o Ti (por ejemplo 10Gi)",
			ErrInvalidInput, size)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil { // inalcanzable: la expresión regular ya acota a 5 dígitos.
		return fmt.Errorf("%w: tamaño de disco inválido %q", ErrInvalidInput, size)
	}
	var gib int
	switch m[2] {
	case "Mi":
		// Se redondea hacia ARRIBA para el tope y hacia abajo para el suelo, de
		// modo que ningún borde se cuele por el redondeo: 512Mi es medio GiB y
		// tiene que caer por debajo del mínimo, no empatar con él.
		gib = n / 1024
	case "Gi":
		gib = n
	case "Ti":
		gib = n * 1024
	}
	if gib < MinPVCSizeGiB {
		return fmt.Errorf("%w: el disco mínimo es %dGi, se pidió %q", ErrInvalidInput, MinPVCSizeGiB, size)
	}
	if gib > MaxPVCSizeGiB {
		return fmt.Errorf("%w: el disco máximo es %dGi, se pidió %q", ErrInvalidInput, MaxPVCSizeGiB, size)
	}
	return nil
}
