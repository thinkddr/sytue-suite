// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Las secciones del carril que no son «Mi unidad» ni la papelera
//
// Se quedaban en «Cargando» para siempre: la pantalla no sabía pedirlas. Cada una tiene
// su vista (vistas.go, compartido.go) y aquí vive lo que habla con el servicio.

// secciones son las vistas de las secciones planas, creadas con la pantalla.
type secciones struct {
	recientes, destacados *VistaPlana
	compartido            *VistaCompartido
	documentos            *VistaDocumentos
	equipos               *VistaEquipos
	nube                  *VistaNube
	// cursorRecientes es el de la página siguiente de «Recientes».
	cursorRecientes string
	// buscarDocs es el rebote del buscador de «Documentos».
	buscarDocs *time.Timer
}

func (p *Pantalla) montarSecciones() {
	s := &p.sec
	s.recientes = NuevaVistaPlana(p.tr, Recientes, p.favoritos)
	s.destacados = NuevaVistaPlana(p.tr, Favoritos, p.favoritos)
	for _, vp := range []*VistaPlana{s.recientes, s.destacados} {
		vp.AlAbrir = p.abrirPlano
		vp.AlEstrella = p.estrellaPlana
		vp.AlMenu = p.menuPlano
	}
	s.recientes.AlReintentar = p.reintentarCon(Recientes, func() { go p.cargarRecientes() })
	s.destacados.AlReintentar = p.reintentarCon(Favoritos, func() { go p.cargarDestacados() })
	s.recientes.AlCargarMas = p.masRecientes

	s.compartido = NuevaVistaCompartido(p.tr)
	c := s.compartido
	c.AlReintentar = p.reintentarCon(CompartidoConmigo, func() { go p.cargarCompartido() })
	c.AlAbrirRaiz = p.abrirRaizCompartida
	c.AlAbrir = p.abrirEnCompartido
	c.AlVolver = func() {
		c.Raiz, c.Subcamino, c.Carpeta, c.Denegada = nil, "", nil, false
		p.Repintar()
		go p.cargarCompartido()
	}
	c.AlIr = func(sub string) { c.Subcamino = sub; go p.cargarCarpetaCompartida() }
	c.AlReintentarCarpeta = func() { go p.cargarCarpetaCompartida() }
	c.AlDescargar = func(raiz, sub, nombre string) { go p.descargarCompartido(raiz, sub, nombre) }
	c.AlSubir = p.subirACompartida

	s.documentos = NuevaVistaDocumentos(p.tr)
	d := s.documentos
	d.AlReintentar = p.reintentarCon(Documentos, func() { go p.cargarDocumentos(strings.TrimSpace(d.Consulta)) })
	d.AlAbrir = p.abrirDocumento
	d.AlBuscar = func(q string) {
		d.Consulta = q
		if s.buscarDocs != nil {
			s.buscarDocs.Stop()
		}
		// El filtro va AL SERVICIO, con el mismo rebote de 300 ms que el explorador.
		s.buscarDocs = time.AfterFunc(RetrasoBusqueda, func() { p.cargarDocumentos(strings.TrimSpace(q)) })
	}
}

// reintentarCon es el botón de los estados de error de una sección: entrar si la sesión
// caducó, pedirla otra vez si no.
func (p *Pantalla) reintentarCon(s Seccion, recargar func()) func() {
	return func() {
		if p.faseDe(s) == SinSesion {
			p.entrar()
			return
		}
		recargar()
	}
}

func (p *Pantalla) faseDe(s Seccion) Fase {
	switch s {
	case Recientes:
		return p.sec.recientes.Fase
	case Favoritos:
		return p.sec.destacados.Fase
	case CompartidoConmigo:
		return p.sec.compartido.Fase
	case Documentos:
		return p.sec.documentos.Fase
	case MiUnidad:
		return p.explor.Fase
	case Papelera:
		return p.papelera.Fase
	case UnidadesDeEquipo:
		return p.sec.equipos.Fase
	case Nube:
		return p.sec.nube.Fase
	}
	return Cargando
}

// ---- «Recientes» y «Destacados» ----

