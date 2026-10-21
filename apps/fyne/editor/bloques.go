// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"context"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El grupo «Bloques» de la cinta (`BloquesGroup.tsx`): un desplegable con los bloques
// reutilizables de Sytue y de la organización. Elegir uno pide su contenido —ya vestido
// con la marca— y se lo da al lienzo, que es quien sabe colocarlo: HTML en el cursor de un
// documento, una diapositiva detrás de la actual, o unas celdas en la celda activa.

// BloquesDoc es lo que el grupo necesita del producto.
type BloquesDoc struct {
	Svc *api.Docs
	// Lanzar corre una petición fuera del hilo de la interfaz; nil, una rutina nueva.
	Lanzar func(func())
	// Idioma es el de los nombres y el texto de los bloques.
	Idioma string
	// Colocar inserta el contenido en el documento; corre en el hilo de la interfaz.
	Colocar func(contenido string)

	lista  *api.DocBlocks
	pedido bool
}

func (b *BloquesDoc) lanzar(f func()) {
	if b.Lanzar != nil {
		b.Lanzar(f)
		return
	}
	go f()
}

// cargar pide la lista una vez; `listo` repinta la cinta con ella.
func (b *BloquesDoc) cargar(kind api.DocKind, listo func()) {
	if b.pedido || b.Svc == nil {
		return
	}
	b.pedido = true
	b.lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelar()
		l, err := b.Svc.Blocks(ctx, kind, b.Idioma)
		fyne.Do(func() {
			if err == nil {
				b.lista = &l
			}
			if listo != nil {
				listo()
			}
		})
	})
}

// Grupo es el grupo «Bloques». El desplegable está desde el principio —inerte mientras la
// lista no ha llegado o si no llega— para que la orden tenga siempre su sitio en la cinta.
func (b *BloquesDoc) Grupo(c *Contexto) *Grupo {
	b.cargar(c.Tipo.Kind(), c.Refrescar)
	opciones := []Opcion{{Valor: "", Rotulo: c.Traducir("blocks.placeholder")}}
	if b.lista != nil {
		for _, x := range b.lista.Blocks {
			opciones = append(opciones, Opcion{Valor: x.Key, Rotulo: x.Name})
		}
		for _, x := range b.lista.OrgBlocks {
			no := 1
			if len(x.Versions) > 0 {
				no = x.Versions[0].No
			}
			opciones = append(opciones, Opcion{Valor: x.Key, Rotulo: c.Traducir("blocks.org", i18n.Vars{"name": x.Name, "version": no})})
		}
	}
	sel := NuevoSelectorCinta(c.Traducir("blocks.insert"), 150, opciones, "", func(clave string) {
		if clave != "" {
			b.Insertar(c, clave)
		}
	})
	sel.Inerte = c.SoloLectura || c.SinRed || b.lista == nil
	return NuevoGrupo(c.Traducir("blocks.group"), sel)
}

// Insertar pide el bloque y lo coloca; si falla, lo dice.
func (b *BloquesDoc) Insertar(c *Contexto, clave string) {
	b.lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelar()
		contenido, err := b.Svc.Block(ctx, clave, b.Idioma)
		fyne.Do(func() {
			if err != nil || contenido == "" || b.Colocar == nil {
				if c.Notificar != nil {
					c.Notificar(ui.AvisoPeligro, c.Traducir("blocks.msg.failed"))
				}
				return
			}
			b.Colocar(contenido)
		})
	})
}
