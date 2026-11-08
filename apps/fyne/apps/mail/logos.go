// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Logos es lo que el buzón necesita del servicio para el avatar de un remitente: el
// logo de la web de su dominio (`api.Mail.SenderLogo`), que el servicio busca,
// normaliza y guarda. Es una interfaz APARTE de Servicio porque es opcional: un
// servicio que no la tiene (los dobles de los guardianes, una cuenta externa) se
// queda con las iniciales.
type Logos interface {
	SenderLogo(ctx context.Context, dominio string) ([]byte, error)
}

var _ Logos = (*api.Mail)(nil)

// estadoLogo es lo que se sabe del logo de un dominio en esta sesión: nil mientras
// se pide o si no tiene. Un dominio se pide UNA vez por sesión: el servicio ya
// cachea días, y una web sin icono no tiene por qué contestar distinto en un rato.
type estadoLogo struct{ img image.Image }

// repintarLogos agrupa los logos que llegan seguidos —una bandeja nueva pide veinte
// a la vez— en un solo repintado.
const repintarLogos = 120 * time.Millisecond

// dominioCorreo es la parte de dominio de una dirección, en minúsculas, o "". No se
// normaliza aquí: el servicio lo hace (`corp.vodafone.es` → `vodafone.es`) y decide
// qué no tiene logo (los buzones personales gratuitos).
func dominioCorreo(correo string) string {
	i := strings.LastIndex(correo, "@")
	if i < 1 || i == len(correo)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(correo[i+1:]))
}

// enSpam es la vista de spam, donde NO se pintan logos: ahí abundan los remitentes
// suplantados, y el logo de un banco en un correo falso le presta una credibilidad
// que no tiene. Es el `enSpam` de MailApp.tsx.
func (b *Buzon) enSpam() bool {
	if b.externa != nil {
		return b.rolExterno() == "junk"
	}
	i := slices.IndexFunc(b.carpetas, func(m api.Mailbox) bool { return m.ID == b.carpetaID })
	return i >= 0 && b.carpetas[i].Role == "junk"
}

// LogoDe es el logo del remitente `correo` si ya ha llegado, o nil —y entonces se
// pide, y la bandeja y el lector se repintan cuando llegue—. Lo llaman la fila y la
// tarjeta del lector al pintarse.
func (b *Buzon) LogoDe(correo string) image.Image {
	s, ok := b.S.(Logos)
	if !ok || b.enSpam() {
		return nil
	}
	d := dominioCorreo(correo)
	if d == "" {
		return nil
	}
	if b.logos == nil {
		b.logos = map[string]*estadoLogo{}
	}
	if e, hay := b.logos[d]; hay {
		return e.img
	}
	e := &estadoLogo{}
	b.logos[d] = e
	pedir(b, func(ctx context.Context) (image.Image, error) {
		datos, err := s.SenderLogo(ctx, d)
		if err != nil {
			return nil, err
		}
		// El servicio siempre manda PNG: es él quien reescribe lo que llega.
		return png.Decode(bytes.NewReader(datos))
	}, func(img image.Image, err error) {
		if err != nil || img == nil {
			return
		}
		e.img = img
		b.repintarPorLogos()
	})
	return nil
}

// repintarPorLogos repinta la bandeja y el lector, una vez por tanda de logos.
func (b *Buzon) repintarPorLogos() {
	if b.pendienteLogos {
		return
	}
	b.pendienteLogos = true
	b.Esperar(repintarLogos, func() {
		b.pendienteLogos = false
		b.P.Bandeja.Refresh()
		b.P.Lector.Refresh()
	})
}
