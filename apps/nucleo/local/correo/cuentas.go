// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package correo es Sytue Mail de Sytue Office: un cliente de correo para las cuentas
// externas de la persona (Outlook, Gmail, iCloud, Yahoo, cualquier IMAP, y el IMAP de
// Sytue Mail como una cuenta más), sobre un almacén local en claro
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.9, D-16).
//
// Todo se lee y se escribe en local; un motor de sincronización por cuenta reconcilia
// con el servidor cuando hay red:
//
//   - el índice (`<Datos>/Correo/indice.sqlite`): carpetas, sobres, marcas, hilos,
//     etiquetas locales, reglas, firma y bandeja de salida;
//   - los mensajes, en `.eml` legibles (`<Datos>/Correo/<cuenta>/<id>.eml`);
//   - la bajada es incremental por UID (IMAP), la subida es una cola de cambios
//     pendientes, y el envío sale por SMTP desde la bandeja de salida local.
//
// Sin ninguna cuenta configurada no se abre ni una conexión (R-RED).
package correo

import (
	"cmp"
	"errors"
	"fmt"
	"net/mail"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// Seguridad de una conexión.
const (
	SeguridadTLS      = "tls"      // TLS implícito (993, 465)
	SeguridadSTARTTLS = "starttls" // texto que sube a TLS (143, 587); nunca baja a claro
	SeguridadNinguna  = "ninguna"  // solo con un servidor de este equipo (HostLocal): pruebas, o un puente local
)

// Cómo entra la cuenta.
const (
	AccesoContrasena = "contrasena"      // contraseña o contraseña de aplicación
	AccesoMicrosoft  = "oauth-microsoft" // OAuth con PKCE y XOAUTH2 (D-19)
)

// Estados de una cuenta.
const (
	EstadoOK         = ""
	EstadoReconectar = "reconectar" // el servidor rechazó la contraseña o el token
)

// Servidor es un extremo IMAP o SMTP.
type Servidor struct {
	Host      string `json:"host"`
	Puerto    int    `json:"puerto"`
	Seguridad string `json:"seguridad"`
	// Usuario; vacío, la dirección de la cuenta.
	Usuario string `json:"usuario,omitempty"`
}

// Cuenta es una cuenta de correo conectada. Se guarda en claro en `cuentas.json`; los
// secretos van al llavero con la clave `cuenta-<ID>-<clase>` (§5.10).
type Cuenta struct {
	ID        string   `json:"id"`
	Proveedor string   `json:"proveedor"`
	Rotulo    string   `json:"rotulo,omitempty"`
	Nombre    string   `json:"nombre,omitempty"`
	Direccion string   `json:"direccion"`
	IMAP      Servidor `json:"imap"`
	SMTP      Servidor `json:"smtp"`
	Acceso    string   `json:"acceso"`
	// ClienteOAuth es el identificador de cliente con el que se conectó (el de la
	// compilación o el propio de la persona, D-18).
	ClienteOAuth string `json:"cliente_oauth,omitempty"`
	// Sieve es el servidor de la respuesta automática (ManageSieve) que escribió la
	// persona; vacío, el de siempre (ver ServidorSieve). Solo Host y Puerto.
	Sieve Servidor `json:"sieve,omitzero"`
	// GuardaEnviados: el servidor deja él solo la copia en Enviados al enviar por SMTP
	// (Gmail, Outlook). Si no, la app la sube con APPEND.
	GuardaEnviados bool      `json:"guarda_enviados,omitempty"`
	Estado         string    `json:"estado,omitempty"`
	Error          string    `json:"error,omitempty"`
	Sincronizada   time.Time `json:"sincronizada,omitzero"`
}

// Titulo es lo que la persona ve de la cuenta.
func (c Cuenta) Titulo() string { return cmp.Or(c.Rotulo, c.Direccion) }

func (c Cuenta) usuario(s Servidor) string { return cmp.Or(s.Usuario, c.Direccion) }

// Claves del llavero.
func claveContrasena(id string) string { return "cuenta-" + id + "-contrasena" }
func claveRefresco(id string) string   { return "cuenta-" + id + "-refresco" }

// Secretos es el llavero de la edición (`local/secretos`).
type Secretos interface {
	Leer(nombre string) (string, error)
	Guardar(nombre, valor string) error
	Borrar(nombre string) error
}

// ErrSinCuenta: la cuenta no existe (se quitó).
var ErrSinCuenta = errors.New("correo: la cuenta no existe")

// registro son las cuentas de `cuentas.json`.
type registro struct {
	ruta    string
	mu      sync.Mutex
	cuentas []Cuenta
}

func abrirRegistro(raiz string) (*registro, error) {
	r := &registro{ruta: filepath.Join(raiz, "cuentas.json")}
	if err := espacio.LeerJSON(r.ruta, &r.cuentas); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *registro) lista() []Cuenta {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.cuentas)
}

