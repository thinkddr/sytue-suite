// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Compartir una base, los enlaces públicos de un formulario y exportar una tabla
// (DialogoCompartir.tsx, PanelFormulario.tsx y los enlaces de exportación de BaseApp.tsx).

// Las clases de con quién se comparte y los roles que se pueden conceder, en el orden
// de los selectores de la web.
var (
	clasesDePermiso = []string{"tenant", "group", "user", "external"}
	rolesConcedidos = []string{api.BasesViewer, api.BasesCommenter, api.BasesEditor}
)

// nombreDePermiso es con quién se comparte, como se lee en la lista.
func (p *Pantalla) nombreDePermiso(x api.BasesPermission) string {
	nombre := x.PrincipalLabel
	if nombre == "" {
		nombre = x.PrincipalID
	}
	switch x.PrincipalKind {
	case "tenant":
		return p.t("share.kind.tenant")
	case "group":
		return p.t("share.groupLabel", i18n.Vars{"name": x.PrincipalID})
	case "external":
		return p.t("share.externalLabel", i18n.Vars{"name": nombre})
	}
	return nombre
}

// mitades es `grid grid-cols-2 gap-*`: dos hijos, cada uno con la mitad del ancho.
type mitades struct{ Hueco float32 }

func (m mitades) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(2*max(a.Width, b.Width)+m.Hueco, max(a.Height, b.Height))
}

func (m mitades) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	ancho := (t.Width - m.Hueco) / 2
	for i, o := range objs[:2] {
		o.Move(fyne.NewPos(float32(i)*(ancho+m.Hueco), 0))
		o.Resize(fyne.NewSize(ancho, o.MinSize().Height))
	}
}

// formCompartir es el diálogo «Compartir esta base»: quién tiene acceso, conceder,
// retirar y transferir la propiedad.
type formCompartir struct {
	p             *Pantalla
	b             *Base
	d             *ui.Dialogo
	permisos      []api.BasesPermission
	grupos        []string
	clase, rol    *ui.Selector
	quien         *ui.Entrada
	destino       *ui.Entrada
	conceder      *ui.Boton
	transferir    *ui.Boton
	ocupado       bool
	fallo         string
	rotulosClases []string
}

// dialogoCompartir abre «Compartir esta base» (solo el propietario).
func (p *Pantalla) dialogoCompartir() {
	f := &formCompartir{p: p, b: p.abierta}
	rotulosRol := make([]string, len(rolesConcedidos))
	for i, r := range rolesConcedidos {
		rotulosRol[i] = p.t("role." + r)
	}
	for _, c := range clasesDePermiso {
		f.rotulosClases = append(f.rotulosClases, p.t("share.kind."+c))
	}
	f.clase = ui.NuevoSelector(f.rotulosClases, func(int) { f.quien.PonerTexto(""); f.rehacer() })
	f.clase.Rotulo = p.t("share.who")
	f.rol = ui.NuevoSelector(rotulosRol, nil)
	f.rol.Rotulo = p.t("share.role")
	f.quien = ui.NuevaEntrada("")
	f.destino = ui.NuevaEntrada("")
	f.destino.Rotulo = p.t("transfer.email")
	f.conceder = ui.NuevoBoton(p.t("share.grant"), f.concederPermiso)
	f.transferir = ui.NuevoBoton(p.t("transfer.run"), f.pedirTransferir).Con(ui.Secundario, ui.TallaMD)
	f.quien.AlCambiar(func(string) { f.ajustar() })
	f.destino.AlCambiar(func(string) { f.ajustar() })
	f.d = ui.NuevoDialogo(p.t("share.title"), container.New(ui.Columna{})).Con(ui.DialogoMD, nil)
	f.d.RotuloCerrar = p.t("ui.close")
	f.rehacer()
	f.d.Mostrar(p.ventana.Canvas())
	f.cargar()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		// Los grupos de quien comparte, para ofrecerlos; sin ellos se escribe el nombre.
		if grupos, err := p.svc.MyGroups(ctx); err == nil {
			enHilo(func() { f.grupos = grupos; f.rehacer() })
		}
	})
}

func (f *formCompartir) claseElegida() string { return clasesDePermiso[f.clase.Elegida] }

func (f *formCompartir) ajustar() {
	f.conceder.Inerte = f.ocupado || (f.claseElegida() != "tenant" && strings.TrimSpace(f.quien.Texto()) == "")
	f.transferir.Inerte = f.ocupado || strings.TrimSpace(f.destino.Texto()) == ""
	f.conceder.Refresh()
	f.transferir.Refresh()
}

