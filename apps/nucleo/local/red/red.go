// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package red es el portero de red de Sytue Office (PLAN-EDICION-LOCAL §5.8, C-09): la
// única puerta por la que una app de la edición sale a la red.
//
//   - La lista blanca **se deriva de la configuración**: cada [Fuente] (las cuentas de
//     nube, la IA elegida, las cuentas de correo y de chat) dice a qué destinos se puede
//     ir y por qué. No hay ningún host fijo que valga sin configurar nada: recién
//     instalada, la lista está vacía y no sale ni una conexión (R-RED).
//   - **Solo TLS**, salvo `localhost` (un modelo de IA en el equipo) y lo que la persona
//     marque a mano en una cuenta (un servidor IMAP de casa: [Destino.Claro]).
//   - El interruptor **«Sin conexión»** ([Portero.PonerSinConexion]) lo corta todo menos
//     este mismo equipo.
//   - [Denegar] es el transporte que la cáscara instala como `http.DefaultTransport` y
//     `api.Transporte`: cualquier cliente heredado que no pase por aquí falla cerrado.
//   - Las consultas DNS del alta de una cuenta (los SRV y TXT de su dominio) también
//     pasan por aquí: [Portero.Resolvedor] (dns.go).
//
// Los permisos de un alta (la prueba de una cuenta antes de guardarla, el intercambio
// OAuth) son temporales: [Portero.Permitir] los abre y la función que devuelve los
// cierra.
package red

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// Proposito es para qué se sale a la red (Anexo B del plan).
type Proposito string

// Los propósitos de la edición.
const (
	Correo         Proposito = "correo"
	Calendario     Proposito = "calendario"
	Contactos      Proposito = "contactos"
	Chat           Proposito = "chat"
	IA             Proposito = "ia"
	Nube           Proposito = "nube"
	OAuth          Proposito = "oauth"
	SelloDeTiempo  Proposito = "sello-de-tiempo"
	SuscripcionICS Proposito = "suscripcion-ics"
	ImagenRemota   Proposito = "imagen-remota"
	BajaDeLista    Proposito = "baja-de-lista"
	AEAT           Proposito = "aeat"
	Descubrimiento Proposito = "descubrimiento"
	// VIES es comprobar un NIF-IVA europeo en el servicio de la Comisión Europea desde
	// el ERP: solo con [Portero.Puntual], mientras dura la consulta que pide la persona.
	VIES Proposito = "vies"
)

// Destino es un sitio al que se puede ir, y por qué.
type Destino struct {
	Proposito Proposito `json:"proposito"`
	// Host es el nombre exacto, en minúsculas. Con un punto delante («.1drv.com») vale
	// para sus subdominios (ver casa); eso solo lo ponen los proveedores, nunca la persona.
	Host string `json:"host"`
	// Puerto; 0 vale cualquiera (en HTTP, el de la dirección).
	Puerto int `json:"puerto,omitempty"`
	// Claro admite la conexión sin TLS. Solo lo pone la persona a mano en una cuenta, o
	// el propio equipo (localhost), que lo es siempre.
	Claro bool `json:"claro,omitempty"`
	// Motivo es lo que enseña Ajustes → Red: «Cuenta de nube Nextcloud — ana».
	Motivo string `json:"motivo,omitempty"`
	// Temporal: abierto por un alta en curso ([Portero.Permitir]).
	Temporal bool `json:"temporal,omitempty"`
}

// Fuente da los destinos que permite una parte de la configuración. Se le pregunta en
// cada conexión, así que lo que cambia en Ajustes vale en el acto.
type Fuente func() []Destino

// ErrDenegado es una conexión que el portero no deja salir.
var ErrDenegado = errors.New("red: conexión no permitida en Sytue Office")

// Portero decide y abre las conexiones.
type Portero struct {
	ruta string

	mu          sync.Mutex
	fuentes     []Fuente
	temporales  map[int]Destino
	siguiente   int
	sinConexion bool

	// marcar abre la conexión TCP; las pruebas lo sustituyen.
	marcar func(ctx context.Context, red, direccion string) (net.Conn, error)
	// marcarPublica es el de [Portero.Puntual]: solo a direcciones públicas de Internet.
	marcarPublica func(ctx context.Context, red, direccion string) (net.Conn, error)
	// base es el transporte real, capturado antes de que la cáscara instale [Denegar].
	base *http.Transport
	// raicesPuntual son las autoridades en que confía el permiso puntual en vez de las
	// del sistema: solo las pone [Portero.PuntualDePrueba].
	raicesPuntual *x509.CertPool
	// dns es el resolvedor de [Portero.Resolvedor]: el del sistema; las pruebas lo
	// sustituyen.
	dns consultasDNS
}

