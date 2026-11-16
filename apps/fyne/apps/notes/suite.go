// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Lo que Notepads toma de otras apps de la suite
//
// Dos cosas de la web dependían de su BFF (`/api/suite` y `/api/share`) y aquí se hacen
// con los clientes de `api.Elegir`, que llevan el token de la persona:
//
//   - «Compartir en Rooms» (`share.toRoom`) abre el selector de la suite con las salas y
//     publica en la elegida el ENLACE de la nota, nunca su contenido.
//   - La herramienta de enlaces ofrece, además de las notas, los ficheros de Drive.
//
// Sin `Suite` —o sin su cliente de Rooms o de Drive— la entrada no se ofrece: una acción
// que siempre falla es peor que ninguna.

// VincularAutorizacionSuite binds the presenter to its persistent login witness.
// Each gesture obtains a fresh short intent, so a normal token refresh is allowed.
func (p *Pantalla) VincularAutorizacionSuite(preparar func(context.Context) context.Context, verificar func(context.Context) error) {
	p.prepararSuite, p.verificarSuite = preparar, verificar
}

func (p *Pantalla) capturarAutorizacionSuite() context.Context {
	if p.prepararSuite == nil || p.verificarSuite == nil {
		return nil
	}
	ctx, _ := p.contextoTrabajo()
	if p.verificarSuite(ctx) != nil {
		return nil
	}
	ctx = p.prepararSuite(ctx)
	if ctx == nil || p.verificarSuite(ctx) != nil || !auth.RequestAuthorizationIntent(ctx).Current() {
		return nil
	}
	return ctx
}

// A callback belongs to the persistent login, not the short token revision:
// another request may refresh that token without changing the presenter owner.
func (p *Pantalla) suiteVidaVigente(ctx context.Context) bool {
	return ctx != nil && p.verificarSuite != nil && p.verificarSuite(context.WithoutCancel(ctx)) == nil
}

func (p *Pantalla) suiteSigueVigente(ctx context.Context) bool {
	return p.suiteVidaVigente(ctx) && auth.RequestAuthorizationIntent(ctx).Current()
}

// ofreceCompartir dice si «Compartir en Rooms» tiene con qué hacerse.
func (p *Pantalla) ofreceCompartir() bool { return p.Suite != nil && p.Suite.Rooms != nil }

// textosSelector son los del selector de la suite, del catálogo de Notepads.
func (p *Pantalla) textosSelector(titulo string) ui.TextosSelector {
	t := func(k string) string { return p.tr.T(k, nil) }
	return ui.TextosSelector{
		Titulo: t(titulo), Buscar: t("picker.search"), Migas: t("picker.breadcrumb"),
		Raiz: t("picker.myDrive"), Vacio: t("picker.empty"), Fallo: t("picker.failed"),
		Reintentar: t("common.retry"), MasResultados: t("picker.loadMore"),
		Cancelar: t("common.cancel"), Cerrar: t("common.close"),
	}
}

// permiteEnviarContenidoExterno impide entregar texto de repositorios privados a
// Rooms y Calendar mediante sus clientes HTTP, que reciben contenido en claro. El ámbito se comprueba de nuevo
// al elegir sala, pues el selector puede seguir abierto al cambiar de cuaderno.
func (p *Pantalla) permiteEnviarContenidoExterno(cuaderno string, n Nota) bool {
	return p.vigente() && p.prepararSuite != nil && p.verificarSuite != nil && p.vida().work == nil && !p.activo.Cifrado &&
		!n.Cifrada && p.activo.ID == cuaderno && n.Cuaderno == cuaderno
}

// compartirEnSala es el `onShare` de la web: elegir la sala y publicar. El texto es el
// que compone su BFF (`api.MensajeNota`), con el título en claro de la nota.
func (p *Pantalla) compartirEnSala(n Nota) {
	if !p.ofreceCompartir() || p.win == nil || !p.permiteEnviarContenidoExterno(p.activo.ID, n) {
		return
	}
	cuaderno := p.activo.ID
	owner := p.claveAlmacen()
	trabajo := p.capturarAutorizacionSuite()
	if trabajo == nil {
		return
	}
	suite := p.Suite
	sel := &ui.SelectorSuite{
		Origen: api.ElegirRooms, Locale: p.loc, Textos: p.textosSelector("picker.rooms.title"),
		// Los glifos de SuitePicker.tsx: `link` para una sala; el de carpeta no se ve.
		GlifoCarpeta: CuadernoD, GlifoElemento: EnlaceD,
		Cargar: func(ctx context.Context, carpeta, q, cursor string) (api.PaginaSuite, error) {
			if err := ctx.Err(); err != nil {
				return api.PaginaSuite{}, err
			}
			vinculado, cancelar := context.WithCancel(trabajo)
			detener := context.AfterFunc(ctx, cancelar)
			defer detener()
			defer cancelar()
			return suite.Listar(vinculado, api.ElegirRooms, carpeta, q, cursor)
		},
		AlElegir: func(e api.Elegible) {
			if !p.permiteEnviarContenidoExterno(cuaderno, n) || trabajo.Err() != nil || p.claveAlmacen() != owner || p.Suite != suite {
				return
			}
			organizacion := auth.RequestAuthorizationIntent(trabajo).Identity().Organization
			p.iniciar(func() { p.publicarEnSalaConContexto(trabajo, suite, organizacion, cuaderno, n, e) })
		},
	}
	sel.Mostrar(p.win.Canvas())
}

