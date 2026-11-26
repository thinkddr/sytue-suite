// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command erp es Sytue ERP de Sytue Office: la misma pantalla de la suite, con los datos
// de la empresa en este equipo y en claro, sin cuenta y sin servidor.
//
// Comparado con `cmd/erp`: no hay `sesion.Abrir` (ni OIDC ni llavero de sesión) ni
// `cifrado.NewPrivateApplication` (ni Matrix ni dispositivo verificado). `api.ERP` va
// sobre un transporte EN PROCESO (`apps/nucleo/local/erp`, D-10) que atiende `/api/v1`
// con un SQLite de la carpeta de datos; ninguna petición sale del proceso. Lo que aún no
// se sirve contesta 501 `no_disponible_local`, que la pantalla dice con su texto.
//
// Las facturas se emiten SIN VERI*FACTU (D-29), y una franja fija lo dice.
package main

import (
	"errors"
	"log/slog"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/apps/erp"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	localerp "git.sytue.eu/sytue/one/apps/nucleo/local/erp"
)

func main() {
	a := local.NuevaApp("")
	l, err := local.Abrir(a, "erp")
	if err != nil {
		slog.Error("erp", "error", err)
		os.Exit(1)
	}
	tema.ElegirCromo(tema.CromoPapel)
	idioma := l.Idioma()
	t, err := erp.Traductor(idioma)
	if err != nil {
		panic(err) // el catálogo va embebido: si falla, es un error de compilación
	}

	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))

	svc, err := abrirERP(l)
	if err != nil {
		slog.Error("erp: no se pudo abrir el almacén", "error", err)
		w.SetContent(l.Montar(w, iconos.ERP, widget.NewLabel(err.Error())))
		l.Fallo(w, err)
		w.ShowAndRun()
		return
	}

	// La IA del ERP (informes, fichas) es la que la persona configure en Ajustes; se vuelve
	// a enchufar cada vez que la cambia, y sin ella las rutas contestan 503.
	svc.PonerIA(l.Asistir())
	l.AlCambiarIA(func() { svc.PonerIA(l.Asistir()) })
	// La factura sale por correo con las cuentas de Sytue Mail de este equipo (factura.go).
	svc.PonerCorreo(correoDeMail(l))

	p := erp.NuevaPantalla(t, api.NewERP(localerp.Base, nil, svc.Cliente(), string(idioma)), nil)
	p.ElegirFicheros = elegirFicheros
	p.OcultarSinVerbo = true
	p.Salida = salidaFactura(l)
	// «Comprobar en VIES» el NIF-IVA de un contacto o de la propuesta de Sytue AI (vies.go).
	p.ComprobarVIES = comprobarVIES(l)
	p.Yo = func() string { return localerp.Persona }
	p.Atajos(w.Canvas(), nil)
	if n := l.Espacio.Perfil.Nombre; n != "" {
		p.PonerNombre(n)
	}

	w.SetContent(l.Montar(w, iconos.ERP, container.NewBorder(franjaVerifactu(string(idioma)), nil, nil, nil, p.Objeto())))
	p.Arrancar()
	// Una vista del ERP (`?clase=vista&id=tareas`), como el aviso de una tarea asignada.
	l.AlAbrirElemento(func(clase, id string) {
		if clase == "vista" {
			p.IrA(id)
		}
	})
	w.ShowAndRun()
}

// abrirERP abre el almacén de la carpeta de datos y el servidor en proceso. El almacén
// se cierra al cerrar la ventana.
func abrirERP(l *local.App) (*localerp.Servidor, error) {
	cat, obl, err := localerp.Cargar()
	if err != nil {
		return nil, err
	}
	alm, err := localerp.Abrir(l.Espacio.Rutas.Producto("erp"), cat)
	if err != nil {
		return nil, err
	}
	l.AlCerrar(func() {
		if err := alm.Cerrar(); err != nil {
			slog.Warn("erp: cerrar el almacén", "error", err)
		}
	})
	// La copia de seguridad copia la base abierta en caliente (§5.13).
	l.AlCerrar(l.Copia.BaseAbierta(alm.Ruta(), alm.CopiarEnCaliente))
	s := localerp.NuevoServidor(cat, obl, alm)
	s.Nombre = l.Espacio.Perfil.Nombre
	return s, nil
}

// franjaVerifactu dice, siempre a la vista, que las facturas salen sin VERI*FACTU.
func franjaVerifactu(idioma string) fyne.CanvasObject {
	texto := widget.NewLabel(localerp.AvisoVerifactu(idioma))
	texto.Wrapping = fyne.TextWrapWord
	texto.Importance = widget.WarningImportance
	icono := widget.NewIcon(theme.WarningIcon())
	return container.NewVBox(container.NewBorder(nil, nil, container.NewPadded(icono), nil, texto), widget.NewSeparator())
}

// elegirFicheros abre el selector del sistema —bloquea, así que va aparte— y le pasa a
// `listo` lo elegido en el hilo de la interfaz. Cancelar no hace nada.
func elegirFicheros(listo func([]string)) {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil {
			if !errors.Is(err, ficheros.ErrNoDisponible) {
				slog.Warn("erp: el selector de ficheros ha fallado", "error", err)
			}
			return
		}
		if len(rutas) > 0 {
			fyne.Do(func() { listo(rutas) })
		}
	}()
}
