// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/portal/src/pages/bienvenida.astro

package hub

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Te damos la bienvenida a Sytue»: el alta guiada de `Onboarding.tsx`, el primer
// contacto con el producto. Cuatro pasos para una persona y seis para una organización,
// y cada uno HACE algo de verdad contra Sytue ID —crear la organización y publicar su
// dirección, comprobar un DNS, invitar, cobrar, crear roles— y guarda el progreso al
// avanzar: cerrar la app a medias y volver retoma donde se dejó.
//
// Habla con Sytue ID directo (`api.Alta`), con el token de la sesión: la web lo hace por
// un proxy de su BFF que no hace nada más que reenviarlo.
//
// Lo que cambia por no ser un navegador, y solo eso:
//
//   - El cobro abre la pasarela en el navegador del sistema. La web vuelve a esta página
//     con `?payment=return`; aquí se vuelve a la app, y el botón pasa a «Comprobar el
//     pago», que es lo que la web enseña al volver.
//   - Al terminar, la web cierra la sesión para que Sytue ID emita un token con la
//     organización; aquí se vuelve a entrar (`sesion.Suite.Entrar`), que con la sesión
//     del navegador viva es una redirección y no pedir la contraseña.
//   - Los enlaces a Sytue Admin abren la web: Admin es solo web (§13.6).

// Altero es lo que el asistente le pide a Sytue ID. Lo cumple `*api.Alta`.
type Altero interface {
	Estado(ctx context.Context) (api.AltaEstado, error)
	Guardar(ctx context.Context, g api.AltaGuardado) error
	Identificador(ctx context.Context, slug string) (api.AltaIdentificador, error)
	CrearOrganizacion(ctx context.Context, nombre, slug string) (api.AltaOrg, error)
	Suscripcion(ctx context.Context) (api.AltaSuscripcion, error)
	Pagar(ctx context.Context, asientos int) (string, error)
	Dominio(ctx context.Context, host string) (api.AltaDominio, error)
	Cloudflare(ctx context.Context, p api.AltaCloudflarePeticion) (api.AltaCloudflare, error)
	Disponibilidad(ctx context.Context, dominio string) (api.AltaDisponibilidad, error)
	Invitar(ctx context.Context, correos, rol string) (int, []api.AltaInvitacion, error)
	CrearRoles(ctx context.Context, claves []string) error
}

var _ Altero = (*api.Alta)(nil)

// Medidas de `bienvenida.astro`: `mx-auto max-w-3xl px-4 py-8 sm:px-6 sm:py-12` y la
// cabecera con `mb-8`, el `h1` en `sytue-h2` y la entrada con `mt-1 text-base`.
const (
	AnchoBienvenida       float32 = 48 * tema.Rem
	MargenBienvenidaY     float32 = 8 * 0.25 * tema.Rem
	MargenBienvenidaYSM   float32 = 12 * 0.25 * tema.Rem
	MargenCabeceraAlta    float32 = 8 * 0.25 * tema.Rem
	MargenEntradaAlta     float32 = 1 * 0.25 * tema.Rem
	MargenDescripcionPaso float32 = 1.5 * 0.25 * tema.Rem
)

// CajaBienvenida es dónde empieza y cuánto mide la columna para un ancho de ventana.
func CajaBienvenida(ventana float32) (x, ancho float32) {
	margen := MargenAjustesX
	if ventana >= ui.CorteSM {
		margen = MargenX
	}
	caja := min(ventana, AnchoBienvenida)
	return (ventana-caja)/2 + margen, max(caja-2*margen, 0)
}

// pasosDe son los identificadores de cada recorrido, los que valida Sytue ID.
func pasosDe(modo string) []string {
	if modo == api.AltaIndividual {
		return []string{api.AltaPasoModo, api.AltaPasoIdentidad, api.AltaPasoFacturacion, api.AltaPasoListo}
	}
	return []string{
		api.AltaPasoModo, api.AltaPasoIdentidad, api.AltaPasoPersonas,
		api.AltaPasoFacturacion, api.AltaPasoRoles, api.AltaPasoListo,
	}
}