// transporteReal es el `http.DefaultTransport` de Go, guardado al cargar el paquete:
// cuando la cáscara lo sustituye por [Denegar], el portero sigue teniendo el de verdad.
var transporteReal = func() *http.Transport {
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		return t.Clone()
	}
	return &http.Transport{Proxy: http.ProxyFromEnvironment, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second}
}()

// Abrir lee el estado del interruptor de `ruta` (`<Ajustes>/red.json`; vacía, solo en
// memoria).
func Abrir(ruta string) *Portero {
	p := &Portero{ruta: ruta, temporales: map[int]Destino{}, base: transporteReal.Clone()}
	var guardado struct {
		SinConexion bool `json:"sin_conexion"`
	}
	if ruta != "" {
		_ = espacio.LeerJSON(ruta, &guardado)
	}
	p.sinConexion = guardado.SinConexion
	var d net.Dialer
	p.marcar = d.DialContext
	publico := net.Dialer{Timeout: 15 * time.Second, Control: func(_, direccion string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(direccion)
		if err != nil || !Publica(net.ParseIP(host)) {
			return fmt.Errorf("%w: %s no es una dirección pública", ErrDenegado, host)
		}
		return nil
	}}
	p.marcarPublica = publico.DialContext
	p.base.ResponseHeaderTimeout = 2 * time.Minute
	p.dns = net.DefaultResolver
	return p
}

// Fuente añade una parte de la configuración a la lista blanca.
func (p *Portero) Fuente(f Fuente) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fuentes = append(p.fuentes, f)
}

// Permitir abre destinos mientras dura un alta (probar una cuenta antes de guardarla,
// el intercambio OAuth). La función que devuelve los cierra; llamarla dos veces no hace
// nada.
func (p *Portero) Permitir(ds ...Destino) (cerrar func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var ids []int
	for _, d := range ds {
		d = normalizar(d)
		if d.Host == "" {
			continue
		}
		d.Temporal = true
		p.siguiente++
		p.temporales[p.siguiente] = d
		ids = append(ids, p.siguiente)
	}
	var una sync.Once
	return func() {
		una.Do(func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			for _, id := range ids {
				delete(p.temporales, id)
			}
		})
	}
}

// PermitirURL es [Portero.Permitir] para la dirección de un servidor que la persona
// acaba de escribir («https://matrix.example.org»).
func (p *Portero) PermitirURL(pr Proposito, direccion, motivo string) (cerrar func()) {
	d, err := DestinoDeURL(pr, direccion, motivo)
	if err != nil {
		return func() {}
	}
	return p.Permitir(d)
}

// DestinoDeURL es el destino de una dirección http(s). Sin esquema se entiende https.
// Una dirección http solo da un destino si es de este equipo (localhost).
func DestinoDeURL(pr Proposito, direccion, motivo string) (Destino, error) {
	direccion = strings.TrimSpace(direccion)
	if !strings.Contains(direccion, "://") {
		direccion = "https://" + direccion
	}
	u, err := url.Parse(direccion)
	if err != nil || u.Hostname() == "" || strings.HasPrefix(u.Hostname(), ".") {
		return Destino{}, fmt.Errorf("red: dirección no válida %q", direccion)
	}
	d := Destino{Proposito: pr, Host: u.Hostname(), Motivo: motivo}
	if puerto := u.Port(); puerto != "" {
		d.Puerto, _ = strconv.Atoi(puerto)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !EsLocal(d.Host) {
			return Destino{}, fmt.Errorf("red: %s no es https", d.Host)
		}
		d.Claro = true
	default:
		return Destino{}, fmt.Errorf("red: esquema %q no admitido", u.Scheme)
	}
	return normalizar(d), nil
}

func normalizar(d Destino) Destino {
	d.Host = strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(d.Host), "[]"), "."))
	if EsLocal(d.Host) {
		d.Claro = true
	}
	return d
}

// EsLocal dice si host es este mismo equipo.
func EsLocal(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// PonerSinConexion enciende o apaga «Sin conexión», y lo recuerda.
func (p *Portero) PonerSinConexion(si bool) error {
	p.mu.Lock()
	p.sinConexion = si
	ruta := p.ruta
	p.mu.Unlock()
	if ruta == "" {
		return nil
	}
	return espacio.EscribirJSON(ruta, struct {
		SinConexion bool `json:"sin_conexion"`
	}{si})
}

// SinConexion dice si el interruptor está encendido.
func (p *Portero) SinConexion() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sinConexion
}

