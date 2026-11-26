// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	cchat "git.sytue.eu/sytue/one/apps/nucleo/cuentas/chat"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Las cuentas de Teams y de Google Chat entran con OAuth de cliente público con PKCE y
// vuelta a 127.0.0.1 (D-18), con el mismo `nube.OAuth` de la nube y de Calendar: abre el
// navegador del sistema, espera la vuelta y guarda el token de refresco en el llavero.

// Extremos OAuth que sustituyen a los de `nube` (vacíos: los de siempre). Solo los
// cambian las pruebas visuales contra un proveedor de mentira (`-ldflags -X`).
var (
	AutorizarMicrosoft string
	TokenMicrosoft     string
	AutorizarGoogle    string
	TokenGoogle        string
)

// Permisos de Teams. Todos los puede conceder la persona sin el administrador salvo
// leer los mensajes de los canales (`ChannelMessage.Read.All`), que solo se piden si
// marca los canales al entrar: si su organización no lo ha consentido, Microsoft no deja
// entrar con él, y se vuelve a intentar sin los canales.
//
// Los adjuntos (`Files.ReadWrite.All`: subir al OneDrive de la persona lo que manda y
// abrir lo que le comparten) también se piden solo si se marcan: es un permiso más que
// la persona concede sin el administrador, pero no todas lo quieren dar. Sin él, se
// abren las imágenes pegadas en los mensajes, que van con el chat.
var (
	alcancesTeams    = []string{"User.Read", "Chat.ReadWrite", "OnlineMeetings.ReadWrite", "Presence.Read.All", "offline_access"}
	alcancesCanales  = []string{"Team.ReadBasic.All", "Channel.ReadBasic.All", "ChannelMessage.Read.All", "ChannelMessage.Send"}
	alcancesFicheros = []string{"Files.ReadWrite.All"}
)

// OAuthTeams es el OAuth de Microsoft con los permisos de Teams. Teams solo existe en las
// cuentas de organización: la autoridad `organizations` en vez de `common` hace que
// Microsoft lo diga al entrar con una cuenta personal, en vez de fallar después.
func OAuthTeams(cliente string, canales bool) nube.OAuth {
	return OAuthTeamsCon(cliente, canales, false)
}

// OAuthTeamsCon es OAuthTeams con o sin los permisos de los adjuntos.
func OAuthTeamsCon(cliente string, canales, ficheros bool) nube.OAuth {
	o := nube.OAuthMicrosoft(cliente)
	o.Autorizar = cmp.Or(AutorizarMicrosoft, strings.Replace(o.Autorizar, "/common/", "/organizations/", 1))
	o.Token = cmp.Or(TokenMicrosoft, strings.Replace(o.Token, "/common/", "/organizations/", 1))
	o.Alcances = slices.Clone(alcancesTeams)
	if canales {
		o.Alcances = append(o.Alcances, alcancesCanales...)
	}
	if ficheros {
		o.Alcances = append(o.Alcances, alcancesFicheros...)
	}
	return o
}

// Permisos de Google Chat y Meet: ver los espacios, escribir en ellos (también ficheros)
// y crear reuniones. Leer (y ver quién está) solo con el cliente propio del Workspace
// (D-22). Abrir un mensaje directo con quien no se tenía (`chat.spaces.create`) es un
// permiso más que solo se pide si se marca: sin él, se abren los que ya hay.
var (
	alcancesGoogle = []string{
		"openid", "email", "profile",
		"https://www.googleapis.com/auth/chat.spaces.readonly",
		"https://www.googleapis.com/auth/chat.messages.create",
		"https://www.googleapis.com/auth/meetings.space.created",
	}
	alcancesLectura = []string{
		"https://www.googleapis.com/auth/chat.messages.readonly",
		"https://www.googleapis.com/auth/chat.memberships.readonly",
	}
	alcancesDirectos = []string{"https://www.googleapis.com/auth/chat.spaces.create"}
)

// OAuthGoogleChat es el OAuth de Google con los permisos de Chat y Meet.
func OAuthGoogleChat(cliente, secreto string, lectura bool) nube.OAuth {
	return OAuthGoogleChatCon(cliente, secreto, lectura, false)
}