func (p *Pantalla) cargarRecientes() {
	vp := p.sec.recientes
	enHilo(func() { vp.Fase, vp.HayMas, vp.CargandoMas = Cargando, false, false; p.Repintar() })
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	items, cursor, err := p.svc.Recent(ctx, "")
	elementos := DePlanos(items)
	enHilo(func() {
		vp.Elementos, p.sec.cursorRecientes = elementos, cursor
		vp.HayMas = err == nil && cursor != ""
		vp.Fase = FaseDe(err, p.hayRed() || err == nil, len(elementos))
		p.Repintar()
	})
}

// masRecientes pide la tanda siguiente. Un fallo no tumba lo ya pintado: se avisa y se
// conserva el cursor para volver a intentarlo.
func (p *Pantalla) masRecientes() {
	vp := p.sec.recientes
	if vp.CargandoMas || p.sec.cursorRecientes == "" || p.svc == nil {
		return
	}
	vp.CargandoMas = true
	cursor := p.sec.cursorRecientes
	p.Repintar()
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		items, sig, err := p.svc.Recent(ctx, cursor)
		enHilo(func() {
			vp.CargandoMas = false
			if err != nil {
				p.avisar(ui.AvisoPeligro, p.tr.T("files.loadMoreFailed", nil), "")
				p.Repintar()
				return
			}
			vp.Elementos = append(vp.Elementos, DePlanos(items)...)
			p.sec.cursorRecientes = sig
			vp.HayMas = sig != ""
			p.Repintar()
		})
	}()
}

func (p *Pantalla) cargarDestacados() {
	vp := p.sec.destacados
	enHilo(func() { vp.Fase = Cargando; p.Repintar() })
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	items, err := p.svc.Favorites(ctx)
	elementos := DePlanos(items)
	enHilo(func() {
		vp.Elementos = elementos
		// En «Destacados» todo lo listado es favorito: así la estrella sale bien aunque
		// la carga del conjunto no haya vuelto todavía.
		for _, el := range elementos {
			p.favoritos[el.ID] = true
		}
		vp.Fase = FaseDe(err, p.hayRed() || err == nil, len(elementos))
		p.Repintar()
	})
}

// DePlanos convierte una lista del servicio en elementos SIN reordenarla: «Recientes» y
// «Destacados» van en el orden en que el servicio los manda.
func DePlanos(c api.Compartidos) []Elemento {
	out := make([]Elemento, 0, len(c))
	for _, x := range c {
		e := deCompartido(x)
		e.Ruta = x.Path
		out = append(out, e)
	}
	return out
}

// abrirPlano es la misma puerta que el explorador: en su app si la suite tiene una para
// ese formato, y si no, se descarga. Que dependiera de por dónde se llegó sería un
// comportamiento que nadie puede explicar.
func (p *Pantalla) abrirPlano(el Elemento) {
	if p.abrirEnSuApp(el) {
		return
	}
	if TipoVista(el) != "" {
		p.pedirVista(el)
		return
	}
	go p.descargar(el)
}

// estrellaPlana pone o quita la estrella. En «Destacados», quitarla retira la fila al
// momento; si el servicio no la acepta, se vuelve a pedir la lista.
func (p *Pantalla) estrellaPlana(el Elemento, quiere bool) {
	vp := p.sec.destacados
	enDestacados := p.seccion == Favoritos
	if !quiere && enDestacados {
		quedan := vp.Elementos[:0:0]
		for _, x := range vp.Elementos {
			if x.ID != el.ID {
				quedan = append(quedan, x)
			}
		}
		vp.Elementos = quedan
		if len(quedan) == 0 {
			vp.Fase = Vacia
		}
	}
	p.favoritos[el.ID] = quiere
	p.Repintar()
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		if err := p.svc.SetFavorite(ctx, claseDe(el), el.ID, quiere); err != nil {
			enHilo(func() { p.favoritos[el.ID] = !quiere; p.Repintar() })
			if !quiere && enDestacados {
				p.cargarDestacados()
			}
			p.avisar(ui.AvisoPeligro, p.tr.T("files.favorite.failed", nil), "")
		}
	}()
}

