// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	portero "git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Lo que un CORREO pide sacar a la red, siempre por un acto de la persona: darse de baja
// de una lista con un clic (RFC 8058) y cargar las imágenes remotas de un mensaje (M-16,
// M-18). Sale por el cliente que inyecta quien abre el buzón (Red.Web y Red.Baja: el
// permiso puntual del portero, `red.Portero.Puntual`, que solo llega a direcciones
// públicas de Internet: nunca a este equipo ni a la red de casa o de la oficina, y así un
// correo no puede hacer que la app llame a la impresora o al router).

// clienteFuera es el cliente HTTP de lo que pide un correo: Red.Web si se inyectó, o
// ninguno (R-RED): sin el portero, lo que pide un correo no sale.
func (b *Buzon) clienteFuera() *http.Client {
	if b.red.Web != nil {
		return b.red.Web
	}
	return &http.Client{Transport: portero.Denegar{}}
}

// ---- Baja de listas (List-Unsubscribe, RFC 2369 y RFC 8058) ----

// metodosBaja son los caminos de baja que trae un mensaje.
type metodosBaja struct {
	unClic string   // https para el POST de un clic (con List-Unsubscribe-Post)
	correo *url.URL // mailto:
}

func (b *Buzon) cabecera(ctx context.Context, id string) (sobre, mail.Header, error) {
	n, err := leerID(id)
	if err != nil {
		return sobre{}, nil, err
	}
	s, err := b.x.sobre(ctx, n)
	if err != nil {
		return sobre{}, nil, err
	}
	rc, err := b.Raw(ctx, id)
	if err != nil {
		return s, nil, err
	}
	defer rc.Close()
	m, err := mail.ReadMessage(io.LimitReader(rc, 1<<20))
	if err != nil {
		return s, nil, err
	}
	return s, m.Header, nil
}

func leerBaja(h mail.Header) metodosBaja {
	var out metodosBaja
	unClic := strings.EqualFold(strings.TrimSpace(h.Get("List-Unsubscribe-Post")), "List-Unsubscribe=One-Click")
	for _, trozo := range strings.Split(h.Get("List-Unsubscribe"), ",") {
		trozo = strings.TrimSpace(trozo)
		if !strings.HasPrefix(trozo, "<") || !strings.HasSuffix(trozo, ">") {
			continue
		}
		u, err := url.Parse(strings.TrimSpace(trozo[1 : len(trozo)-1]))
		if err != nil {
			continue
		}
		switch {
		case u.Scheme == "https" && u.Host != "" && unClic && out.unClic == "":
			out.unClic = u.String()
		case u.Scheme == "mailto" && u.Opaque != "" && out.correo == nil:
			if _, err := mail.ParseAddress(u.Opaque); err == nil {
				out.correo = u
			}
		}
	}
	return out
}

// Unsubscribe dice qué caminos de baja ofrece un mensaje.
func (b *Buzon) Unsubscribe(ctx context.Context, id string) (api.MailBaja, error) {
	_, h, err := b.cabecera(ctx, id)
	if err != nil {
		return api.MailBaja{}, err
	}
	m := leerBaja(h)
	return api.MailBaja{OneClick: m.unClic != "", Mailto: m.correo != nil}, nil
}

// DoUnsubscribe da de baja: el POST de un clic si lo hay y, si no o si falla, el correo
// de baja desde la cuenta que recibió el mensaje, por la bandeja de salida. Devuelve
// api.BajaHecha, api.BajaEnviada o api.BajaFallida.
func (b *Buzon) DoUnsubscribe(ctx context.Context, id string) (string, error) {
	s, h, err := b.cabecera(ctx, id)
	if err != nil {
		return api.BajaFallida, err
	}
	m := leerBaja(h)
	if m.unClic != "" {
		if b.bajaUnClic(ctx, m.unClic) == nil {
			return api.BajaHecha, nil
		}
	}
	if m.correo != nil {
		if err := b.bajaPorCorreo(ctx, s, m.correo); err != nil {
			return api.BajaFallida, nil //nolint:nilerr // que no salga el correo de baja ES la respuesta (BajaFallida), no un error de la llamada.
		}
		return api.BajaEnviada, nil
	}
	return api.BajaFallida, nil
}