// publicarEnSala manda el mensaje y lo dice: «Compartida en General» o que no se pudo.
// Corre fuera del hilo de la interfaz.
func (p *Pantalla) publicarEnSala(cuaderno string, n Nota, sala api.Elegible) {
	if !p.permiteEnviarContenidoExterno(cuaderno, n) || !p.ofreceCompartir() {
		return
	}
	ctx := p.capturarAutorizacionSuite()
	if ctx == nil {
		return
	}
	p.publicarEnSalaConContexto(ctx, p.Suite, auth.RequestAuthorizationIntent(ctx).Identity().Organization, cuaderno, n, sala)
}

func (p *Pantalla) publicarEnSalaConContexto(trabajo context.Context, suite *api.Elegir, organizacion, cuaderno string, n Nota, sala api.Elegible) {
	t := p.tr.T
	texto := api.MensajeNota(suite.Apex, organizacion, cuaderno, n.ID, n.Titulo, t("note.untitled", nil))
	ctx, cancelar := context.WithTimeout(trabajo, p.Plazo)
	defer cancelar()
	_, err := suite.Rooms.Post(ctx, sala.ID, texto)
	p.enHilo(func() {
		if !p.suiteVidaVigente(trabajo) {
			return
		}
		if err != nil {
			p.avisar(ui.AvisoPeligro, t("share.failed", nil), "")
			return
		}
		p.avisar(ui.AvisoExito, t("share.sent", i18n.Vars{"channel": sala.Nombre}), "")
	})
}

// opcionesDrive son los ficheros de Drive cuyo nombre casa con lo escrito, para la
// herramienta de enlaces: hasta ocho, rotulados «Sytue Drive» como en la web (es una
// marca y no se traduce). Las carpetas no se enlazan.
func (p *Pantalla) opcionesDrive(ctx context.Context, q string) []opcionEnlace {
	if p.Suite == nil || p.Suite.Drive == nil {
		return nil
	}
	pg, err := p.Suite.Listar(ctx, api.ElegirDrive, "", q, "")
	if err != nil {
		return nil // como la web: un fallo de Drive deja la lista sin sus ficheros
	}
	var out []opcionEnlace
	for _, e := range pg.Elegibles {
		if e.Carpeta || e.URL == "" {
			continue
		}
		out = append(out, opcionEnlace{url: e.URL, rotulo: e.Nombre, nota: e.Nombre, cuaderno: MarcaDrive, tipo: enlaceDrive})
		if len(out) == maxOpcionesEnlace {
			break
		}
	}
	return out
}

// MarcaDrive es el rótulo del grupo y la meta de cada fichero: el nombre del producto.
const MarcaDrive = "Sytue Drive"

// buscarFuera hace las dos búsquedas de la herramienta de enlaces a la vez —las notas de
// los otros cuadernos y, con `conDrive`, los ficheros de Drive— y devuelve cuando han
// vuelto las dos. El `[[` del editor busca sin Drive, como la web.
//
// ponytail: la web pinta cada lista en cuanto llega la suya; aquí se espera a la más
// lenta. Con dos servicios del mismo clúster la diferencia no se ve.
func (p *Pantalla) buscarFuera(q string, conDrive bool) (remotas, drive []opcionEnlace) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	var wg sync.WaitGroup
	if p.svc != nil {
		wg.Go(func() {
			if hits, err := p.svc.Search(ctx, q, limiteBusquedaEnl, false); err == nil {
				remotas = p.opcionesRemotas(q, hits)
			}
		})
	}
	if conDrive {
		wg.Go(func() { drive = p.opcionesDrive(ctx, strings.TrimSpace(q)) })
	}
	wg.Wait()
	return remotas, drive
}