// Destinos son todos los sitios a los que se puede ir ahora mismo y por qué: lo que
// enseña Ajustes → Red. Sin repetidos y en orden (propósito, host, puerto).
func (p *Portero) Destinos() []Destino {
	p.mu.Lock()
	fuentes := slices.Clone(p.fuentes)
	var ds []Destino
	for _, d := range p.temporales {
		ds = append(ds, d)
	}
	p.mu.Unlock()
	for _, f := range fuentes {
		for _, d := range f() {
			if d = normalizar(d); d.Host != "" {
				ds = append(ds, d)
			}
		}
	}
	slices.SortFunc(ds, func(a, b Destino) int {
		return cmp.Or(cmp.Compare(a.Proposito, b.Proposito), cmp.Compare(a.Host, b.Host), cmp.Compare(a.Puerto, b.Puerto), cmp.Compare(a.Motivo, b.Motivo))
	})
	return slices.CompactFunc(ds, func(a, b Destino) bool {
		return a.Proposito == b.Proposito && a.Host == b.Host && a.Puerto == b.Puerto && a.Claro == b.Claro
	})
}

// Permitido dice si se puede abrir una conexión a host:puerto para pr; conTLS es si va
// cifrada (https, IMAP con TLS…). nil es que sí.
func (p *Portero) Permitido(pr Proposito, host string, puerto int, conTLS bool) error {
	host = normalizar(Destino{Host: host}).Host
	local := EsLocal(host)
	if p.SinConexion() && !local {
		return fmt.Errorf("%w: «Sin conexión» está encendido (%s)", ErrDenegado, host)
	}
	for _, d := range p.Destinos() {
		if d.Proposito != pr || !casa(d.Host, host) || (d.Puerto != 0 && d.Puerto != puerto) {
			continue
		}
		if !conTLS && !d.Claro {
			return fmt.Errorf("%w: %s solo con TLS", ErrDenegado, host)
		}
		return nil
	}
	return fmt.Errorf("%w: %s (%s) no está configurado", ErrDenegado, host, pr)
}

// casa dice si el host de un destino vale para host. Un destino que empieza por punto
// («.sharepoint.com») vale para sus subdominios: es solo para los servidores de descarga
// de un proveedor que la persona conectó (OneDrive redirige cada descarga a uno
// distinto), nunca para lo que escribe la persona, que es siempre un nombre exacto.
func casa(destino, host string) bool {
	if strings.HasPrefix(destino, ".") {
		return strings.HasSuffix(host, destino) && len(host) > len(destino)
	}
	return destino == host
}

// Cliente es el HTTP(S) para pr: cada petición —también cada redirección— pasa por
// [Portero.Permitido]. Sin plazo total (una descarga larga), pero con plazo para que
// empiece la respuesta.
func (p *Portero) Cliente(pr Proposito) *http.Client {
	return &http.Client{Transport: p.Transporte(pr)}
}

// Transporte es el http.RoundTripper de [Portero.Cliente].
func (p *Portero) Transporte(pr Proposito) http.RoundTripper {
	t := p.base.Clone()
	t.DialContext = p.marcar
	return transporte{p: p, pr: pr, base: t}
}

type transporte struct {
	p    *Portero
	pr   Proposito
	base http.RoundTripper
}

func (t transporte) RoundTrip(r *http.Request) (*http.Response, error) {
	avisarPeticion(t.pr, r)
	puerto := 443
	conTLS := r.URL.Scheme == "https"
	if !conTLS {
		puerto = 80
		if r.URL.Scheme != "http" {
			return nil, fmt.Errorf("%w: esquema %q", ErrDenegado, r.URL.Scheme)
		}
	}
	if s := r.URL.Port(); s != "" {
		puerto, _ = strconv.Atoi(s)
	}
	if err := t.p.Permitido(t.pr, r.URL.Hostname(), puerto, conTLS); err != nil {
		if r.Body != nil {
			_ = r.Body.Close()
		}
		return nil, err
	}
	return t.base.RoundTrip(r)
}

// Marcar abre una conexión TCP para pr (IMAP, SMTP, POP3) si está permitida. Quien la
// usa pone el TLS (implícito o STARTTLS); solo un destino marcado [Destino.Claro] puede
// quedarse sin él, y eso lo decide la cuenta.
func (p *Portero) Marcar(ctx context.Context, pr Proposito, red, direccion string) (net.Conn, error) {
	avisar(pr, red, direccion)
	host, puertoTxt, err := net.SplitHostPort(direccion)
	if err != nil {
		return nil, err
	}
	puerto, _ := strconv.Atoi(puertoTxt)
	// El cifrado lo pone quien marca; aquí se comprueba el destino y el puerto.
	if err := p.Permitido(pr, host, puerto, true); err != nil {
		return nil, err
	}
	return p.marcar(ctx, red, direccion)
}

