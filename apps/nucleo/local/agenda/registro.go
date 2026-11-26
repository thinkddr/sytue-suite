// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
)

// Proveedores de una cuenta de calendario y contactos.
const (
	ProveedorDAV       = "caldav" // cualquier servidor CalDAV/CardDAV (Nextcloud, Radicale…)
	ProveedorGoogle    = "google"
	ProveedorMicrosoft = "microsoft"
	// Y los preajustes de dav.Preajustes: icloud, fastmail, sytue, yahoo.
)

// Estados de una cuenta (los de `local/cuentas`).
const (
	EstadoOK         = ""
	EstadoReconectar = "reconectar"
)

// Cuenta es una cuenta de calendario y contactos, sin secretos. Se guarda en
// `<Ajustes>/agenda.json`, que comparten Calendar y Contacts (una cuenta de iCloud da
// los dos) y que lee el registro común de cuentas (`local/cuentas`) para enseñarlas en
// Ajustes → Cuentas y abrir sus servidores al portero.
type Cuenta struct {
	ID        string     `json:"id"`
	Proveedor string     `json:"proveedor"`
	Rotulo    string     `json:"rotulo"`
	Usuario   string     `json:"usuario,omitempty"`
	Servicios []Servicio `json:"servicios"`
	// Calendario y Contactos son las URL de contexto DAV.
	Calendario string `json:"calendario,omitempty"`
	Contactos  string `json:"contactos,omitempty"`
	// Servidores son los hosts con los que habla la cuenta, por servicio: los de sus
	// URL y los que el servidor dio al probarla (iCloud reparte en `pNN-caldav…`).
	Servidores []Servidor `json:"servidores,omitempty"`
	// OAuth es el servidor de tokens (Google, Microsoft); ClienteID, el identificador
	// de cliente con que se conectó.
	OAuth     string `json:"oauth,omitempty"`
	ClienteID string `json:"cliente_id,omitempty"`
	Estado    string `json:"estado,omitempty"`
}

// Servidor es un host de una cuenta para un servicio.
type Servidor struct {
	Servicio Servicio `json:"servicio"`
	Host     string   `json:"host"`
	Puerto   int      `json:"puerto,omitempty"`
}

// Da dice si la cuenta da un servicio.
func (c Cuenta) Da(s Servicio) bool { return slices.Contains(c.Servicios, s) }

func (c Cuenta) hosts(s Servicio) []string {
	var hs []string
	for _, v := range c.Servidores {
		if v.Servicio == s {
			h := v.Host
			if v.Puerto != 0 {
				h = net.JoinHostPort(v.Host, fmt.Sprint(v.Puerto))
			}
			hs = append(hs, h)
		}
	}
	return hs
}

// Proposito es el del portero para un servicio.
func Proposito(s Servicio) red.Proposito {
	if s == Contactos {
		return red.Contactos
	}
	return red.Calendario
}

// Registro son las cuentas de calendario y contactos de la edición.
type Registro struct {
	ruta     string
	secretos *secretos.Almacen
	red      *red.Portero
	// Clientes OAuth de la compilación (los de `local/nube`).
	Clientes nube.ClientesOAuth
	// Resolutor es el DNS del autodescubrimiento; nil, el del portero
	// (`red.Portero.Resolvedor`), y sin portero no se miran los SRV.
	Resolutor dav.Resolutor
	// HTTP da el cliente de red de un propósito; nil, el portero (las pruebas ponen el
	// de su servidor).
	HTTP func(red.Proposito) *http.Client
	// MarcaCopia es lo que se pone en el nombre de una copia en conflicto, en el idioma
	// de la app.
	MarcaCopia string
	// Correo, si no es nil, manda por correo las invitaciones de los calendarios cuyo
	// servidor no las manda (iMIP, A-11), con los textos de TextosIMIP. Se pone antes de
	// la primera sincronización: los sincronizadores lo toman al crearse.
	Correo     Correo
	TextosIMIP TextosIMIP
	// Cola, si no es nil, guarda las invitaciones por correo que no salieron para
	// reintentarlas ([ColaIMIP]); como Correo, antes de la primera sincronización.
	Cola *ColaIMIP

	mu      sync.Mutex
	cuentas []Cuenta
	firma   time.Time
	sincs   map[string]*Sincronizador
}

// Abrir lee las cuentas de `<Ajustes>/agenda.json`.
func Abrir(e *espacio.Espacio, s *secretos.Almacen, p *red.Portero) *Registro {
	return AbrirEn(e.Ajuste("agenda.json"), s, p)
}

