// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// ProveedorMatrix es el proveedor de una cuenta Matrix (también Sytue Rooms online).
const ProveedorMatrix = "matrix"

// Cuenta es una cuenta de mensajería, SIN secretos: se guarda en claro en
// `<Ajustes>/chat-cuentas.json`. El token va al llavero como `cuenta-<ID>-token`
// (§5.10). Cuando exista el modelo común de cuentas (`nucleo/cuentas/modelo`), esto pasa
// a ser una cuenta más con el servicio «chat».
type Cuenta struct {
	ID          string `json:"id"`
	Proveedor   string `json:"proveedor"`
	Rotulo      string `json:"rotulo"`
	Servidor    string `json:"servidor"`
	Usuario     string `json:"usuario"`
	Dispositivo string `json:"dispositivo,omitempty"`

	// Lo de las cuentas con OAuth (Teams, Google Chat), en oauth.go. El token de
	// refresco va al llavero como `cuenta-<ID>-refresco` y, con cliente propio de
	// Google, su secreto como `cuenta-<ID>-secreto`.
	//
	// ClienteID es el cliente OAuth con el que se entró (el de la compilación o el de
	// la persona).
	ClienteID string `json:"cliente_id,omitempty"`
	// Servidores son las direcciones de sus API y OAuth el servidor de tokens: los lee
	// `local/cuentas` para que el portero deje ir a ellos (y a nada más).
	Servidores []string `json:"servidores,omitempty"`
	OAuth      string   `json:"oauth,omitempty"`
	// Canales: Teams con los canales de sus equipos. Lectura: Google Chat con cliente
	// propio del Workspace, que puede leer (D-22). Ficheros: Teams con los adjuntos.
	// Directos: Google Chat abre mensajes directos nuevos.
	Canales  bool `json:"canales,omitempty"`
	Lectura  bool `json:"lectura,omitempty"`
	Ficheros bool `json:"ficheros,omitempty"`
	Directos bool `json:"directos,omitempty"`
	// Estado es "reconectar" cuando el proveedor rechazó el token de refresco: Ajustes →
	// Cuentas ofrece volver a entrar.
	Estado string `json:"estado,omitempty"`
}

// Secretos es el llavero de la edición (`secretos.Almacen`).
type Secretos interface {
	Leer(nombre string) (string, error)
	Guardar(nombre, valor string) error
	Borrar(nombre string) error
}

// Registro son las cuentas configuradas.
type Registro struct {
	ruta     string
	secretos Secretos
	// Cliente es el HTTP con el que hablan las cuentas (el del portero); nil, uno propio.
	Cliente *http.Client
	// ClienteOAuth es el del intercambio y la renovación de tokens (el del portero con
	// el propósito `oauth`); nil, Cliente.
	ClienteOAuth *http.Client
	// Clientes son los identificadores OAuth de la compilación (`nube.ClientesOAuth`).
	Clientes nube.ClientesOAuth
	// Avisos son los textos de las cuentas, ya en el idioma de la app.
	Avisos Avisos

	mu      sync.Mutex
	cuentas []Cuenta
}

// AbrirRegistro lee las cuentas de ruta.
func AbrirRegistro(ruta string, s Secretos) (*Registro, error) {
	r := &Registro{ruta: ruta, secretos: s}
	if err := espacio.LeerJSON(ruta, &r.cuentas); err != nil {
		return nil, err
	}
	return r, nil
}

func secretoToken(id string) string { return "cuenta-" + id + "-token" }

// Cuentas son las configuradas.
func (r *Registro) Cuentas() []Cuenta {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.cuentas)
}

// Abrir construye el proveedor de una cuenta con su token del llavero. No habla con la
// red: la primera conexión es al pedir los canales.
func (r *Registro) Abrir(c Cuenta) (Proveedor, error) {
	switch c.Proveedor {
	case ProveedorTeams, ProveedorGoogle:
		return r.abrirOAuth(c)
	case ProveedorMatrix, ProveedorSlack:
	default:
		return nil, fmt.Errorf("chat: proveedor %q desconocido", c.Proveedor)
	}
	tok, err := r.secretos.Leer(secretoToken(c.ID))
	if err != nil || tok == "" {
		return nil, fmt.Errorf("chat: la cuenta %s no tiene token; vuelve a darla de alta", c.Rotulo)
	}
	if c.Proveedor == ProveedorSlack {
		app, _ := r.secretos.Leer(secretoApp(c.ID))
		return NuevaSlack(tok, app, r.Cliente), nil
	}
	return NuevaMatrix(c.Servidor, c.Usuario, tok, r.Cliente), nil
}