// OAuthGoogleChatCon es OAuthGoogleChat con o sin el permiso de abrir directos nuevos.
func OAuthGoogleChatCon(cliente, secreto string, lectura, directos bool) nube.OAuth {
	o := nube.OAuthGoogle(cliente, secreto)
	o.Autorizar = cmp.Or(AutorizarGoogle, o.Autorizar)
	o.Token = cmp.Or(TokenGoogle, o.Token)
	o.Alcances = slices.Clone(alcancesGoogle)
	if lectura {
		o.Alcances = append(o.Alcances, alcancesLectura...)
	}
	if directos {
		o.Alcances = append(o.Alcances, alcancesDirectos...)
	}
	return o
}

// ajustarOAuth lo cambian las pruebas para hablar con un proveedor de mentira.
var ajustarOAuth func(*nube.OAuth)

// ErrReconectar es una cuenta cuyo token de refresco ya no vale: hay que volver a entrar.
var ErrReconectar = errors.New("chat: la cuenta pide volver a entrar (Rooms → Cuentas de mensajería)")

// EstadoReconectar es el Estado de esa cuenta (el mismo que `local/cuentas`).
const EstadoReconectar = "reconectar"

func secretoRefresco(id string) string { return "cuenta-" + id + "-refresco" }
func secretoCliente(id string) string  { return "cuenta-" + id + "-secreto" }

// EsOAuth dice si la cuenta entra por OAuth (y por tanto se vuelve a entrar en el
// navegador).
func EsOAuth(c Cuenta) bool { return c.Proveedor == ProveedorTeams || c.Proveedor == ProveedorGoogle }

func (r *Registro) clienteHTTP() *http.Client {
	if r.Cliente != nil {
		return r.Cliente
	}
	// Sin el del portero, ninguno: lo no configurado falla cerrado (R-RED).
	return &http.Client{Transport: red.Denegar{}, Timeout: 60 * time.Second}
}

func (r *Registro) clienteOAuth() *http.Client {
	if r.ClienteOAuth != nil {
		return r.ClienteOAuth
	}
	return r.clienteHTTP()
}

// oauthDe es el OAuth de la cuenta: con su cliente y, en Google, el secreto que lo
// acompaña (el propio del llavero o el de la compilación).
func (r *Registro) oauthDe(c Cuenta) (nube.OAuth, error) {
	var o nube.OAuth
	switch c.Proveedor {
	case ProveedorTeams:
		o = OAuthTeamsCon(c.ClienteID, c.Canales, c.Ficheros)
	case ProveedorGoogle:
		secreto, _ := r.secretos.Leer(secretoCliente(c.ID))
		if secreto == "" && c.ClienteID == r.Clientes.Google {
			secreto = r.Clientes.GoogleSecreto
		}
		o = OAuthGoogleChatCon(c.ClienteID, secreto, c.Lectura, c.Directos)
	default:
		return o, fmt.Errorf("chat: %q no entra por OAuth", c.Proveedor)
	}
	if ajustarOAuth != nil {
		ajustarOAuth(&o)
	}
	return o, nil
}

