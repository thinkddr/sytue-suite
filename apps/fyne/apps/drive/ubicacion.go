// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Abrir carpetas y «Mostrar en su carpeta» desde lo que cruza el árbol
//
// «Recientes», «Destacados» y los resultados de búsqueda traen elementos de cualquier
// carpeta. Una carpeta de esas listas era texto muerto, y de un fichero no había forma de
// ir a la suya: lo primero que se echa de menos frente al «Mostrar ubicación» de otros.
// Es `lib/ubicacion.ts` de la web, que lo hace escribiendo `?path=&select=` y montando
// «Mi unidad»; aquí se cambia de sección y se carga la carpeta con sus migas verdaderas
// (`RutaHasta`, la misma subida de padre en padre que al abrir una carpeta buscada).

// errSinCarpeta es un elemento que no cuelga de ninguna carpeta: la raíz misma.
var errSinCarpeta = errors.New("drive: el elemento no está en ninguna carpeta")

// marcaPendiente es lo que marcar y poner a la vista cuando llegue su carpeta.
type marcaPendiente struct {
	carpeta string // "" es la raíz
	id      string
}

// abrirCarpetaLejana abre en «Mi unidad» una carpeta de «Recientes» o «Destacados».
// Se llama en la rutina de la interfaz.
func (p *Pantalla) abrirCarpetaLejana(el Elemento) {
	p.unidadPara(el) // una carpeta de una nube se mira dentro de su unidad
	p.aMiUnidad()
	p.enSegundoPlano(func() { p.abrirDesdeLejos(el) })
}

// mostrarEnSuCarpeta abre la carpeta que contiene `el`, con `el` marcado y a la vista.
// Se llama en la rutina de la interfaz: lo que ya se sabe —su carpeta, si llegó en el
// listado; la raíz— se lee aquí, y lo que hay que preguntar va aparte.
func (p *Pantalla) mostrarEnSuCarpeta(el Elemento) {
	p.unidadPara(el)
	padre, raiz, actual := p.padreConocido(el), p.raizID, p.carpetaActual()
	p.aMiUnidad()
	p.enSegundoPlano(func() { p.ubicar(el, padre, raiz, actual) })
}

// aMiUnidad cambia a «Mi unidad» sin cargar nada (lo carga quien llama) y deja la lista
// en «Cargando» y arriba, como al entrar en cualquier carpeta.
func (p *Pantalla) aMiUnidad() {
	if p.explor.EnBusqueda || p.explor.Consulta != "" {
		p.salirDeLaBusqueda()
	}
	p.seccion = MiUnidad
	p.lateral.Activa = MiUnidad
	p.explor.Fase = Cargando
	p.explor.AlPrincipio()
	p.Repintar()
}

// ubicar resuelve la carpeta de `el` y sus migas y la carga. Corre en su propia rutina.
// `actual` es la carpeta que se miraba, adonde se vuelve si no se puede saber la suya.
func (p *Pantalla) ubicar(el Elemento, padre, raiz, actual string) {
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	if padre == "" {
		var err error
		if padre, err = PadreDe(ctx, p.svc.File, p.svc.Folder, el); err != nil {
			// Sin saber dónde vive no se puede ir: se queda en la carpeta que había y se
			// dice, en vez de abrir otra cualquiera.
			p.avisar(ui.AvisoPeligro, p.tr.T("files.error.network", nil), motivo(err))
			p.cargarCarpeta(actual)
			return
		}
	}
	ruta := MigasDe(ctx, p.svc.Folder, padre, raiz)
	if len(ruta) == 0 {
		padre = "" // la raíz se pide como raíz: así sirve también lo guardado sin conexión
	}
	enHilo(func() {
		p.explor.Ruta = ruta
		p.alLlegar = marcaPendiente{carpeta: padre, id: el.ID}
	})
	p.cargarCarpeta(padre)
}

// PadreDe es el id de la carpeta que contiene `el`, preguntando al servicio: por el
// fichero, su `folder_id`; por la carpeta, su `parent_id`. Las funciones son
// `api.Drive.File` y `api.Drive.Folder`; se pasan para que el guardián no necesite
// servicio. Una carpeta sin padre es la raíz y no tiene «su carpeta».
func PadreDe(ctx context.Context, fichero func(context.Context, string) (api.File, error),
	carpeta func(context.Context, string) (api.Folder, error), el Elemento,
) (string, error) {
	if !el.Carpeta {
		f, err := fichero(ctx, el.ID)
		if err != nil {
			return "", err
		}
		if f.FolderID == "" {
			return "", errSinCarpeta
		}
		return f.FolderID, nil
	}
	c, err := carpeta(ctx, el.ID)
	if err != nil {
		return "", err
	}
	if c.ParentID == nil || *c.ParentID == "" {
		return "", errSinCarpeta
	}
	return *c.ParentID, nil
}

// MigasDe son las migas de la carpeta `id`: vacías si es la raíz, y si no, de la raíz
// hasta ella. Sin poder subir —sin red y sin esas carpetas guardadas— se entra igual con
// la carpeta sola, que es lo que hace `abrirDesdeLejos`: mejor unas migas cortas que no
// abrir la carpeta.
func MigasDe(ctx context.Context, pedir func(context.Context, string) (api.Folder, error), id, raiz string) []Elemento {
	if id == "" || id == raiz {
		return nil
	}
	c, err := pedir(ctx, id)
	if err != nil {
		return []Elemento{{ID: id, Carpeta: true, Tamano: -1}}
	}
	if c.IsRoot || c.ParentID == nil {
		return nil
	}
	ruta, err := RutaHasta(ctx, pedir, DeCarpeta(c))
	if err != nil {
		return []Elemento{DeCarpeta(c)}
	}
	return ruta
}

// padreConocido es la carpeta de `el` si ya se sabe sin preguntar: la trae el listado
// (`folder_id`, `parent_id`) o se vio al cargar esa carpeta. "" si no se sabe.
func (p *Pantalla) padreConocido(el Elemento) string {
	if el.Padre != "" {
		return el.Padre
	}
	return p.padres[el.ID]
}

// ubicable dice si «Mostrar en su carpeta» se puede ofrecer. Con red, siempre que el
// elemento tenga id. Sin red, solo si su carpeta se cargó en esta sesión —y por tanto la
// guarda la caché del transporte—: llevar a una carpeta que no se puede pintar sería un
// menú que promete y luego enseña un error. La raíz cuenta como cargada si se cargó.
func (p *Pantalla) ubicable(el Elemento, sinRed bool) bool {
	if el.ID == "" {
		return false
	}
	if !sinRed {
		return true
	}
	padre := p.padreConocido(el)
	return padre != "" && p.cargadas[padre]
}

// aplicarMarca marca el elemento pendiente si la carpeta que acaba de llegar es la suya.
// Se llama en la rutina de la interfaz con la carpeta ya pintada en `explor`; `pedida`
// es el id con que se pidió ("" la raíz) y `id` el resuelto.
func (p *Pantalla) aplicarMarca(pedida, id string) {
	m := p.alLlegar
	if m.id == "" || (m.carpeta != pedida && m.carpeta != id) {
		return
	}
	p.alLlegar = marcaPendiente{}
	for i, el := range p.explor.EnOrden() {
		if el.ID == m.id {
			p.explor.Seleccion[m.id] = true
			p.ancla = i
			p.explor.AVista = m.id
			return
		}
	}
}