func (r *registro) cuenta(id string) (Cuenta, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := slices.IndexFunc(r.cuentas, func(c Cuenta) bool { return c.ID == id })
	if i < 0 {
		return Cuenta{}, false
	}
	return r.cuentas[i], true
}

func (r *registro) porDireccion(dir string) (Cuenta, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := slices.IndexFunc(r.cuentas, func(c Cuenta) bool { return strings.EqualFold(c.Direccion, dir) })
	if i < 0 {
		return Cuenta{}, false
	}
	return r.cuentas[i], true
}

func (r *registro) guardarLocked() error { return espacio.EscribirJSON(r.ruta, r.cuentas) }

func (r *registro) poner(c Cuenta) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i := slices.IndexFunc(r.cuentas, func(x Cuenta) bool { return x.ID == c.ID }); i >= 0 {
		r.cuentas[i] = c
	} else {
		r.cuentas = append(r.cuentas, c)
	}
	return r.guardarLocked()
}

// cambiar aplica f a la cuenta id y guarda; false si ya no está.
func (r *registro) cambiar(id string, f func(*Cuenta)) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := slices.IndexFunc(r.cuentas, func(x Cuenta) bool { return x.ID == id })
	if i < 0 {
		return false
	}
	f(&r.cuentas[i])
	_ = r.guardarLocked()
	return true
}

func (r *registro) quitar(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cuentas = slices.DeleteFunc(r.cuentas, func(c Cuenta) bool { return c.ID == id })
	return r.guardarLocked()
}

// validar comprueba una cuenta antes de darla de alta.
func validar(c *Cuenta) error {
	c.Direccion = strings.TrimSpace(c.Direccion)
	a, err := mail.ParseAddress(c.Direccion)
	if err != nil {
		return fmt.Errorf("correo: «%s» no es una dirección de correo", c.Direccion)
	}
	c.Direccion = a.Address
	for _, s := range []*Servidor{&c.IMAP, &c.SMTP} {
		s.Host = strings.TrimSpace(s.Host)
		if s.Host == "" || strings.ContainsAny(s.Host, " /:@") || s.Puerto <= 0 || s.Puerto > 65535 {
			return fmt.Errorf("correo: falta el servidor o su puerto")
		}
		switch s.Seguridad {
		case SeguridadTLS, SeguridadSTARTTLS:
		case SeguridadNinguna:
			if !HostLocal(s.Host) {
				return ErrSinCifrar
			}
		default:
			s.Seguridad = SeguridadTLS
		}
	}
	// El servidor de la respuesta automática es opcional: solo nombre y puerto (la
	// seguridad la decide ServidorSieve).
	c.Sieve = Servidor{Host: strings.TrimSpace(c.Sieve.Host), Puerto: c.Sieve.Puerto}
	if strings.ContainsAny(c.Sieve.Host, " /:@") || c.Sieve.Puerto < 0 || c.Sieve.Puerto > 65535 {
		return fmt.Errorf("correo: el servidor de la respuesta automática no es válido")
	}
	if c.Acceso == "" {
		c.Acceso = AccesoContrasena
	}
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	return nil
}

// ---- Preajustes ----

// Preajuste son los servidores fijos de un proveedor conocido.
type Preajuste struct {
	Proveedor string
	Nombre    string
	IMAP      Servidor
	SMTP      Servidor
	Acceso    string
	// GuardaEnviados: el servidor guarda la copia en Enviados él solo.
	GuardaEnviados bool
	// ContrasenaApp es dónde se crea la contraseña de aplicación, si hace falta una.
	ContrasenaApp string
}

// Proveedores conocidos.
const (
	ProveedorGmail     = "gmail"
	ProveedorMicrosoft = "microsoft"
	ProveedorICloud    = "icloud"
	ProveedorYahoo     = "yahoo"
	ProveedorSytue     = "sytue"
	ProveedorIMAP      = "imap"
)

