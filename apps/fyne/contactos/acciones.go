// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"context"
	"log/slog"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Lo que hacen las etiquetas y las libretas: crearlas, cambiarles el nombre y
// eliminarlas (`LabelModal`, `AddressBookModal`, `LabelActions` y `BookActions`).

// engancharEtiquetasYLibretas conecta los callbacks de etiquetas y libretas.
func (a *App) engancharEtiquetasYLibretas() {
	p := a.Pantalla
	p.AlCrearEtiqueta = func() { a.dialogoEtiqueta(nil) }
	p.AlRenombrarEtiqueta = func() {
		if g := a.etiquetaActiva(); g != nil {
			a.dialogoEtiqueta(g)
		}
	}
	p.AlBorrarEtiqueta = a.borrarEtiqueta
	p.AlCrearLibreta = func() { a.dialogoLibreta(nil) }
	p.AlRenombrarLibreta = func() {
		if p.Activa != nil {
			b := *p.Activa
			a.dialogoLibreta(&b)
		}
	}
	p.AlBorrarLibreta = a.borrarLibreta
}

func (a *App) etiquetaActiva() *api.ContactGroup {
	for _, g := range a.Pantalla.Etiquetas {
		if g.ID == a.Pantalla.EtiquetaActiva {
			return &g
		}
	}
	return nil
}

// peticion lanza `hacer` en segundo plano con su plazo y trae el resultado al hilo de
// la interfaz: avisa del éxito o del fallo con esas claves (el fallo, con la frase de
// `pista`) y llama a `listo`.
func (a *App) peticion(hacer func(context.Context) error, exito, fallo, pista string, listo func(error)) {
	a.enFondo(func() {
		ctx, fin := a.ctx()
		err := hacer(ctx)
		fin()
		fyne.Do(func() {
			if err != nil {
				slog.Warn("contactos: "+fallo, "error", err)
				a.avisar(ui.AvisoPeligro, fallo, pista)
			} else {
				a.avisar(ui.AvisoExito, exito, "")
			}
			if listo != nil {
				listo(err)
			}
		})
	})
}

// dialogoEtiqueta es LabelModal: alta (nil) o cambio de nombre. Al renombrar se
// reenvían SUS miembros: el PUT sustituye la pertenencia y mandar [] la vaciaría.
func (a *App) dialogoEtiqueta(g *api.ContactGroup) *ui.Dialogo {
	lienzo, p := a.lienzo(), a.Pantalla
	if lienzo == nil || a.svc == nil || p.Activa == nil {
		return nil
	}
	libreta := p.Activa.ID
	titulo, inicial := "label.newTitle", ""
	if g != nil {
		titulo, inicial = "label.renameTitle", g.Name
	}
	d := p.dialogoNombre(lienzo, titulo, "label.namePlaceholder", "label.nameRequired", inicial,
		func(nombre string, listo func(error)) {
			exito, fallo := "label.created", "label.createFailed"
			hacer := func(ctx context.Context) error {
				_, err := a.svc.CreateGroup(ctx, libreta, nombre)
				return err
			}
			if g != nil {
				exito, fallo = "label.renamed", "label.renameFailed"
				hacer = func(ctx context.Context) error {
					_, err := a.svc.RenameGroup(ctx, g.ID, nombre, g.Members)
					return err
				}
			}
			a.peticion(hacer, exito, fallo, "common.tryAgainSoon", func(err error) {
				listo(err)
				if err == nil {
					a.enFondo(a.cargarEtiquetas)
				}
			})
		})
	a.mostrarEscritura("etiqueta", d, lienzo)
	return d
}

// borrarEtiqueta pregunta antes: no hay papelera de etiquetas.
func (a *App) borrarEtiqueta() {
	lienzo, p, g := a.lienzo(), a.Pantalla, a.etiquetaActiva()
	if lienzo == nil || a.svc == nil || g == nil {
		return
	}
	id := g.ID
	p.confirmar(lienzo, p.t("label.delete", nil), p.t("label.deleteConfirm", i18n.Vars{"label": g.Name}),
		p.t("label.delete", nil), func() {
			a.peticion(func(ctx context.Context) error { return a.svc.DeleteGroup(ctx, id) },
				"label.deleted", "label.deleteFailed", "common.tryAgainSoon", func(err error) {
					if err == nil {
						p.EtiquetaActiva = ""
						a.repintar()
						a.enFondo(a.cargarEtiquetas)
					}
				})
		})
}

// dialogoLibreta es AddressBookModal. Al terminar se vuelven a cargar las libretas,
// como hace la web (`setBooksAttempt`).
func (a *App) dialogoLibreta(b *api.AddressBook) {
	lienzo, p := a.lienzo(), a.Pantalla
	if lienzo == nil || a.svc == nil {
		return
	}
	titulo, inicial := "book.newTitle", ""
	if b != nil {
		titulo, inicial = "book.renameTitle", nombreLibreta(*b)
	}
	d := p.dialogoNombre(lienzo, titulo, "book.namePlaceholder", "book.nameRequired", inicial,
		func(nombre string, listo func(error)) {
			exito, fallo := "book.created", "book.createFailed"
			hacer := func(ctx context.Context) error {
				_, err := a.svc.CreateAddressBook(ctx, nombre)
				return err
			}
			if b != nil {
				exito, fallo = "book.renamed", "book.renameFailed"
				hacer = func(ctx context.Context) error {
					_, err := a.svc.RenameAddressBook(ctx, b.ID, nombre)
					return err
				}
			}
			a.peticion(hacer, exito, fallo, "common.tryAgainSoon", func(err error) {
				listo(err)
				if err == nil {
					a.enFondo(a.Cargar)
				}
			})
		})
	a.mostrarEscritura("libreta", d, lienzo)
}

// borrarLibreta pregunta antes: se lleva sus contactos y no se puede deshacer.
func (a *App) borrarLibreta() {
	lienzo, p := a.lienzo(), a.Pantalla
	if lienzo == nil || a.svc == nil || p.Activa == nil || p.Activa.IsDefault {
		return
	}
	b := *p.Activa
	p.confirmar(lienzo, p.t("book.delete", nil), p.t("book.deleteConfirm", i18n.Vars{"book": nombreLibreta(b)}),
		p.t("book.delete", nil), func() {
			a.peticion(func(ctx context.Context) error { return a.svc.DeleteAddressBook(ctx, b.ID) },
				"book.deleted", "book.deleteFailed", "common.tryAgainSoon", func(err error) {
					if err == nil {
						a.enFondo(a.Cargar)
					}
				})
		})
}
