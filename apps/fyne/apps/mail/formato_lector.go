// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"  // las imágenes de los correos
	_ "image/jpeg" // las imágenes de los correos
	_ "image/png"  // las imágenes de los correos
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"golang.org/x/net/html"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El lector con formato (M-16) y las invitaciones (M-17)
//
// Son ADICIONES que solo enciende quien las pide (Sytue Office, `ConFormato` y
// `ConInvitaciones`); sin ellas la tarjeta es la de siempre, en texto.
//
// Con formato, el HTML del correo pasa por el saneador de la web (`SanearHTML`: lista
// blanca de etiquetas y estilos, enlaces solo http(s)/mailto, imágenes solo `https:` y
// `cid:`) y se pinta con el modelo de Notepads (`notes.Leer` y `notes.Prosa`), el mismo
// que ya pinta lo pegado en el redactor. Las imágenes `cid:` salen de las partes del
// propio mensaje; las REMOTAS están bloqueadas por defecto (`reader.remoteContentBlocked`)
// y se cargan por mensaje («Mostrar imágenes») o para siempre por remitente.
//
// Con invitaciones, un correo con su `text/calendar` lleva encima del cuerpo la tarjeta
// de `InvitacionCalendario.tsx`: qué, cuándo, dónde, quién convoca, y Sí/Quizá/No.

// ImagenesRemotas es lo que el servicio sabe hacer con las imágenes remotas de un
// correo. Va aparte de Servicio: sin él, las remotas no se cargan nunca.
type ImagenesRemotas interface {
	ImagenRemota(ctx context.Context, direccion string) ([]byte, error)
	RemitenteConfiado(ctx context.Context, direccion string) bool
	ConfiarRemitente(ctx context.Context, direccion string) error
}

// ResumenInvitacion es lo que la tarjeta enseña de una invitación.
type ResumenInvitacion struct {
	Resumen, Lugar, Organizador, NombreOrganizador string
	Inicio, Fin                                    time.Time
	TodoElDia, Contestable                         bool
}

// Invitaciones lee y contesta la invitación de un mensaje. Respuesta es el PARTSTAT
// (`ACCEPTED`, `TENTATIVE`, `DECLINED`); enviada, que el REPLY salió a quien organiza.
type Invitaciones interface {
	LeerInvitacion(ctx context.Context, id string) (ResumenInvitacion, bool, error)
	ContestarInvitacion(ctx context.Context, id, respuesta string) (enviada bool, err error)
}

// Las respuestas de la tarjeta, en su orden, con sus textos.
var respuestasInvitacion = []struct{ partstat, rotulo, hecho string }{
	{"ACCEPTED", "reader.invite.yes", "reader.invite.accepted"},
	{"TENTATIVE", "reader.invite.maybe", "reader.invite.tentative"},
	{"DECLINED", "reader.invite.no", "reader.invite.declined"},
}

// estadoFormato es lo que el Buzón guarda para el lector con formato.
type estadoFormato struct {
	verMensaje  map[string]bool        // mensajes cuyas imágenes remotas se ven
	confiados   map[string]bool        // remitente → se ven siempre (lo sabido hasta ahora)
	preguntados map[string]bool        // remitentes por los que ya se preguntó
	imagenes    map[string]image.Image // src (o id+cid) → imagen
	pedidas     map[string]bool
	// invitaciones son las leídas, por mensaje (nil: no trae); contestadas, lo elegido.
	invitaciones map[string]*ResumenInvitacion
	contestadas  map[string]string
	sinAviso     map[string]bool
	contestando  string
}

// ConFormato enciende el lector con formato. Sin él, la tarjeta es de texto.
func (b *Buzon) ConFormato() *Buzon {
	b.formato()
	l := b.P.Lector
	l.Imagen, l.ImagenesVisibles, l.AlVerImagenes = b.imagenDe, b.imagenesVisibles, b.verImagenes
	return b
}

// ConInvitaciones engancha las invitaciones del lector.
func (b *Buzon) ConInvitaciones(i Invitaciones) *Buzon {
	b.formato()
	b.Invitaciones = i
	l := b.P.Lector
	l.Invitacion, l.AlContestar = b.invitacionDe, b.contestar
	return b
}

