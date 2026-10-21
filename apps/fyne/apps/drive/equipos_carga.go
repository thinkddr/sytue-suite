// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"image/color"
	"io"
	"net/http"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Lo que las unidades de equipo piden al servicio y sus diálogos.

func (p *Pantalla) montarEquipos() {
	q := NuevaVistaEquipos(p.tr)
	p.sec.equipos = q
	q.AlReintentar = p.reintentarCon(UnidadesDeEquipo, func() { go p.cargarEquipos() })
	q.AlNueva = p.pedirUnidad
	q.AlAbrirUnidad = func(u api.UnidadEquipo) {
		unidad := u
		q.Abierta, q.Pila = &unidad, nil
		go p.cargarUnidad()
	}
	q.AlVolver = func() {
		q.Abierta, q.Pila, q.Contenido, q.Denegada = nil, nil, nil, false
		p.Repintar()
		go p.cargarEquipos()
	}
	q.AlIr = func(nivel int) {
		if nivel < 0 {
			q.Pila = nil
		} else if nivel < len(q.Pila) {
			q.Pila = q.Pila[:nivel+1]
		}
		go p.cargarUnidad()
	}
	q.AlAbrir = func(el Elemento) {
		if el.Carpeta {
			q.Pila = append(q.Pila, el)
			go p.cargarUnidad()
			return
		}
		if TipoVista(el) != "" {
			p.pedirVistaEquipo(el)
			return
		}
		go p.descargarDeEquipo(el)
	}
	q.AlDescargar = func(el Elemento) { go p.descargarDeEquipo(el) }
	q.AlBorrar = p.pedirBorrarDeEquipo
	q.AlNuevaCarpeta = p.pedirCarpetaEquipo
	q.AlSubir = p.subirAEquipo
	q.AlMiembros = func() { p.pedirMiembros() }
	q.AlRenombrar = p.pedirRenombrarUnidad
	q.AlArchivar = func() { go p.archivarUnidad() }
	q.AlReintentarDetalle = func() { go p.cargarUnidad() }
	q.AlCargarMas = p.masDeUnidad
}

// cargarEquipos pide las unidades de las que se es miembro.
func (p *Pantalla) cargarEquipos() {
	q := p.sec.equipos
	enHilo(func() { q.Fase = Cargando; p.Repintar() })
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	unidades, err := p.svc.TeamDrives(ctx)
	enHilo(func() {
		q.Unidades = unidades
		q.Fase = FaseDe(err, p.hayRed() || err == nil, len(unidades))
		p.Repintar()
	})
}

// carpetaEquipo es el id de la carpeta de la unidad que se mira; vacío, su raíz.
func (q *VistaEquipos) carpetaEquipo() string {
	if n := len(q.Pila); n > 0 {
		return q.Pila[n-1].ID
	}
	return ""
}

// cargarUnidad pide la carpeta de la unidad abierta. Un 403 o 404 es que se revocó el
// acceso o la unidad dejó de existir (deny-by-default).
func (p *Pantalla) cargarUnidad() {
	q := p.sec.equipos
	var id, carpeta string
	fyne.DoAndWait(func() {
		q.FaseDetalle, q.Denegada, q.HayMas, q.CargandoMas = Cargando, false, false, false
		if q.Abierta != nil {
			id, carpeta = q.Abierta.ID, q.carpetaEquipo()
		}
		p.Repintar()
	})
	if id == "" || p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	c, err := p.svc.TeamDriveChildren(ctx, id, carpeta, "")
	els := DeHijos(api.Children{Folders: c.Folders, Files: c.Files})
	enHilo(func() {
		if q.Abierta == nil || q.Abierta.ID != id || q.carpetaEquipo() != carpeta {
			return
		}
		n := codigo(err)
		q.Denegada = n == http.StatusForbidden || n == http.StatusNotFound
		q.Contenido, q.cursorUnidad = els, c.NextCursor
		q.HayMas = err == nil && c.NextCursor != ""
		if c.Role != "" {
			q.Abierta.Role = c.Role
		}
		q.FaseDetalle = FaseDe(err, p.hayRed() || err == nil, len(els))
		p.Repintar()
	})
}

