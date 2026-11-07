// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

// En la web: DialogoAdjuntos.tsx y el `DialogoImportar` de Dialogos.tsx, dentro de
// la pantalla de la base (cuyo marcador de paridad lleva base.go).

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los ficheros de una base: los adjuntos de un registro y el CSV que se importa como
// tabla nueva. Llegan por el selector del sistema o SOLTÁNDOLOS sobre la ventana:
//
//   - sobre la ficha abierta de un registro, son adjuntos de ese registro;
//   - sobre una fila de la rejilla, adjuntos de esa fila (en la columna de adjuntos
//     sobre la que caen, o en la primera que tenga la tabla);
//   - un CSV fuera de una ficha abre «Importar CSV» con él ya elegido.

// Soltado recibe los ficheros soltados sobre la ventana (`Window.SetOnDropped`). `pos`
// es la del puntero en el lienzo.
func (p *Pantalla) Soltado(pos fyne.Position, uris []fyne.URI) {
	p.Arrastrando(false)
	b := p.abierta
	if b == nil {
		return
	}
	var rutas []string
	for _, u := range uris {
		if u.Scheme() == "file" {
			rutas = append(rutas, u.Path())
		}
	}
	if len(rutas) == 0 {
		return
	}
	if f := p.ficha; f != nil {
		if campo, ok := f.campoDeAdjuntos(); ok && f.puedeEditar {
			f.subir(campo, rutas)
		}
		return
	}
	if csv := slices.IndexFunc(rutas, esCSV); csv >= 0 && puedeEsquema(b.Datos.Role) {
		p.dialogoImportar(rutas[csv])
		return
	}
	if b.rejilla == nil || !puedeEscribir(b.rol()) {
		return
	}
	r, campo, ok := b.rejilla.adjuntosEn(pos)
	if !ok {
		return
	}
	p.subirAdjuntos(r, campo, rutas, func(string) {}, nil)
}

// Arrastrando enciende (o apaga) el aviso de que hay ficheros encima de la ventana: la
// rejilla se recuadra para decir que se pueden soltar ahí (el `dragover` de la web).
func (p *Pantalla) Arrastrando(encima bool) {
	b := p.abierta
	if b == nil || b.rejilla == nil {
		return
	}
	b.rejilla.Resaltar(encima && puedeEscribir(b.rol()))
}

// esCSV dice si un fichero es un CSV, por su extensión.
func esCSV(ruta string) bool { return strings.EqualFold(filepath.Ext(ruta), ".csv") }

// errorDeAdjunto es el mensaje de un fallo al subir, como DialogoAdjuntos.tsx.
func (p *Pantalla) errorDeAdjunto(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		switch e.Status {
		case http.StatusRequestEntityTooLarge:
			return p.t("attach.tooLarge")
		case http.StatusUnprocessableEntity:
			return p.t("attach.badType")
		case http.StatusServiceUnavailable:
			return p.t("attach.unavailable")
		}
	}
	return p.mensajeDeError(err)
}

// errDemasiadoGrande es un fichero que no se llega a subir porque pasa del tope.
var errDemasiadoGrande = errors.New("bases: el fichero pasa del tope")

