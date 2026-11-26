// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
)

// Remitente manda correo por el SMTP de las cuentas de Sytue Mail de este equipo SIN
// abrir su buzón: es lo que usan las demás apps de la edición para enviar algo suyo (la
// factura del ERP con su PDF). El buzón es de Mail —su índice, su bandeja de salida, su
// sincronización— y puede estar abierto en otro proceso; aquí solo se leen las cuentas
// de `cuentas.json` (cada vez: Mail pudo añadir o quitar una) y sus secretos del
// llavero de la edición, con las mismas reglas de conexión que Mail (TLS salvo en
// localhost, por la Red que se inyecta: el portero de red).
//
// La copia en «Enviados» es del buzón: las cuentas cuyo servidor guarda solo lo enviado
// por SMTP (GuardaEnviados: Gmail, Outlook) la tienen sin más; para las demás, lo enviado
// se deja en la cola compartida de enviados (enviados.go) y Mail lo sube con su índice
// al abrirse (o en su vuelta siguiente, si está abierta).
type Remitente struct {
	raiz     string
	secretos Secretos
	red      Red
}

// NuevoRemitente envía con las cuentas de raiz (`<Datos>/Correo`, la de Mail).
func NuevoRemitente(raiz string, s Secretos, r Red) *Remitente {
	return &Remitente{raiz: raiz, secretos: s, red: r}
}

// Cuentas son las cuentas de correo de este equipo, en el orden de Mail. Sin ninguna
// (Mail sin configurar), una lista vacía.
func (r *Remitente) Cuentas() ([]Cuenta, error) {
	reg, err := abrirRegistro(r.raiz)
	if err != nil {
		return nil, err
	}
	return reg.lista(), nil
}

// Enviar manda un mensaje ya compuesto (RFC 5322) desde la cuenta `cuenta` a `para`. El
// sobre lleva la dirección de la cuenta como remitente. Devuelve ErrSinCuenta si la
// cuenta ya no está, y los errores de Mail (ErrAcceso, ErrSinRed, ErrRechazado,
// ErrEnvioIncierto) si no sale. Nunca reintenta.
func (r *Remitente) Enviar(ctx context.Context, cuenta string, para []string, crudo []byte) error {
	if len(para) == 0 || slices.ContainsFunc(para, func(d string) bool { return strings.TrimSpace(d) == "" }) {
		return errors.New("correo: el mensaje no tiene destinatarios")
	}
	cs, err := r.Cuentas()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(cs, func(c Cuenta) bool { return c.ID == cuenta })
	if i < 0 {
		return ErrSinCuenta
	}
	ctx, parar := context.WithTimeout(ctx, PlazoConexion)
	defer parar()
	if err := mandarSMTP(ctx, r.secretos, r.red, cs[i], cs[i].Direccion, para, crudo); err != nil {
		return err
	}
	if !cs[i].GuardaEnviados {
		// Ya ha salido: si la copia no se puede dejar, el envío no falla por ello.
		if err := dejarParaEnviados(r.raiz, cs[i].ID, crudo); err != nil {
			slog.Warn("correo: lo enviado no tendrá copia en Enviados", "cuenta", cs[i].Direccion, "error", err)
		}
	}
	return nil
}
