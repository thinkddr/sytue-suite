// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/cuentas"
)

// Agenda es el registro de las cuentas de calendario y contactos (§6.11), uno por app:
// Calendar y Contacts sincronizan con él y Ajustes → Cuentas da de alta, vuelve a
// entrar y quita con él, así que una contraseña nueva vale en el acto en la app abierta.
func (l *App) Agenda() *agenda.Registro {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.agenda == nil {
		l.agenda = agenda.Abrir(l.Espacio, l.Secretos, l.Red)
	}
	return l.agenda
}

// proveedoresAgenda son los que se ofrecen al dar de alta: los preajustes con
// contraseña de aplicación, cualquier servidor DAV y las dos cuentas con OAuth.
func (l *App) proveedoresAgenda() (rotulos, claves []string) {
	for _, p := range dav.Preajustes {
		rotulos, claves = append(rotulos, p.Rotulo), append(claves, p.Proveedor)
	}
	rotulos = append(rotulos, "Google", "Microsoft 365 / Outlook", l.T("agenda.otro"))
	claves = append(claves, agenda.ProveedorGoogle, agenda.ProveedorMicrosoft, agenda.ProveedorDAV)
	return rotulos, claves
}

// altaAgenda da de alta una cuenta de calendario y contactos. Los campos cambian con
// el proveedor: usuario y contraseña de aplicación con DAV (y el servidor si no es uno
// conocido); nada con Google y Microsoft, que piden permiso en el navegador.
func (l *App) altaAgenda(w fyne.Window, listo func()) {
	rotulos, claves := l.proveedoresAgenda()
	usuario, clave, servidor := widget.NewEntry(), widget.NewPasswordEntry(), widget.NewEntry()
	servidor.SetPlaceHolder("https://")
	pista := ayuda("")
	campos := container.NewVBox()
	elegido := claves[0]
	esOAuth := func() bool { return elegido == agenda.ProveedorGoogle || elegido == agenda.ProveedorMicrosoft }
	quien := widget.NewSelect(rotulos, nil)
	rehacer := func() {
		items := []*widget.FormItem{widget.NewFormItem(l.T("ia.proveedor"), quien)}
		if !esOAuth() {
			items = append(items, widget.NewFormItem(l.T("agenda.usuario"), usuario), widget.NewFormItem(l.T("agenda.clave"), clave))
		}
		switch p, ok := dav.PreajusteDe(elegido); {
		case ok:
			pista.SetText(fmt.Sprintf(l.T("agenda.ayuda.preajuste"), p.Ayuda))
		case elegido == agenda.ProveedorDAV:
			items = append(items, widget.NewFormItem(l.T("agenda.servidor"), servidor))
			pista.SetText(l.T("agenda.ayuda.otro"))
		default:
			pista.SetText(l.T("nube.navegador"))
		}
		campos.Objects = []fyne.CanvasObject{widget.NewForm(items...)}
		campos.Refresh()
	}
	quien.OnChanged = func(string) {
		if i := quien.SelectedIndex(); i >= 0 {
			elegido = claves[i]
			rehacer()
		}
	}
	quien.SetSelectedIndex(0)
	var d *ui.Dialogo
	d = ui.Confirmacion(l.T("agenda.anadir"), l.T("cancelar"), l.T("guardar"), false, container.NewVBox(campos, pista), nil, func() {
		oauth := esOAuth()
		if !oauth && (strings.TrimSpace(usuario.Text) == "" || clave.Text == "") {
			return
		}
		d.Cerrar()
		alta := agenda.Alta{Proveedor: elegido, Usuario: strings.TrimSpace(usuario.Text), Clave: clave.Text, Servidor: strings.TrimSpace(servidor.Text)}
		texto := l.T("agenda.probando")
		if oauth {
			texto = l.T("nube.navegador")
		}
		aviso := Espera(w, l.T("agenda.anadir"), texto)
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), 5*time.Minute)
			defer parar()
			var err error
			if oauth {
				_, err = l.Agenda().ConectarOAuth(ctx, alta.Proveedor, "", "", func(destino string) error { return api.AbrirFuera(l.A, destino) })
			} else {
				_, err = l.Agenda().AnadirDAV(ctx, alta)
			}
			fyne.Do(func() {
				aviso.Cerrar()
				l.Fallo(w, err)
				listo()
			})
		}()
	})
	d.Mostrar(w.Canvas())
}

// reconectarAgenda vuelve a entrar en una cuenta que el servidor rechazó, sin perder
// sus calendarios y libretas: una contraseña de aplicación nueva, o el permiso otra vez.
func (l *App) reconectarAgenda(w fyne.Window, c cuentas.Cuenta, listo func()) {
	hacer := func(clave string) {
		aviso := Espera(w, l.T("cuentas.volver"), l.T("agenda.probando"))
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), 5*time.Minute)
			defer parar()
			err := l.Agenda().Reconectar(ctx, c.ID, clave, func(destino string) error { return api.AbrirFuera(l.A, destino) })
			fyne.Do(func() {
				aviso.Cerrar()
				l.Fallo(w, err)
				listo()
			})
		}()
	}
	for _, a := range l.Agenda().Cuentas() {
		if a.ID == c.ID && (a.Proveedor == agenda.ProveedorGoogle || a.Proveedor == agenda.ProveedorMicrosoft) {
			hacer("")
			return
		}
	}
	clave := widget.NewPasswordEntry()
	Formulario(w, l.T("cuentas.volver"), l.T("cancelar"), l.T("guardar"), []*widget.FormItem{
		widget.NewFormItem(l.T("agenda.clave"), clave),
	}, func(d *ui.Dialogo) {
		if clave.Text != "" {
			d.Cerrar()
			hacer(clave.Text)
		}
	})
}

// quitarAgenda quita una cuenta; sus calendarios y libretas se quedan en el equipo.
func (l *App) quitarAgenda(w fyne.Window, c cuentas.Cuenta, listo func()) {
	Confirmar(w, l.T("nube.quitar"), l.T("cuentas.quitar.pregunta"), l.T("cancelar"), l.T("nube.quitar"), true, func() {
		l.Fallo(w, l.Agenda().Quitar(c.ID))
		listo()
	})
}