// subirAdjuntos sube los ficheros, uno tras otro, al campo de adjuntos de un registro.
// Tras cada uno el registro se actualiza en la rejilla; al final, `listo` recibe el
// fallo (vacío si no lo hubo) y `cambio`, si no es nil, el registro como ha quedado.
func (p *Pantalla) subirAdjuntos(r api.BasesRecord, campo api.BasesField, rutas []string, listo func(fallo string), cambio func(api.BasesRecord)) {
	b := p.abierta
	b.Estado = p.t("attach.uploading")
	p.Repintar()
	p.fondo(func() {
		var subidos []api.BasesAttachment
		var err error
		for _, ruta := range rutas {
			var datos []byte
			if datos, err = leerConTope(ruta, api.MaxBytesAdjuntoBases); err != nil {
				break
			}
			ctx, cancelar := p.ctx()
			var a api.BasesAttachment
			a, err = p.svc.UploadAttachment(ctx, r.ID, campo.ID, filepath.Base(ruta), datos)
			cancelar()
			if err != nil {
				break
			}
			subidos = append(subidos, a)
		}
		enHilo(func() {
			fallo := ""
			switch {
			case errors.Is(err, errDemasiadoGrande):
				fallo = p.t("attach.tooLarge")
			case err != nil:
				fallo = p.errorDeAdjunto(err)
			}
			if p.abierta != b {
				return
			}
			b.Estado = ""
			if len(subidos) > 0 {
				r = conAdjuntos(r, campo.ID, append(slices.Clone(r.Attachments[campo.ID]), subidos...))
				p.ponerRegistro(r)
				if cambio != nil {
					cambio(r)
				}
			}
			if fallo != "" && p.ficha == nil {
				b.Fallo = fallo
			}
			p.Repintar()
			listo(fallo)
		})
	})
}

// leerConTope lee un fichero entero si no pasa de `tope` bytes.
func leerConTope(ruta string, tope int64) ([]byte, error) {
	info, err := os.Stat(ruta)
	if err != nil {
		return nil, err
	}
	if info.Size() > tope {
		return nil, errDemasiadoGrande
	}
	return os.ReadFile(ruta)
}

// conAdjuntos es el registro con otra lista de adjuntos en un campo. No toca el mapa
// del registro de partida, que puede seguir en otra ventana de la rejilla.
func conAdjuntos(r api.BasesRecord, campo string, lista []api.BasesAttachment) api.BasesRecord {
	m := make(map[string][]api.BasesAttachment, len(r.Attachments)+1)
	for k, v := range r.Attachments {
		m[k] = v
	}
	m[campo] = lista
	r.Attachments = m
	return r
}

// ponerRegistro cambia un registro en lo que hay en memoria y en la rejilla.
func (p *Pantalla) ponerRegistro(r api.BasesRecord) {
	b := p.abierta
	b.vent.editar(func(x api.BasesRecord) (api.BasesRecord, bool) {
		if x.ID == r.ID {
			return r, true
		}
		return x, true
	})
	p.ponerRejilla()
}

// adjuntosEn es el registro y el campo de adjuntos a los que va lo que se suelta en
// `pos` (en el lienzo): la fila que hay debajo y, de la tabla, la columna de adjuntos
// sobre la que cae o, si cae en otra, la primera.
func (g *rejilla) adjuntosEn(pos fyne.Position) (api.BasesRecord, api.BasesField, bool) {
	if g.r == nil || !g.Visible() {
		return api.BasesRecord{}, api.BasesField{}, false
	}
	local := pos.Subtract(fyne.CurrentApp().Driver().AbsolutePositionForObject(g))
	if local.X < 0 || local.Y < 0 || local.X >= g.Size().Width || local.Y >= g.Size().Height {
		return api.BasesRecord{}, api.BasesField{}, false
	}
	fila := g.filaEn(local)
	if fila < 0 {
		return api.BasesRecord{}, api.BasesField{}, false
	}
	if c, ok := g.r.celdaEn(local); ok && g.Campos[c.col].Kind == api.BasesKindAttachment {
		return g.Registros[fila], g.Campos[c.col], true
	}
	for _, c := range g.Campos {
		if c.Kind == api.BasesKindAttachment {
			return g.Registros[fila], c, true
		}
	}
	return api.BasesRecord{}, api.BasesField{}, false
}

// descargarAdjunto deja el fichero donde el sistema guarda las descargas y dice dónde.
func (p *Pantalla) descargarAdjunto(r api.BasesRecord, a api.BasesAttachment, aviso func(string)) {
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		ruta := ""
		cuerpo, err := p.svc.DownloadAttachment(ctx, r.ID, a.ID)
		if err == nil {
			ruta, err = p.Guardar(a.Name, cuerpo)
			_ = cuerpo.Close()
		}
		enHilo(func() {
			if err != nil {
				aviso(p.mensajeDeError(err))
				return
			}
			aviso(a.Name + " — " + ruta)
		})
	})
}

