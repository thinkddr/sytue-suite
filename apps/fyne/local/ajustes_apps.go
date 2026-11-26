// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/cuentas"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/exportar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// Lo que cada app añade a los Ajustes de la cáscara (§5.5): una sección propia, quién
// gestiona sus cuentas y cómo se exporta lo suyo en formatos abiertos. Se registra desde
// el `main` de la app, antes o después de montar la ventana.

// seccionApp es una pestaña de Ajustes que pone la app.
type seccionApp struct {
	titulo string
	crear  func(w fyne.Window) fyne.CanvasObject
}

// AjustesDeLaApp añade a Ajustes una pestaña propia de esta app (CertiPDF: la autoridad
// de sellado y las anclas; Cast: el ffmpeg). crear se llama cada vez que se abre Ajustes.
func (l *App) AjustesDeLaApp(titulo string, crear func(w fyne.Window) fyne.CanvasObject) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.secciones = append(l.secciones, seccionApp{titulo: titulo, crear: crear})
}

func (l *App) seccionesApp() []seccionApp {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]seccionApp(nil), l.secciones...)
}

// GestorCuentas es lo que la app DUEÑA de unas cuentas sabe hacer con ellas: Mail con
// las de correo, Rooms con las de mensajería. Cualquier campo nil no se ofrece.
type GestorCuentas struct {
	Anadir     func()
	Reconectar func(id string)
	Quitar     func(id string)
}

// GestionarCuentas registra el gestor de las cuentas de un origen
// (`cuentas.OrigenCorreo`, `cuentas.OrigenChat`). En las demás apps, Ajustes → Cuentas
// manda la acción a la app dueña por su esquema (`sytueoffice-mail:/cuentas?accion=…`),
// que la hace con sus propios diálogos.
func (l *App) GestionarCuentas(origen string, g GestorCuentas) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.gestores == nil {
		l.gestores = map[string]GestorCuentas{}
	}
	l.gestores[origen] = g
}

// appDeOrigen es la app que gestiona las cuentas de un origen.
var appDeOrigen = map[string]string{cuentas.OrigenCorreo: "mail", cuentas.OrigenChat: "rooms"}

// Acciones del enlace de cuentas.
const (
	AccionAnadir     = "anadir"
	AccionReconectar = "reconectar"
	AccionQuitar     = "quitar"
)

// EnlaceCuentas es el enlace que pide a la app de producto una acción sobre sus cuentas.
func EnlaceCuentas(producto, accion, id string) string {
	q := url.Values{"accion": {accion}}
	if id != "" {
		q.Set("id", id)
	}
	return espacio.Esquema(producto) + ":/cuentas?" + q.Encode()
}

