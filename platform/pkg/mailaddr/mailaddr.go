// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package mailaddr reúne lo que Sytue ID y Sytue Mail deben decidir IGUAL sobre
// una dirección de correo antes de dársela a alguien como alias. Hoy es una sola
// regla: en el dominio raíz de la plataforma (donde viven las cuentas personales
// y la propia Sytue) nadie se queda con «admin@», «soporte@» o «sytue-id@»,
// porque un correo así, firmado por nuestro dominio, es una campaña de phishing
// lista para enviar. Los dos servicios crean alias por caminos distintos (la
// consola de administración y el autoservicio del buzón); la lista vive aquí
// para que no se desincronicen.
package mailaddr

import "strings"

// reservados son los local-part que suplantan a la plataforma o a sus funciones
// de confianza. Se comparan tras plegar (ver plegar), así que «no-reply»,
// «no.reply» y «noreply2024» caen todos. Español e inglés, que son los idiomas
// en los que escribimos a la gente; una palabra en otro idioma no engaña «en
// nuestro nombre» porque nunca hemos escrito en él.
var reservados = map[string]struct{}{}

func init() {
	for _, p := range strings.Fields(`
		admin administrator administrador administracion administration
		postmaster hostmaster webmaster abuse root sysadmin system sistema
		security seguridad noreply donotreply mailerdaemon
		support soporte help ayuda helpdesk info contact contacto
		billing facturacion factura facturas invoice invoices payment payments pago pagos
		account accounts cuenta cuentas login password passwords
		verify verification verificacion recovery recuperacion reset
		notification notifications notificacion notificaciones alert alerts alerta alertas
		team equipo staff legal privacy privacidad dpo
		sales ventas marketing news newsletter unsubscribe welcome bienvenida
		official oficial service servicio services servicios
		id mail correo drive calendar calendario rooms hub cloud erp docs photos fotos notepads pdf
	`) {
		reservados[p] = struct{}{}
	}
}

// plegar deja solo letras ASCII en minúsculas: quita puntos, guiones, guiones
// bajos y dígitos, que es con lo que se disfraza una palabra reservada.
// ponytail: no pliega tildes ni homógrafos («sóporte», «аdmin» cirílico); la
// normalización del local-part que hace cada servicio ya rechaza lo no ASCII.
func plegar(local string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(local) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Reservado dice si el local-part suplanta a la plataforma: está en la lista o
// contiene la marca.
func Reservado(local string) bool {
	p := plegar(local)
	if p == "" {
		return false
	}
	if _, ok := reservados[p]; ok {
		return true
	}
	return strings.Contains(p, "sytue")
}

// Suplanta decide si una dirección local@dom no puede darse como alias porque
// haría pasar a su dueño por la plataforma. Solo importa en el dominio raíz
// (`raiz`, p. ej. sytue.eu): en «acme.sytue.eu» o en el dominio propio de una
// organización, «admin@» es asunto de esa organización, no nuestro.
func Suplanta(local, dom, raiz string) bool {
	dom = strings.ToLower(strings.TrimSpace(dom))
	raiz = strings.ToLower(strings.TrimSpace(raiz))
	return raiz != "" && dom == raiz && Reservado(local)
}