// AbrirEn es Abrir con la ruta del fichero.
func AbrirEn(ruta string, s *secretos.Almacen, p *red.Portero) *Registro {
	r := &Registro{ruta: ruta, secretos: s, red: p, Clientes: nube.ClientesCompilados(), sincs: map[string]*Sincronizador{}}
	r.Cuentas()
	return r
}

type guardadas struct {
	Cuentas []Cuenta `json:"cuentas"`
}

// Cuentas son las cuentas. Si otra app cambió el fichero (Calendar dio de alta una que
// también es de Contacts), se vuelve a leer.
func (r *Registro) Cuentas() []Cuenta {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.releer()
	return slices.Clone(r.cuentas)
}

func (r *Registro) releer() {
	var firma time.Time
	if fi, err := os.Stat(r.ruta); err == nil {
		firma = fi.ModTime()
	}
	if !firma.IsZero() && firma.Equal(r.firma) {
		return
	}
	var g guardadas
	_ = espacio.LeerJSON(r.ruta, &g)
	r.cuentas, r.firma = g.Cuentas, firma
}

func (r *Registro) guardar() error {
	if err := espacio.EscribirJSON(r.ruta, guardadas{r.cuentas}); err != nil {
		return err
	}
	if fi, err := os.Stat(r.ruta); err == nil {
		r.firma = fi.ModTime()
	}
	return nil
}

func secretoDe(id, clase string) string { return "cuenta-" + id + "-" + clase }

func (r *Registro) cliente(pr red.Proposito) *http.Client {
	if r.HTTP != nil {
		return r.HTTP(pr)
	}
	return &http.Client{Transport: r.red.Transporte(pr), Timeout: 90 * time.Second}
}

// ----- Alta -----

// Alta es lo que la persona escribe para dar de alta una cuenta DAV.
type Alta struct {
	// Proveedor: ProveedorDAV o uno de dav.Preajustes («icloud»…).
	Proveedor string
	Rotulo    string
	// Usuario es el de la cuenta (en iCloud y Fastmail, el correo).
	Usuario string
	// Clave es la contraseña de aplicación.
	Clave string
	// Calendario y Contactos son las URL; con ProveedorDAV y vacías, se descubren con
	// el dominio del usuario (si es un correo) o de Servidor.
	Calendario, Contactos string
	// Servidor es la dirección que la persona escribe («nube.example.org»), si no da las
	// dos URL.
	Servidor string
}

// Descubrir busca los servidores CalDAV y CardDAV del dominio de un correo (o de una
// dirección): SRV y `.well-known` (RFC 6764). Es red hacia ese dominio que la persona
// pide; el portero la deja pasar solo mientras dura, y el DNS va por su resolvedor (con
// «Sin conexión», no se pregunta).
func (r *Registro) Descubrir(ctx context.Context, correo string) (dav.Descubierto, error) {
	res := r.Resolutor
	if res == nil && r.red != nil {
		res = r.red.Resolvedor(red.Descubrimiento, "descubrimiento")
	}
	hc := r.cliente(red.Descubrimiento)
	if r.HTTP == nil && r.red != nil {
		// Cada host al que lleve el descubrimiento (el del dominio, el del SRV, el de la
		// redirección) se abre solo para esa petición.
		base := r.red.Transporte(red.Descubrimiento)
		hc = &http.Client{Timeout: 30 * time.Second, Transport: abrirPorPeticion{p: r.red, base: base}}
	}
	return dav.Descubrir(ctx, correo, res, hc)
}

type abrirPorPeticion struct {
	p    *red.Portero
	base http.RoundTripper
}

func (a abrirPorPeticion) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("%w: el descubrimiento solo va por https", red.ErrDenegado)
	}
	cerrar := a.p.Permitir(red.Destino{Proposito: red.Descubrimiento, Host: req.URL.Hostname(), Motivo: "descubrimiento"})
	defer cerrar()
	return a.base.RoundTrip(req)
}

// ErrSinServicio: la cuenta no dio ni calendario ni contactos.
var ErrSinServicio = errors.New("agenda: la cuenta no tiene calendario ni contactos")

