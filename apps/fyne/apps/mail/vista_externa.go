// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Una carpeta externa en el panel principal (fase B, puntos 4 y 5)
//
// Una carpeta de una cuenta externa, o un buzón unificado («Todos los buzones de
// entrada», «Todos los enviados», «Todos los borradores»), se abre con la Bandeja y el
// Lector de siempre, como un buzón compartido: es
// apps/frontend/apps/mail/src/components/vistaExterna.ts, con los mismos casos de
// mezcla (`testdata/mezcla-de-entradas.json`) y de buzones mezclados
// (`testdata/buzones-mezclados.json`). Antes se abría el mini-buzón del diálogo de
// cuentas, que tenía su propia lista, su lector y sus botones.

// rolesMezclados son los buzones unificados que se abren en el panel, mezclados por
// fecha: la Entrada, los Enviados y los Borradores (`ROLES_MEZCLADOS`). Archivo, Spam
// y Papelera solo se despliegan en la carpeta de cada cuenta: vaciar o borrar para
// siempre es de una carpeta, no de una mezcla.
var rolesMezclados = []string{"inbox", "drafts", "sent"}

// esMezclado dice si el buzón unificado de ese rol se abre en el panel, o solo se
// despliega.
func esMezclado(rol string) bool { return slices.Contains(rolesMezclados, rol) }

// VistaExterna es lo que se mira cuando no es una carpeta de Sytue: la carpeta de una
// cuenta (Carpeta vacía: su Entrada, que se elige al cargar) o, con Unificado, la
// carpeta de ese rol de todas mezclada por fecha.
type VistaExterna struct {
	// Unificado es el rol del buzón unificado (uno de rolesMezclados); vacío, la
	// carpeta de una cuenta.
	Unificado       string
	Cuenta, Carpeta string
}

// Mezclada dice si la vista es un buzón unificado, y no la carpeta de una cuenta.
func (v VistaExterna) Mezclada() bool { return v.Unificado != "" }

// fuentesDeUnificado son las carpetas que se piden para un buzón unificado: las de SU
// rol y ninguna más. El refresco de 30 s y «Cargar más» piden solo estas, las de la
// carpeta a la vista: con «Todos los enviados» abierto no se vuelve a pedir ninguna
// Entrada. Una copia: lo que la pida no toca el árbol del lateral.
func fuentesDeUnificado(unificados []BuzonUnificado, rol string) []FuenteDeBuzon {
	for _, u := range unificados {
		if u.Rol == rol {
			return slices.Clone(u.Fuentes)
		}
	}
	return nil
}

// abreRedactor dice si abrir una fila reabre el redactor en vez del lector: en
// Borradores de Sytue, y en «Todos los borradores» con un borrador de Sytue. Uno de una
// cuenta se lee: su borrador vive en su proveedor, y el redactor guarda en Borradores
// de Sytue (`abreRedactor` en la web).
//
// Con una búsqueda, nunca: la búsqueda es de todo el buzón y no quita la vista, así que
// un resultado puede ser un recibido aunque se esté en «Todos los borradores»; abierto
// en el redactor, guardarlo (PUT /drafts/{id}) borraría el original.
func abreRedactor(v *VistaExterna, rolSytue, id string, enBusqueda bool) bool {
	if enBusqueda {
		return false
	}
	if v == nil {
		return rolSytue == "drafts"
	}
	return v.Unificado == "drafts" && !esExterno(id)
}

// OrigenExterno es dónde vive un mensaje externo: su cuenta, su carpeta y su id en el
// proveedor.
type OrigenExterno struct{ Cuenta, Carpeta, ID string }

const prefijoExterno = "ext:"

// idExterno es el id de un mensaje externo en la lista: lleva su cuenta y su carpeta,
// porque cada acción las pide y en un buzón unificado van mezclados con los de
// Sytue. Nunca choca con uno de Sytue (un UUID).
func idExterno(o OrigenExterno) string {
	return prefijoExterno + url.QueryEscape(o.Cuenta) + ":" + url.QueryEscape(o.Carpeta) + ":" + url.QueryEscape(o.ID)
}

