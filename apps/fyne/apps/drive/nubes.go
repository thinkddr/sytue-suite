// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"io"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Nubes personales
//
// `PersonalClouds.tsx`: el Google Drive, OneDrive o Dropbox de la propia persona como
// una ubicación más. Se conecta con solo el consentimiento del proveedor, en el
// navegador del sistema y con vuelta a `sytue-drive://connect/callback`; la credencial
// se queda en la pasarela de integraciones. Aquí se navega por carpetas, se descarga
// al disco y se sube con el selector de siempre (hasta 100 MB por fichero).
//
// ponytail: un fichero de la nube se descarga y no se previsualiza, igual que en la web.

// VistaNube es la nube abierta.
type VistaNube struct {
	tr  *i18n.Translator
	loc i18n.Locale

	Nube api.NubePersonal
	// Pila son las carpetas por las que se ha entrado, sin la raíz.
	Pila        []api.ElementoNube
	Fase        Fase
	Elementos   []api.ElementoNube
	Cursor      string
	CargandoMas bool
	Subiendo    bool
	Ocupado     bool

	Desconectado bool
	Ventana      float32

	AlIr          func(nivel int) // -1 es la raíz de la nube
	AlAbrir       func(api.ElementoNube)
	AlSubir       func()
	AlDesconectar func()
	AlReintentar  func()
	AlCargarMas   func()

	desplazable *container.Scroll
}

// NuevaVistaNube crea la vista vacía; se le pone la nube al elegirla.
func NuevaVistaNube(tr *i18n.Translator) *VistaNube {
	return &VistaNube{tr: tr, loc: tr.Locale(), Fase: Cargando}
}

func (q *VistaNube) t(k string) string { return q.tr.T(k, nil) }

// carpeta es el id de la carpeta que se mira; vacío, la raíz.
func (q *VistaNube) carpeta() string {
	if n := len(q.Pila); n > 0 {
		return q.Pila[n-1].ID
	}
	return ""
}

// SinRed apaga subir y desconectar.
func (q *VistaNube) SinRed() bool { return q.Fase == SinConexion || q.Desconectado }

// Objeto es la cabecera con la ruta y las acciones, el aviso de Google y el contenido.
func (q *VistaNube) Objeto(v tema.Variante) fyne.CanvasObject {
	partes := []fyne.CanvasObject{q.cabecera(v)}
	if q.Nube.Provider == "google-drive" {
		// `drive.file`: solo se ve lo que se subió o creó desde Sytue. Se dice siempre.
		canalon := CanalonCabecera(q.Ventana)
		partes = append(partes, container.New(ui.Margen{Izq: canalon, Der: canalon, Arr: tema.Espacio(2), Abj: tema.Espacio(2)},
			ui.NuevaAyudaEnLinea(q.t("cloud.googleNote"))))
	}
	partes = append(partes, q.cuerpo(v))
	return container.New(Pila{Elastico: len(partes) - 1}, partes...)
}

// cabecera es el `header` de CloudView: la ruta (la nube y sus carpetas) y, a la
// derecha, «Subir» y «Desconectar».
func (q *VistaNube) cabecera(v tema.Variante) fyne.CanvasObject {
	nombres := make([]string, 1, 1+len(q.Pila))
	nombres[0] = api.NombreNube[q.Nube.Provider]
	for _, c := range q.Pila {
		nombres = append(nombres, c.Name)
	}
	migas := ui.NuevasMigas(nombres[0], nombres, func(i int) {
		if q.AlIr != nil {
			q.AlIr(i - 1)
		}
	})
	inerte := q.Ocupado || q.SinRed()
	subir := ui.NuevoBoton(q.t("files.upload"), q.AlSubir).Con(ui.Primario, ui.TallaSM).ConIcono(iconos.Mas, 16)
	subir.Inerte, subir.Cargando = inerte && !q.Subiendo, q.Subiendo
	desconectar := ui.NuevoBoton(q.t("cloud.disconnect"), q.AlDesconectar).Con(ui.Fantasma, ui.TallaSM)
	desconectar.Inerte = inerte || q.Subiendo
	acciones := container.New(ui.Fila{Hueco: HuecoAcciones}, subir, desconectar)
	canalon := CanalonCabecera(q.Ventana)
	linea := container.New(ui.Margen{Izq: canalon, Der: canalon, Arr: RellenoYCabecera, Abj: RellenoYCabecera},
		container.NewBorder(nil, nil, nil, acciones, migas))
	borde := canvas.NewRectangle(tema.Color("border", v))
	borde.SetMinSize(fyne.NewSize(0, tema.Borde))
	return container.NewStack(canvas.NewRectangle(tema.Color("surface", v)), container.NewBorder(nil, borde, nil, nil, linea))
}