var (
	noAlnum   = regexp.MustCompile(`[^a-z0-9]+`)
	guionesEx = regexp.MustCompile(`^-+|-+$`)
)

// Slugify es `slugify` de la web: el identificador que se propone a partir del nombre,
// sin acentos ni símbolos y de 40 como mucho.
func Slugify(nombre string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(nombre)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	s := guionesEx.ReplaceAllString(noAlnum.ReplaceAllString(b.String(), "-"), "")
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.TrimRight(s, "-")
}

// Bienvenida es la pantalla `/bienvenida`: el estado del alta que dice Sytue ID y el
// formulario en curso, que es lo que se escribe ahora y todavía no se ha guardado.
type Bienvenida struct {
	pagina

	T    *i18n.Translator
	Alta Altero

	// Avisar enseña un aviso flotante; Copiar lleva un valor al portapapeles con su
	// aviso; AbrirURL delega en el sistema; Ir cambia de pantalla del Hub; Reentrar
	// vuelve a entrar para que el token lleve la organización nueva.
	Avisar   func(tono ui.TonoAviso, titulo, cuerpo string)
	Copiar   func(valor, clave string)
	AbrirURL func(string)
	Ir       func(Destino)
	Reentrar func(context.Context) error
	Apex     string

	// lanzar ejecuta una acción que habla con la red, y demorar la deja para dentro de
	// un rato; en las pruebas, las dos en el acto.
	lanzar  func(func())
	demorar func(time.Duration, func()) *time.Timer
	// sincronizando es que el renderer está llevando el estado a los campos: sus
	// `AlCambiar` no tienen que tomarlo por algo escrito.
	sincronizando atomic.Bool

	mu          sync.Mutex
	estado      *api.AltaEstado
	fallo       bool
	ocupado     bool
	paso        int
	modo        string
	nombre      string
	slug        string
	slugEditado bool
	comprobado  *api.AltaIdentificador
	eleccion    string // subdomain, own, search
	dominio     string
	dnsHost     string // cloudflare, manual
	cfToken     string
	cfCorreo    bool
	cfRecordar  bool
	cfSustituir []string
	cfPlan      *api.AltaCloudflare
	manualAbier bool
	buscado     string
	disponible  *api.AltaDisponibilidad
	correos     string
	rolInvitar  string
	invitadas   []api.AltaInvitacion
	facturacion api.AltaFacturacion
	suscripcion *api.AltaSuscripcion
	asientos    int
	pagoVuelto  bool
	elegidos    []string
	permisos    map[string]bool
	relojSlug   *time.Timer
}

var _ fyne.Widget = (*Bienvenida)(nil)

// NuevaBienvenida crea la pantalla, preparando el alta.
func NuevaBienvenida(t *i18n.Translator) *Bienvenida {
	b := &Bienvenida{
		T: t, Apex: api.ApexProd, eleccion: "subdomain", dnsHost: "cloudflare",
		rolInvitar: "member", asientos: 1, permisos: map[string]bool{},
		lanzar: func(f func()) { go f() }, demorar: time.AfterFunc,
	}
	b.ExtendBaseWidget(b)
	return b
}

// Resize recuerda el ancho: el alto depende de él.
func (b *Bienvenida) Resize(s fyne.Size) {
	if b.recordar(s) {
		b.BaseWidget.Resize(s)
		b.Refresh()
		return
	}
	b.BaseWidget.Resize(s)
}

// AccessibilityRole: la pantalla agrupa.
func (b *Bienvenida) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// enUI cambia el estado y repinta en el hilo de la interfaz.
func (b *Bienvenida) enUI(f func()) {
	fyne.Do(func() {
		b.mu.Lock()
		f()
		b.mu.Unlock()
		b.Refresh()
	})
}

// modoEfectivo es `mode ?? 'organization'`.
func (b *Bienvenida) modoEfectivo() string {
	if b.modo == "" {
		return api.AltaOrganizacion
	}
	return b.modo
}

func (b *Bienvenida) esOrg() bool { return b.modoEfectivo() == api.AltaOrganizacion }