// menuPlano es el «⋯» de «Recientes» y «Destacados»: `ActionsMenu` solo con descargar
// —un fichero; una carpeta no se descarga desde aquí— y compartir. Sin red, a una
// carpeta no le queda nada y el menú no se abre, en vez de abrirse vacío.
func (p *Pantalla) menuPlano(el Elemento, disparador fyne.CanvasObject) {
	var acciones []accionMenu
	if !el.Carpeta {
		acciones = append(acciones, accionMenu{
			texto: p.tr.T("action.download", nil), cuerpo: glifoDescargar,
			hacer: func() { go p.descargar(el) },
		})
	}
	if !p.sec.recientes.SinRed() {
		acciones = append(acciones, accionMenu{
			texto: p.tr.T("action.shareLink", nil), cuerpo: glifoEnlace,
			hacer: func() { p.pedirCompartir(el) },
		})
	}
	if len(acciones) == 0 {
		return
	}
	m := ui.NuevoMenu(elementosMenu(acciones)...)
	m.Rotulo = p.tr.T("action.menu", i18n.Vars{"name": el.Nombre})
	m.Mostrar(disparador)
}

// ---- «Compartido conmigo» ----

func (p *Pantalla) cargarCompartido() {
	c := p.sec.compartido
	enHilo(func() { c.Fase = Cargando; p.Repintar() })
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	raices, err := p.svc.SharedWithMe(ctx)
	enHilo(func() {
		c.Raices = raices
		c.Fase = FaseDe(err, p.hayRed() || err == nil, len(raices))
		p.Repintar()
		if id := p.pedidoCompartido; id != "" && err == nil {
			p.pedidoCompartido = ""
			if i := slices.IndexFunc(raices, func(r api.Compartido) bool { return r.ID == id }); i >= 0 {
				p.abrirRaizCompartida(raices[i])
			}
		}
	})
}

// abrirRaizCompartida entra en una carpeta compartida o descarga un fichero compartido.
func (p *Pantalla) abrirRaizCompartida(r api.Compartido) {
	if r.ID == "" {
		return
	}
	if !r.IsDir {
		go p.descargarCompartido(r.ID, "", r.Name)
		return
	}
	c := p.sec.compartido
	raiz := r
	c.Raiz, c.Subcamino, c.Permiso = &raiz, "", r.Permission
	p.engancharPeticion(r)
	go p.cargarCarpetaCompartida()
}

// engancharPeticion pone la salida de la franja de una raíz en lectura: pedir acceso a
// quien la compartió, y, al llegar su concesión por el Hub, volver a leerla con permiso.
func (p *Pantalla) engancharPeticion(r api.Compartido) {
	c := p.sec.compartido
	if p.svc == nil || r.Permission == "edit" {
		c.Peticion.Soltar()
		c.Peticion = peticion.Estado{}
		return
	}
	pedida := r.EditRequested
	peticion.Enganchar(&c.Peticion, p.svc.Peticiones(r.ID), peticion.Opciones{
		SinPermiso: true,
		YaPedida:   func(context.Context) (bool, error) { return pedida, nil },
		Recomponer: p.Repintar,
		T:          p.tr.T,
		Concedido:  "drive.access.granted", Recurso: r.ID,
		AlConceder: func() {
			if c.Raiz != nil && c.Raiz.ID == r.ID {
				c.Permiso = "edit"
				go p.cargarCarpetaCompartida()
			}
		},
	})
}

// abrirEnCompartido entra en una subcarpeta o descarga el fichero.
func (p *Pantalla) abrirEnCompartido(x api.Compartido) {
	c := p.sec.compartido
	if c.Raiz == nil {
		return
	}
	if x.IsDir {
		c.Subcamino = x.Path
		go p.cargarCarpetaCompartida()
		return
	}
	go p.descargarCompartido(c.Raiz.ID, x.Path, x.Name)
}