// cuerpo es lo de la carpeta, o su estado.
func (q *VistaNube) cuerpo(v tema.Variante) fyne.CanvasObject {
	switch q.Fase {
	case Cargando:
		return container.NewStack(canvas.NewRectangle(tema.Color("bg", v)), container.NewVScroll(esqueletoFilas()))
	case Vacia:
		x := ui.NuevoVacio(q.t("team.folder.emptyTitle"), "")
		x.Ilustracion = ilustracion(iconos.VacioDrive, v)
		return centrar(x, v)
	case Fallo, Prohibida:
		// La web no distingue: no se pudo llegar a la nube, o ya no deja.
		x := ui.NuevoVacio(q.t("cloud.failed"), "")
		x.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger-text", v).Objeto()
		x.Accion = ui.NuevoBoton(q.t("files.retry"), q.AlReintentar).Con(ui.Secundario, ui.TallaSM)
		return centrar(x, v)
	case SinSesion, SinConexion:
		return centrar(problemaVista(q.tr, q.Fase, "cloud.failed", q.AlReintentar, v), v)
	case Exito:
	}
	filas := make([]fyne.CanvasObject, 0, len(q.Elementos))
	for _, el := range q.Elementos {
		filas = append(filas, q.fila(el, v))
	}
	var contenido fyne.CanvasObject = container.New(&listaEquipo{}, filas...)
	if q.Cursor != "" {
		contenido = container.New(Pila{Elastico: -1}, contenido, pieCargarMas(q.tr, q.CargandoMas, q.AlCargarMas, v))
	}
	if q.desplazable == nil {
		q.desplazable = container.NewVScroll(contenido)
	} else {
		q.desplazable.Content = contenido
		q.desplazable.Refresh()
	}
	return container.NewStack(canvas.NewRectangle(tema.Color("bg", v)), q.desplazable)
}

// fila es un elemento: la carpeta en `primary-text` entra, el fichero en `text-muted`
// se descarga; el tamaño solo en ficheros, como la web.
func (q *VistaNube) fila(el api.ElementoNube, v tema.Variante) *FilaPlana {
	elemento := el
	f := NuevaFilaPlana(el.Name)
	icono, tinta := iconos.DriveFichero, "text-muted"
	if el.Folder {
		icono, tinta = iconos.DriveCarpeta, "primary-text"
	}
	f.Glifo = iconos.Trazo(icono, LadoGlifoEquipo, tinta, v)
	f.LadoGlifo, f.CajaGlifo, f.TintaGlifo = LadoGlifoEquipo, CajaGlifoEquipo, tinta
	if !el.Folder && el.Size > 0 {
		f.Meta = Tamano(q.loc, el.Size)
	}
	f.Grueso = punteroGrueso()
	f.AlAbrir = func() {
		if q.AlAbrir != nil {
			q.AlAbrir(elemento)
		}
	}
	return f
}

// ---- Lo que habla con la pasarela ----

func (p *Pantalla) montarNube() {
	q := NuevaVistaNube(p.tr)
	p.sec.nube = q
	p.lateral.AlElegirNube = func(c api.NubePersonal) {
		q.Nube, q.Pila, q.Elementos, q.Cursor = c, nil, nil, ""
		p.lateral.NubeActiva = c.InstanceID
		p.irA(Nube)
	}
	q.AlIr = func(nivel int) {
		if nivel < 0 {
			q.Pila = nil
		} else if nivel < len(q.Pila) {
			q.Pila = q.Pila[:nivel+1]
		}
		go p.cargarNube()
	}
	q.AlAbrir = func(el api.ElementoNube) {
		if el.Folder {
			q.Pila = append(q.Pila, el)
			go p.cargarNube()
			return
		}
		id := q.Nube.InstanceID
		go p.guardarDescarga(el.Name, func(ctx context.Context) (io.ReadCloser, error) {
			return p.integraciones.NubeDescargar(ctx, id, el.ID)
		})
	}
	q.AlReintentar = func() { go p.cargarNube() }
	q.AlCargarMas = p.masDeNube
	q.AlSubir = p.subirANube
	q.AlDesconectar = func() { p.enSegundoPlano(p.desconectarNube) }
}

// cargarNubes pide las nubes de la persona para el menú. Un fallo deja la sección sin
// pintar, que es lo que hace la web.
func (p *Pantalla) cargarNubes() {
	if p.integraciones == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	n, err := p.integraciones.NubesPersonales(ctx)
	enHilo(func() {
		p.lateral.Nubes = nil
		if err == nil {
			p.lateral.Nubes = &n
		}
		p.Repintar()
	})
}

