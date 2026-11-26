// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

// La respuesta automática de cada cuenta (§6.9, M-10): solo donde el proveedor la ofrece,
// que es un servidor ManageSieve (RFC 5804). Se guarda EN EL SERVIDOR de la cuenta —así
// responde aunque este equipo esté apagado— con el cliente de `cuentas/sieve`, que no
// toca las reglas que la persona ya tenga allí (ver `sieve.Guardar`).
//
// Nada de esto corre solo: la pantalla pregunta cuando la persona abre los Ajustes de
// Mail, con un plazo corto ([PlazoSieve]), y esa misma pregunta es la prueba de si la
// cuenta tiene ManageSieve. Gmail y Outlook (y iCloud, Yahoo y Sytue Mail) no lo tienen:
// no se les pregunta nunca, y la sección no aparece. La conexión sale por el portero
// (`Red.Marcar`): el servidor Sieve es uno más de la cuenta en `local/cuentas`.

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/sieve"
)

// NombreScriptSieve es el script de Sytue Office en el servidor de cada cuenta.
const NombreScriptSieve = "sytue-office"

// PuertoSieve es el de ManageSieve.
const PuertoSieve = 4190

// Plazos con el servidor Sieve.
const (
	// PlazoSieve es lo que se espera al leer la respuesta automática al abrir los
	// Ajustes: corto, porque también es la prueba de si el servidor la ofrece.
	PlazoSieve = 8 * time.Second
	// PlazoSieveGuardar es el de guardarla.
	PlazoSieveGuardar = 30 * time.Second
)

var _ api.MailRespuestas = (*Buzon)(nil)

// ServidorSieve es el servidor de la respuesta automática de la cuenta: el que escribió
// la persona o, en una cuenta de un IMAP cualquiera, el host de IMAP en el 4190. Los
// proveedores conocidos no tienen ManageSieve: ok es false. Va con STARTTLS, y sin cifrar
// solo con un servidor de este equipo (la regla de `local/cuentas`, que abre el portero).
func (c Cuenta) ServidorSieve() (Servidor, bool) {
	s := Servidor{Host: strings.TrimSpace(c.Sieve.Host), Puerto: c.Sieve.Puerto, Seguridad: SeguridadSTARTTLS}
	if s.Host == "" {
		if c.Proveedor != ProveedorIMAP {
			return Servidor{}, false
		}
		s.Host = c.IMAP.Host
	}
	if s.Host == "" {
		return Servidor{}, false
	}
	if s.Puerto <= 0 || s.Puerto > 65535 {
		s.Puerto = PuertoSieve
	}
	if HostLocal(s.Host) {
		s.Seguridad = SeguridadNinguna
	}
	return s, true
}

// CuentasRespuesta son las cuentas que podrían tener respuesta automática, sin preguntar a
// ningún servidor.
func (b *Buzon) CuentasRespuesta() []api.MailAlias {
	var out []api.MailAlias
	for _, c := range b.reg.lista() {
		if _, ok := c.ServidorSieve(); ok {
			out = append(out, api.MailAlias{ID: c.ID, Address: c.Titulo()})
		}
	}
	return out
}

// LeerRespuesta lee del servidor de la cuenta su respuesta automática. Si la cuenta no
// tiene servidor Sieve, o no contesta como tal, o no admite `vacation`, el error es
// api.ErrMailSinRespuesta.
func (b *Buzon) LeerRespuesta(ctx context.Context, id string) (api.MailRespuesta, error) {
	c, ok := b.reg.cuenta(id)
	if !ok {
		return api.MailRespuesta{}, ErrSinCuenta
	}
	ctx, parar := context.WithTimeout(ctx, PlazoSieve)
	defer parar()
	cli, err := b.abrirSieve(ctx, c)
	if err != nil {
		return api.MailRespuesta{}, err
	}
	defer func() { _ = cli.Salir() }()
	st, err := sieve.LeerEstado(cli, NombreScriptSieve)
	if err != nil {
		return api.MailRespuesta{}, errorSieve(err)
	}
	r := api.MailRespuesta{
		Cuenta: c.ID, Direccion: c.Direccion, Activa: st.Activa, Asunto: st.Asunto, Mensaje: st.Mensaje,
		Dias: st.Dias, ConFechas: st.ConFechas, Bloqueada: st.Bloqueada, Reglas: st.Anterior,
	}
	if !st.Activa && st.Bloqueada == "" {
		r.Reglas = st.Otro
	}
	if !st.Desde.IsZero() {
		r.Desde = &st.Desde
	}
	if !st.Hasta.IsZero() {
		r.Hasta = &st.Hasta
	}
	return r, nil
}