func (b *Bienvenida) org() *api.AltaOrg {
	if b.estado == nil {
		return nil
	}
	return b.estado.Organization
}

// Cargar lee el alta de Sytue ID y se coloca en el paso guardado: la carga inicial de la
// web. `pagoVuelto` es el `?payment=return`: al volver de la pasarela se espera unos
// segundos al aviso del cobro, que puede llegar después que la persona.
func (b *Bienvenida) Cargar(ctx context.Context, pagoVuelto bool) {
	b.enUI(func() { b.fallo, b.estado = false, nil })
	e, err := b.Alta.Estado(ctx)
	if err != nil {
		b.enUI(func() { b.fallo = true })
		return
	}
	var sub *api.AltaSuscripcion
	if e.Organization != nil {
		s, err := b.esperarPago(ctx, pagoVuelto)
		if err != nil {
			b.enUI(func() { b.fallo = true })
			return
		}
		sub = &s
	}
	b.enUI(func() {
		b.estado, b.pagoVuelto = &e, pagoVuelto
		if e.Mode != "" {
			b.modo = e.Mode
		}
		if o := e.Organization; o != nil {
			b.nombre, b.slug, b.slugEditado = o.Name, o.Slug, true
			b.suscripcion = sub
			b.asientos = max(sub.Seats, sub.ActiveMembers, 1)
		}
		if e.Domain != nil {
			b.eleccion, b.dominio = "own", e.Domain.Host
		}
		if e.Billing != nil {
			b.facturacion = *e.Billing
		}
		if e.Account == nil {
			return
		}
		if b.facturacion.BillingEmail == "" {
			b.facturacion.BillingEmail = e.Account.Email
		}
		if e.Organization == nil && e.Mode == api.AltaIndividual {
			b.nombre = e.Account.Name
		}
		objetivo := e.Step
		if e.Completed {
			objetivo = api.AltaPasoListo
		}
		if i := slices.Index(pasosDe(b.modoEfectivo()), objetivo); i > 0 {
			b.paso = i
		}
	})
}

// esperarPago lee la suscripción y, si se vuelve de pagar y aún no hay derecho, lo
// vuelve a mirar seis veces con un segundo y medio: el aviso de la pasarela puede
// llegar después que la persona, y ofrecer un segundo cobro mientras tanto sería peor.
func (b *Bienvenida) esperarPago(ctx context.Context, pagoVuelto bool) (api.AltaSuscripcion, error) {
	s, err := b.Alta.Suscripcion(ctx)
	for intento := 0; err == nil && pagoVuelto && !s.Entitled && intento < 6; intento++ {
		select {
		case <-ctx.Done():
			return s, ctx.Err()
		case <-time.After(1500 * time.Millisecond):
		}
		s, err = b.Alta.Suscripcion(ctx)
	}
	return s, err
}

// hacer es `run` de la web: una acción a la vez, con el botón ocupado, y el fallo dicho
// en un aviso —el mensaje de Sytue ID si lo manda, que ya viene traducido; si no, el
// genérico del catálogo—.
func (b *Bienvenida) hacer(f func(ctx context.Context) error) {
	b.mu.Lock()
	if b.ocupado {
		b.mu.Unlock()
		return
	}
	b.ocupado = true
	b.mu.Unlock()
	b.Refresh()
	b.lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancelar()
		err := f(ctx)
		b.enUI(func() { b.ocupado = false })
		if err != nil {
			b.fallar(err)
		}
	})
}

// errMensaje es un fallo que ya trae su texto (el `throw new Error(...)` de la web).
type errMensaje string

func (e errMensaje) Error() string { return string(e) }

func (b *Bienvenida) fallar(err error) {
	cuerpo := b.T.T("ob.error.generic", nil)
	var a *api.Error
	var m errMensaje
	switch {
	case errors.As(err, &m):
		cuerpo = string(m)
	case errors.As(err, &a) && a.Message() != "":
		cuerpo = a.Message()
	}
	b.avisar(ui.AvisoPeligro, b.T.T("ob.saveFailedTitle", nil), cuerpo)
}