// Probar comprueba una cuenta DAV sin guardarla: entra con la contraseña en cada URL,
// apunta los servidores a los que manda y dice qué servicios da. Abre al portero los
// servidores de la cuenta solo mientras prueba.
func (r *Registro) Probar(ctx context.Context, a Alta) (Cuenta, error) {
	c := Cuenta{Proveedor: cmp.Or(a.Proveedor, ProveedorDAV), Rotulo: strings.TrimSpace(a.Rotulo), Usuario: strings.TrimSpace(a.Usuario)}
	var extra []string
	if p, ok := dav.PreajusteDe(c.Proveedor); ok {
		a.Calendario, a.Contactos = cmp.Or(a.Calendario, p.Calendario), cmp.Or(a.Contactos, p.Contactos)
		extra = p.Hosts
		c.Rotulo = cmp.Or(c.Rotulo, p.Rotulo)
	} else if c.Proveedor != ProveedorDAV {
		return c, fmt.Errorf("agenda: proveedor %q desconocido", c.Proveedor)
	}
	if a.Calendario == "" && a.Contactos == "" {
		dominio := cmp.Or(strings.TrimSpace(a.Servidor), c.Usuario)
		if u, err := url.Parse(dominio); err == nil && u.Host != "" {
			dominio = u.Host
		}
		d, err := r.Descubrir(ctx, dominio)
		if err != nil {
			return c, err
		}
		a.Calendario, a.Contactos = d.Calendario, d.Contactos
	}
	c.Rotulo = cmp.Or(c.Rotulo, c.Usuario, hostDe(a.Calendario), hostDe(a.Contactos))
	var ultimo error
	for _, sv := range []struct {
		s   Servicio
		dir string
	}{{Calendario, a.Calendario}, {Contactos, a.Contactos}} {
		if sv.dir == "" {
			continue
		}
		hosts, err := r.probarUno(ctx, sv.s, sv.dir, a.Usuario, a.Clave, extra)
		if err != nil {
			ultimo = err
			continue
		}
		c.Servicios = append(c.Servicios, sv.s)
		if sv.s == Calendario {
			c.Calendario = sv.dir
		} else {
			c.Contactos = sv.dir
		}
		for _, h := range hosts {
			sr := Servidor{Servicio: sv.s, Host: h}
			if host, puerto, err := net.SplitHostPort(h); err == nil {
				sr.Host = host
				_, _ = fmt.Sscan(puerto, &sr.Puerto)
			}
			c.Servidores = append(c.Servidores, sr)
		}
	}
	if len(c.Servicios) == 0 {
		return c, cmp.Or(ultimo, ErrSinServicio)
	}
	return c, nil
}

func hostDe(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return p.Hostname()
}

// probarUno entra en una URL y devuelve los hosts que usa la cuenta para ese servicio.
func (r *Registro) probarUno(ctx context.Context, s Servicio, direccion, usuario, clave string, extra []string) ([]string, error) {
	pr := Proposito(s)
	u, err := url.Parse(direccion)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("agenda: dirección no válida %q", direccion)
	}
	if r.red != nil && r.HTTP == nil {
		ds := []red.Destino{{Proposito: pr, Host: u.Hostname(), Motivo: "alta"}}
		for _, h := range extra {
			ds = append(ds, red.Destino{Proposito: pr, Host: h, Motivo: "alta"})
		}
		defer r.red.Permitir(ds...)()
	}
	cli, err := dav.Nuevo(dav.Cuenta{Direccion: direccion, Usuario: usuario, Clave: clave, Hosts: extra}, r.cliente(pr))
	if err != nil {
		return nil, err
	}
	principal, cal, lib, err := cli.Principal(ctx)
	if err != nil {
		return nil, err
	}
	casa := cal
	if s == Contactos {
		casa = lib
	}
	if casa == "" {
		return nil, fmt.Errorf("agenda: %s no tiene %s", u.Hostname(), s)
	}
	hosts := []string{strings.ToLower(u.Host)}
	for _, x := range []string{principal, casa} {
		if p, err := url.Parse(x); err == nil && p.Host != "" && !slices.Contains(hosts, strings.ToLower(p.Host)) {
			hosts = append(hosts, strings.ToLower(p.Host))
		}
	}
	return hosts, nil
}

// AnadirDAV prueba y guarda una cuenta DAV con su contraseña de aplicación (al
// llavero). Es la función de alta que usa Ajustes → Cuentas.
func (r *Registro) AnadirDAV(ctx context.Context, a Alta) (Cuenta, error) {
	c, err := r.Probar(ctx, a)
	if err != nil {
		return c, err
	}
	c.ID = uuid.NewString()
	if err := r.secretos.Guardar(secretoDe(c.ID, "contrasena"), a.Clave); err != nil {
		return c, err
	}
	return c, r.anadir(c)
}

func (r *Registro) anadir(c Cuenta) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.releer()
	r.cuentas = append(r.cuentas, c)
	return r.guardar()
}

