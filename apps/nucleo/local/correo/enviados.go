// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
)

// La COPIA EN «ENVIADOS» de lo que mandan otras apps de la edición por el Remitente (la
// factura del ERP), en las cuentas cuyo servidor no la guarda solo al enviar por SMTP
// (iCloud, Yahoo, cualquier IMAP: las que no tienen GuardaEnviados).
//
// El Remitente no abre el buzón de Mail —su índice es de Mail, que puede estar abierto en
// otro proceso— ni hace un APPEND por su cuenta, que dejaría en el servidor un mensaje
// que el índice no sabe que es suyo. Deja lo enviado en una COLA COMPARTIDA, un `.eml`
// por mensaje en `<Datos>/Correo/enviados-por-subir/<cuenta>/` (escrito entero con un
// temporal y un renombrado: nunca se ve a medias), y Mail lo recoge al abrirse y, con la
// app abierta, en cada vuelta de su bucle: lo guarda en su «Enviados» como lo que envía
// ella (copiaEnviada: en el índice ya, y con APPEND en la próxima vuelta de esa cuenta) y
// lo quita de la cola. Recoger es idempotente: un mensaje cuyo Message-ID ya está en
// «Enviados» de su cuenta no se duplica (un corte entre guardarlo y quitarlo de la cola).

// carpetaColaEnviados es la cola, dentro de la carpeta del correo.
const carpetaColaEnviados = "enviados-por-subir"

// extensionEML es la de los mensajes de la cola. Partida para que el guardián de los
// catálogos no la tome por una clave de texto.
const extensionEML = "." + "eml"

func rutaColaEnviados(raiz, cuenta string) string {
	return filepath.Join(raiz, carpetaColaEnviados, filepath.Base(cuenta))
}

// dejarParaEnviados deja un mensaje ya enviado en la cola de su cuenta.
func dejarParaEnviados(raiz, cuenta string, crudo []byte) error {
	return biblioteca.EscribirAtomico(filepath.Join(rutaColaEnviados(raiz, cuenta), uuid.NewString()+extensionEML), crudo)
}

// recogerEnviados guarda en «Enviados» lo que dejaron otras apps en la cola y lo quita de
// ella. Una cuenta cuyas carpetas aún no se conocen (nunca se ha sincronizado) espera: su
// copia se quedaría en una carpeta solo de este equipo. Lo de una cuenta que ya no está
// se tira. Devuelve cuántos ha recogido.
func (b *Buzon) recogerEnviados(ctx context.Context) int {
	dirs, err := os.ReadDir(filepath.Join(b.raiz, carpetaColaEnviados))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("correo: no se pudo leer la cola de enviados", "error", err)
		}
		return 0
	}
	recogidos := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(b.raiz, carpetaColaEnviados, d.Name())
		c, ok := b.reg.cuenta(d.Name())
		if !ok {
			_ = os.RemoveAll(dir)
			continue
		}
		mensajes, err := os.ReadDir(dir)
		if err != nil || len(mensajes) == 0 || !b.x.conCarpetasDelServidor(ctx, c.ID) {
			continue
		}
		n := 0
		for _, m := range mensajes {
			nombre := m.Name()
			if m.IsDir() || strings.HasPrefix(nombre, ".") || !strings.HasSuffix(nombre, extensionEML) {
				continue // un temporal a medio escribir
			}
			ruta := filepath.Join(dir, nombre)
			crudo, err := os.ReadFile(ruta)
			if err != nil {
				continue
			}
			if err := b.copiaEnviadaUnaVez(ctx, c, crudo); err != nil {
				// Se queda en la cola: la vuelta siguiente lo intenta otra vez.
				slog.Warn("correo: no se pudo guardar en Enviados lo que envió otra app", "cuenta", c.Direccion, "error", err)
				continue
			}
			_ = os.Remove(ruta)
			n++
		}
		if n > 0 {
			recogidos += n
			b.despertarCuenta(c.ID) // el APPEND, ya
		}
	}
	if recogidos > 0 {
		b.avisar()
	}
	return recogidos
}

// copiaEnviadaUnaVez es copiaEnviada si el mensaje no está ya en «Enviados» de la cuenta.
func (b *Buzon) copiaEnviadaUnaVez(ctx context.Context, c Cuenta, crudo []byte) error {
	var s sobre
	sobreDeCabecera(&s, crudo)
	if s.messageID != "" {
		var ya bool
		err := b.x.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM mensajes m JOIN carpetas f ON f.cuenta = m.cuenta AND f.nombre = m.carpeta
			WHERE m.cuenta = ? AND m.message_id = ? AND f.rol = 'sent')`, c.ID, s.messageID).Scan(&ya)
		if err != nil {
			return err
		}
		if ya {
			return nil
		}
	}
	return b.copiaEnviada(ctx, c, crudo)
}

// conCarpetasDelServidor dice si ya se conocen las carpetas de la cuenta en su servidor
// (alguna sincronización ha ido bien).
func (x *indice) conCarpetasDelServidor(ctx context.Context, cuenta string) bool {
	var hay bool
	err := x.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM carpetas WHERE cuenta = ? AND local = 0)", cuenta).Scan(&hay)
	return err == nil && hay
}