// hostDe es la máquina de una dirección.
func hostDe(direccion string) string {
	u, err := url.Parse(direccion)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// serviciosDe son las direcciones de las API de un proveedor OAuth y su servidor de
// tokens.
func serviciosDe(proveedor string) (apis []string, tokens string) {
	var o nube.OAuth
	switch proveedor {
	case ProveedorTeams:
		apis, o = []string{APIGraph}, OAuthTeams("", false)
	case ProveedorGoogle:
		apis, o = []string{APIChatGoogle, APIMeet, APIUsuarioGoogle, APIPersonasGoogle}, OAuthGoogleChat("", "", false)
	default:
		return nil, ""
	}
	if ajustarOAuth != nil {
		ajustarOAuth(&o)
	}
	return apis, o.Token
}

// DestinosAlta son los destinos que el portero abre mientras dura el alta (o el volver a
// entrar) de una cuenta de proveedor: su servidor de tokens y sus API, para probarla.
func DestinosAlta(proveedor, motivo string) []red.Destino {
	apis, tokens := serviciosDe(proveedor)
	var ds []red.Destino
	if d, err := red.DestinoDeURL(red.OAuth, tokens, motivo); err == nil {
		ds = append(ds, d)
	}
	for _, a := range apis {
		if d, err := red.DestinoDeURL(red.Chat, a, motivo); err == nil {
			ds = append(ds, d)
		}
	}
	return ds
}

// DestinosDescarga son los servidores de descarga de SharePoint (los de la organización,
// también su OneDrive) a los que Graph redirige al bajar un fichero de Teams, para el
// portero: solo con una cuenta de Teams que entró con los adjuntos. Es un nombre con
// punto delante porque cada organización tiene el suyo.
func (r *Registro) DestinosDescarga() []red.Destino {
	var ds []red.Destino
	for _, c := range r.Cuentas() {
		if c.Proveedor == ProveedorTeams && c.Ficheros {
			ds = append(ds, red.Destino{Proposito: red.Chat, Host: ".sharepoint.com", Motivo: strings.TrimSuffix(c.Rotulo+" — "+c.Usuario, " — ")})
		}
	}
	return ds
}

// fuenteOAuth da el token de acceso de una cuenta: lo renueva con el de refresco del
// llavero cuando caduca (o cuando el proveedor dice 401), guarda el nuevo de refresco si
// el proveedor lo rota y, si lo rechaza, deja la cuenta en «volver a entrar».
type fuenteOAuth struct {
	r     *Registro
	id    string
	o     nube.OAuth
	ahora func() time.Time

	mu     sync.Mutex
	acceso string
	vence  time.Time
}

var _ cchat.Token = (*fuenteOAuth)(nil)

func (f *fuenteOAuth) reloj() time.Time {
	if f.ahora != nil {
		return f.ahora()
	}
	return time.Now()
}

// Acceso es el token vigente, renovado si hace falta.
func (f *fuenteOAuth) Acceso(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.acceso != "" && f.reloj().Before(f.vence) {
		return f.acceso, nil
	}
	refresco, err := f.r.secretos.Leer(secretoRefresco(f.id))
	if err != nil || refresco == "" {
		f.r.marcarEstado(f.id, EstadoReconectar)
		return "", ErrReconectar
	}
	t, err := f.o.Renovar(ctx, f.r.clienteOAuth(), refresco)
	if err != nil {
		// Sin red (o con el portero cerrado) no es que el token no valga.
		var ue *url.Error
		if errors.As(err, &ue) || errors.Is(err, red.ErrDenegado) || ctx.Err() != nil {
			return "", err
		}
		f.r.marcarEstado(f.id, EstadoReconectar)
		return "", errors.Join(ErrReconectar, err)
	}
	if t.Refresco != "" && t.Refresco != refresco {
		_ = f.r.secretos.Guardar(secretoRefresco(f.id), t.Refresco)
	}
	f.poner(t)
	f.r.marcarEstado(f.id, "")
	return f.acceso, nil
}

func (f *fuenteOAuth) poner(t nube.Tokens) {
	f.acceso, f.vence = t.Acceso, f.reloj().Add(time.Duration(max(t.Expira-60, 30))*time.Second)
}

// Invalidar tira el token: la próxima petición lo renueva.
func (f *fuenteOAuth) Invalidar() {
	f.mu.Lock()
	f.acceso = ""
	f.mu.Unlock()
}

// marcarEstado cambia el estado de la cuenta id (y lo escribe si cambia).
func (r *Registro) marcarEstado(id, estado string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := slices.IndexFunc(r.cuentas, func(c Cuenta) bool { return c.ID == id })
	if i < 0 || r.cuentas[i].Estado == estado {
		return
	}
	cs := slices.Clone(r.cuentas)
	cs[i].Estado = estado
	if espacio.EscribirJSON(r.ruta, cs) == nil {
		r.cuentas = cs
	}
}

// proveedorOAuth construye el proveedor de la cuenta con su fuente de tokens.
func (r *Registro) proveedorOAuth(c Cuenta, f *fuenteOAuth) Proveedor {
	hc := r.clienteHTTP()
	switch c.Proveedor {
	case ProveedorTeams:
		t := NuevaTeams(&cchat.Graph{Base: APIGraph, Token: f, HTTP: hc}, RotuloCarril(c), c.Canales, r.Avisos)
		t.Ficheros = c.Ficheros
		return t
	default:
		usuario := func(ctx context.Context) (cchat.UsuarioGoogle, error) {
			return cchat.Usuario(ctx, hc, f, APIUsuarioGoogle)
		}
		g := NuevaGoogleChat(&cchat.GoogleChat{Base: APIChatGoogle, Token: f, HTTP: hc}, &cchat.Meet{Base: APIMeet, Token: f, HTTP: hc},
			usuario, RotuloCarril(c), c.Lectura, r.Avisos)
		g.P = &cchat.PersonasGoogle{Base: APIPersonasGoogle, Token: f, HTTP: hc}
		g.Directos = c.Directos
		return g
	}
}

// RotuloCarril es como se llama en el carril una cuenta de Teams o de Google: el
// servicio y la persona («Teams · ana@empresa.eu»), para distinguir varias del mismo.
// Su Rotulo es solo el servicio, y la persona va en Usuario.
func RotuloCarril(c Cuenta) string {
	servicio := "Teams"
	if c.Proveedor == ProveedorGoogle {
		servicio = "Google Chat"
	}
	if c.Usuario == "" {
		return c.Rotulo
	}
	return servicio + " · " + c.Usuario
}

func (r *Registro) abrirOAuth(c Cuenta) (Proveedor, error) {
	o, err := r.oauthDe(c)
	if err != nil {
		return nil, err
	}
	r.ponerServidores(c)
	if v, err := r.secretos.Leer(secretoRefresco(c.ID)); err != nil || v == "" {
		return nil, fmt.Errorf("chat: la cuenta %s no tiene sesión; vuelve a entrar", c.Rotulo)
	}
	return r.proveedorOAuth(c, &fuenteOAuth{r: r, id: c.ID, o: o}), nil
}

// ponerServidores pone al día las direcciones de las API de una cuenta OAuth guardada
// antes de que el proveedor usara una más (la People API de Google): el portero solo
// deja ir a las que dice la cuenta.
func (r *Registro) ponerServidores(c Cuenta) {
	apis, _ := serviciosDe(c.Proveedor)
	falta := false
	for _, a := range apis {
		if !slices.Contains(c.Servidores, a) {
			falta = true
		}
	}
	if !falta || len(c.Servidores) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cs := slices.Clone(r.cuentas)
	i := slices.IndexFunc(cs, func(x Cuenta) bool { return x.ID == c.ID })
	if i < 0 {
		return
	}
	for _, a := range apis {
		if !slices.Contains(cs[i].Servidores, a) {
			cs[i].Servidores = append(slices.Clone(cs[i].Servidores), a)
		}
	}
	if espacio.EscribirJSON(r.ruta, cs) == nil {
		r.cuentas = cs
	}
}

// AltaOAuth es lo que la persona pone al dar de alta una cuenta de Teams o de Google.
type AltaOAuth struct {
	// Cliente y Secreto son su cliente OAuth propio (vacíos: los de la compilación).
	// En Google, el propio es lo que permite leer (D-22).
	Cliente, Secreto string
	// Canales: en Teams, también los canales de los equipos.
	Canales bool
	// Ficheros: en Teams, los adjuntos (un permiso más).
	Ficheros bool
	// Directos: en Google, abrir mensajes directos nuevos (un permiso más).
	Directos bool
}

// AltaTeams da de alta una cuenta de Microsoft Teams: abre el navegador con abrir,
// espera la vuelta, comprueba quién es y la guarda.
func (r *Registro) AltaTeams(ctx context.Context, a AltaOAuth, abrir func(string) error) (Cuenta, Proveedor, error) {
	c := Cuenta{ID: nuevoID(), Proveedor: ProveedorTeams, ClienteID: cmp.Or(strings.TrimSpace(a.Cliente), r.Clientes.Microsoft), Canales: a.Canales, Ficheros: a.Ficheros}
	return r.altaOAuth(ctx, c, "", abrir)
}

// AltaGoogle da de alta una cuenta de Google Chat y Meet. Con cliente propio, lee.
func (r *Registro) AltaGoogle(ctx context.Context, a AltaOAuth, abrir func(string) error) (Cuenta, Proveedor, error) {
	propio := strings.TrimSpace(a.Cliente)
	c := Cuenta{ID: nuevoID(), Proveedor: ProveedorGoogle, ClienteID: cmp.Or(propio, r.Clientes.Google), Lectura: propio != "", Directos: a.Directos}
	secreto := ""
	if propio != "" {
		secreto = strings.TrimSpace(a.Secreto)
	}
	return r.altaOAuth(ctx, c, secreto, abrir)
}

// conectar hace el OAuth de c y devuelve la fuente con el primer token.
func (r *Registro) conectar(ctx context.Context, c Cuenta, secreto string, abrir func(string) error) (*fuenteOAuth, string, error) {
	if secreto != "" {
		if err := r.secretos.Guardar(secretoCliente(c.ID), secreto); err != nil {
			return nil, "", err
		}
	}
	o, err := r.oauthDe(c)
	if err != nil {
		return nil, "", err
	}
	t, err := o.Conectar(ctx, r.clienteOAuth(), abrir)
	if err != nil {
		return nil, "", err
	}
	if t.Refresco == "" {
		return nil, "", errors.New("chat: el proveedor no dio acceso sin conexión (falta el token de refresco)")
	}
	f := &fuenteOAuth{r: r, id: c.ID, o: o}
	f.poner(t)
	return f, t.Refresco, nil
}

// probar pregunta quién es con la fuente nueva: el rótulo y el usuario de la cuenta.
func (r *Registro) probar(ctx context.Context, c *Cuenta, f *fuenteOAuth) (Proveedor, error) {
	p := r.proveedorOAuth(*c, f)
	switch x := p.(type) {
	case *Teams:
		u, err := x.QuienSoy(ctx)
		if err != nil {
			return nil, err
		}
		c.Usuario = cmp.Or(u.Correo, u.UPN, u.Nombre, u.ID)
		c.Rotulo = "Microsoft Teams"
		x.Nombre = RotuloCarril(*c)
	case *GoogleChat:
		u, err := x.QuienSoy(ctx)
		if err != nil {
			return nil, err
		}
		c.Usuario = cmp.Or(u.Correo, u.Nombre, u.Recurso())
		c.Rotulo = "Google Chat"
		x.Nombre = RotuloCarril(*c)
	}
	return p, nil
}

func (r *Registro) altaOAuth(ctx context.Context, c Cuenta, secreto string, abrir func(string) error) (Cuenta, Proveedor, error) {
	apis, tokens := serviciosDe(c.Proveedor)
	c.Servidores, c.OAuth = apis, hostDe(tokens)
	limpiar := func() {
		_ = r.secretos.Borrar(secretoCliente(c.ID))
		_ = r.secretos.Borrar(secretoRefresco(c.ID))
	}
	f, refresco, err := r.conectar(ctx, c, secreto, abrir)
	if err != nil {
		limpiar()
		return Cuenta{}, nil, err
	}
	if err := r.secretos.Guardar(secretoRefresco(c.ID), refresco); err != nil {
		limpiar()
		return Cuenta{}, nil, fmt.Errorf("chat: no se pudo guardar la sesión: %w", err)
	}
	p, err := r.probar(ctx, &c, f)
	if err != nil {
		limpiar()
		return Cuenta{}, nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cs := append(slices.Clone(r.cuentas), c)
	if err := espacio.EscribirJSON(r.ruta, cs); err != nil {
		limpiar()
		return Cuenta{}, nil, err
	}
	r.cuentas = cs
	return c, p, nil
}

// Reconectar vuelve a entrar en una cuenta de Teams o de Google sin cambiar nada más
// (ni el identificador ni lo que se eligió al darla de alta) y da su proveedor nuevo.
func (r *Registro) Reconectar(ctx context.Context, id string, abrir func(string) error) (Cuenta, Proveedor, error) {
	var c Cuenta
	for _, x := range r.Cuentas() {
		if x.ID == id {
			c = x
		}
	}
	if c.ID == "" || !EsOAuth(c) {
		return Cuenta{}, nil, fmt.Errorf("chat: la cuenta %q no entra por OAuth", id)
	}
	f, refresco, err := r.conectar(ctx, c, "", abrir)
	if err != nil {
		return Cuenta{}, nil, err
	}
	if err := r.secretos.Guardar(secretoRefresco(c.ID), refresco); err != nil {
		return Cuenta{}, nil, err
	}
	p, err := r.probar(ctx, &c, f)
	if err != nil {
		return Cuenta{}, nil, err
	}
	c.Estado = ""
	r.mu.Lock()
	defer r.mu.Unlock()
	cs := slices.Clone(r.cuentas)
	if i := slices.IndexFunc(cs, func(x Cuenta) bool { return x.ID == id }); i >= 0 {
		cs[i] = c
	}
	if err := espacio.EscribirJSON(r.ruta, cs); err != nil {
		return Cuenta{}, nil, err
	}
	r.cuentas = cs
	return c, p, nil
}