// masDeUnidad pide la página siguiente de la carpeta de la unidad.
func (p *Pantalla) masDeUnidad() {
	q := p.sec.equipos
	if q.CargandoMas || q.cursorUnidad == "" || q.Abierta == nil {
		return
	}
	q.CargandoMas = true
	id, carpeta, cursor := q.Abierta.ID, q.carpetaEquipo(), q.cursorUnidad
	p.Repintar()
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		c, err := p.svc.TeamDriveChildren(ctx, id, carpeta, cursor)
		enHilo(func() {
			q.CargandoMas = false
			if err != nil {
				p.avisar(ui.AvisoPeligro, p.tr.T("team.loadMoreFailed", nil), "")
			} else {
				q.Contenido = append(q.Contenido, DeHijos(api.Children{Folders: c.Folders, Files: c.Files})...)
				q.cursorUnidad, q.HayMas = c.NextCursor, c.NextCursor != ""
			}
			p.Repintar()
		})
	}()
}

// pedirUnidad es «Nueva unidad»: el nombre con su pista, la nota de que lo de aquí es de
// la organización, y al crearla se entra en ella.
func (p *Pantalla) pedirUnidad() {
	if p.win == nil || p.svc == nil {
		return
	}
	v := p.variante()
	t := func(k string) string { return p.tr.T(k, nil) }
	nota := ui.NuevoParrafo(t("team.create.note"), tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}, ui.Normal, "text-muted", v)
	d := p.dialogoCampoCon(formCampo{
		titulo: t("team.create.title"), etiqueta: t("team.create.nameLabel"), cta: t("team.create.submit"),
		cancelar: t("team.create.cancel"), campo: ui.NuevaEntrada(t("team.create.namePlaceholder")),
		pista: ui.NuevaPistaCampo(t("team.create.nameHint")), nota: nota,
	}, func(nombre string) bool {
		ctx, cancelar := p.ctx()
		defer cancelar()
		u, err := p.svc.CreateTeamDrive(ctx, nombre)
		if err != nil {
			p.avisar(ui.AvisoPeligro, t("team.createFailed"), "")
			return false
		}
		p.avisar(ui.AvisoExito, p.tr.T("team.created", i18n.Vars{"name": nombre}), "")
		enHilo(func() {
			unidad := u
			p.sec.equipos.Abierta, p.sec.equipos.Pila = &unidad, nil
		})
		go p.cargarUnidad()
		return true
	})
	d.Mostrar(p.win.Canvas())
}

// pedirCarpetaEquipo es «Nueva carpeta» dentro de la unidad.
func (p *Pantalla) pedirCarpetaEquipo() {
	q := p.sec.equipos
	if p.win == nil || q.Abierta == nil {
		return
	}
	t := func(k string) string { return p.tr.T(k, nil) }
	id, padre := q.Abierta.ID, q.carpetaEquipo()
	d := p.dialogoCampoCon(formCampo{
		titulo: t("team.newFolder.title"), etiqueta: t("team.newFolder.label"), cta: t("team.create.submit"),
		cancelar: t("team.create.cancel"), campo: ui.NuevaEntrada(t("team.newFolder.placeholder")),
	}, func(nombre string) bool {
		ctx, cancelar := p.ctx()
		defer cancelar()
		if _, err := p.svc.CreateTeamDriveFolder(ctx, id, padre, nombre); err != nil {
			clave := "team.newFolder.failed"
			if codigo(err) == http.StatusForbidden {
				clave = "team.newFolder.denied"
			}
			p.avisar(ui.AvisoPeligro, t(clave), "")
			return false
		}
		p.avisar(ui.AvisoExito, p.tr.T("team.newFolder.created", i18n.Vars{"name": nombre}), "")
		go p.cargarUnidad()
		return true
	})
	d.Mostrar(p.win.Canvas())
}