func (b *Buzon) formato() *estadoFormato {
	if b.fmto == nil {
		b.fmto = &estadoFormato{
			verMensaje: map[string]bool{}, confiados: map[string]bool{}, preguntados: map[string]bool{},
			imagenes: map[string]image.Image{}, pedidas: map[string]bool{}, invitaciones: map[string]*ResumenInvitacion{},
			contestadas: map[string]string{}, sinAviso: map[string]bool{},
		}
	}
	return b.fmto
}

// conFormatoCuerpo completa el cuerpo recién leído con su HTML saneado, si el lector
// pinta formato, y pide su invitación si la trae.
func (b *Buzon) conFormatoCuerpo(id string, c Cuerpo, htmlOriginal string) Cuerpo {
	if b.P.Lector.Imagen != nil && strings.TrimSpace(htmlOriginal) != "" {
		c.HTML = SanearHTML(htmlOriginal)
	}
	if b.Invitaciones != nil {
		for _, a := range c.Adjuntos {
			if t := strings.ToLower(a.Tipo); t == "text/calendar" || t == "application/ics" {
				b.pedirInvitacion(id)
				break
			}
		}
	}
	return c
}

func remitente(m Mensaje) string {
	if len(m.De) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(m.De[0].Correo))
}

func (b *Buzon) imagenesVisibles(m Mensaje) bool {
	f := b.formato()
	if f.verMensaje[m.ID] {
		return true
	}
	d := remitente(m)
	if d == "" {
		return false
	}
	if ri, ok := b.S.(ImagenesRemotas); ok && !f.preguntados[d] {
		f.preguntados[d] = true
		pedir(b, func(ctx context.Context) (bool, error) { return ri.RemitenteConfiado(ctx, d), nil }, func(si bool, _ error) {
			if si {
				f.confiados[d] = true
				b.P.Lector.VersionImagenes++
				b.P.Lector.Refresh()
			}
		})
	}
	return f.confiados[d]
}

func (b *Buzon) verImagenes(m Mensaje, siempre bool) {
	f := b.formato()
	f.verMensaje[m.ID] = true
	if d := remitente(m); siempre && d != "" {
		f.confiados[d] = true
		if ri, ok := b.S.(ImagenesRemotas); ok {
			pedir(b, func(ctx context.Context) (struct{}, error) { return struct{}{}, ri.ConfiarRemitente(ctx, d) }, func(_ struct{}, err error) {
				if err == nil {
					b.avisar(ui.AvisoExito, "reader.trustedSenderAdded", i18n.Vars{"address": d})
				}
			})
		}
	}
	b.P.Lector.VersionImagenes++
	b.P.Lector.Refresh()
}

// imagenDe da la imagen de un `src` del cuerpo de m: una `cid:` de sus partes, o una
// remota ya bajada (remotas: si se ven). Lo que falta se pide y repinta al llegar.
func (b *Buzon) imagenDe(m Mensaje, src string, remotas bool) image.Image {
	f := b.formato()
	if cid, ok := strings.CutPrefix(src, "cid:"); ok {
		clave := m.ID + "\x00" + src
		if im, ya := f.imagenes[clave]; ya {
			return im
		}
		c := b.cuerpos[m.ID]
		var im image.Image
		for _, a := range c.Adjuntos {
			if strings.EqualFold(a.CID, cid) {
				im, _, _ = image.Decode(bytes.NewReader(a.Datos))
				break
			}
		}
		f.imagenes[clave] = im
		return im
	}
	if !remotas || !strings.HasPrefix(src, "https://") {
		return nil
	}
	if im, ya := f.imagenes[src]; ya {
		return im
	}
	ri, ok := b.S.(ImagenesRemotas)
	if !ok || f.pedidas[src] {
		return nil
	}
	f.pedidas[src] = true
	pedir(b, func(ctx context.Context) (image.Image, error) {
		datos, err := ri.ImagenRemota(ctx, src)
		if err != nil {
			return nil, err
		}
		im, _, err := image.Decode(bytes.NewReader(datos))
		return im, err
	}, func(im image.Image, err error) {
		if err == nil && im != nil {
			f.imagenes[src] = im
			b.P.Lector.VersionImagenes++
			b.P.Lector.Refresh()
		}
	})
	return nil
}