var preajustes = []Preajuste{
	{
		Proveedor: ProveedorMicrosoft, Nombre: "Outlook y Microsoft 365", Acceso: AccesoMicrosoft, GuardaEnviados: true,
		IMAP: Servidor{Host: "outlook.office365.com", Puerto: 993, Seguridad: SeguridadTLS},
		SMTP: Servidor{Host: "smtp.office365.com", Puerto: 587, Seguridad: SeguridadSTARTTLS},
	},
	{
		Proveedor: ProveedorGmail, Nombre: "Gmail", Acceso: AccesoContrasena, GuardaEnviados: true,
		IMAP:          Servidor{Host: "imap.gmail.com", Puerto: 993, Seguridad: SeguridadTLS},
		SMTP:          Servidor{Host: "smtp.gmail.com", Puerto: 465, Seguridad: SeguridadTLS},
		ContrasenaApp: "https://myaccount.google.com/apppasswords",
	},
	{
		Proveedor: ProveedorICloud, Nombre: "iCloud Mail", Acceso: AccesoContrasena,
		IMAP:          Servidor{Host: "imap.mail.me.com", Puerto: 993, Seguridad: SeguridadTLS},
		SMTP:          Servidor{Host: "smtp.mail.me.com", Puerto: 587, Seguridad: SeguridadSTARTTLS},
		ContrasenaApp: "https://account.apple.com/",
	},
	{
		Proveedor: ProveedorYahoo, Nombre: "Yahoo Mail", Acceso: AccesoContrasena,
		IMAP:          Servidor{Host: "imap.mail.yahoo.com", Puerto: 993, Seguridad: SeguridadTLS},
		SMTP:          Servidor{Host: "smtp.mail.yahoo.com", Puerto: 465, Seguridad: SeguridadTLS},
		ContrasenaApp: "https://login.yahoo.com/account/security",
	},
	{
		Proveedor: ProveedorSytue, Nombre: "Sytue Mail", Acceso: AccesoContrasena,
		IMAP:          Servidor{Host: "imap.sytue.eu", Puerto: 993, Seguridad: SeguridadTLS},
		SMTP:          Servidor{Host: "smtp.sytue.eu", Puerto: 465, Seguridad: SeguridadTLS},
		ContrasenaApp: "https://id.sytue.eu/account#app-passwords",
	},
}

// Preajustes son los proveedores conocidos, en el orden del asistente; el último es
// «Otro (IMAP)».
func Preajustes() []Preajuste {
	return append(slices.Clone(preajustes), Preajuste{Proveedor: ProveedorIMAP, Nombre: "IMAP", Acceso: AccesoContrasena})
}

// PreajusteDe devuelve el preajuste de un proveedor.
func PreajusteDe(proveedor string) (Preajuste, bool) {
	i := slices.IndexFunc(preajustes, func(p Preajuste) bool { return p.Proveedor == proveedor })
	if i < 0 {
		return Preajuste{}, false
	}
	return preajustes[i], true
}

// dominios de cada proveedor conocido.
var dominios = map[string]string{
	"gmail.com": ProveedorGmail, "googlemail.com": ProveedorGmail,
	"outlook.com": ProveedorMicrosoft, "outlook.es": ProveedorMicrosoft, "hotmail.com": ProveedorMicrosoft,
	"hotmail.es": ProveedorMicrosoft, "live.com": ProveedorMicrosoft, "live.es": ProveedorMicrosoft, "msn.com": ProveedorMicrosoft,
	"icloud.com": ProveedorICloud, "me.com": ProveedorICloud, "mac.com": ProveedorICloud,
	"yahoo.com": ProveedorYahoo, "yahoo.es": ProveedorYahoo, "ymail.com": ProveedorYahoo,
	"sytue.eu": ProveedorSytue,
}

// Detectar elige el preajuste por el dominio de la dirección. Sin red: con un dominio
// desconocido propone `imap.<dominio>` y `smtp.<dominio>`, que es lo más común, y la
// persona los corrige si hace falta.
func Detectar(direccion string) Preajuste {
	dominio := ""
	if i := strings.LastIndex(direccion, "@"); i >= 0 {
		dominio = strings.ToLower(strings.TrimSpace(direccion[i+1:]))
	}
	if p, ok := PreajusteDe(dominios[dominio]); ok {
		return p
	}
	if strings.HasPrefix(dominio, "yahoo.") {
		p, _ := PreajusteDe(ProveedorYahoo)
		return p
	}
	p := Preajuste{Proveedor: ProveedorIMAP, Nombre: "IMAP", Acceso: AccesoContrasena}
	if dominio != "" {
		p.IMAP = Servidor{Host: "imap." + dominio, Puerto: 993, Seguridad: SeguridadTLS}
		p.SMTP = Servidor{Host: "smtp." + dominio, Puerto: 587, Seguridad: SeguridadSTARTTLS}
	}
	return p
}