// pedirRenombrarUnidad cambia el nombre de la unidad. Solo un gestor.
func (p *Pantalla) pedirRenombrarUnidad() {
	q := p.sec.equipos
	if p.win == nil || q.Abierta == nil {
		return
	}
	t := func(k string) string { return p.tr.T(k, nil) }
	u := *q.Abierta
	campo := ui.NuevaEntrada("")
	campo.PonerTexto(u.Name)
	d := p.dialogoCampoCon(formCampo{
		titulo: t("team.rename.title"), etiqueta: t("team.rename.label"), cta: t("team.rename"),
		cancelar: t("team.create.cancel"), campo: campo,
	}, func(nombre string) bool {
		if nombre == u.Name {
			return true
		}
		ctx, cancelar := p.ctx()
		defer cancelar()
		nueva, err := p.svc.RenameTeamDrive(ctx, u.ID, nombre)
		if err != nil {
			clave := "team.rename.failed"
			if codigo(err) == http.StatusForbidden {
				clave = "team.rename.denied"
			}
			p.avisar(ui.AvisoPeligro, t(clave), "")
			return false
		}
		enHilo(func() {
			if q.Abierta != nil && q.Abierta.ID == u.ID {
				q.Abierta.Name = nueva.Name
				p.Repintar()
			}
		})
		p.avisar(ui.AvisoExito, p.tr.T("team.rename.done", i18n.Vars{"name": nueva.Name}), "")
		return true
	})
	d.Mostrar(p.win.Canvas())
}

// archivarUnidad la archiva o la reactiva.
func (p *Pantalla) archivarUnidad() {
	q := p.sec.equipos
	var u api.UnidadEquipo
	fyne.DoAndWait(func() {
		if q.Abierta != nil {
			u = *q.Abierta
			q.Ocupado = true
			p.Repintar()
		}
	})
	if u.ID == "" {
		return
	}
	defer enHilo(func() { q.Ocupado = false; p.Repintar() })
	archivar := u.ArchivedAt == nil
	ctx, cancelar := p.ctx()
	defer cancelar()
	nueva, err := p.svc.ArchiveTeamDrive(ctx, u.ID, archivar)
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("team.archive.failed", nil), "")
		return
	}
	enHilo(func() {
		if q.Abierta != nil && q.Abierta.ID == u.ID {
			q.Abierta.ArchivedAt = nueva.ArchivedAt
		}
	})
	clave := "team.unarchive.done"
	if archivar {
		clave = "team.archive.done"
	}
	p.avisar(ui.AvisoExito, p.tr.T(clave, nil), "")
}

// pedirBorrarDeEquipo confirma y manda a la papelera de la unidad.
func (p *Pantalla) pedirBorrarDeEquipo(el Elemento) {
	q := p.sec.equipos
	if p.win == nil || q.Abierta == nil {
		return
	}
	titulo := "team.delete.fileTitle"
	if el.Carpeta {
		titulo = "team.delete.folderTitle"
	}
	id := q.Abierta.ID
	cuerpo := parrafo(p.variante(), p.tr.T("team.delete.body", i18n.Vars{"name": el.Nombre}), 0)
	var d *ui.Dialogo
	d = ui.Confirmacion(p.tr.T(titulo, nil), p.tr.T("team.create.cancel", nil), p.tr.T("team.delete.cta", nil), true, cuerpo,
		func() { d.Cerrar() },
		func() {
			d.Cerrar()
			go func() {
				enHilo(func() { q.Ocupado = true; p.Repintar() })
				defer enHilo(func() { q.Ocupado = false; p.Repintar() })
				ctx, cancelar := p.ctx()
				defer cancelar()
				if err := p.svc.DeleteTeamDriveItem(ctx, id, claseDe(el), el.ID); err != nil {
					p.avisar(ui.AvisoPeligro, p.tr.T("team.delete.failed", nil), "")
					return
				}
				p.avisar(ui.AvisoExito, p.tr.T("team.delete.done", i18n.Vars{"name": el.Nombre}), "")
				p.cargarUnidad()
			}()
		})
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Mostrar(p.win.Canvas())
}

