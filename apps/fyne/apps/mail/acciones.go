// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import "time"

// Acciones son los manejadores del TRIAJE: `onToggleFlag`, `onArchive`, `onTrash`… de
// MailApp.tsx, que la web reparte por props a la lista y al lector. Aquí los monta el
// Buzón según la vista y los leen la fila, la barra de lote y el lector.
//
// Uno a nil ESCONDE su botón, que es lo que hace el `null` de la web y no un detalle:
// en la Papelera no hay «Mover a la papelera» (ya está ahí), fuera de Spam no hay «No
// es spam», el borrado definitivo solo existe en Papelera y Spam, y en un buzón de
// solo lectura no hay nada.
type Acciones struct {
	// Destacar y Leido conmutan: la estrella y el sobre de la fila y del lector.
	Destacar, Leido func(Conversacion)
	// Archivar, Papelera, Spam y NoSpam MUEVEN el hilo entero a su carpeta.
	Archivar, Papelera, Spam, NoSpam func(Conversacion)
	// Purgar pide confirmación y borra PARA SIEMPRE: solo en Papelera y Spam.
	Purgar func(Conversacion)
	// Mover es «Mover a…», con sus Destinos: todas las carpetas menos la actual y
	// Borradores (un mensaje movido allí no se podría editar como borrador).
	Mover    func(Conversacion, Carpeta)
	Destinos []Carpeta
	// Etiquetar pone o quita una etiqueta del registro a TODO el hilo.
	Etiquetar func(c Conversacion, etiqueta string, poner bool)
	// Etiquetas es el registro de la cuenta: pinta los chips y el menú «Etiquetar».
	Etiquetas []Etiqueta
	// CrearRegla abre el asistente de reglas con el remitente puesto.
	CrearRegla func(Conversacion)
	// Bloquear es «Bloquear a …»: la regla «de esta dirección → Spam» y el hilo a Spam,
	// en un clic. PuedeBloquear dice si se ofrece para ese remitente (no a uno mismo).
	// Nil fuera del buzón propio y dentro de Spam.
	Bloquear      func(Conversacion)
	PuedeBloquear func(correo string) bool
	// BuscarRemitente es «Buscar correos de esta persona»: busca lo que ha mandado esa
	// dirección.
	BuscarRemitente func(correo string)
	// FichaDe dice si se ofrece «Añadir a Contactos» para ese remitente y, si ya tiene
	// ficha, su id («Ver en Contactos»). AnadirAContactos la crea y VerEnContactos la
	// abre en Sytue Contacts. Nil: sin Contacts que lo atienda (libreta.go).
	FichaDe          func(correo string) (id string, ofrecer bool)
	AnadirAContactos func(nombre, correo string)
	VerEnContactos   func(id string)
	// Posponer saca el hilo de Recibidos hasta una fecha (o se la cambia desde
	// «Pospuestos»), QuitarPosposicion es «Volver a Recibidos» y Silenciar silencia o
	// deja de silenciar el hilo (posponer.go). Nil donde no se ofrece: fuera del buzón
	// propio y en el modo privado.
	Posponer          func(Conversacion, time.Time)
	QuitarPosposicion func(Conversacion)
	Silenciar         func(c Conversacion, silenciar bool)
}

// EtiquetasDe es `conversationLabels`: las etiquetas del registro que lleva ALGÚN
// mensaje de la conversación, en el orden del registro (no en el de aparición: así
// dos filas con las mismas etiquetas las pintan igual).
func EtiquetasDe(c Conversacion, registro []Etiqueta) []Etiqueta {
	if len(registro) == 0 {
		return nil
	}
	puestas := map[string]bool{}
	for _, m := range c.Mensajes {
		for k := range m.Etiquetas {
			puestas[k] = true
		}
	}
	var out []Etiqueta
	for _, e := range registro {
		if puestas[e.ID] {
			out = append(out, e)
		}
	}
	return out
}
