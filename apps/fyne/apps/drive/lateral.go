// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Seccion es una de las ubicaciones del menú de Sytue Drive (el `Section` de
// DriveSidebar.tsx).
type Seccion uint8

// Las siete, en el orden en que la web las pinta.
const (
	MiUnidad Seccion = iota
	Documentos
	UnidadesDeEquipo
	CompartidoConmigo
	Recientes
	Favoritos
	Papelera
	// Nube es una nube personal: no es una fila de `ubicaciones`, se elige desde la
	// sección «Tus nubes» del menú (`CloudsSidebar` de PersonalClouds.tsx).
	Nube
)

// ubicacion es una fila del menú: su clave de catálogo y su glifo.
type ubicacion struct {
	seccion Seccion
	clave   string
	icono   iconos.Icono
}

// Las ubicaciones son el `ITEMS` de DriveSidebar.tsx, con el mismo orden y los mismos
// glifos. «Documentos» va justo detrás de «Mi unidad» a propósito: un documento de Docs
// no vive en ninguna carpeta y sin esa entrada desaparecería de la vista.
var ubicaciones = [...]ubicacion{
	{MiUnidad, "nav.myDrive", iconos.DriveCarpeta},
	{Documentos, "nav.documents", iconos.DriveFichero},
	{UnidadesDeEquipo, "nav.teamDrives", iconos.Edificio},
	{CompartidoConmigo, "nav.sharedWithMe", iconos.Usuarios},
	{Recientes, "nav.recent", iconos.Reloj},
	{Favoritos, "nav.favorites", iconos.DriveEstrella},
	{Papelera, "nav.trash", iconos.Papelera},
}

// Lateral es el menú de Sytue Drive.
//
// Medido a 1280×800: carril de 260, filas de 33,25 × 231 separadas por 1,75, la primera
// en y=85,75 contando desde el alto de la barra superior (56), que en nativo no existe.
// El icono es el `size={18}` de la web dentro de la caja de 17,5 de `h-5 w-5`.
type Lateral struct {
	tr  *i18n.Translator
	loc i18n.Locale

	// Activa es la ubicación en la que estamos.
	Activa Seccion
	// Cuota es el uso de almacenamiento; nil mientras no haya cargado o si falló, que
	// es lo que la web hace: el pie se enseña sin barra y no estorba.
	Cuota *Cuota
	// AlElegir lo llama una fila al pulsarse.
	AlElegir func(Seccion)
	// AlAbrirDispositivos abre el asistente «Conectar tus dispositivos» del portal.
	// Es OTRA aplicación: se delega en el sistema por su esquema de URL, no es una
	// pantalla más de este binario (CONVENTIONS.md §13.6).
	AlAbrirDispositivos func()
	// Externo es el almacén de terceros de la organización; nil si no se puede
	// administrar (el servicio contesta 403) o no ha cargado. Conectado, sale en «Tus
	// nubes» y AlAbrirExterno abre su gestión.
	Externo        *api.AlmacenExterno
	AlAbrirExterno func()
	// AlAnadirAlmacen abre el selector «Añadir almacenamiento»: la ÚNICA entrada para
	// conectar una nube personal o el almacén de la organización.
	AlAnadirAlmacen func()
	// Importar es «Traer mis datos», que solo se pinta si la organización tiene algún
	// origen conectado. Nil, no hay.
	Importar *ui.ImportarDatos
	// Nubes son las nubes personales y los proveedores que se pueden conectar; nil si
	// no han cargado o falló, y entonces la sección no se pinta, como en la web.
	// NubeActiva es la instancia que se mira (con Activa == Nube).
	Nubes        *api.NubesPersonales
	NubeActiva   string
	AlElegirNube func(api.NubePersonal)
}

// Cuota es el espacio de la organización, ya en bytes.
type Cuota struct {
	Usado, Tope int64
	// Externo: lo nuevo va al bucket propio de la organización y no gasta este espacio.
	Externo bool
}

// NuevoLateral crea el menú.
func NuevoLateral(tr *i18n.Translator, activa Seccion, alElegir func(Seccion)) *Lateral {
	return &Lateral{tr: tr, loc: tr.Locale(), Activa: activa, AlElegir: alElegir}
}

func (l *Lateral) t(clave string, vars ...i18n.Vars) string {
	var v i18n.Vars
	if len(vars) > 0 {
		v = vars[0]
	}
	return l.tr.T(clave, v)
}

// Objeto arma el carril con su sección de ubicaciones y su pie.
func (l *Lateral) Objeto(v tema.Variante) fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 0, len(ubicaciones))
	for _, u := range ubicaciones {
		f := ui.NuevaFilaLateral(l.t(u.clave), l.icono(u.icono, u.seccion == l.Activa, v), l.elegir(u.seccion))
		f.Activa = u.seccion == l.Activa
		filas = append(filas, f)
	}
	var cuerpo fyne.CanvasObject = ui.NuevaSeccionLateral(l.t("nav.locations"), filas...)
	if nubes := l.seccionNubes(v); nubes != nil {
		cuerpo = container.New(ui.Columna{}, cuerpo, nubes)
	}
	lat := ui.NuevoLateral(l.t("nav.locations"), cuerpo)
	lat.Pie = l.pie(v)
	return lat
}