// subirAEquipo es «Subir»: el selector del sistema y, uno a uno, a la carpeta de la unidad.
func (p *Pantalla) subirAEquipo() {
	q := p.sec.equipos
	if p.ElegirFicheros == nil || q.Abierta == nil || q.Subiendo || q.SinRed() {
		return
	}
	id, carpeta := q.Abierta.ID, q.carpetaEquipo()
	p.ElegirFicheros(func(entradas []Entrada) {
		if len(entradas) == 0 {
			return
		}
		q.Subiendo = true
		p.Repintar()
		go func() {
			ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Minute)
			defer cancelar()
			fallidos := 0
			for _, e := range entradas {
				if err := subirAEquipoUno(ctx, p.svc, id, carpeta, e); err != nil {
					fallidos++
				}
			}
			enHilo(func() { q.Subiendo = false; p.Repintar() })
			if fallidos == 0 {
				p.avisar(ui.AvisoExito, p.tr.T("team.upload.done", i18n.Vars{"count": len(entradas)}), "")
			} else {
				p.avisar(ui.AvisoPeligro, p.tr.T("team.upload.partial", i18n.Vars{"uploaded": len(entradas) - fallidos, "count": len(entradas)}), "")
			}
			p.cargarUnidad()
		}()
	})
}

func subirAEquipoUno(ctx context.Context, svc *api.Drive, id, carpeta string, e Entrada) error {
	r, tamano, err := e.abrir()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	nombre := e.nombre()
	_, err = svc.UploadTeamDrive(ctx, id, carpeta, nombre, TipoDe(nombre), r, tamano)
	return err
}

// descargarDeEquipo baja un fichero de la unidad a la carpeta de descargas.
func (p *Pantalla) descargarDeEquipo(el Elemento) {
	var id string
	fyne.DoAndWait(func() {
		if p.sec.equipos.Abierta != nil {
			id = p.sec.equipos.Abierta.ID
		}
	})
	if id == "" || p.svc == nil {
		return
	}
	p.guardarDescarga(el.Nombre, func(ctx context.Context) (io.ReadCloser, error) {
		return p.svc.DownloadTeamDrive(ctx, id, el.ID)
	})
}

// guardarDescarga baja un fichero a la carpeta de descargas y avisa de dónde quedó.
// La comparten las unidades de equipo y las nubes personales.
func (p *Pantalla) guardarDescarga(nombre string, abrir func(context.Context) (io.ReadCloser, error)) {
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancelar()
	cuerpo, err := abrir(ctx)
	var ruta string
	if err == nil {
		defer func() { _ = cuerpo.Close() }()
		var carpeta string
		if carpeta, err = ficheros.Carpeta(); err == nil {
			ruta, err = ficheros.Guardar(carpeta, nombre, cuerpo)
		}
	}
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("files.error.network", nil), motivo(err))
		return
	}
	p.avisar(ui.AvisoExito, nombre, ruta)
}

// pedirVistaEquipo es la vista previa de un fichero de la unidad.
func (p *Pantalla) pedirVistaEquipo(el Elemento) {
	q := p.sec.equipos
	if q.Abierta == nil {
		return
	}
	id := q.Abierta.ID
	p.abrirVista(el, func(ctx context.Context) (io.ReadCloser, error) { return p.svc.DownloadTeamDrive(ctx, id, el.ID) },
		func() { go p.descargarDeEquipo(el) })
}

// ---- Miembros ----