// RemotasDe son las imágenes `https:` que cita un HTML (ya saneado).
func RemotasDe(h string) []string {
	var out []string
	z := html.NewTokenizer(strings.NewReader(h))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return out
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			if t.Data != "img" {
				continue
			}
			for _, a := range t.Attr {
				if a.Key == "src" && strings.HasPrefix(a.Val, "https://") {
					out = append(out, a.Val)
				}
			}
		default:
			// El texto, los cierres, los comentarios y el doctype no traen imágenes.
		}
	}
}

func (b *Buzon) pedirInvitacion(id string) {
	f := b.formato()
	if _, ya := f.invitaciones[id]; ya {
		return
	}
	f.invitaciones[id] = nil
	pedir(b, func(ctx context.Context) (*ResumenInvitacion, error) {
		r, ok, err := b.Invitaciones.LeerInvitacion(ctx, id)
		if err != nil || !ok {
			return nil, err
		}
		return &r, nil
	}, func(r *ResumenInvitacion, _ error) {
		if r != nil {
			f.invitaciones[id] = r
			b.P.Lector.Refresh()
		}
	})
}

// invitacionDe es lo que la tarjeta de m enseña de su invitación: el resumen, la
// respuesta elegida, si falló el aviso y la que está en camino.
func (b *Buzon) invitacionDe(m Mensaje) (inv *ResumenInvitacion, elegida string, sinAviso bool, enCamino string) {
	f := b.formato()
	return f.invitaciones[m.ID], f.contestadas[m.ID], f.sinAviso[m.ID], f.contestando
}

func (b *Buzon) contestar(m Mensaje, respuesta string) {
	f := b.formato()
	if f.contestando != "" || f.contestadas[m.ID] == respuesta || b.Invitaciones == nil {
		return
	}
	f.contestando = respuesta
	b.P.Lector.Refresh()
	pedir(b, func(ctx context.Context) (bool, error) {
		return b.Invitaciones.ContestarInvitacion(ctx, m.ID, respuesta)
	}, func(enviada bool, err error) {
		f.contestando = ""
		if err != nil {
			b.avisar(ui.AvisoPeligro, "reader.invite.failed", nil)
		} else {
			f.contestadas[m.ID], f.sinAviso[m.ID] = respuesta, !enviada
		}
		b.P.Lector.Refresh()
	})
}

// ---- La tarjeta ----

// formatoTarjeta es el cuerpo con formato de una tarjeta, ya montado.
type formatoTarjeta struct {
	clave        string
	prosa        *notes.Prosa
	remotas      bool // hay imágenes remotas y están bloqueadas
	ancho, alto  float32
	ver, siempre *ui.Boton
	invitacion   fyne.CanvasObject
}