func (b *Bienvenida) avisar(tono ui.TonoAviso, titulo, cuerpo string) {
	if b.Avisar != nil {
		fyne.Do(func() { b.Avisar(tono, titulo, cuerpo) })
	}
}

// guardarYSeguir guarda el paso alcanzado —lo que hace real la reanudación— y avanza.
func (b *Bienvenida) guardarYSeguir(ctx context.Context, siguiente string) error {
	extra := api.AltaGuardado{}
	b.mu.Lock()
	extra.Mode, extra.Step = b.modo, siguiente
	b.mu.Unlock()
	if err := b.Alta.Guardar(ctx, extra); err != nil {
		return err
	}
	b.enUI(func() { b.paso = slices.Index(pasosDe(b.modoEfectivo()), siguiente) })
	return nil
}

func (b *Bienvenida) releer(ctx context.Context) error {
	e, err := b.Alta.Estado(ctx)
	if err != nil {
		return err
	}
	b.enUI(func() { b.estado = &e })
	return nil
}

func (b *Bienvenida) releerSuscripcion(ctx context.Context) (api.AltaSuscripcion, error) {
	s, err := b.Alta.Suscripcion(ctx)
	if err != nil {
		return s, err
	}
	b.enUI(func() {
		b.suscripcion = &s
		b.asientos = max(b.asientos, s.ActiveMembers, 1)
	})
	return s, nil
}

// rolCreado dice si ya existe un rol con ese nombre. Se llama con el cerrojo tomado.
func (b *Bienvenida) rolCreado(nombre string) bool {
	if b.estado == nil {
		return false
	}
	for _, r := range b.estado.Roles {
		if r.Name == nombre {
			return true
		}
	}
	return false
}

// comprobarSlug es la disponibilidad en vivo, a 350 ms de la última pulsación.
func (b *Bienvenida) comprobarSlug() {
	b.mu.Lock()
	b.comprobado = nil
	if b.relojSlug != nil {
		b.relojSlug.Stop()
	}
	v := strings.ToLower(strings.TrimSpace(b.slug))
	fuera := b.org() != nil || v == "" || b.Alta == nil
	b.mu.Unlock()
	if fuera {
		return
	}
	reloj := b.demorar(350*time.Millisecond, func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelar()
		r, err := b.Alta.Identificador(ctx, v)
		b.enUI(func() {
			if err == nil && strings.ToLower(strings.TrimSpace(b.slug)) == v {
				b.comprobado = &r
			}
		})
	})
	b.mu.Lock()
	b.relojSlug = reloj
	b.mu.Unlock()
}

// ---- Lo que hace cada paso ----

// avanzar es el `onNext` del paso en curso.
func (b *Bienvenida) avanzar() {
	b.mu.Lock()
	id := pasosDe(b.modoEfectivo())[min(b.paso, len(pasosDe(b.modoEfectivo()))-1)]
	b.mu.Unlock()
	switch id {
	case api.AltaPasoModo:
		b.hacer(func(ctx context.Context) error {
			return b.guardarYSeguir(ctx, api.AltaPasoIdentidad)
		})
	case api.AltaPasoIdentidad:
		b.hacer(b.seguirIdentidad)
	case api.AltaPasoPersonas:
		b.hacer(func(ctx context.Context) error {
			return b.guardarYSeguir(ctx, api.AltaPasoFacturacion)
		})
	case api.AltaPasoFacturacion:
		b.hacer(b.seguirFacturacion)
	case api.AltaPasoRoles:
		b.hacer(b.seguirRoles)
	case api.AltaPasoListo:
		b.hacer(b.terminar)
	}
}