// hacer lanza una escritura del diálogo; al terminar deja el fallo (o lo quita) y sigue.
func (f *formCompartir) hacer(escribir func() error, despues func()) {
	p := f.p
	f.ocupado, f.fallo = true, ""
	f.ajustar()
	p.fondo(func() {
		err := escribir()
		enHilo(func() {
			f.ocupado = false
			if err != nil {
				f.fallo = p.mensajeDeError(err)
				f.rehacer()
				return
			}
			despues()
		})
	})
}

func (f *formCompartir) cargar() {
	p := f.p
	var permisos []api.BasesPermission
	f.hacer(func() (err error) {
		ctx, cancelar := p.ctx()
		defer cancelar()
		permisos, err = p.svc.ListPermissions(ctx, f.b.ID)
		return err
	}, func() { f.permisos = permisos; f.rehacer() })
}

// concederPermiso comparte con quien se ha elegido: toda la organización, un grupo por su
// nombre, una persona por su correo, o alguien de otra organización por su @usuario o su correo.
func (f *formCompartir) concederPermiso() {
	p := f.p
	valor := strings.TrimSpace(f.quien.Texto())
	g := api.BasesGrant{PrincipalKind: f.claseElegida(), Role: rolesConcedidos[f.rol.Elegida]}
	switch g.PrincipalKind {
	case "tenant":
		if p.Org != nil {
			g.PrincipalID = p.Org()
		}
	case "group":
		g.PrincipalID = valor
	case "external":
		g.Query = valor
	default:
		g.Email = valor
	}
	if g.PrincipalKind != "tenant" && valor == "" {
		return
	}
	f.hacer(func() error {
		ctx, cancelar := p.ctx()
		defer cancelar()
		_, err := p.svc.GrantPermission(ctx, f.b.ID, g)
		return err
	}, func() { f.quien.PonerTexto(""); f.cargar() })
}

func (f *formCompartir) revocar(id string) {
	p := f.p
	f.hacer(func() error {
		ctx, cancelar := p.ctx()
		defer cancelar()
		return p.svc.RevokePermission(ctx, f.b.ID, id)
	}, func() {
		for i, x := range f.permisos {
			if x.ID == id {
				f.permisos = append(f.permisos[:i:i], f.permisos[i+1:]...)
				break
			}
		}
		f.rehacer()
	})
}

// pedirTransferir pregunta antes: quien transfiere deja de ser propietario y no lo puede
// deshacer por su cuenta.
func (f *formCompartir) pedirTransferir() {
	p := f.p
	email := strings.TrimSpace(f.destino.Texto())
	if email == "" {
		return
	}
	p.confirmar(p.t("transfer.confirm", i18n.Vars{"name": f.b.Datos.Name, "email": email}), p.t("transfer.run"), func() {
		f.hacer(func() error {
			ctx, cancelar := p.ctx()
			defer cancelar()
			return p.svc.TransferBase(ctx, f.b.ID, email)
		}, func() {
			// Ya no es propietario: el diálogo se cierra y la base se vuelve a leer con su rol nuevo.
			f.d.Cerrar()
			p.Abrir(f.b.ID)
		})
	})
}

// campoQuien es el campo de con quién se comparte, según la clase: ninguno para toda la
// organización.
func (f *formCompartir) campoQuien() fyne.CanvasObject {
	p := f.p
	// La ayuda de estos campos es larga y parte en líneas: va en un párrafo propio, que
	// la de GrupoCampo es de una sola línea y ensancharía el diálogo.
	conAyuda := func(clave, ayuda string) []fyne.CanvasObject {
		f.quien.Rotulo = p.t(clave)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		return []fyne.CanvasObject{
			ui.GrupoCampo(p.t(clave), true, f.quien, "", ""),
			ui.NuevoParrafo(p.t(ayuda), tema.TextoXS, ui.Normal, "text-muted", v).A(p.anchoDialogo(560)),
		}
	}
	switch f.claseElegida() {
	case "user":
		f.quien.Rotulo = p.t("share.email")
		return ui.GrupoCampo(p.t("share.email"), true, f.quien, "", "")
	case "external":
		return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, conAyuda("share.external", "share.externalHelp")...)
	case "group":
		grupo := conAyuda("share.group", "share.groupHelp")
		// Los grupos propios, a un clic: el `<datalist>` de la web.
		if len(f.grupos) > 0 {
			sugeridos := make([]fyne.CanvasObject, len(f.grupos))
			for i, g := range f.grupos {
				sugeridos[i] = botonSM(g, ui.Fantasma, func() { f.quien.PonerTexto(g) })
			}
			grupo = append(grupo, container.New(flujo{Hueco: tema.Espacio(1), Ancho: p.anchoDialogo(560)}, sugeridos...))
		}
		return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, grupo...)
	}
	return nil
}

