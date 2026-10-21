// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

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