// seguirIdentidad crea (o renombra) la organización y avanza. Con dominio propio se
// queda en el paso justo después de crearla: es cuando existe el registro exacto que hay
// que añadir. El siguiente clic ya avanza.
func (b *Bienvenida) seguirIdentidad(ctx context.Context) error {
	b.mu.Lock()
	org, nombre, slug, eleccion, esOrg := b.org(), strings.TrimSpace(b.nombre), strings.ToLower(strings.TrimSpace(b.slug)), b.eleccion, b.esOrg()
	b.mu.Unlock()
	primera := org == nil
	if primera || org.Name != nombre {
		creada, err := b.Alta.CrearOrganizacion(ctx, nombre, slug)
		if err != nil {
			return err
		}
		b.enUI(func() {
			if b.estado != nil {
				b.estado.Organization = &creada
			}
		})
		if _, err := b.releerSuscripcion(ctx); err != nil {
			return err
		}
	}
	if primera && eleccion == "own" {
		return nil
	}
	siguiente := api.AltaPasoFacturacion
	if esOrg {
		siguiente = api.AltaPasoPersonas
	}
	return b.guardarYSeguir(ctx, siguiente)
}

// seguirFacturacion guarda los datos y, sin derecho aún, abre el cobro en el navegador.
func (b *Bienvenida) seguirFacturacion(ctx context.Context) error {
	b.mu.Lock()
	fact, asientos, vuelto, esOrg := b.facturacion, b.asientos, b.pagoVuelto, b.esOrg()
	b.mu.Unlock()
	if err := b.guardarDatos(ctx, fact); err != nil {
		return err
	}
	actual, err := b.esperarPago(ctx, vuelto)
	if err != nil {
		return err
	}
	b.enUI(func() { b.suscripcion = &actual })
	if !actual.Entitled {
		if vuelto {
			// Al volver sin cobro confirmado, la siguiente vez se ofrece pagar otra
			// vez: es lo que en la web hace recargar la página sin `?payment=return`.
			b.enUI(func() { b.pagoVuelto = false })
			return errMensaje(b.T.T("ob.billing.pendingBody", nil))
		}
		u, err := b.Alta.Pagar(ctx, asientos)
		if err != nil {
			return err
		}
		if b.AbrirURL != nil {
			fyne.Do(func() { b.AbrirURL(u) })
		}
		b.enUI(func() { b.pagoVuelto = true })
		return nil
	}
	siguiente := api.AltaPasoListo
	if esOrg {
		siguiente = api.AltaPasoRoles
	}
	return b.guardarYSeguir(ctx, siguiente)
}

func (b *Bienvenida) guardarDatos(ctx context.Context, fact api.AltaFacturacion) error {
	b.mu.Lock()
	g := api.AltaGuardado{Mode: b.modo, Step: api.AltaPasoFacturacion, Billing: &fact}
	b.mu.Unlock()
	return b.Alta.Guardar(ctx, g)
}

func (b *Bienvenida) seguirRoles(ctx context.Context) error {
	b.mu.Lock()
	var pendientes []string
	if b.estado != nil {
		creados := map[string]bool{}
		for _, r := range b.estado.Roles {
			creados[r.Name] = true
		}
		for _, k := range b.elegidos {
			for _, r := range b.estado.RecommendedRoles {
				if r.Key == k && !creados[r.Name] {
					pendientes = append(pendientes, k)
				}
			}
		}
	}
	elegidos := slices.Clone(b.elegidos)
	b.mu.Unlock()
	if len(pendientes) > 0 {
		if err := b.Alta.CrearRoles(ctx, elegidos); err != nil {
			return err
		}
		if err := b.releer(ctx); err != nil {
			return err
		}
	}
	return b.guardarYSeguir(ctx, api.AltaPasoListo)
}

// terminar marca el alta y vuelve a entrar: el token del alta no lleva organización, y
// el nuevo sí, después de que Sytue ID compruebe el cobro.
func (b *Bienvenida) terminar(ctx context.Context) error {
	if err := b.Alta.Guardar(ctx, api.AltaGuardado{Step: api.AltaPasoListo, Completed: true}); err != nil {
		return err
	}
	if b.Reentrar != nil {
		if err := b.Reentrar(ctx); err != nil {
			return err
		}
	}
	if b.Ir != nil {
		fyne.Do(func() { b.Ir(AInicio) })
	}
	return nil
}