func (f *formCompartir) rehacer() {
	p := f.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	parrafo := func(clave string) fyne.CanvasObject {
		return ui.NuevoParrafo(p.t(clave), tema.TextoSM, ui.Normal, "text-muted", v).A(p.anchoDialogo(560))
	}
	conceder := []fyne.CanvasObject{container.New(mitades{Hueco: tema.Espacio(3)},
		ui.GrupoCampo(p.t("share.who"), false, f.clase, "", ""),
		ui.GrupoCampo(p.t("share.role"), false, f.rol, "", ""))}
	if quien := f.campoQuien(); quien != nil {
		conceder = append(conceder, quien)
	}
	conceder = append(conceder, container.New(ui.Fila{}, f.conceder))
	hijos := []fyne.CanvasObject{parrafo("share.intro"), container.New(ui.Columna{Hueco: tema.Espacio(3)}, conceder...)}
	if f.fallo != "" {
		hijos = append(hijos, alerta(f.fallo, v))
	}
	if len(f.permisos) == 0 {
		hijos = append(hijos, parrafo("share.none"))
	} else {
		filas := make([]fyne.CanvasObject, len(f.permisos))
		for i, x := range f.permisos {
			texto := p.nombreDePermiso(x) + " · " + p.t("role."+x.Role)
			if x.TableID != "" {
				texto += " · " + p.t("share.oneTable")
			}
			filas[i] = container.New(extremos{Hueco: tema.Espacio(2), Centro: true},
				ui.NuevoParrafo(texto, tema.TextoSM, ui.Normal, "text", v).A(max(p.anchoDialogo(560)-12*tema.Rem, 0)).Recortado(1),
				botonSM(p.t("share.revoke"), ui.Fantasma, func() { f.revocar(x.ID) }))
		}
		hijos = append(hijos, container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...))
	}
	hijos = append(hijos, ui.NuevoSeparadorMenu(), container.New(ui.Columna{Hueco: tema.Espacio(3)},
		ui.NuevoRotulo(p.t("transfer.title"), tema.TextoSM, ui.SemiNegrita, "text"),
		parrafo("transfer.help"),
		ui.GrupoCampo(p.t("transfer.email"), false, f.destino, "", ""),
		container.New(ui.Fila{}, f.transferir)))
	f.d.CambiarCuerpo(container.New(ui.Columna{Hueco: tema.Espacio(5)}, hijos...))
	f.ajustar()
}

// ---- Los enlaces públicos de un formulario ----

// estadoDeEnlace dice cómo está un enlace público: revocado, caducado, completo o activo.
func estadoDeEnlace(e api.BasesFormLink, ahora time.Time) string {
	switch {
	case e.RevokedAt != nil:
		return "revoked"
	case e.ExpiresAt != nil && !e.ExpiresAt.After(ahora):
		return "expired"
	case e.Responses >= e.MaxResponses:
		return "full"
	}
	return "active"
}

// cargarEnlaces pide los enlaces públicos de la vista de formulario que se mira.
func (p *Pantalla) cargarEnlaces() {
	b := p.abierta
	vista := b.VistaID
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		enlaces, err := p.svc.ListFormLinks(ctx, vista)
		enHilo(func() {
			if p.abierta != b || b.VistaID != vista {
				return
			}
			if err != nil {
				b.falloEnlaces = p.mensajeDeError(err)
			} else {
				b.enlaces = enlaces
			}
			p.Repintar()
		})
	})
}

// crearEnlace crea un enlace público con el cupo escrito. Su dirección solo se enseña ahora.
func (p *Pantalla) crearEnlace() {
	b := p.abierta
	vista := b.VistaID
	n, err := strconv.Atoi(strings.TrimSpace(b.cupo.Texto()))
	if err != nil || n < 1 || n > 10000 {
		b.falloEnlaces = p.t("formlink.cupoInvalid")
		p.Repintar()
		return
	}
	b.falloEnlaces = ""
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		enlace, err := p.svc.CreateFormLink(ctx, vista, n)
		enHilo(func() {
			if p.abierta != b || b.VistaID != vista {
				return
			}
			if err != nil {
				b.falloEnlaces = p.mensajeDeError(err)
				p.Repintar()
				return
			}
			b.enlaceNuevo, b.enlaceCopiado = "/f/"+enlace.Token, false
			if p.URLPublica != nil {
				b.enlaceNuevo = p.URLPublica(b.enlaceNuevo)
			}
			p.cargarEnlaces()
		})
	})
}