// Marcador es [Portero.Marcar] con el propósito ya puesto: la forma que piden
// `correo.Red.Marcar` y compañía.
func (p *Portero) Marcador(pr Proposito) func(ctx context.Context, red, direccion string) (net.Conn, error) {
	return func(ctx context.Context, red, direccion string) (net.Conn, error) {
		return p.Marcar(ctx, pr, red, direccion)
	}
}

// ----- Permisos puntuales -----

// Puntual es el cliente de lo que la persona acaba de pedir y NO sale de la
// configuración: ver las imágenes remotas de un correo (por mensaje o por remitente),
// darse de baja de una lista con un clic. Quien lo usa lo llama SOLO después de ese
// gesto; el portero deja salir cada petición —también cada redirección— mientras dura,
// y Ajustes → Red la enseña como temporal con su motivo. Topes: solo https, solo
// direcciones públicas de Internet (nada de la red de casa ni de este equipo), sin
// proxy, con plazo, cinco saltos como mucho, y nada con «Sin conexión».
func (p *Portero) Puntual(pr Proposito, motivo string) *http.Client {
	t := p.base.Clone()
	p.mu.Lock()
	if p.raicesPuntual != nil {
		t.TLSClientConfig = &tls.Config{RootCAs: p.raicesPuntual, MinVersion: tls.VersionTLS12}
	}
	p.mu.Unlock()
	t.Proxy = nil // la dirección que se comprueba es la que se marca
	t.DialContext = func(ctx context.Context, red, direccion string) (net.Conn, error) {
		p.mu.Lock()
		marcar := p.marcarPublica
		p.mu.Unlock()
		return marcar(ctx, red, direccion)
	}
	t.ResponseHeaderTimeout = 20 * time.Second
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: puntual{p: p, pr: pr, motivo: motivo, base: t},
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("red: más de %d redirecciones", len(via))
			}
			return nil
		},
	}
}

type puntual struct {
	p      *Portero
	pr     Proposito
	motivo string
	base   http.RoundTripper
}

func (t puntual) RoundTrip(r *http.Request) (*http.Response, error) {
	avisarPeticion(t.pr, r)
	host := normalizar(Destino{Host: r.URL.Hostname()}).Host
	falla := func(err error) (*http.Response, error) {
		if r.Body != nil {
			_ = r.Body.Close()
		}
		return nil, err
	}
	if r.URL.Scheme != "https" || host == "" {
		return falla(fmt.Errorf("%w: %s solo con https", ErrDenegado, cmp.Or(host, r.URL.Scheme)))
	}
	if t.p.SinConexion() {
		return falla(fmt.Errorf("%w: «Sin conexión» está encendido (%s)", ErrDenegado, host))
	}
	puerto := 443
	if s := r.URL.Port(); s != "" {
		puerto, _ = strconv.Atoi(s)
	}
	defer t.p.Permitir(Destino{Proposito: t.pr, Host: host, Puerto: puerto, Motivo: t.motivo})()
	return t.base.RoundTrip(r)
}

// PuntualDePrueba hace que el permiso puntual de este portero marque con `marcar` en vez
// de solo a direcciones públicas, y que confíe en `raices` en vez de en las autoridades
// del sistema. Es SOLO para las pruebas de otros paquetes: su servidor `httptest` escucha
// en este equipo, adonde el permiso puntual nunca llega. Todo lo demás es lo de verdad:
// solo https, nada con «Sin conexión», el permiso temporal que enseña Ajustes → Red
// mientras dura cada petición (también cada redirección), el plazo y los saltos. Vale
// para los clientes que se pidan después. Ningún fichero que no sea de pruebas lo llama
// (TestPuntualDePruebaSoloEnPruebas).
func (p *Portero) PuntualDePrueba(marcar func(ctx context.Context, red, direccion string) (net.Conn, error), raices *x509.CertPool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.marcarPublica, p.raicesPuntual = marcar, raices
}

// Publica dice si ip es una dirección pública de Internet: ni este equipo, ni la red de
// casa (privadas, enlace local, CGNAT), ni multidifusión.
func Publica(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	if v4 := ip.To4(); v4 != nil && (v4[0] == 0 || (v4[0] == 100 && v4[1]&0xc0 == 64)) {
		return false
	}
	return true
}

// Denegar es el transporte que lo deniega todo: la cáscara lo pone como
// `http.DefaultTransport` y `api.Transporte`, para que un cliente que no pase por el
// portero falle cerrado en vez de salir a la red.
type Denegar struct{}

// RoundTrip deniega.
func (Denegar) RoundTrip(r *http.Request) (*http.Response, error) {
	avisarPeticion("", r)
	if r.Body != nil {
		_ = r.Body.Close()
	}
	return nil, fmt.Errorf("%w: %s no pasa por el portero", ErrDenegado, r.URL.Host)
}