// ---- La ficha y sus adjuntos ----

// campoDeAdjuntos es el primer campo de adjuntos de la tabla, al que va lo soltado
// sobre la ficha.
func (f *fichaRegistro) campoDeAdjuntos() (api.BasesField, bool) {
	for _, c := range f.b.Campos {
		if c.Kind == api.BasesKindAttachment {
			return c, true
		}
	}
	return api.BasesField{}, false
}

// subir sube ficheros a un campo de adjuntos desde la ficha: se suben ya, sin esperar
// a «Guardar», como en la web.
func (f *fichaRegistro) subir(campo api.BasesField, rutas []string) {
	f.subiendo, f.avisoAdjuntos = true, ""
	f.rehacer()
	f.p.subirAdjuntos(f.r, campo, rutas, func(fallo string) {
		f.subiendo, f.avisoAdjuntos = false, fallo
		f.rehacer()
	}, func(r api.BasesRecord) { f.r.Attachments = r.Attachments })
}

// borrarAdjunto lo quita del registro, tras preguntar.
func (f *fichaRegistro) borrarAdjunto(campo api.BasesField, a api.BasesAttachment) {
	p := f.p
	p.confirmar(p.t("attach.delete", i18n.Vars{"name": a.Name}), p.t("common.delete"), func() {
		p.fondo(func() {
			ctx, cancelar := p.ctx()
			defer cancelar()
			err := p.svc.DeleteAttachment(ctx, f.r.ID, a.ID)
			enHilo(func() {
				if err != nil {
					f.avisoAdjuntos = p.mensajeDeError(err)
					f.rehacer()
					return
				}
				resto := slices.DeleteFunc(slices.Clone(f.r.Attachments[campo.ID]), func(x api.BasesAttachment) bool { return x.ID == a.ID })
				r := conAdjuntos(f.r, campo.ID, resto)
				f.r.Attachments = r.Attachments
				if p.abierta == f.b {
					p.ponerRegistro(r)
					p.Repintar()
				}
				f.avisoAdjuntos = ""
				f.rehacer()
			})
		})
	})
}

// grupoAdjuntos es un campo de adjuntos en la ficha: cada fichero con su descarga y su
// borrado, «Añadir un fichero» y la ayuda de qué se admite (que también se suelta).
func (f *fichaRegistro) grupoAdjuntos(c api.BasesField, v tema.Variante, ancho float32) fyne.CanvasObject {
	p := f.p
	editable := f.puedeEditar
	filas := []fyne.CanvasObject{}
	lista := f.r.Attachments[c.ID]
	if len(lista) == 0 {
		filas = append(filas, ui.NuevoRotulo(p.t("attach.none"), tema.TextoSM, ui.Normal, "text-muted"))
	}
	for _, a := range lista {
		nombre := ui.NuevoRotulo(a.Name, tema.TextoSM, ui.Normal, "text")
		nombre.Truncar = true
		linea := []fyne.CanvasObject{nombre}
		if p.Guardar != nil {
			bajar := ui.NuevoBoton(p.t("attach.download", i18n.Vars{"name": a.Name}), func() {
				p.descargarAdjunto(f.r, a, func(m string) { f.avisoAdjuntos = m; f.rehacer() })
			}).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Descargar, 14)
			bajar.SoloIcono = true
			linea = append(linea, bajar)
		}
		if editable {
			quitar := ui.NuevoBoton(p.t("attach.delete", i18n.Vars{"name": a.Name}), func() { f.borrarAdjunto(c, a) }).
				Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Papelera, 14)
			quitar.SoloIcono = true
			linea = append(linea, quitar)
		}
		filas = append(filas, container.New(extremos{Hueco: tema.Espacio(1), Centro: true}, linea...))
	}
	if editable && p.Elegir != nil {
		texto := p.t("attach.add")
		if f.subiendo {
			texto = p.t("attach.uploading")
		}
		anadir := ui.NuevoBoton(texto, func() {
			p.Elegir(func(rutas []string) { f.subir(c, rutas) })
		}).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Mas, 14)
		anadir.Inerte = f.subiendo
		filas = append(filas, container.New(ui.Fila{}, anadir))
	}
	ayuda := ""
	if editable {
		ayuda = p.t("attach.help")
	}
	if f.avisoAdjuntos != "" {
		filas = append(filas, ui.NuevoParrafo(f.avisoAdjuntos, tema.TextoSM, ui.Normal, "text-muted", v).A(ancho))
	}
	return ui.GrupoCampo(c.Name, false, container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...), ayuda, "")
}