// dialogoMiembros es `MembersDialog`: añadir a alguien con su rol, y la lista con el rol
// de cada uno y quitarlo. Solo un gestor lo abre.
type dialogoMiembros struct {
	p        *Pantalla
	id       string
	d        *ui.Dialogo
	miembros []api.MiembroEquipo
	fallo    bool
	ocupado  bool

	sujeto *ui.Entrada
	rol    *ui.Selector
	anadir *ui.Boton
	cuerpo *fyne.Container
	ancho  float32
}

// pedirMiembros abre el diálogo de miembros y lo devuelve (para el guardián).
func (p *Pantalla) pedirMiembros() *dialogoMiembros {
	q := p.sec.equipos
	if p.win == nil || q.Abierta == nil {
		return nil
	}
	t := func(k string) string { return p.tr.T(k, nil) }
	dm := &dialogoMiembros{p: p, id: q.Abierta.ID, cuerpo: container.NewStack()}
	dm.ancho = AnchoCuerpoDialogo(ui.DialogoMD, q.Ventana)
	dm.sujeto = ui.NuevaEntrada(t("team.member.subjectPlaceholder"))
	dm.sujeto.AlCambiar(func(string) { dm.pintarBoton() })
	dm.sujeto.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			dm.agregar()
			return true
		}
		return false
	}
	nombres := make([]string, 0, len(rolesEquipos))
	for _, r := range rolesEquipos {
		nombres = append(nombres, t(claveRol[r]))
	}
	dm.rol = ui.NuevoSelector(nombres, func(int) { dm.pintar() })
	dm.anadir = ui.NuevoBoton(t("team.member.add"), dm.agregar).Con(ui.Primario, ui.TallaMD)
	dm.d = ui.NuevoDialogo(p.tr.T("team.member.title", i18n.Vars{"name": q.Abierta.Name}), dm.cuerpo)
	dm.d.RotuloCerrar = t("ui.close")
	dm.pintar()
	dm.d.Mostrar(p.win.Canvas())
	p.enSegundoPlano(dm.cargar)
	return dm
}

func (dm *dialogoMiembros) t(k string, v ...i18n.Vars) string {
	var vars i18n.Vars
	if len(v) > 0 {
		vars = v[0]
	}
	return dm.p.tr.T(k, vars)
}

func (dm *dialogoMiembros) cargar() {
	ctx, cancelar := dm.p.ctx()
	defer cancelar()
	m, err := dm.p.svc.TeamDriveMembers(ctx, dm.id)
	enHilo(func() {
		dm.miembros, dm.fallo = m, err != nil
		if dm.miembros == nil && err == nil {
			dm.miembros = []api.MiembroEquipo{}
		}
		dm.pintar()
	})
}

// falla es el motivo concreto que devolvió el servicio, que manda sobre el genérico.
func (dm *dialogoMiembros) falla(err error, generico string) {
	clave := generico
	switch codigo(err) {
	case http.StatusConflict:
		clave = "team.member.lastManager"
	case http.StatusForbidden:
		clave = "team.member.denied"
	}
	dm.p.avisar(ui.AvisoPeligro, dm.t(clave), "")
}

// poner añade o cambia de rol y recarga.
func (dm *dialogoMiembros) poner(sujeto, rol string, alFinal func(error)) {
	if dm.ocupado {
		return
	}
	dm.ocupado = true
	dm.pintar()
	dm.p.enSegundoPlano(func() {
		ctx, cancelar := dm.p.ctx()
		defer cancelar()
		err := dm.p.svc.PutTeamDriveMember(ctx, dm.id, sujeto, rol)
		enHilo(func() { dm.ocupado = false; alFinal(err); dm.pintar() })
		if err == nil {
			dm.cargar()
		}
	})
}

func (dm *dialogoMiembros) agregar() {
	sujeto := strings.TrimSpace(dm.sujeto.Texto())
	if sujeto == "" {
		return
	}
	rol := rolesEquipos[max(dm.rol.Elegida, 0)]
	dm.poner(sujeto, rol, func(err error) {
		if err != nil {
			dm.falla(err, "team.member.addFailed")
			return
		}
		dm.sujeto.PonerTexto("")
		dm.p.avisar(ui.AvisoExito, dm.t("team.member.added", i18n.Vars{"subject": sujeto, "role": dm.t(claveRol[rol])}), "")
	})
}

