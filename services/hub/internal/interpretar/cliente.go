// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package interpretar

import (
	"context"
	"fmt"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
)

// Cliente pregunta al servicio de IA cómo se traduce una frase.
type Cliente struct {
	ia *asistente.Cliente
}

// NewCliente construye el cliente. `base` es la URL interna del servicio de IA
// (http://sytue-ai); vacía = intérprete deshabilitado, y quien llama lo consulta
// con Activo().
//
// EL PLAZO ES CORTO A PROPÓSITO: esto va en el camino de una búsqueda, donde el
// usuario está esperando con el cursor puesto. Si el modelo tarda más que esto,
// la búsqueda literal —que ya se ha hecho— es mejor respuesta que un
// desplegable congelado.
func NewCliente(base string, timeout time.Duration) *Cliente {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &Cliente{ia: asistente.Nuevo(base, timeout)}
}

// Activo indica si hay servicio de IA configurado.
func (c *Cliente) Activo() bool { return c != nil && c.ia.Activo() }

// Interpretar traduce `consulta` a los parámetros de una búsqueda federada.
//
// VA EN MODO INTERACTIVO Y CON ESFUERZO `none`, que es lo contrario de lo que
// usan los asistentes de las apps. Aquí no se redacta nada: se clasifica una
// frase de ocho palabras contra una lista cerrada de valores, y en texto `none`
// acierta igual por la octava parte del coste (medido). El modo calidad
// añadiría quince segundos a cada búsqueda para decidir lo mismo.
func (c *Cliente) Interpretar(ctx context.Context, token, consulta string, ahora time.Time) (Interpretacion, error) {
	if !c.Activo() {
		return Interpretacion{}, fmt.Errorf("intérprete: sin servicio de IA configurado")
	}
	texto, err := c.ia.Pedir(ctx, token, asistente.Peticion{
		Action:      "interpretar",
		Instruction: Instruccion(ahora),
		Context:     consulta,
		Kind:        "search",
		Modo:        "interactivo",
		Esfuerzo:    "none",
	})
	if err != nil {
		return Interpretacion{}, fmt.Errorf("intérprete: %w", err)
	}
	return De(texto, consulta, ahora)
}