func (b *Bienvenida) comprobarDominio(ctx context.Context) error {
	b.mu.Lock()
	host := strings.TrimSpace(b.dominio)
	b.mu.Unlock()
	d, err := b.Alta.Dominio(ctx, host)
	if err != nil {
		return err
	}
	b.enUI(func() {
		if b.estado != nil {
			b.estado.Domain = &d
		}
		// El servidor lo devuelve normalizado: lo que se ve es lo que se comprobó.
		b.dominio = d.Host
	})
	if d.Status == "verified" {
		b.avisar(ui.AvisoExito, b.T.T("ob.domain.verified", i18n.Vars{"host": d.Host}), "")
	}
	return nil
}

// cloudflare mira (`aplicar` falso) o aplica el plan en la zona DEL CLIENTE. El token se
// borra del campo en cuanto se ha usado, salvo que se haya pedido recordarlo.
func (b *Bienvenida) cloudflare(aplicar bool) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		b.mu.Lock()
		p := api.AltaCloudflarePeticion{
			Domain: strings.TrimSpace(b.dominio), Token: strings.TrimSpace(b.cfToken), Mail: b.cfCorreo,
			Apply: aplicar, RememberToken: b.cfRecordar,
		}
		if aplicar {
			p.Replace = slices.Clone(b.cfSustituir)
		}
		recordar := b.cfRecordar
		b.mu.Unlock()
		out, err := b.Alta.Cloudflare(ctx, p)
		if err != nil {
			return err
		}
		b.enUI(func() {
			b.cfPlan = &out
			if b.estado != nil {
				d := out.Domain
				b.estado.Domain = &d
			}
			b.dominio = out.Domain.Host
			if aplicar && !recordar {
				b.cfToken = ""
			}
		})
		if !aplicar {
			return nil
		}
		fallidos := 0
		for _, r := range out.Records {
			if r.State == "failed" {
				fallidos++
			}
		}
		switch {
		case fallidos > 0:
			b.avisar(ui.AvisoAdvertencia, b.T.T("ob.cf.someFailed", i18n.Vars{"count": fallidos}), b.T.T("ob.cf.someFailedBody", nil))
		case out.Domain.Verified:
			b.avisar(ui.AvisoExito, b.T.T("ob.cf.configured", i18n.Vars{"host": out.Domain.Host}), "")
		default:
			b.avisar(ui.AvisoExito, b.T.T("ob.cf.created", nil), "")
		}
		return nil
	}
}

func (b *Bienvenida) buscarDominio(ctx context.Context) error {
	b.mu.Lock()
	q := strings.TrimSpace(b.buscado)
	b.mu.Unlock()
	if q == "" {
		return nil
	}
	b.enUI(func() { b.disponible = nil })
	d, err := b.Alta.Disponibilidad(ctx, q)
	if err != nil {
		return err
	}
	b.enUI(func() { b.disponible = &d })
	return nil
}

func (b *Bienvenida) invitar(ctx context.Context) error {
	b.mu.Lock()
	correos, rol := b.correos, b.rolInvitar
	b.mu.Unlock()
	enviadas, res, err := b.Alta.Invitar(ctx, correos, rol)
	if err != nil {
		return err
	}
	b.enUI(func() {
		b.invitadas = res
		if enviadas > 0 {
			b.correos = ""
		}
	})
	return b.releer(ctx)
}

// ---- El armazón ----

// CreateRenderer construye la pantalla. Los controles se crean UNA vez y sobreviven a
// los cambios de estado: rehacerlos le quitaría el foco y el cursor a quien escribe.
func (b *Bienvenida) CreateRenderer() fyne.WidgetRenderer {
	r := &bienvenidaRenderer{b: b, fondo: ui.NuevoFondoSuite(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))}
	r.montar()
	r.rehacer()
	return r
}

type bienvenidaRenderer struct {
	b *Bienvenida
	c controlesAlta

	fondo           *ui.FondoSuite
	titulo, entrada *ui.Parrafo
	// encabezado y descripcion son los del paso en curso (el `h2` y su frase).
	encabezado, descripcion *ui.Parrafo
	cuerpo                  bloque
	objetos                 []fyne.CanvasObject
}

func (r *bienvenidaRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, r.alto(r.b.Ancho())) }
func (r *bienvenidaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *bienvenidaRenderer) Destroy()                     {}