func (dm *dialogoMiembros) quitar(m api.MiembroEquipo) {
	if dm.ocupado {
		return
	}
	dm.ocupado = true
	dm.pintar()
	dm.p.enSegundoPlano(func() {
		ctx, cancelar := dm.p.ctx()
		defer cancelar()
		err := dm.p.svc.RemoveTeamDriveMember(ctx, dm.id, m.Subject)
		enHilo(func() {
			dm.ocupado = false
			if err != nil {
				dm.falla(err, "team.member.removeFailed")
			}
			dm.pintar()
		})
		if err == nil {
			dm.cargar()
		}
	})
}

func (dm *dialogoMiembros) pintarBoton() {
	dm.anadir.Inerte = strings.TrimSpace(dm.sujeto.Texto()) == ""
	dm.anadir.Cargando = dm.ocupado
	dm.anadir.Refresh()
}

func (dm *dialogoMiembros) pintar() {
	v := dm.p.variante()
	dm.pintarBoton()
	rol := rolesEquipos[max(dm.rol.Elegida, 0)]
	// La caja de añadir: `border bg-bg-subtle rounded-lg p-3`, los dos campos y el botón
	// alineados abajo (`items-end`) y la pista a todo el ancho debajo.
	fila := filaForm{
		campos: []fyne.CanvasObject{
			grupoCampo(dm.t("team.member.subjectLabel"), dm.sujeto),
			grupoCampo(dm.t("team.member.roleLabel"), dm.rol),
		},
		anchos: []float32{0, 40 * 0.25 * tema.Rem}, boton: dm.anadir,
	}.objeto()
	pista := ui.NuevaPistaCampo(dm.t(clavePodra[rol]))
	dentro := container.New(&pilaSeparada{separas: []float32{0, tema.Espacio(2)}}, fila, pista)
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioLG)
	anadir := container.New(&conMargen{m: tema.Espacio(3) + tema.Borde}, append(caja.Objetos(), dentro)...)

	bloques := []fyne.CanvasObject{anadir, dm.lista(v)}
	separas := []float32{0, tema.Espacio(4)}
	if len(dm.miembros) > 0 {
		bloques = append(bloques, lineaTexto(ui.Texto(dm.t("team.member.note"), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)), tema.TextoXS))
		separas = append(separas, tema.Espacio(4))
	}
	dm.cuerpo.Objects = []fyne.CanvasObject{container.New(&pilaSeparada{separas: separas, ancho: &dm.ancho}, bloques...)}
	dm.cuerpo.Refresh()
	dm.d.Recolocar()
}

// lista son los miembros, su esqueleto o su fallo.
func (dm *dialogoMiembros) lista(v tema.Variante) fyne.CanvasObject {
	switch {
	case dm.miembros == nil && !dm.fallo:
		// `space-y-2 py-2` con tres barras de `2.25rem`.
		barras := make([]fyne.CanvasObject, 3)
		for i := range barras {
			barras[i] = ui.NuevoEsqueleto(0, 2.25*tema.Rem)
		}
		return container.New(ui.Margen{Arr: tema.Espacio(2), Abj: tema.Espacio(2)},
			container.New(ui.Columna{Hueco: tema.Espacio(2)}, barras...))
	case dm.fallo:
		texto := lineaTexto(ui.Texto(dm.t("team.member.loadFailed"), tema.TextoSM, ui.Normal, tema.Color("text-muted", v)), tema.TextoSM)
		reintentar := ui.NuevoBoton(dm.t("team.retry"), func() {
			dm.fallo, dm.miembros = false, nil
			dm.pintar()
			dm.p.enSegundoPlano(dm.cargar)
		}).Con(ui.Secundario, ui.TallaSM)
		return container.New(ui.Margen{Arr: tema.Espacio(4), Abj: tema.Espacio(4)}, container.NewBorder(nil, nil, texto, reintentar))
	}
	filas := make([]fyne.CanvasObject, 0, len(dm.miembros))
	for i, m := range dm.miembros {
		filas = append(filas, dm.fila(m, i > 0, v))
	}
	borde := ui.NuevaCaja(color.NRGBA{}, tema.Color("border", v), tema.RadioLG)
	return container.New(&conMargen{m: tema.Borde}, append(borde.Objetos(), container.New(ui.Columna{}, filas...))...)
}