// LeerEnlaceCuentas desarma [EnlaceCuentas]: la acción y la cuenta, u ok=false si no es
// uno (o la acción no es de las conocidas). El enlace lo puede escribir cualquiera: quien
// lo atiende pregunta antes de quitar nada.
func LeerEnlaceCuentas(u *url.URL) (accion, id string, ok bool) {
	if u == nil || !strings.HasPrefix(u.Scheme, espacio.PrefijoEsquema) {
		return "", "", false
	}
	if strings.Trim(cmpOr(u.Opaque, u.Path), "/") != "cuentas" {
		return "", "", false
	}
	q := u.Query()
	switch accion = q.Get("accion"); accion {
	case AccionAnadir:
		return accion, "", true
	case AccionReconectar, AccionQuitar:
		id = strings.TrimSpace(q.Get("id"))
		return accion, id, id != ""
	}
	return "", "", false
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// accionCuenta hace una acción sobre una cuenta de origen: con el gestor de esta app si
// lo hay, o pidiéndoselo a la app dueña.
func (l *App) accionCuenta(w fyne.Window, origen, accion, id string) {
	l.mu.Lock()
	g, ok := l.gestores[origen]
	l.mu.Unlock()
	if ok {
		switch {
		case accion == AccionAnadir && g.Anadir != nil:
			g.Anadir()
			return
		case accion == AccionReconectar && g.Reconectar != nil:
			g.Reconectar(id)
			return
		case accion == AccionQuitar && g.Quitar != nil:
			g.Quitar(id)
			return
		}
	}
	producto := appDeOrigen[origen]
	if producto == "" {
		return
	}
	if err := l.AbrirEnlace(EnlaceCuentas(producto, accion, id)); err != nil {
		Info(w, l.T("cuentas"), l.T("cuentas.sinApp"))
	}
}

// ajustesCuentas enseña las cuentas de toda la edición, de dónde vienen y su estado, y
// deja añadir, volver a entrar y quitar desde aquí: las de nube en el acto, las de
// correo y mensajería con su app (en la misma app, sus diálogos; en otra, por su esquema).
func (l *App) ajustesCuentas(w fyne.Window, reabrir func()) fyne.CanvasObject {
	caja := container.NewVBox(ayuda(l.T("cuentas.ayuda")))
	cs := l.Cuentas.Cuentas()
	if len(cs) == 0 {
		caja.Add(ayuda(l.T("cuentas.ninguna")))
	}
	for _, c := range cs {
		titulo := c.Rotulo
		if c.Direccion != "" && c.Direccion != c.Rotulo {
			titulo += " — " + c.Direccion
		}
		servicios := make([]string, 0, len(c.Servicios))
		for _, s := range c.Servicios {
			servicios = append(servicios, l.T("servicio."+string(s)))
		}
		detalle := strings.Join(servicios, ", ") + " · " + l.T("cuentas.desde."+c.Origen)
		estado := widget.NewLabel(l.T("cuentas.ok"))
		botones := container.NewHBox()
		if c.Estado == cuentas.EstadoReconectar {
			estado.SetText(l.T("cuentas.reconectar"))
			estado.Importance = widget.WarningImportance
			switch c.Origen {
			case cuentas.OrigenNube:
			case cuentas.OrigenAgenda:
				botones.Add(widget.NewButton(l.T("cuentas.volver"), func() { l.reconectarAgenda(w, c, reabrir) }))
			default:
				botones.Add(widget.NewButton(l.T("cuentas.volver"), func() { l.accionCuenta(w, c.Origen, AccionReconectar, c.ID) }))
			}
		}
		botones.Add(widget.NewButton(l.T("nube.quitar"), func() {
			if c.Origen == cuentas.OrigenAgenda {
				l.quitarAgenda(w, c, reabrir)
				return
			}
			if c.Origen != cuentas.OrigenNube {
				// La app dueña pregunta con sus palabras (qué pasa con lo descargado).
				l.accionCuenta(w, c.Origen, AccionQuitar, c.ID)
				return
			}
			Confirmar(w, l.T("nube.quitar"), l.T("cuentas.quitar.pregunta"), l.T("cancelar"), l.T("nube.quitar"), true, func() {
				l.Fallo(w, l.Nube.Quitar(c.ID))
				reabrir()
			})
		}))
		caja.Add(container.NewBorder(nil, nil, nil, container.NewHBox(estado, botones), container.NewVBox(negrita(titulo), ayuda(detalle))))
	}
	caja.Add(widget.NewSeparator())
	caja.Add(negrita(l.T("cuentas.anadir")))
	caja.Add(container.NewGridWithColumns(2,
		widget.NewButton(l.T("servicio.correo"), func() { l.accionCuenta(w, cuentas.OrigenCorreo, AccionAnadir, "") }),
		widget.NewButton(l.T("servicio.chat"), func() { l.accionCuenta(w, cuentas.OrigenChat, AccionAnadir, "") }),
		widget.NewButton(l.T("agenda.anadir"), func() { l.altaAgenda(w, reabrir) }),
		widget.NewButton(l.T("nube.sytue"), func() { l.altaDAV(w, nube.SytueDrive, reabrir) }),
		widget.NewButton(l.T("nube.webdav"), func() { l.altaDAV(w, nube.WebDAV, reabrir) }),
		widget.NewButton(l.T("nube.google"), func() { l.altaOAuth(w, nube.Google, l.Nube.Clientes.Google, reabrir) }),
		widget.NewButton(l.T("nube.onedrive"), func() { l.altaOAuth(w, nube.OneDrive, l.Nube.Clientes.Microsoft, reabrir) }),
	))
	return caja
}

// ----- Exportar en formatos abiertos -----

// AlExportarTodo registra cómo se exporta TODO lo de esta app en formatos abiertos
// (documentos a OOXML, notas a Markdown, contactos a vCard, el calendario a ICS, las
// bases a CSV, las fotos tal cual) dentro de carpeta, que la cáscara crea. Ajustes →
// Copia de seguridad lo ofrece en las apps que lo registran.
func (l *App) AlExportarTodo(f func(ctx context.Context, carpeta string) error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.exportarTodo = f
}

// exportarAbierto pide una carpeta (en el móvil, la de datos) y exporta ahí, en
// `Sytue Office <fecha>/<app>/`.
func (l *App) exportarAbierto(w fyne.Window) {
	l.mu.Lock()
	f := l.exportarTodo
	l.mu.Unlock()
	if f == nil {
		return
	}
	hacer := func(base string) {
		destino := filepath.Join(base, espacio.Nombre+" "+time.Now().Format("2006-01-02 15.04"), NombreCarpeta(l.A.Metadata().Name, l.Producto))
		aviso := Espera(w, l.T("copia.exportar"), l.T("copia.exportando"))
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), time.Hour)
			defer parar()
			err := os.MkdirAll(destino, 0o700)
			if err == nil {
				err = f(ctx, destino)
			}
			fyne.Do(func() {
				aviso.Cerrar()
				if err != nil {
					l.Fallo(w, err)
					return
				}
				Info(w, l.T("copia.exportar"), l.T("copia.exportado")+"\n\n"+destino)
				if !Tactil() {
					_ = ficheros.Abrir(destino)
				}
			})
		}()
	}
	if Tactil() {
		hacer(filepath.Join(l.Espacio.Rutas.Datos, "Exportado"))
		return
	}
	ElegirCarpeta(l.T("copia.exportar"), hacer)
}

// NombreCarpeta es un nombre de carpeta para la app: su nombre sin lo que el sistema
// de ficheros no admite, o el producto ([exportar.NombreCarpeta]).
func NombreCarpeta(nombre, producto string) string { return exportar.NombreCarpeta(nombre, producto) }

// NombreFichero es NombreCarpeta para un fichero: sin separadores ni caracteres que
// Windows no admite, con la extensión, y sin pisar otro de la misma carpeta (vistos).
func NombreFichero(titulo, respaldo, ext string, vistos map[string]int) string {
	return exportar.NombreFichero(titulo, respaldo, ext, vistos)
}