// Refresh rehace la pantalla (ver el comentario del lanzador).
func (r *bienvenidaRenderer) Refresh() {
	r.rehacer()
	r.Layout(r.b.Size())
	canvas.Refresh(r.b)
}

func (r *bienvenidaRenderer) rehacer() {
	b, t := r.b, r.b.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.PonerVariante(v)
	r.titulo = ui.NuevoParrafo(t.T("page.welcome", nil), ui.EscalonTitulo(2), ui.PesoTitulo(2), "text", v)
	r.entrada = ui.NuevoParrafo(t.T("ob.lead", nil), tema.TextoBase, ui.Normal, "text-muted", v)
	r.sincronizar()
	b.mu.Lock()
	r.cuerpo = r.hacerCuerpo(v)
	b.mu.Unlock()
	r.objetos = append([]fyne.CanvasObject{r.fondo, r.titulo, r.entrada}, r.cuerpo.objetos()...)
}

func (r *bienvenidaRenderer) alto(ventana float32) float32 { return r.colocar(ventana, false) }

func (r *bienvenidaRenderer) Layout(t fyne.Size) {
	if t.Width <= 0 {
		return
	}
	r.fondo.Resize(fyne.NewSize(t.Width, max(t.Height, r.alto(t.Width))))
	r.colocar(t.Width, true)
}

func (r *bienvenidaRenderer) colocar(ventana float32, mover bool) float32 {
	x, ancho := CajaBienvenida(ventana)
	margenY := MargenBienvenidaY
	if ventana >= ui.CorteSM {
		margenY = MargenBienvenidaYSM
	}
	y := ponerParrafo(r.titulo, x, margenY, ancho, mover) + MargenEntradaAlta
	y = ponerParrafo(r.entrada, x, y, ancho, mover) + MargenCabeceraAlta
	if mover {
		r.cuerpo.poner(x, y, ancho)
	}
	return y + r.cuerpo.alto(ancho) + margenY
}

// hacerCuerpo es lo de debajo de la cabecera: el estado de carga, el fallo o el
// asistente con el paso en curso. Se llama con el cerrojo tomado.
func (r *bienvenidaRenderer) hacerCuerpo(v tema.Variante) bloque {
	b, t := r.b, r.b.T
	switch {
	case b.fallo:
		vacio := ui.NuevoVacio(t.T("ob.failedTitle", nil), t.T("ob.failedBody", nil))
		vacio.Ilustracion = Glifo(iconos.Trazo(iconos.Peligro, 32, "text-muted", v), 32)
		vacio.Accion = r.c.reintentar
		return vacioB{vacio}
	case b.estado == nil:
		return &preparando{
			rueda: ui.NuevaRueda(ui.LadoRueda, "text", v),
			texto: ui.Texto(t.T("ob.preparing", nil), tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
		}
	}
	return r.asistente(v)
}

// asistente es `Wizard`: el progreso, el encabezado del paso, lo de dentro y el pie.
func (r *bienvenidaRenderer) asistente(v tema.Variante) bloque {
	b, t := r.b, r.b.T
	ids := pasosDe(b.modoEfectivo())
	actual := min(b.paso, len(ids)-1)
	p := r.paso(ids[actual], v)

	titulos := make([]string, len(ids))
	for i, id := range ids {
		titulos[i] = r.tituloPaso(id)
	}
	r.c.pasos.Titulos = titulos
	r.c.pasos.Actual, r.c.pasos.Ventana = actual, b.Ancho()
	// Ningún paso del alta es terminal (`done`): el resumen también lleva su pie, con el
	// botón que termina.
	if b.ocupado {
		r.c.pasos.AlElegir = nil
	} else {
		r.c.pasos.AlElegir = func(i int) { b.enUI(func() { b.paso = i }) }
	}
	r.c.pasos.Refresh()

	r.encabezado = ui.NuevoParrafo(p.encabezado, ui.EscalonTitulo(3), ui.PesoTitulo(3), "text", v)
	cabeza := col(MargenDescripcionPaso, parrafo{r.encabezado})
	r.descripcion = nil
	if p.descripcion != "" {
		r.descripcion = ui.NuevoParrafo(p.descripcion, tema.TextoBase, ui.Normal, "text-muted", v)
		cabeza.hijos = append(cabeza.hijos, tope{AnchoIntro, parrafo{r.descripcion}})
	}
	r.c.siguiente.Texto = p.siguiente
	if p.siguiente == "" {
		r.c.siguiente.Texto = t.T("ui.wizard.next", nil)
	}
	r.c.siguiente.Inerte = !p.puede
	r.c.siguiente.Cargando = b.ocupado
	r.c.siguiente.Refresh()
	r.c.atras.Inerte = b.ocupado
	r.c.atras.Refresh()
	var botones []fyne.CanvasObject
	if actual > 0 {
		botones = append(botones, r.c.atras)
	}
	pie := &pieAsistente{
		filete:  canvas.NewRectangle(tema.Color("border-subtle", v)),
		botones: append(botones, r.c.siguiente),
	}
	partes := []bloque{fijoAncho{r.c.pasos, ui.AltoPasos}, cabeza, p.cuerpo, pie}
	return col(HuecoAsistente, partes...)
}

// ---- Piezas del armazón ----

// fijoAncho es un objeto a todo el ancho y con su alto propio (el progreso).
type fijoAncho struct {
	o fyne.CanvasObject
	h float32
}

func (f fijoAncho) alto(float32) float32         { return f.h }
func (f fijoAncho) poner(x, y, ancho float32)    { ponerObjeto(f.o, x, y, fyne.NewSize(ancho, f.h), true) }
func (f fijoAncho) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{f.o} }