// fila es un miembro: `px-3 py-2 gap-3`, el sujeto en `text-sm font-medium`, el selector
// de su rol y el botón de quitar (`p-1.5`, papelera de 16, `hover:bg-danger-bg`).
func (dm *dialogoMiembros) fila(m api.MiembroEquipo, filete bool, v tema.Variante) fyne.CanvasObject {
	miembro := m
	nombres := make([]string, 0, len(rolesEquipos))
	elegido := 0
	for i, r := range rolesEquipos {
		nombres = append(nombres, dm.t(claveRol[r]))
		if r == m.Role {
			elegido = i
		}
	}
	sel := ui.NuevoSelector(nombres, func(i int) {
		if rolesEquipos[i] == miembro.Role {
			return
		}
		dm.poner(miembro.Subject, rolesEquipos[i], func(err error) {
			if err != nil {
				dm.falla(err, "team.member.roleFailed")
			}
		})
	})
	sel.Elegida = elegido
	sel.Inerte(dm.ocupado)
	quitar := nuevoBotonTexto("", tema.TextoSM, ui.Normal, func() { dm.quitar(miembro) })
	quitar.cuerpo, quitar.ladoIcono = glifoPapelera, 16
	quitar.margenX, quitar.alto = 1.5*0.25*tema.Rem, 16+2*1.5*0.25*tema.Rem
	if ui.EsTactil() { // `pointer-coarse:h-11 w-11`: medía 28 con el dedo
		quitar.margenX, quitar.alto = (ui.SueloTactilBarra-16)/2, ui.SueloTactilBarra
	}
	quitar.tinta, quitar.fondoEncima, quitar.tintaEncima = "text-muted", "danger-bg", "danger-text"
	quitar.Nombre = dm.t("team.member.remove", i18n.Vars{"subject": m.Subject})
	quitar.Inerte = dm.ocupado
	nombre := ui.Texto(m.Subject, tema.TextoSM, ui.Medio, tema.Color("text", v))
	linea := canvas.NewRectangle(tema.Color("border", v))
	linea.Hidden = !filete
	return container.New(&filaMiembro{nombre: nombre, sel: sel, quitar: quitar, linea: linea}, linea, nombre, sel, quitar)
}

type filaMiembro struct {
	nombre *canvas.Text
	sel    *ui.Selector
	quitar *botonTexto
	linea  *canvas.Rectangle
}

func (f *filaMiembro) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, 2*tema.Espacio(2)+max(f.sel.MinSize().Height, f.quitar.MinSize().Height))
}

func (f *filaMiembro) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	f.linea.Move(fyne.NewPos(0, 0))
	f.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	derecha := t.Width - tema.Espacio(3)
	mq := f.quitar.MinSize()
	f.quitar.Move(fyne.NewPos(derecha-mq.Width, (t.Height-mq.Height)/2))
	f.quitar.Resize(mq)
	derecha -= mq.Width + tema.Espacio(3)
	ms := f.sel.MinSize()
	f.sel.Move(fyne.NewPos(derecha-ms.Width, (t.Height-ms.Height)/2))
	f.sel.Resize(ms)
	derecha -= ms.Width + tema.Espacio(3)
	x := tema.Espacio(3)
	ui.Truncar(f.nombre, f.nombre.Text, tema.TextoSM, ui.Medio, max(derecha-x, 0))
	ui.ColocarTexto(f.nombre, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
}