// ---- Importar un CSV ----

// importarCSV pide el fichero al sistema y abre el diálogo con él.
func (p *Pantalla) importarCSV() {
	if p.Elegir == nil {
		return
	}
	p.Elegir(func(rutas []string) {
		if len(rutas) > 0 {
			p.dialogoImportar(rutas[0])
		}
	})
}

// dialogoImportar es DialogoImportar: el fichero elegido, el nombre de la tabla nueva
// (el del fichero, que se puede cambiar) y «Importar».
func (p *Pantalla) dialogoImportar(ruta string) {
	b := p.abierta
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	nombre := ui.NuevaEntrada("")
	nombre.Rotulo = p.t("import.name")
	base := strings.TrimSuffix(filepath.Base(ruta), filepath.Ext(ruta))
	nombre.PonerTexto(string([]rune(base)[:min(len([]rune(base)), 120)]))
	csv, errLeer := leerConTope(ruta, api.MaxBytesCSVBases)
	fallo := ""
	switch {
	case errors.Is(errLeer, errDemasiadoGrande):
		fallo = p.t("import.tooBig")
	case errLeer != nil:
		fallo = p.t("err.generic")
	}
	var d *ui.Dialogo
	var importar *ui.Boton
	cuerpo := func() fyne.CanvasObject {
		hijos := []fyne.CanvasObject{
			ui.NuevoParrafo(p.t("import.help"), tema.TextoSM, ui.Normal, "text-muted", v).A(p.anchoDialogo(560)),
			ui.GrupoCampo(p.t("import.file"), false, ui.NuevoRotulo(filepath.Base(ruta), tema.TextoSM, ui.Normal, "text"), "", ""),
			ui.GrupoCampo(p.t("import.name"), true, nombre, "", ""),
		}
		if fallo != "" {
			hijos = append(hijos, alerta(fallo, v))
		}
		return container.New(ui.Columna{Hueco: tema.Espacio(4)}, hijos...)
	}
	ajustar := func() {
		importar.Inerte = errLeer != nil || strings.TrimSpace(nombre.Texto()) == ""
		importar.Refresh()
	}
	d, importar = p.dialogo(p.t("import.title"), cuerpo(), p.t("import.run"), func() {
		if errLeer != nil || strings.TrimSpace(nombre.Texto()) == "" {
			return
		}
		var t api.BasesTable
		p.enDialogo(d, func() (err error) {
			ctx, cancelar := p.ctx()
			defer cancelar()
			t, err = p.svc.ImportCSV(ctx, b.ID, strings.TrimSpace(nombre.Texto()), csv)
			return err
		}, func(m string) { fallo = m; d.CambiarCuerpo(cuerpo()) }, func() {
			b.Activa, b.rejilla = t.ID, nil // la estructura, al llegar, la encuentra y la abre
			p.cargarEstructura()
		})
	})
	nombre.AlCambiar(func(string) { ajustar() })
	d.Foco = nombre
	ajustar()
	d.Mostrar(p.ventana.Canvas())
}