// conectarNube abre el consentimiento en el navegador (api.AbrirFuera, dentro de la
// sesión) y, a la vuelta, abre la nube recién conectada. Se llega desde «Añadir
// almacenamiento» (externo.go).
func (p *Pantalla) conectarNube(proveedor string) {
	if p.AutorizarNube == nil || p.integraciones == nil {
		return
	}
	id, err := p.AutorizarNube(context.Background(), proveedor)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			p.avisar(ui.AvisoPeligro, p.tr.T("cloud.failed", nil), "")
		}
		return
	}
	p.cargarNubes()
	enHilo(func() {
		if p.lateral.Nubes == nil {
			return
		}
		for _, c := range p.lateral.Nubes.Conectadas() {
			if c.InstanceID == id && p.lateral.AlElegirNube != nil {
				p.lateral.AlElegirNube(c)
			}
		}
	})
}

// cargarNube pide la carpeta de la nube abierta.
func (p *Pantalla) cargarNube() {
	q := p.sec.nube
	var id, carpeta string
	fyne.DoAndWait(func() {
		q.Fase, q.Cursor, q.CargandoMas = Cargando, "", false
		id, carpeta = q.Nube.InstanceID, q.carpeta()
		p.Repintar()
	})
	if id == "" || p.integraciones == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	pag, err := p.integraciones.NubeElementos(ctx, id, carpeta, "")
	enHilo(func() {
		if q.Nube.InstanceID != id || q.carpeta() != carpeta {
			return
		}
		q.Elementos, q.Cursor = pag.Items, pag.NextCursor
		q.Fase = FaseDe(err, p.hayRed() || err == nil, len(pag.Items))
		p.Repintar()
	})
}

// masDeNube es «Cargar más»: la página siguiente con el cursor del proveedor.
func (p *Pantalla) masDeNube() {
	q := p.sec.nube
	if q.CargandoMas || q.Cursor == "" {
		return
	}
	q.CargandoMas = true
	id, carpeta, cursor := q.Nube.InstanceID, q.carpeta(), q.Cursor
	p.Repintar()
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		pag, err := p.integraciones.NubeElementos(ctx, id, carpeta, cursor)
		enHilo(func() {
			if q.Nube.InstanceID != id || q.carpeta() != carpeta {
				return
			}
			q.CargandoMas = false
			if err != nil {
				p.avisar(ui.AvisoPeligro, p.tr.T("cloud.failed", nil), "")
			} else {
				q.Elementos, q.Cursor = append(q.Elementos, pag.Items...), pag.NextCursor
			}
			p.Repintar()
		})
	}()
}

// subirANube es «Subir»: el selector del sistema y, uno a uno, a la carpeta abierta.
// Lo que pasa de 100 MB se avisa y no se manda.
func (p *Pantalla) subirANube() {
	q := p.sec.nube
	if p.ElegirFicheros == nil || p.integraciones == nil || q.Subiendo || q.SinRed() {
		return
	}
	id, carpeta := q.Nube.InstanceID, q.carpeta()
	p.ElegirFicheros(func(entradas []Entrada) {
		if len(entradas) == 0 {
			return
		}
		q.Subiendo = true
		p.Repintar()
		go func() {
			ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Minute)
			defer cancelar()
			for _, e := range entradas {
				if err := p.subirANubeUno(ctx, id, carpeta, e); err != nil {
					clave := "cloud.failed"
					if errors.Is(err, errNubeGrande) {
						clave = "cloud.tooLarge"
					}
					p.avisar(ui.AvisoPeligro, p.tr.T(clave, nil), e.nombre())
				}
			}
			enHilo(func() { q.Subiendo = false })
			p.cargarNube()
		}()
	})
}

var errNubeGrande = errors.New("nube: el fichero supera el tope de subida")

func (p *Pantalla) subirANubeUno(ctx context.Context, id, carpeta string, e Entrada) error {
	r, tamano, err := e.abrir()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if tamano > api.NubeMaxBytes {
		return errNubeGrande
	}
	_, err = p.integraciones.NubeSubir(ctx, id, carpeta, e.nombre(), r, tamano)
	return err
}

// desconectarNube revoca la nube en la pasarela y vuelve a «Mi unidad».
func (p *Pantalla) desconectarNube() {
	q := p.sec.nube
	var id string
	fyne.DoAndWait(func() {
		id = q.Nube.InstanceID
		q.Ocupado = true
		p.Repintar()
	})
	if id == "" || p.integraciones == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	err := p.integraciones.NubeDesconectar(ctx, id)
	enHilo(func() {
		q.Ocupado = false
		if err != nil {
			p.Repintar()
			return
		}
		q.Nube, p.lateral.NubeActiva = api.NubePersonal{}, ""
		p.irA(MiUnidad)
	})
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("cloud.failed", nil), "")
		return
	}
	p.cargarNubes()
}