// Quitar borra la cuenta y su secreto. Los calendarios y las libretas de aquí se
// quedan con lo que tienen, como calendarios y libretas del equipo.
func (r *Registro) Quitar(id string) error {
	_ = r.secretos.Borrar(secretoDe(id, "contrasena"))
	_ = r.secretos.Borrar(secretoDe(id, "refresco"))
	r.mu.Lock()
	defer r.mu.Unlock()
	r.releer()
	r.cuentas = slices.DeleteFunc(r.cuentas, func(c Cuenta) bool { return c.ID == id })
	for k := range r.sincs {
		if strings.HasPrefix(k, id+"/") {
			delete(r.sincs, k)
		}
	}
	return r.guardar()
}

// PonerEstado marca una cuenta (EstadoReconectar si el servidor rechazó la contraseña).
func (r *Registro) PonerEstado(id, estado string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.releer()
	for i := range r.cuentas {
		if r.cuentas[i].ID == id && r.cuentas[i].Estado != estado {
			r.cuentas[i].Estado = estado
			_ = r.guardar()
		}
	}
}

// ErrSinCuenta: la cuenta no existe o le falta su secreto.
var ErrSinCuenta = errors.New("agenda: cuenta no configurada")

// Remoto abre el proveedor de una cuenta para un servicio.
func (r *Registro) Remoto(c Cuenta, s Servicio) (Remoto, error) {
	if !c.Da(s) {
		return nil, ErrSinCuenta
	}
	switch c.Proveedor {
	case ProveedorGoogle, ProveedorMicrosoft:
		return r.remotoOAuth(c, s)
	}
	clave, err := r.secretos.Leer(secretoDe(c.ID, "contrasena"))
	if err != nil {
		return nil, err
	}
	dir, clase := c.Calendario, dav.Calendario
	if s == Contactos {
		dir, clase = c.Contactos, dav.Libreta
	}
	cli, err := dav.Nuevo(dav.Cuenta{Direccion: dir, Usuario: c.Usuario, Clave: clave, Hosts: c.hosts(s)}, r.cliente(Proposito(s)))
	if err != nil {
		return nil, err
	}
	return &RemotoDAV{Cliente: cli, Clase: clase}, nil
}

// Sincronizadores son los de las cuentas que dan el servicio, con su estado en dir
// (`<Datos>/<app>/agenda`). Se reutilizan entre llamadas: cada uno lleva su cerrojo.
func (r *Registro) Sincronizadores(s Servicio, local Local, dir, marca string) []*Sincronizador {
	var out []*Sincronizador
	for _, c := range r.Cuentas() {
		if !c.Da(s) {
			continue
		}
		clave := c.ID + "/" + string(s)
		r.mu.Lock()
		sc := r.sincs[clave]
		r.mu.Unlock()
		if sc == nil {
			rem, err := r.Remoto(c, s)
			if err != nil {
				continue
			}
			sc = &Sincronizador{Remoto: rem, Local: local, Estado: filepath.Join(dir, c.ID+".json"), MarcaCopia: cmp.Or(marca, r.MarcaCopia)}
			if s == Calendario {
				sc.Ventana = VentanaCalendario
				if r.Correo != nil {
					sc.Invitar = (&Cartero{
						Correo: r.Correo, Textos: r.TextosIMIP, Propia: strings.ToLower(strings.TrimSpace(c.Usuario)),
						Direccion: sc.Direccion, Cola: r.Cola,
					}).Invitar
				}
			}
			r.mu.Lock()
			r.sincs[clave] = sc
			r.mu.Unlock()
		}
		out = append(out, sc)
	}
	return out
}

// Grupo es una cuenta y sus colecciones de aquí, para agrupar la lista (A-12, K-06).
type Grupo struct {
	Cuenta      Cuenta
	Colecciones []ColeccionLocal
}

// Grupos son las cuentas que dan el servicio con sus colecciones de aquí (sin red).
func (r *Registro) Grupos(s Servicio, local Local, dir string) []Grupo {
	var out []Grupo
	cuentas := r.Cuentas()
	sincs := r.Sincronizadores(s, local, dir, "")
	for _, c := range cuentas {
		if !c.Da(s) {
			continue
		}
		g := Grupo{Cuenta: c}
		for _, sc := range sincs {
			if sc.Estado == filepath.Join(dir, c.ID+".json") {
				g.Colecciones = sc.Colecciones()
			}
		}
		out = append(out, g)
	}
	return out
}