// seccionNubes es «Tus nubes»: las nubes personales conectadas, el almacén de la
// organización si lo hay y una sola fila «Añadir almacenamiento». Sin nada que enseñar,
// no se pinta.
func (l *Lateral) seccionNubes(v tema.Variante) fyne.CanvasObject {
	var filas []fyne.CanvasObject
	var conectadas []api.NubePersonal
	puedeAnadir := l.Externo != nil
	if l.Nubes != nil {
		conectadas = l.Nubes.Conectadas()
		for _, p := range l.Nubes.Providers {
			puedeAnadir = puedeAnadir || p.Available
		}
	}
	for _, c := range conectadas {
		nube := c
		activa := l.Activa == Nube && l.NubeActiva == c.InstanceID
		f := ui.NuevaFilaLateral(api.NombreNube[c.Provider], l.icono(iconos.DriveCarpeta, activa, v), func() {
			if l.AlElegirNube != nil {
				l.AlElegirNube(nube)
			}
		})
		f.Activa = activa
		filas = append(filas, f)
	}
	if l.Externo != nil && l.Externo.Connected {
		filas = append(filas, ui.NuevaFilaLateral(RotuloExterno(*l.Externo), l.icono(iconos.Edificio, false, v), l.AlAbrirExterno))
	}
	if puedeAnadir {
		filas = append(filas, ui.NuevaFilaLateral(l.t("storage.add"), l.icono(iconos.Enlace, false, v), l.AlAnadirAlmacen))
	}
	if len(filas) == 0 {
		return nil
	}
	return ui.NuevaSeccionLateral(l.t("nav.clouds"), filas...)
}

// icono pinta el glifo de una fila con la tinta que le toca: el activo va en
// `sidebar-fg` y el resto en `sidebar-fg-muted`, igual que su rótulo.
func (l *Lateral) icono(i iconos.Icono, activa bool, v tema.Variante) fyne.CanvasObject {
	// `tinta` y no `token`: con ese nombre, gosec lo toma por una credencial.
	tinta := "sidebar-fg-muted"
	if activa {
		tinta = "sidebar-fg"
	}
	return iconos.Trazo(i, ui.GlifoMenu, tinta, v).Objeto()
}

func (l *Lateral) elegir(s Seccion) func() {
	return func() {
		if l.AlElegir != nil {
			l.AlElegir(s)
		}
	}
}

// pie es la columna de abajo: el camino al asistente de dispositivos, el medidor de
// almacenamiento y la nota de soberanía. Van con `space-y-1` (3,5).
func (l *Lateral) pie(v tema.Variante) fyne.CanvasObject {
	dispositivos := ui.NuevaFilaLateral(l.t("nav.connectDevices"),
		iconos.Trazo(iconos.DriveCarpeta, ui.GlifoMenu, "sidebar-fg-muted", v).Objeto(),
		l.AlAbrirDispositivos)
	dispositivos.Compacta = true

	// «Trae tus archivos» va el primero: quien abre Drive vacío no va a pedirle a su
	// administración que le migre los archivos.
	var hijos []fyne.CanvasObject
	if l.Importar != nil {
		if o := l.Importar.Objeto(ui.AnchoUtilMenu, v); o != nil {
			hijos = append(hijos, o)
		}
	}
	hijos = append(hijos, dispositivos)
	if m := l.medidor(); m != nil {
		hijos = append(hijos, m)
		if l.Cuota.Externo {
			// Con bucket propio, lo nuevo no gasta este espacio: se dice junto al
			// medidor, que es donde se mira cuando se acaba el sitio. `mt-1 text-xs
			// leading-relaxed`, dentro del `px-1.5` del medidor.
			nota := ui.NuevoParrafo(l.t("storage.externalNote"), tema.Escalon{
				Tam: tema.TextoXS.Tam, Linea: 1.625 * tema.TextoXS.Tam,
			}, ui.Normal, "sidebar-fg-muted", v).A(ui.AnchoUtilMenu - 2*1.5*0.25*tema.Rem)
			hijos = append(hijos, nota)
		}
	}
	nota := ui.Texto(l.t("storage.euNote"), tema.TextoXS, ui.Normal, tema.Color("sidebar-fg-muted", v))
	hijos = append(hijos, nota)
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, hijos...)
}

// medidor es el `StorageMeter` de la web: el medidor COMPARTIDO y no una copia. El de
// Drive pintaba siempre del mismo gris y al 99 % avisaba igual que al 3 %; `ui.Medidor`
// cambia de rampa al 80 % y al 95 % y además lo dice con palabras.
//
// Sin cuota cargada no se pinta: la web tampoco enseña una barra a cero, que sería
// afirmar que no se ha gastado nada.
func (l *Lateral) medidor() fyne.CanvasObject {
	if l.Cuota == nil || l.Cuota.Tope <= 0 {
		return nil
	}
	f := float64(l.Cuota.Usado) / float64(l.Cuota.Tope)
	// `FormatPercent` y no un «%» pegado al número: el sitio del signo y lo que va
	// delante son del idioma —pegado en inglés, con espacio duro en los otros
	// veinticinco—, y esto escribía «0» donde la web escribe «0 %».
	m := ui.NuevoMedidor(l.t("storage.title"), f, i18n.FormatPercent(l.loc, f))
	m.Cuanto = l.t("storage.usage", i18n.Vars{
		"used":  i18n.FormatBytes(l.loc, float64(l.Cuota.Usado)),
		"total": i18n.FormatBytes(l.loc, float64(l.Cuota.Tope)),
	})
	m.SobreCromo = true
	return m
}