func (b *Buzon) bajaUnClic(ctx context.Context, direccion string) error {
	ctx, parar := context.WithTimeout(ctx, 30*time.Second)
	defer parar()
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, direccion, strings.NewReader("List-Unsubscribe=One-Click"))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cliente := b.red.Baja
	if cliente == nil {
		cliente = b.clienteFuera()
	}
	resp, err := cliente.Do(r)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("correo: la baja contestó %d", resp.StatusCode)
	}
	return nil
}

func (b *Buzon) bajaPorCorreo(ctx context.Context, s sobre, u *url.URL) error {
	c, ok := b.reg.cuenta(s.cuenta)
	if !ok {
		var err error
		if c, err = b.cuentaDe(""); err != nil {
			return err
		}
	}
	q := u.Query()
	crudo, err := eml.Componer(eml.Salida{
		De: c.Direccion, DeNombre: c.Nombre, Para: []string{u.Opaque},
		Asunto: cmpOr(q.Get("subject"), "unsubscribe"), Texto: cmpOr(q.Get("body"), "unsubscribe"),
	}, time.Now())
	if err != nil {
		return err
	}
	_, err = b.Send(ctx, c.Direccion, []string{u.Opaque}, crudo, 0)
	return err
}

func cmpOr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// ---- Imágenes remotas (M-16) ----

// TopeImagen es lo más que se baja de una imagen remota.
const TopeImagen = 5 << 20

// ImagenRemota baja una imagen de un correo (http o https, a una dirección pública).
// Solo se llama cuando la persona ha pedido ver las imágenes de ese mensaje o de ese
// remitente: por defecto están bloqueadas, que es lo que protege de los píxeles espía.
func (b *Buzon) ImagenRemota(ctx context.Context, direccion string) ([]byte, error) {
	u, err := url.Parse(direccion)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, ErrNoDisponible
	}
	b.mu.Lock()
	if d, ok := b.imagenes[direccion]; ok {
		b.mu.Unlock()
		return d, nil
	}
	b.mu.Unlock()
	ctx, parar := context.WithTimeout(ctx, 30*time.Second)
	defer parar()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Accept", "image/*")
	resp, err := b.clienteFuera().Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("correo: la imagen contestó %d", resp.StatusCode)
	}
	datos, err := io.ReadAll(io.LimitReader(resp.Body, TopeImagen+1))
	if err != nil {
		return nil, err
	}
	if len(datos) > TopeImagen {
		return nil, errors.New("correo: la imagen es demasiado grande")
	}
	if !bytes.HasPrefix([]byte(http.DetectContentType(datos)), []byte("image/")) {
		return nil, errors.New("correo: no es una imagen")
	}
	b.mu.Lock()
	if len(b.imagenes) >= 256 {
		clear(b.imagenes) // sencillo: se rehace al volver a abrir
	}
	b.imagenes[direccion] = datos
	b.mu.Unlock()
	return datos, nil
}

// RemitenteConfiado dice si las imágenes remotas de esa dirección se enseñan siempre.
func (b *Buzon) RemitenteConfiado(ctx context.Context, direccion string) bool {
	var n int
	_ = b.x.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM confiados WHERE direccion = ?", strings.ToLower(strings.TrimSpace(direccion))).Scan(&n)
	return n > 0
}

// ConfiarRemitente enseña siempre las imágenes remotas de esa dirección (en este equipo).
func (b *Buzon) ConfiarRemitente(ctx context.Context, direccion string) error {
	d := strings.ToLower(strings.TrimSpace(direccion))
	if _, err := mail.ParseAddress(d); err != nil {
		return err
	}
	_, err := b.x.db.ExecContext(ctx, "INSERT OR IGNORE INTO confiados (direccion) VALUES (?)", d)
	return err
}
