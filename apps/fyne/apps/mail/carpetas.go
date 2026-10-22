// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"image/color"
	"net/http"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Carpetas» (`FoldersModal.tsx`)
//
// Las carpetas PROPIAS —las de sistema no se listan: el servidor las protege y solo
// saldrían para decir que no—: renombrar al salir del campo, borrar con confirmación
// en la fila (solo vacías: el recuento dice por qué se puede negar) y una nueva. Parte
// de las carpetas que ya tiene el lateral, sin pedirlas otra vez.

// GestorCarpetas es lo que el diálogo le pide al servidor. Lo cumple *api.Mail.
type GestorCarpetas interface {
	CreateMailbox(ctx context.Context, nombre string) (api.Mailbox, error)
	RenameMailbox(ctx context.Context, id, nombre string) (api.MailboxSetError, error)
	DeleteMailbox(ctx context.Context, id string) (api.MailboxSetError, error)
}

var _ GestorCarpetas = (*api.Mail)(nil)

// ClaveRechazoCarpeta es `FAIL_KEYS`: el motivo del servidor, en palabras. Vacío si el
// motivo no es de los que se explican (una avería), que se avisa como fallo.
func ClaveRechazoCarpeta(m api.MailboxSetError) string {
	switch m {
	case "alreadyExists":
		return "folderNew.exists"
	case "invalidProperties", "notFound":
		return "folderNew.invalid"
	case "forbidden":
		return "folders.systemFolder"
	case "mailboxHasEmail":
		return "folders.notEmpty"
	}
	return ""
}

type gestorCarpetas struct {
	b       *Buzon
	d       *ui.Dialogo
	caja    *fyne.Container
	ancho   float32
	nombre  *ui.Entrada
	creando bool
	// borrando es la carpeta que pregunta «¿Borrar carpeta?»; ocupada, la que espera
	// al servidor.
	borrando, ocupada string
}

// verCarpetas abre «Carpetas». Devuelve el gestor para las pruebas.
func (b *Buzon) verCarpetas() *gestorCarpetas {
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return nil
	}
	t := b.P.T
	g := &gestorCarpetas{b: b, ancho: AnchoCuerpoDialogo(ui.DialogoLG, lienzo.Size().Width)}
	g.nombre = ui.NuevaEntrada(t.T("folderNew.placeholder", nil))
	g.nombre.Ancho = g.ancho
	g.nombre.AlCambiar(func(string) { g.pintar() })
	g.nombre.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			g.crear()
			return true
		}
		return false
	}
	g.caja = container.New(ui.Columna{Hueco: huecoSeccionesEtiq})
	cerrar := ui.NuevoBoton(t.T("common.close", nil), nil).Con(ui.Secundario, ui.TallaMD)
	g.d = ui.NuevoDialogo(t.T("folders.title", nil), g.caja).Con(ui.DialogoLG, cerrar)
	cerrar.AlPulsar = g.d.Cerrar
	g.d.RotuloCerrar = t.T("ui.close", nil)
	g.d.Mostrar(lienzo)
	g.pintar()
	return g
}

// propias son las carpetas sin rol de sistema, en el orden del lateral.
func (g *gestorCarpetas) propias() []api.Mailbox {
	var xs []api.Mailbox
	for _, m := range g.b.carpetas {
		if m.Role == "" {
			xs = append(xs, m)
		}
	}
	return xs
}

// duplicada: el nombre escrito ya lo tiene una carpeta, de sistema o no (sin mayúsculas).
func (g *gestorCarpetas) duplicada() bool {
	n := strings.TrimSpace(g.nombre.Texto())
	for _, m := range g.b.carpetas {
		if n != "" && strings.EqualFold(m.Name, n) {
			return true
		}
	}
	return false
}