// cargarCarpetaCompartida pide lo de dentro. Un 403 o un 404 es que el share se revocó o
// caducó mientras se miraba: el servicio deniega por defecto y no dice si existe.
func (p *Pantalla) cargarCarpetaCompartida() {
	c := p.sec.compartido
	var raiz, sub string
	fyne.DoAndWait(func() {
		c.FaseCarpeta, c.Denegada = Cargando, false
		if c.Raiz != nil {
			raiz, sub = c.Raiz.ID, c.Subcamino
		}
		p.Repintar()
	})
	if raiz == "" || p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	l, err := p.svc.SharedList(ctx, raiz, sub)
	enHilo(func() {
		if c.Raiz == nil || c.Raiz.ID != raiz || c.Subcamino != sub {
			return
		}
		codigoErr := codigo(err)
		c.Denegada = codigoErr == http.StatusForbidden || codigoErr == http.StatusNotFound
		c.Carpeta = l.Items
		if l.Permission != "" {
			c.Permiso = l.Permission
		}
		c.FaseCarpeta = FaseDe(err, p.hayRed() || err == nil, len(l.Items))
		p.Repintar()
	})
}

// descargarCompartido baja un fichero compartido a la carpeta de descargas.
func (p *Pantalla) descargarCompartido(raiz, sub, nombre string) {
	if p.svc == nil {
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancelar()
	ruta, err := DescargarCompartido(ctx, p.svc, raiz, sub, nombre)
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("files.error.network", nil), motivo(err))
		return
	}
	p.avisar(ui.AvisoExito, nombre, ruta)
}

// subirACompartida es «Subir aquí»: el selector del sistema y, fichero a fichero, a la
// carpeta compartida. Con permiso de solo lectura el servicio dice 403, y se dice.
func (p *Pantalla) subirACompartida() {
	c := p.sec.compartido
	if p.ElegirFicheros == nil || c.Raiz == nil || !c.PuedeEditar() || c.Subiendo {
		return
	}
	raiz, sub := c.Raiz.ID, c.Subcamino
	p.ElegirFicheros(func(entradas []Entrada) {
		if len(entradas) == 0 {
			return
		}
		c.Subiendo = true
		p.Repintar()
		go func() {
			ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Minute)
			defer cancelar()
			var err error
			for i, e := range entradas {
				if err = subirACompartidaUno(ctx, p.svc, raiz, sub, e); err != nil {
					cerrar(entradas[i+1:]) // los flujos que ya no se suben
					break
				}
			}
			enHilo(func() { c.Subiendo = false; p.Repintar() })
			switch {
			case err == nil:
				p.avisar(ui.AvisoExito, p.tr.T("files.upload.done", i18n.Vars{"count": len(entradas)}), "")
			case codigo(err) == http.StatusForbidden:
				p.avisar(ui.AvisoPeligro, p.tr.T("shared.uploadDenied", nil), "")
			default:
				p.avisar(ui.AvisoPeligro, p.tr.T("shared.uploadFailed", nil), "")
			}
			p.cargarCarpetaCompartida()
		}()
	})
}

// ---- «Documentos» ----

// cargarDocumentos pide los documentos de Docs, filtrados por título en el servicio.
func (p *Pantalla) cargarDocumentos(q string) {
	d := p.sec.documentos
	enHilo(func() { d.Fase = Cargando; p.Repintar() })
	if p.Docs == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	docs, _, err := p.Docs.List(ctx, api.DocsFilter{Query: q, Limit: 50})
	out := make([]Documento, 0, len(docs))
	for _, x := range docs {
		out = append(out, Documento{ID: x.ID, Titulo: x.Title, Tipo: x.Kind, Cambiado: x.UpdatedAt})
	}
	enHilo(func() {
		if strings.TrimSpace(d.Consulta) != q {
			return // se ha seguido escribiendo
		}
		d.Documentos, d.Ahora = out, time.Now()
		d.Fase = FaseDe(err, p.hayRed() || err == nil, len(out))
		p.Repintar()
	})
}

// abrirDocumento lleva al editor de Docs, que es una página web (ver crearDoc).
func (p *Pantalla) abrirDocumento(d Documento) {
	_ = api.AbrirFuera(p.abridor(), api.EnlaceDocumento(p.apex(), p.org(), d.ID, d.Tipo))
}
