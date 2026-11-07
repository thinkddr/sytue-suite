// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"cmp"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El árbol de buzones del lateral, como el de Apple Mail: arriba los buzones
// unificados de cada rol —«Entrada» junta la de Sytue y la de cada cuenta externa—
// y debajo una sección por cuenta con sus carpetas. Es `arbolDeBuzones` de
// apps/frontend/apps/mail/src/components/buzones.ts, con los mismos casos
// (`testdata/arbol-de-buzones.json`).

// cuentaSytue es la cuenta de las fuentes que son carpetas de Sytue (`CUENTA_SYTUE`).
const cuentaSytue = ""

// FuenteDeBuzon es una carpeta que forma parte de un buzón unificado.
type FuenteDeBuzon struct {
	// Cuenta es la instancia de la cuenta externa, o cuentaSytue.
	Cuenta  string
	Carpeta string
}

// BuzonUnificado es «Todos los buzones de entrada», «Todos los enviados»…: un rol
// de todas las cuentas, con Sytue primero y luego las cuentas en su orden.
type BuzonUnificado struct {
	Rol      string
	Fuentes  []FuenteDeBuzon
	NoLeidos int64
}

// CuentaDeBuzones es la sección de una cuenta externa.
type CuentaDeBuzones struct {
	ID string
	// Titulo es la dirección; sin ella, el nombre de la conexión.
	Titulo string
	ICloud bool
	// Estado es el del resumen; una cuenta desconectada pide volver a iniciar
	// sesión, diga lo que diga.
	Estado string
	// NoLeidos son los de su Entrada, como el contador de Apple Mail junto a la cuenta.
	NoLeidos int64
	// Carpetas van en el orden del lateral; vacías salvo con Estado «ok».
	Carpetas []api.ExternalMailFolder
}

// ArbolDeBuzones es lo que pinta el lateral.
type ArbolDeBuzones struct {
	Unificados []BuzonUnificado
	Cuentas    []CuentaDeBuzones
}

// arbolDeBuzones arma el árbol con las carpetas de Sytue y el resumen de las
// cuentas externas (MailSummary). Un rol sale en los unificados si lo tiene alguna
// cuenta; las carpetas propias (sin rol) no se unifican. Las cuentas conectadas
// solo para Calendario, Contactos o Notas no salen.
func arbolDeBuzones(l i18n.Locale, sytue []api.Mailbox, externas []api.ExternalMailAccountSummary) ArbolDeBuzones {
	var arbol ArbolDeBuzones
	for _, c := range externas {
		if !c.ServesMail() {
			continue
		}
		estado := c.State
		if !c.Connected {
			estado = api.ExternalMailReconnect
		}
		cuenta := CuentaDeBuzones{ID: c.InstanceID, Titulo: cmp.Or(c.Address, c.Name), ICloud: c.IsICloud(), Estado: estado, Carpetas: []api.ExternalMailFolder{}}
		if estado == api.ExternalMailOK {
			cuenta.Carpetas = ordenarCarpetasExternas(l, c.Folders)
		}
		for _, f := range cuenta.Carpetas {
			if f.Role == "inbox" {
				cuenta.NoLeidos = f.Unread
				break
			}
		}
		arbol.Cuentas = append(arbol.Cuentas, cuenta)
	}
	for _, rol := range ordenRoles {
		u := BuzonUnificado{Rol: rol}
		for _, m := range sytue {
			if m.Role == rol {
				u.Fuentes = append(u.Fuentes, FuenteDeBuzon{Cuenta: cuentaSytue, Carpeta: m.ID})
				u.NoLeidos += int64(m.UnreadEmails)
				break
			}
		}
		for _, c := range arbol.Cuentas {
			for _, f := range c.Carpetas {
				if f.Role == rol {
					u.Fuentes = append(u.Fuentes, FuenteDeBuzon{Cuenta: c.ID, Carpeta: f.ID})
					u.NoLeidos += f.Unread
					break
				}
			}
		}
		if len(u.Fuentes) > 0 {
			arbol.Unificados = append(arbol.Unificados, u)
		}
	}
	return arbol
}