func (g *gestorCarpetas) pintar() {
	t := g.b.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ayuda := ui.NuevaAyudaEnLinea(t.T("folders.whatBody", nil)).ConTono(ui.AyudaInfo).ConTitulo(t.T("folders.whatTitle", nil))

	var tuyas fyne.CanvasObject
	if propias := g.propias(); len(propias) == 0 {
		vacio := ui.NuevoVacio(t.T("folders.emptyTitle", nil), t.T("folders.emptyDescription", nil))
		vacio.Ilustracion = Glifo(CarpetaD, 32, "text-subtle", v).Objeto()
		vacio.Accion = ui.NuevoBoton(t.T("folders.emptyAction", nil), func() {
			if c := g.b.P.Lienzo(); c != nil {
				c.Focus(g.nombre)
			}
		}).Con(ui.Secundario, ui.TallaMD)
		tuyas = vacio
	} else {
		filas := make([]fyne.CanvasObject, 0, len(propias))
		for _, m := range propias {
			filas = append(filas, g.fila(m))
		}
		tuyas = enCajaConFiletes(filas)
	}

	dup := g.duplicada()
	pista := ui.NuevaPistaCampo(t.T("folderNew.help", nil))
	if dup {
		pista = ui.NuevaPistaCampo(t.T("folderNew.exists", nil)).ConTono(ui.AyudaPeligro)
	}
	pista.Ancho = g.ancho
	g.nombre.Invalida = dup
	g.nombre.Refresh()
	campo := ui.GrupoCampo(t.T("common.name", nil), false, g.nombre, "", "")
	campo.Add(pista)
	crear := ui.NuevoBoton(t.T("folderNew.create", nil), g.crear)
	crear.Icono, crear.Cargando = iconos.Mas, g.creando
	crear.Inerte = strings.TrimSpace(g.nombre.Texto()) == "" || dup
	nueva := seccion(t.T("folders.newTitle", nil), t.T("folders.newDescription", nil), g.ancho,
		campo, container.New(&extremos{}, canvas.NewRectangle(color.NRGBA{}), crear))

	g.caja.Objects = []fyne.CanvasObject{
		ayuda,
		seccion(t.T("folders.yoursTitle", nil), t.T("folders.yoursDescription", nil), g.ancho, tuyas),
		container.New(ui.Columna{Hueco: aireSeccionNueva}, canvasFilete(v), nueva),
	}
	g.caja.Refresh()
	if g.d != nil {
		g.d.Recolocar() // el diálogo se vuelve a medir con su contenido nuevo
	}
}

// fila es `FolderRow`: el glifo, el nombre editable (se guarda al salir del campo), el
// recuento y borrar, que pregunta en la propia fila.
func (g *gestorCarpetas) fila(m api.Mailbox) fyne.CanvasObject {
	t := g.b.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ocupada := g.ocupada == m.ID
	nombre := ui.NuevaEntrada("")
	nombre.PonerTexto(m.Name)
	nombre.Inerte(ocupada)
	nombre.AlEnfocar = func(si bool) {
		if !si {
			g.renombrar(m, nombre)
		}
	}
	nombre.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		if c := g.b.P.Lienzo(); c != nil {
			c.Unfocus() // `blur()`: el guardado lo hace la salida del campo
		}
		return true
	}
	recuento := ui.NuevoRotulo(t.T("folders.messageCount", i18n.Vars{"count": m.TotalEmails}), tema.TextoXS, ui.Normal, "text-subtle")
	var fin fyne.CanvasObject
	if g.borrando == m.ID {
		si := ui.NuevoBoton(t.T("common.yes", nil), func() { g.borrar(m) }).Con(ui.Peligro, ui.TallaSM)
		si.Cargando = ocupada
		no := ui.NuevoBoton(t.T("common.no", nil), func() { g.borrando = ""; g.pintar() }).Con(ui.Secundario, ui.TallaSM)
		no.Inerte = ocupada
		fin = container.New(ui.Fila{Hueco: tema.Espacio(1.5)},
			ui.NuevoRotulo(t.T("folders.confirmDelete", nil), tema.TextoXS, ui.Normal, "text-muted"), si, no)
	} else {
		x := ui.NuevoBoton(t.T("folders.deleteFolder", i18n.Vars{"folder": m.Name}), func() { g.borrando = m.ID; g.pintar() }).
			Con(ui.Fantasma, ui.TallaSM)
		x.SoloIcono, x.Icono, x.TamIcono, x.Inerte = true, iconos.Cerrar, 16, ocupada
		fin = x
	}
	glifo := Glifo(CarpetaD, 16, "text-subtle", v).Objeto()
	return container.New(ui.Margen{Izq: margenXFilaEtiqueta, Der: margenXFilaEtiqueta, Arr: margenYFilaEtiqueta, Abj: margenYFilaEtiqueta},
		container.New(&filaEtiqueta{lado: 16, finAbajo: g.borrando == m.ID && enTelefono(g.b)}, glifo, nombre, recuento, fin))
}