// origenExterno deshace idExterno; `false` si el mensaje es de Sytue.
func origenExterno(id string) (OrigenExterno, bool) {
	resto, ok := strings.CutPrefix(id, prefijoExterno)
	if !ok {
		return OrigenExterno{}, false
	}
	partes := strings.Split(resto, ":")
	if len(partes) != 3 {
		return OrigenExterno{}, false
	}
	var o OrigenExterno
	for i, dst := range []*string{&o.Cuenta, &o.Carpeta, &o.ID} {
		v, err := url.QueryUnescape(partes[i])
		if err != nil {
			return OrigenExterno{}, false
		}
		*dst = v
	}
	return o, o.Cuenta != "" && o.ID != ""
}

// esExterno dice si un mensaje de la lista es de una cuenta externa.
func esExterno(id string) bool {
	_, ok := origenExterno(id)
	return ok
}

// sobreExterno es el sobre de un mensaje externo como uno de Sytue: así lo pintan la
// Bandeja y el Lector. Sin hilo —el proveedor no lo da—: cada mensaje es una
// conversación de uno. Una fecha que no se lee se queda en cero, la más vieja.
func sobreExterno(cuenta, carpeta string, m api.ExternalMailMessage) api.MailMessage {
	x := api.MailMessage{
		ID:            idExterno(OrigenExterno{Cuenta: cuenta, Carpeta: carpeta, ID: m.ID}),
		MailboxID:     carpeta,
		Subject:       m.Subject,
		Preview:       m.Snippet,
		HasAttachment: len(m.Attachments) > 0,
		ReceivedAt:    fechaExterna(m.Date),
	}
	if len(m.From) > 0 {
		x.From, x.FromName = m.From[0].Email, m.From[0].Name
	}
	for _, d := range m.To {
		x.To = append(x.To, d.Email)
	}
	for _, d := range m.Cc {
		x.Cc = append(x.Cc, d.Email)
	}
	if m.Seen {
		x.Keywords = append(x.Keywords, api.MailFlagSeen)
	}
	if m.Flagged {
		x.Keywords = append(x.Keywords, api.MailFlagFlagged)
	}
	return x
}

// fechaExterna lee la fecha del proveedor (RFC 3339); la que no se lee es cero.
func fechaExterna(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// fuenteMezcla es una carpeta de un buzón unificado: la de Sytue (clave vacía) o la
// de una cuenta, con lo pedido y aún no enseñado (del más nuevo al más
// viejo, en el orden de su servidor) y el cursor de su página siguiente.
type fuenteMezcla struct {
	clave, cursor string
	pendientes    []api.MailMessage
}

// mezclarPorFecha saca siempre el más nuevo de las cabezas y se para en cuanto una
// fuente que aún tiene páginas se queda sin pendientes: lo siguiente podría ser suyo,
// y enseñar antes uno más viejo de otra lo dejaría fuera de su sitio para siempre. La
// fecha empatada la decide el orden de las fuentes, Sytue primero. No toca lo que
// recibe.
func mezclarPorFecha(fuentes []fuenteMezcla) ([]api.MailMessage, []fuenteMezcla) {
	quedan := make([]fuenteMezcla, len(fuentes))
	for i, f := range fuentes {
		quedan[i] = fuenteMezcla{clave: f.clave, cursor: f.cursor, pendientes: append([]api.MailMessage(nil), f.pendientes...)}
	}
	var salida []api.MailMessage
	for {
		mejor := -1
		for i, f := range quedan {
			if len(f.pendientes) == 0 {
				if f.cursor != "" {
					return salida, quedan
				}
				continue
			}
			if mejor < 0 || f.pendientes[0].ReceivedAt.After(quedan[mejor].pendientes[0].ReceivedAt) {
				mejor = i
			}
		}
		if mejor < 0 {
			return salida, quedan
		}
		salida = append(salida, quedan[mejor].pendientes[0])
		quedan[mejor].pendientes = quedan[mejor].pendientes[1:]
	}
}

// porPedir son las fuentes cuya página siguiente hace falta para seguir mezclando.
func porPedir(fuentes []fuenteMezcla) []string {
	var out []string
	for _, f := range fuentes {
		if len(f.pendientes) == 0 && f.cursor != "" {
			out = append(out, f.clave)
		}
	}
	return out
}

// quedanEnMezcla dice si queda algo por enseñar: pendientes o páginas sin pedir.
func quedanEnMezcla(fuentes []fuenteMezcla) bool {
	for _, f := range fuentes {
		if len(f.pendientes) > 0 || f.cursor != "" {
			return true
		}
	}
	return false
}