// conFormato es el cuerpo con formato de la tarjeta, o nil si se pinta en texto.
func (t *TarjetaMensaje) conFormato() *formatoTarjeta {
	l := t.L
	if !t.Expandida || l == nil {
		return nil
	}
	c, ya := l.cuerpoDe(t.Mensaje.ID)
	var inv *ResumenInvitacion
	var elegida, enCamino string
	var sinAviso bool
	if l.Invitacion != nil {
		inv, elegida, sinAviso, enCamino = l.Invitacion(t.Mensaje)
	}
	if (!ya || c.HTML == "" || l.Imagen == nil) && inv == nil {
		return nil
	}
	visibles := l.ImagenesVisibles != nil && l.ImagenesVisibles(t.Mensaje)
	clave := fmt.Sprint(len(c.HTML), visibles, l.VersionImagenes, inv != nil, elegida, sinAviso, enCamino)
	if t.fmtc != nil && t.fmtc.clave == clave {
		return t.fmtc
	}
	f := &formatoTarjeta{clave: clave}
	if ya && c.HTML != "" && l.Imagen != nil {
		m := t.Mensaje
		d := notes.Leer(c.HTML)
		d.Imagenes = func(src string) image.Image { return l.Imagen(m, src, visibles) }
		f.prosa = notes.NuevaProsa(d)
		f.prosa.AlEnlace = func(h string) {
			if l.AlAbrirEnlace != nil {
				l.AlAbrirEnlace(h)
			}
		}
		f.remotas = !visibles && len(RemotasDe(c.HTML)) > 0 && l.AlVerImagenes != nil
		if f.remotas {
			f.ver = ui.NuevoBoton(l.T.T("reader.remoteContentShow", nil), func() { l.AlVerImagenes(m, false) }).Con(ui.Secundario, ui.TallaSM)
			f.siempre = ui.NuevoBoton(l.T.T("reader.remoteContentAlways", nil), func() { l.AlVerImagenes(m, true) }).Con(ui.Fantasma, ui.TallaSM)
		}
	}
	if inv != nil {
		f.invitacion = t.tarjetaInvitacion(inv, elegida, sinAviso, enCamino)
	}
	t.fmtc = f
	return f
}

// altoProsa es el alto del cuerpo con formato a ese ancho (se recuerda el último).
func (f *formatoTarjeta) altoProsa(ancho float32) float32 {
	if f.prosa == nil {
		return 0
	}
	if f.ancho != ancho {
		f.ancho, f.alto = ancho, f.prosa.AltoPara(ancho)
	}
	return f.alto
}

// altoBotonesImagenes es la fila de «Mostrar imágenes» bajo el aviso.
func (f *formatoTarjeta) altoBotonesImagenes() float32 {
	if !f.remotas {
		return 0
	}
	return altoBotonCita() + bajoAviso
}

func (f *formatoTarjeta) altoInvitacion() float32 {
	if f.invitacion == nil {
		return 0
	}
	return f.invitacion.MinSize().Height + bajoAviso
}

// medidaFormato es medidaCuerpo para el cuerpo con formato: el aviso de las imágenes
// bloqueadas con sus botones, la invitación y la prosa.
func (t *TarjetaMensaje) medidaFormato(f *formatoTarjeta, util float32) (aviso []string, alto float32) {
	if f.remotas {
		aviso = ui.Repartir(t.L.T.T("reader.remoteContentBlocked", nil), tema.TextoSM, ui.Normal,
			util-2*tema.Borde-2*margenAvisoX-TallaGlifoLector-huecoAviso)
		alto += altoAviso(len(aviso)) + bajoAviso + f.altoBotonesImagenes()
	}
	alto += f.altoInvitacion()
	if f.prosa != nil {
		alto += f.altoProsa(util)
	} else {
		// Solo la invitación: el texto del correo va debajo, como siempre.
		texto, _, _, _, _ := t.cuerpo()
		alto += float32(len(repartirPre(texto, tema.TextoBase, ui.Normal, util))) * LineaCuerpo
	}
	return aviso, alto
}

// colocarFormato pone, desde y, los botones de imágenes, la invitación y la prosa.
// Devuelve dónde empiezan las líneas de texto (si no hay prosa).
func (f *formatoTarjeta) colocar(x, y, util float32) float32 {
	if f.remotas {
		mv, ms := f.ver.MinSize(), f.siempre.MinSize()
		f.ver.Move(fyne.NewPos(x, y))
		f.ver.Resize(fyne.NewSize(mv.Width, altoBotonCita()))
		f.siempre.Move(fyne.NewPos(x+mv.Width+tema.Espacio(2), y))
		f.siempre.Resize(fyne.NewSize(min(ms.Width, max(util-mv.Width-tema.Espacio(2), 0)), altoBotonCita()))
		y += f.altoBotonesImagenes()
	}
	if f.invitacion != nil {
		h := f.invitacion.MinSize().Height
		f.invitacion.Move(fyne.NewPos(x, y))
		f.invitacion.Resize(fyne.NewSize(util, h))
		y += h + bajoAviso
	}
	if f.prosa != nil {
		f.prosa.Move(fyne.NewPos(x, y))
		f.prosa.Resize(fyne.NewSize(util, f.altoProsa(util)))
	}
	return y
}