// SincronizarTodo sincroniza todas las cuentas del servicio, una tras otra. Una cuenta
// que no deja entrar queda en EstadoReconectar (Ajustes → Cuentas lo dice) y otra que
// entra vuelve a EstadoOK.
func (r *Registro) SincronizarTodo(ctx context.Context, s Servicio, local Local, dir, marca string) ([]Resultado, []error) {
	cuentas := map[string]Cuenta{}
	for _, c := range r.Cuentas() {
		cuentas[c.ID] = c
	}
	scs := r.Sincronizadores(s, local, dir, marca)
	res := make([]Resultado, 0, len(scs))
	var errs []error
	for _, sc := range scs {
		id := strings.TrimSuffix(filepath.Base(sc.Estado), ".json")
		x, err := sc.Sincronizar(ctx)
		res = append(res, x)
		switch {
		case errors.Is(err, ErrAcceso):
			r.PonerEstado(id, EstadoReconectar)
		case err == nil && cuentas[id].Estado == EstadoReconectar:
			r.PonerEstado(id, EstadoOK)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", cuentas[id].Rotulo, err))
		}
	}
	return res, errs
}

// PuedeCrear dice si en la cuenta se puede crear un calendario (CalDAV con MKCALENDAR,
// Google, Microsoft) o una libreta (CardDAV con el MKCOL extendido, Microsoft con una
// carpeta de contactos) desde aquí, sin preguntar a la red. Google no tiene más que una
// libreta de contactos: sus etiquetas son las de esa libreta.
func (r *Registro) PuedeCrear(c Cuenta, s Servicio) bool {
	if !c.Da(s) {
		return false
	}
	// Todo servidor CalDAV conoce MKCALENDAR (RFC 4791 §5.3.1) y todo CardDAV, el MKCOL
	// extendido (RFC 6352 §6.3.1); el que no lo admita lo dirá al crear.
	return s == Calendario || c.Proveedor != ProveedorGoogle
}

// CrearCalendario crea un calendario en la cuenta (MKCALENDAR en CalDAV; la API en
// Google y en Microsoft). Aparece aquí con la siguiente sincronización.
func (r *Registro) CrearCalendario(ctx context.Context, cuenta, nombre, color string) error {
	return r.crearColeccion(ctx, cuenta, Calendario, nombre, color)
}

// CrearLibreta crea una libreta en la cuenta (el MKCOL extendido en CardDAV; una carpeta
// de contactos en Microsoft). Aparece aquí con la siguiente sincronización.
func (r *Registro) CrearLibreta(ctx context.Context, cuenta, nombre string) error {
	return r.crearColeccion(ctx, cuenta, Contactos, nombre, "")
}

func (r *Registro) crearColeccion(ctx context.Context, cuenta string, s Servicio, nombre, color string) error {
	c, ok := r.cuenta(cuenta)
	if !ok || !r.PuedeCrear(c, s) {
		return ErrSinCuenta
	}
	rem, err := r.Remoto(c, s)
	if err != nil {
		return err
	}
	cc, ok := rem.(CreaColecciones)
	if !ok {
		return ErrNoSoportado
	}
	_, err = cc.CrearColeccion(ctx, nombre, color)
	return err
}

func (r *Registro) cuenta(id string) (Cuenta, bool) {
	for _, x := range r.Cuentas() {
		if x.ID == id {
			return x, true
		}
	}
	return Cuenta{}, false
}

// sincronizador es el que ya está abierto de una cuenta y un servicio, o nil.
func (r *Registro) sincronizador(cuenta string, s Servicio) *Sincronizador {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sincs[cuenta+"/"+string(s)]
}

// Direccion es la dirección de correo de la persona en una cuenta de calendario: la que
// dio el proveedor en la última sincronización o, si aún no, el usuario de la cuenta si
// es un correo. Sin red. "" si no se sabe.
func (r *Registro) Direccion(cuenta string) string {
	if sc := r.sincronizador(cuenta, Calendario); sc != nil {
		if d := sc.Direccion(); d != "" {
			return d
		}
	}
	if c, ok := r.cuenta(cuenta); ok {
		if u := strings.ToLower(strings.TrimSpace(c.Usuario)); strings.Contains(u, "@") {
			return u
		}
	}
	return ""
}

// VaciarIMIP manda lo que toca de la cola de invitaciones por correo (todo, al
// arrancar o cuando la red vuelve). Sin cola o sin correo, nada.
func (r *Registro) VaciarIMIP(ctx context.Context, todo bool) InformeCola {
	if r.Cola == nil || r.Correo == nil {
		return InformeCola{}
	}
	return r.Cola.Vaciar(ctx, r.Correo, todo)
}
