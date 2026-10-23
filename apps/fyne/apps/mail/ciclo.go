// SPDX-License-Identifier: Apache-2.0
package mail

// EnSegundoPlano pausa las lecturas automáticas de listas. No cancela el contexto
// del buzón: el flujo de llegadas, los envíos y el autoguardado siguen vivos.
func (b *Buzon) EnSegundoPlano() {
	if b.segundoPlano {
		return
	}
	b.segundoPlano = true
	b.otraVuelta = false
	if b.pararSondeo != nil {
		b.pararSondeo()
		b.pararSondeo = nil
	}
	if b.correoAbierto != nil {
		b.correoAbierto.pausarLista()
	}
}

// EnPrimerPlano reconcilia una vez sin cerrar el hilo ni el redactor. Si ya hay un
// delta en camino, llegada lo agrupa con ese delta en la única cadena existente.
func (b *Buzon) EnPrimerPlano() {
	if !b.segundoPlano {
		return
	}
	b.segundoPlano = false
	b.llegada()
	if c := b.correoAbierto; c != nil && !c.cerrado {
		if c.puedeActualizarLista() {
			c.cargarMensajes(false)
		} else {
			c.programar()
		}
	}
}