func (p *Pantalla) revocarEnlace(id string) {
	b := p.abierta
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.svc.RevokeFormLink(ctx, id)
		enHilo(func() {
			if p.abierta != b {
				return
			}
			if err != nil {
				b.falloEnlaces = p.mensajeDeError(err)
				p.Repintar()
				return
			}
			b.enlaceNuevo = ""
			p.cargarEnlaces()
		})
	})
}

// panelFormulario es PanelFormulario.tsx: crear un enlace público con su cupo, copiarlo
// la única vez que se enseña, y la lista de los que hay con su estado, su uso y «Revocar».
func (p *Pantalla) panelFormulario(v tema.Variante) fyne.CanvasObject {
	b := p.abierta
	ancho := p.anchoContenido() - 2*(margenTablero+tema.Borde)
	hijos := []fyne.CanvasObject{
		ui.NuevoRotulo(p.t("formlink.title"), tema.TextoSM, ui.SemiNegrita, "text"),
		ui.NuevoParrafo(p.t("formlink.help"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho),
		container.New(ui.Fila{Hueco: tema.Espacio(2)},
			ui.GrupoCampo(p.t("formlink.cupo"), false, b.cupo, "", ""),
			container.New(ui.Margen{Arr: tema.TextoSM.Linea + ui.HuecoGrupoCampo},
				ui.NuevoBoton(p.t("formlink.create"), p.crearEnlace).Con(ui.Secundario, ui.TallaMD))),
	}
	if b.enlaceNuevo != "" {
		direccion := ui.NuevaEntrada("")
		direccion.Rotulo = p.t("formlink.url")
		direccion.PonerTexto(b.enlaceNuevo)
		direccion.SoloLectura(true)
		copiar := p.t("formlink.copy")
		if b.enlaceCopiado {
			copiar = p.t("formlink.copied")
		}
		hijos = append(hijos,
			ui.NuevoRotulo(p.t("formlink.once"), tema.TextoSM, ui.Normal, "text"),
			container.New(extremos{Hueco: tema.Espacio(2), Centro: true}, direccion,
				botonSM(copiar, ui.Fantasma, func() {
					b.enlaceCopiado = ui.Copiar(fyne.CurrentApp().Clipboard(), b.enlaceNuevo)
					p.Repintar()
				})))
	}
	if b.falloEnlaces != "" {
		hijos = append(hijos, alerta(b.falloEnlaces, v))
	}
	if len(b.enlaces) == 0 {
		hijos = append(hijos, ui.NuevoRotulo(p.t("formlink.none"), tema.TextoSM, ui.Normal, "text-muted"))
	}
	for _, e := range b.enlaces {
		estado := estadoDeEnlace(e, time.Now())
		tono := ui.InsigniaNeutra
		if estado == "active" {
			tono = ui.InsigniaExito
		}
		datos := container.New(ui.Fila{Hueco: tema.Espacio(2)},
			ui.NuevaInsignia(p.t("formlink.state."+estado), tono),
			ui.NuevoRotulo(p.t("formlink.usage", i18n.Vars{"n": e.Responses, "max": e.MaxResponses}), tema.TextoSM, ui.Normal, "text"),
			ui.NuevoRotulo(i18n.FormatDate(p.tr.Locale(), e.CreatedAt), tema.TextoSM, ui.Normal, "text-muted"))
		fila := []fyne.CanvasObject{datos}
		if estado == "active" {
			fila = append(fila, botonSM(p.t("formlink.revoke"), ui.Fantasma, func() { p.revocarEnlace(e.ID) }))
		}
		hijos = append(hijos, container.New(extremos{Hueco: tema.Espacio(2), Centro: true}, fila...))
	}
	return caja("bg", tema.RadioLG, v, container.New(ui.Columna{Hueco: tema.Espacio(3)}, hijos...))
}

// ---- Exportar ----

// exportar baja la tabla activa entera en `csv` o `json` y la deja donde el sistema
// guarda las descargas. Los campos de enlace salen con sus valores, no con ids.
func (p *Pantalla) exportar(formato string) {
	b := p.abierta
	t := b.tabla()
	if t == nil || p.Guardar == nil {
		return
	}
	id, nombre := t.ID, strings.TrimSpace(t.Name)+"."+formato
	var ruta string
	p.hacer(func() error {
		ctx, cancelar := p.ctx()
		defer cancelar()
		cuerpo, err := p.svc.Export(ctx, id, formato)
		if err != nil {
			return err
		}
		defer func() { _ = cuerpo.Close() }()
		ruta, err = p.Guardar(nombre, cuerpo)
		return err
	}, func() {
		// El nombre del fichero y dónde ha quedado: en la web lo dice el navegador.
		b.Estado = nombre + " — " + ruta
		p.Repintar()
	})
}