// objetos son los de la tarjeta con formato, en el orden en que se pintan.
func (f *formatoTarjeta) objetos() []fyne.CanvasObject {
	var out []fyne.CanvasObject
	if f.remotas {
		out = append(out, f.ver, f.siempre)
	}
	if f.invitacion != nil {
		out = append(out, f.invitacion)
	}
	if f.prosa != nil {
		out = append(out, f.prosa)
	}
	return out
}

// tarjetaInvitacion es `InvitacionCalendario`: el resumen y Sí/Quizá/No.
func (t *TarjetaMensaje) tarjetaInvitacion(inv *ResumenInvitacion, elegida string, sinAviso bool, enCamino string) fyne.CanvasObject {
	l := t.L
	tr := l.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	titulo := inv.Resumen
	if titulo == "" {
		titulo = tr.T("reader.invite.untitled", nil)
	}
	filas := []fyne.CanvasObject{
		ui.Texto(strings.ToUpper(tr.T("reader.invite.label", nil)), tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v)),
		ui.Texto(titulo, tema.TextoBase, ui.SemiNegrita, tema.Color("text", v)),
	}
	if cuando := cuandoInvitacion(tr, l.Locale, inv); cuando != "" {
		filas = append(filas, ui.Texto(cuando, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)))
	}
	if inv.Lugar != "" {
		filas = append(filas, ui.Texto(inv.Lugar, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)))
	}
	quien := inv.NombreOrganizador
	if quien == "" {
		quien = inv.Organizador
	}
	if quien != "" {
		filas = append(filas, ui.Texto(tr.T("reader.invite.organizer", i18n.Vars{"name": quien}), tema.TextoSM, ui.Normal, tema.Color("text-subtle", v)))
	}
	if inv.Contestable && l.AlContestar != nil {
		botones := make([]fyne.CanvasObject, 0, 1+len(respuestasInvitacion))
		botones = append(botones, ui.Texto(tr.T("reader.invite.question", nil), tema.TextoSM, ui.Medio, tema.Color("text", v)))
		for _, r := range respuestasInvitacion {
			variante := ui.Secundario
			if elegida == r.partstat {
				variante = ui.Primario
			}
			m, ps := t.Mensaje, r.partstat
			bt := ui.NuevoBoton(tr.T(r.rotulo, nil), func() { l.AlContestar(m, ps) }).Con(variante, ui.TallaSM)
			bt.Cargando, bt.Inerte = enCamino == r.partstat, enCamino != ""
			botones = append(botones, bt)
		}
		filas = append(filas, container.New(ui.Fila{Hueco: tema.Espacio(2)}, botones...))
	}
	for _, r := range respuestasInvitacion {
		if elegida == r.partstat {
			estado := tr.T(r.hecho, nil)
			if sinAviso {
				estado += " " + tr.T("reader.invite.replyNotSent", nil)
			}
			filas = append(filas, ui.Texto(estado, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)))
		}
	}
	fondo := canvas.NewRectangle(tema.Color("surface-raised", v))
	fondo.StrokeColor, fondo.StrokeWidth, fondo.CornerRadius = tema.Color("border", v), tema.Borde, tema.RadioLG
	return container.NewStack(fondo, container.NewPadded(container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...)))
}

// cuandoInvitacion es `cuando` de la web: la fecha larga y, si es de día entero, «Todo
// el día»; si no, con la hora de fin.
func cuandoInvitacion(tr *i18n.Translator, l i18n.Locale, inv *ResumenInvitacion) string {
	if inv.Inicio.IsZero() {
		return ""
	}
	if inv.TodoElDia {
		return FechaLarga(l, inv.Inicio) + " · " + tr.T("reader.invite.allDay", nil)
	}
	s := FechaLarga(l, inv.Inicio.In(time.Local))
	if !inv.Fin.IsZero() {
		s += "–" + inv.Fin.In(time.Local).Format("15:04")
	}
	return s
}