// informar es `reportar`: el motivo del rechazo en palabras, o el aviso de hecho y el
// lateral recargado. Devuelve si se aplicó.
func (g *gestorCarpetas) informar(motivo api.MailboxSetError, err error, hecho, fallo string, vars i18n.Vars) bool {
	switch clave := ClaveRechazoCarpeta(motivo); {
	case err != nil || (motivo != "" && clave == ""):
		g.b.avisar(ui.AvisoPeligro, fallo, nil)
	case clave != "":
		g.b.avisar(ui.AvisoPeligro, clave, nil)
	default:
		g.b.avisar(ui.AvisoExito, hecho, vars)
		g.b.refrescarContadores(g.pintar)
		return true
	}
	return false
}

func (g *gestorCarpetas) crear() {
	nombre := strings.TrimSpace(g.nombre.Texto())
	if nombre == "" || g.duplicada() || g.creando {
		return
	}
	g.creando = true
	g.pintar()
	pedir(g.b, func(ctx context.Context) (api.Mailbox, error) {
		return g.b.S.CreateMailbox(ctx, nombre)
	}, func(_ api.Mailbox, err error) {
		g.creando = false
		// El alta va por REST: los motivos llegan como 409 y 400.
		var motivo api.MailboxSetError
		var e *api.Error
		if errors.As(err, &e) {
			switch e.Status {
			case http.StatusConflict:
				motivo, err = "alreadyExists", nil
			case http.StatusBadRequest:
				motivo, err = "invalidProperties", nil
			}
		}
		if g.informar(motivo, err, "folderNew.created", "folderNew.failed", i18n.Vars{"name": nombre}) {
			g.nombre.PonerTexto("")
		}
		g.pintar()
	})
}

// renombrar es `guardarNombre`: vacío o igual, vuelve a lo que había.
func (g *gestorCarpetas) renombrar(m api.Mailbox, campo *ui.Entrada) {
	nuevo := strings.TrimSpace(campo.Texto())
	if g.ocupada != "" || nuevo == "" || nuevo == m.Name {
		if campo.Texto() != m.Name {
			campo.PonerTexto(m.Name)
		}
		return
	}
	g.ocupada = m.ID
	pedir(g.b, func(ctx context.Context) (api.MailboxSetError, error) {
		return g.b.S.RenameMailbox(ctx, m.ID, nuevo)
	}, func(motivo api.MailboxSetError, err error) {
		g.ocupada = ""
		if !g.informar(motivo, err, "folders.renamed", "folders.renameFailed", nil) {
			campo.PonerTexto(m.Name)
		}
		g.pintar()
	})
}

func (g *gestorCarpetas) borrar(m api.Mailbox) {
	if g.ocupada != "" {
		return
	}
	g.ocupada = m.ID
	g.pintar()
	pedir(g.b, func(ctx context.Context) (api.MailboxSetError, error) {
		return g.b.S.DeleteMailbox(ctx, m.ID)
	}, func(motivo api.MailboxSetError, err error) {
		g.ocupada, g.borrando = "", ""
		g.informar(motivo, err, "folders.deleted", "folders.deleteFailed", nil)
		g.pintar()
	})
}