// GuardarRespuesta la escribe en el servidor de la cuenta (y la activa o la apaga),
// desde la dirección de la cuenta y para sus direcciones. Las fechas valen por días.
func (b *Buzon) GuardarRespuesta(ctx context.Context, id string, r api.MailRespuesta) error {
	c, ok := b.reg.cuenta(id)
	if !ok {
		return ErrSinCuenta
	}
	ctx, parar := context.WithTimeout(ctx, PlazoSieveGuardar)
	defer parar()
	cli, err := b.abrirSieve(ctx, c)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Salir() }()
	sr := sieve.Respuesta{Asunto: r.Asunto, Mensaje: r.Mensaje, Dias: r.Dias, De: c.Direccion, Direcciones: direccionesDe(c)}
	if r.Desde != nil {
		sr.Desde = diaLocal(*r.Desde)
	}
	if r.Hasta != nil {
		sr.Hasta = diaLocal(*r.Hasta)
	}
	return errorSieve(sieve.Guardar(cli, NombreScriptSieve, sr, r.Activa))
}

// direccionesDe son las direcciones de la persona en la cuenta: la de la cuenta y el
// usuario de IMAP si es otra dirección de correo.
func direccionesDe(c Cuenta) []string {
	dirs := []string{c.Direccion}
	if u := strings.TrimSpace(c.IMAP.Usuario); strings.Contains(u, "@") && !strings.EqualFold(u, c.Direccion) {
		dirs = append(dirs, u)
	}
	return dirs
}

func diaLocal(t time.Time) time.Time {
	l := t.Local()
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.Local)
}

// abrirSieve conecta con el servidor Sieve de la cuenta por el portero, sube a TLS y
// entra con la contraseña de la cuenta (o su token OAuth). La contraseña solo se pide al
// llavero, y el token solo se renueva, cuando ya se sabe que al otro lado hay un servidor
// ManageSieve con cifrado.
func (b *Buzon) abrirSieve(ctx context.Context, c Cuenta) (*sieve.Cliente, error) {
	s, ok := c.ServidorSieve()
	if !ok {
		return nil, api.ErrMailSinRespuesta
	}
	conn, err := b.red.marcar(ctx, s)
	if err != nil {
		if errors.Is(err, ErrSinCifrar) {
			return nil, err
		}
		// No se llega (no hay nada en ese puerto, no contesta, el portero no lo deja o
		// «Sin conexión»): para la pantalla, la cuenta no la ofrece.
		return nil, conCausa(api.ErrMailSinRespuesta, "", err)
	}
	cli, err := sieve.Conectar(ctx, conn, sieve.Opciones{Host: s.Host, Raices: b.red.Raices, SinCifrar: s.Seguridad == SeguridadNinguna})
	if err != nil {
		return nil, errorSieve(err)
	}
	cred, err := b.credencial(ctx, c)
	if err != nil {
		_ = cli.Cerrar()
		return nil, err
	}
	usuario := c.usuario(c.IMAP)
	if cred.token != "" {
		err = cli.AutenticarToken(usuario, cred.token)
	} else {
		err = cli.Autenticar(usuario, cred.contrasena)
	}
	if err != nil {
		_ = cli.Cerrar()
		return nil, errorSieve(err)
	}
	return cli, nil
}

// errorSieve pasa los errores del cliente Sieve a los que entienden Mail y su pantalla.
func errorSieve(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sieve.ErrNoEsSieve), errors.Is(err, sieve.ErrSinVacaciones):
		return conCausa(api.ErrMailSinRespuesta, "", err)
	case errors.Is(err, sieve.ErrAcceso):
		return ErrAcceso
	case errors.Is(err, sieve.ErrSinTLS), errors.Is(err, sieve.ErrSinCifrar):
		return conCausa(ErrSinCifrar, "", err)
	}
	return err
}