func secretoApp(id string) string { return "cuenta-" + id + "-app" }

// AltaSlack da de alta un espacio de Slack con el token de usuario de la app propia del
// espacio (`xoxp-…`) y, opcional, el de la app (`xapp-…`) para Socket Mode. Comprueba
// el token contra Slack antes de guardar nada.
func (r *Registro) AltaSlack(ctx context.Context, token, tokenApp string) (Cuenta, Proveedor, error) {
	token, tokenApp = strings.TrimSpace(token), strings.TrimSpace(tokenApp)
	if !strings.HasPrefix(token, "xox") {
		return Cuenta{}, nil, errors.New("chat: el token de Slack empieza por «xoxp-» (de usuario) o «xoxb-»")
	}
	if tokenApp != "" && !strings.HasPrefix(tokenApp, "xapp-") {
		return Cuenta{}, nil, errors.New("chat: el token de la app de Slack empieza por «xapp-»")
	}
	s := NuevaSlack(token, tokenApp, r.Cliente)
	quien, err := s.QuienSoy(ctx)
	if err != nil {
		return Cuenta{}, nil, err
	}
	c := Cuenta{ID: nuevoID(), Proveedor: ProveedorSlack, Rotulo: s.Rotulo(), Servidor: "slack.com", Usuario: quien}
	if err := r.secretos.Guardar(secretoToken(c.ID), token); err != nil {
		return Cuenta{}, nil, fmt.Errorf("chat: no se pudo guardar el token: %w", err)
	}
	if tokenApp != "" {
		if err := r.secretos.Guardar(secretoApp(c.ID), tokenApp); err != nil {
			_ = r.secretos.Borrar(secretoToken(c.ID))
			return Cuenta{}, nil, err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cs := append(slices.Clone(r.cuentas), c)
	if err := espacio.EscribirJSON(r.ruta, cs); err != nil {
		_ = r.secretos.Borrar(secretoToken(c.ID))
		_ = r.secretos.Borrar(secretoApp(c.ID))
		return Cuenta{}, nil, err
	}
	r.cuentas = cs
	return c, s, nil
}

// AltaMatrix da de alta una cuenta Matrix: con token si lo hay; si no, con usuario y
// contraseña (que no se guarda). Comprueba la cuenta contra el servidor antes de guardar.
func (r *Registro) AltaMatrix(ctx context.Context, servidor, usuario, clave, token string) (Cuenta, Proveedor, error) {
	servidor = strings.TrimRight(strings.TrimSpace(servidor), "/")
	if servidor == "" {
		return Cuenta{}, nil, errors.New("chat: falta la dirección del servidor")
	}
	if !strings.Contains(servidor, "://") {
		servidor = "https://" + servidor
	}
	c := Cuenta{ID: nuevoID(), Proveedor: ProveedorMatrix, Servidor: servidor}
	if token = strings.TrimSpace(token); token == "" {
		s, err := IniciarMatrix(ctx, r.Cliente, servidor, strings.TrimSpace(usuario), clave, "Sytue Office")
		if err != nil {
			return Cuenta{}, nil, err
		}
		token, c.Dispositivo = s.Token, s.Dispositivo
	}
	m := NuevaMatrix(servidor, "", token, r.Cliente)
	yo, err := m.QuienSoy(ctx)
	if err != nil {
		return Cuenta{}, nil, err
	}
	c.Usuario, c.Rotulo = yo, yo
	if err := r.secretos.Guardar(secretoToken(c.ID), token); err != nil {
		return Cuenta{}, nil, fmt.Errorf("chat: no se pudo guardar el token: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cs := append(slices.Clone(r.cuentas), c)
	if err := espacio.EscribirJSON(r.ruta, cs); err != nil {
		_ = r.secretos.Borrar(secretoToken(c.ID))
		return Cuenta{}, nil, err
	}
	r.cuentas = cs
	return c, m, nil
}

// Baja quita la cuenta y su token del llavero.
func (r *Registro) Baja(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cs := slices.DeleteFunc(slices.Clone(r.cuentas), func(c Cuenta) bool { return c.ID == id })
	if err := espacio.EscribirJSON(r.ruta, cs); err != nil {
		return err
	}
	r.cuentas = cs
	_ = r.secretos.Borrar(secretoApp(id))
	_ = r.secretos.Borrar(secretoRefresco(id))
	_ = r.secretos.Borrar(secretoCliente(id))
	return r.secretos.Borrar(secretoToken(id))
}