// pieAsistente es la barra de acciones: `border-t pt-4` y los botones a la derecha.
type pieAsistente struct {
	filete  *canvas.Rectangle
	botones []fyne.CanvasObject
}

func (p *pieAsistente) alto(float32) float32 {
	var h float32
	for _, o := range p.botones {
		h = max(h, o.MinSize().Height)
	}
	return tema.Borde + MargenPieAsistente + h
}

func (p *pieAsistente) poner(x, y, ancho float32) {
	ponerObjeto(p.filete, x, y, fyne.NewSize(ancho, tema.Borde), true)
	y += tema.Borde + MargenPieAsistente
	derecha := x + ancho
	for i := len(p.botones) - 1; i >= 0; i-- {
		s := p.botones[i].MinSize()
		derecha -= s.Width
		ponerObjeto(p.botones[i], derecha, y, s, true)
		derecha -= HuecoBotonesPie
	}
}

func (p *pieAsistente) objetos() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{p.filete}, p.botones...)
}

// preparando es el `Spinner` con «Preparando tu alta…», centrado con `py-16 gap-3`.
type preparando struct {
	rueda *ui.Rueda
	texto *canvas.Text
}

const margenPreparando float32 = 16 * 0.25 * tema.Rem

func (p *preparando) alto(float32) float32 {
	return 2*margenPreparando + max(ui.LadoRueda, tema.TextoSM.Linea)
}

func (p *preparando) poner(x, y, ancho float32) {
	w := ui.LadoRueda + HuecoOpcionesAsistente + ui.Medir(p.texto.Text, tema.TextoSM, ui.Normal).Width
	h := max(ui.LadoRueda, tema.TextoSM.Linea)
	cx, cy := x+(ancho-w)/2, y+margenPreparando
	ponerObjeto(p.rueda, cx, cy+(h-ui.LadoRueda)/2, fyne.NewSize(ui.LadoRueda, ui.LadoRueda), true)
	ui.ColocarTexto(p.texto, tema.TextoSM, cx+ui.LadoRueda+HuecoOpcionesAsistente, cy+(h-tema.TextoSM.Linea)/2)
}

func (p *preparando) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{p.rueda, p.texto} }

// cantidad lee un número de un campo; lo que no es número deja el valor de antes, como
// el `Number.isFinite` de la web.
func cantidad(s string, antes int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return antes
	}
	return min(10000, max(1, n))
}
