// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Escribir, responder y reenviar (MailApp.tsx + ComposeModal.tsx), en el Buzón

// Remitente es lo que el redactor necesita del servidor: la firma, las direcciones
// «De», y enviar, guardar y deshacer. Lo cumple *api.Mail.
type Remitente interface {
	Signature(ctx context.Context) (string, error)
	SetSignature(ctx context.Context, firma string) error
	Aliases(ctx context.Context) (principal string, alias []api.MailAlias, err error)
	Send(ctx context.Context, de string, destinatarios []string, crudo []byte, undoSegundos int) (api.MailEnvio, error)
	SaveDraft(ctx context.Context, id, de string, crudo []byte) (api.MailMessage, error)
	CancelOutbound(ctx context.Context, id string) error
	// Programar: el envío con hora, la lista de pendientes y el cambio de hora.
	Schedule(ctx context.Context, de string, destinatarios []string, crudo []byte, cuando time.Time) (api.MailEnvio, error)
	Scheduled(ctx context.Context) ([]api.MailScheduled, error)
	Reschedule(ctx context.Context, id string, cuando time.Time) error
}

var _ Remitente = (*api.Mail)(nil)

// redaccion es el redactor abierto con su estado de trabajo.
type redaccion struct {
	r                   *Redactor
	d                   *ui.Dialogo
	guardar, enviar     *ui.Boton
	descartar           *ui.Boton
	enviando, guardando bool
	pararAuto           func()
	// La libreta se pide una vez por redacción (`libreta !== null || fallo`).
	libreta        []EntradaLibreta
	falloLibreta   bool
	libretaAbierta *dialogoLibreta
}

// cargarIdentidad pide la firma y las direcciones «De». Best-effort, como en la web:
// sin ellas la bandeja sigue.
func (b *Buzon) cargarIdentidad() {
	pedir(b, b.S.Signature, func(f string, err error) {
		if err == nil {
			b.firma = f
		}
	})
	pedir(b, func(ctx context.Context) ([]string, error) {
		p, a, err := b.S.Aliases(ctx)
		ds := make([]string, 0, 1+len(a))
		ds = append(ds, p)
		for _, x := range a {
			ds = append(ds, x.Address)
		}
		return ds, err
	}, func(ds []string, err error) {
		if err != nil || len(ds) == 0 {
			return
		}
		b.principal, b.alias = ds[0], ds[1:]
	})
}

// opcionesDe es `fromOptions`: la principal, los alias y los buzones compartidos o
// delegados con permiso de envío, sin repetir ni vacíos.
func (b *Buzon) opcionesDe() []string {
	var out []string
	todas := append([]string{b.principal}, b.alias...)
	for _, x := range b.compartidos {
		if x.Permission != "read" {
			todas = append(todas, x.Address)
		}
	}
	for _, d := range todas {
		if d != "" && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// Redactar es «Redactar»: un mensaje nuevo con la firma debajo.
//
// En un buzón compartido con permiso de envío, sale desde su dirección (`openCompose`).
func (b *Buzon) Redactar() {
	b.abrirRedactor(Borrador{De: b.deCompartido(), Cuerpo: ConFirma("", b.firma, false)})
}

// responder es `handleReply`: con el cuerpo COMPLETO si ya se leyó —si no, la cita
// saldría cortada en el extracto—.
func (b *Buzon) responder(m Mensaje, modo Respuesta) {
	completo := ""
	if c, ya := b.cuerpos[m.ID]; ya {
		completo = c.Texto
	}
	br := BorradorDeRespuesta(b.P.T, b.P.Locale, m, modo, b.principal, completo)
	br.Cuerpo = ConFirma(br.Cuerpo, b.firma, false)
	if de := b.deCompartido(); de != "" {
		br.De = de
	}
	b.abrirRedactor(br)
}

// abrirBorrador es `openDraft`: en Borradores, abrir una fila reabre el redactor con
// su contenido completo (del .eml: el sobre no lleva la copia oculta). Si el .eml no
// baja, se abre con el sobre, que es mejor que no poder abrirlo.
func (b *Buzon) abrirBorrador(m Mensaje) {
	pedir(b, func(ctx context.Context) (eml.Mensaje, error) {
		r, err := b.S.Raw(ctx, m.ID)
		if err != nil {
			return eml.Mensaje{}, err
		}
		defer r.Close()
		return eml.Leer(r)
	}, func(x eml.Mensaje, err error) {
		br := Borrador{ID: m.ID}
		if err != nil && x.Asunto == "" && x.Texto == "" {
			para := make([]string, 0, len(m.Para))
			for _, d := range m.Para {
				para = append(para, d.Correo)
			}
			br.Para, br.Asunto, br.Cuerpo = strings.Join(para, ", "), m.Asunto, m.Extracto
			b.abrirRedactor(br)
			return
		}
		br.Para, br.Cc, br.Cco = strings.Join(x.Para, ", "), strings.Join(x.Cc, ", "), strings.Join(x.Cco, ", ")
		br.Asunto, br.Cuerpo, br.HTML = x.Asunto, strings.ReplaceAll(x.Texto, "\r\n", "\n"), x.HTML
		if strings.TrimSpace(br.Cuerpo) == "" && x.HTML != "" {
			br.Cuerpo = htmltext.Text(x.HTML)
		}
		br.Adjuntos, br.EnRespuestaA, br.Referencias = adjuntosDe(x.Adjuntos), x.EnRespuestaA, x.Referencias
		b.abrirRedactor(br)
	})
}

// abrirRedactor monta el diálogo. Solo uno a la vez: el segundo sustituye al primero,
// que se guarda antes si tenía algo.
func (b *Buzon) abrirRedactor(br Borrador) {
	b.P.CerrarCajon()
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return
	}
	if b.redaccion != nil {
		b.cerrarGuardando(b.redaccion)
	}
	t := b.P.T
	ancho := AnchoCuerpoDialogo(ui.DialogoLG, lienzo.Size().Width)
	rd := &redaccion{r: NuevoRedactorEn(t, b.P.Locale, br, b.opcionesDe(), ancho, lienzo.Size().Width)}
	// Las herramientas y «Guardar borrador» son `Button variant="ghost" iconOnly`
	// de talla por defecto (31,5); «Enviar» conserva el rótulo y la jerarquía principal.
	rd.descartar = ui.NuevoBoton(t.T("compose.discard", nil), func() { b.descartar(rd) }).Con(ui.Fantasma, ui.TallaMD)
	rd.descartar.Rotulo, rd.descartar.SoloIcono, rd.descartar.TamIcono = rd.descartar.Texto, true, TallaGlifoLector
	rd.descartar.Glifo = Glifo(PapeleraD, TallaGlifoLector, "text", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	rd.guardar = ui.NuevoBoton(t.T("compose.saveDraft", nil), func() { b.guardarBorrador(rd, false, true) }).
		Con(ui.Fantasma, ui.TallaMD)
	rd.guardar.Rotulo, rd.guardar.SoloIcono, rd.guardar.Icono, rd.guardar.TamIcono =
		rd.guardar.Texto, true, iconos.Guardar, TallaGlifoLector
	rd.enviar = ui.NuevoBoton(t.T("compose.send", nil), func() { b.enviar(rd, false) }).Con(ui.Primario, ui.TallaMD)
	// El reloj es un conmutador (`aria-expanded`, y `bg-selected` mientras el panel está
	// abierto): por eso es el botón de icono del cromo, que sí sabe estar pulsado, con
	// el lado de un `iconOnly` de talla por defecto.
	reloj := ui.NuevoBotonIcono(iconos.Ninguno, t.T("compose.schedule", nil), nil)
	reloj.Glifo = Glifo(RelojD, TallaGlifoLector, "text", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	reloj.Lado, reloj.TamIcono, reloj.Radio = ui.AltoCampo(ui.CampoMD), TallaGlifoLector, tema.Forma("radius-control")
	reloj.Trazo, reloj.TrazoEncima, reloj.FondoEncima, reloj.FondoActivo = "text", "", "bg-subtle", "border"
	reloj.AlPulsar = func() {
		rd.r.AlternarProgramar()
		reloj.Pulsado = rd.r.verProgramar
		reloj.Refresh()
	}
	rd.r.AlProgramar = func(cuando time.Time, ok bool) { b.programar(rd, cuando, ok) }
	// «Adjuntar» es un menú que se abre HACIA ARRIBA (`side="top"`): vive en el pie.
	adjuntar := ui.NuevoBoton(t.T("compose.attach", nil), nil).Con(ui.Fantasma, ui.TallaMD)
	adjuntar.Rotulo, adjuntar.SoloIcono, adjuntar.Icono, adjuntar.TamIcono = adjuntar.Texto, true, iconos.Mas, TallaGlifoLector
	adjuntar.AlPulsar = func() { b.menuAdjuntar(rd).Mostrar(adjuntar) }
	// «Aa» es «Formato del texto»: pliega o despliega la barra, desplegada de serie y
	// recordada (en la web, por navegador; aquí, en las preferencias de la app).
	formato := ui.NuevoBotonIcono(iconos.Texto, t.T("compose.formatting", nil), nil)
	formato.Lado, formato.TamIcono, formato.Radio = ui.AltoCampo(ui.CampoMD), TallaGlifoLector, tema.Forma("radius-control")
	formato.Trazo, formato.TrazoEncima, formato.FondoEncima, formato.FondoActivo = "text", "", "bg-subtle", "border"
	rd.r.VerFormato = b.formatoVisible()
	formato.Pulsado = rd.r.VerFormato
	formato.AlPulsar = func() {
		rd.r.AlternarFormato()
		formato.Pulsado = rd.r.VerFormato
		formato.Refresh()
		b.recordarFormato(rd.r.VerFormato)
	}
	pie := &pieRedactor{
		ancho: ancho, izquierda: []fyne.CanvasObject{formato, adjuntar, reloj, rd.descartar},
		derecha: []fyne.CanvasObject{rd.guardar, rd.enviar},
	}
	rd.d = ui.NuevoDialogo(TituloRedactor(t, br.Asunto, br.ID != ""), rd.r.Objeto()).
		Con(ui.DialogoLG, container.New(pie, formato, adjuntar, reloj, rd.descartar, rd.guardar, rd.enviar))
	rd.d.RotuloCerrar = t.T("ui.close", nil)
	// Clic fuera y Escape GUARDAN y cierran; el aspa pregunta.
	rd.d.AlCerrar = func() { b.cerrarGuardando(rd) }
	rd.d.AlPulsarAspa = func() { b.aspaRedactor(rd) }
	rd.d.Foco = rd.r.Foco()
	rd.r.AlCambiar = func() { b.programarAutoguardado(rd) }
	rd.r.AlEnviar = func() { b.enviar(rd, false) }
	rd.r.AlPegar = func(texto string) { b.pegar(rd, texto) }
	if b.ElegirImagenes != nil {
		rd.r.AlImagenDelEquipo = func() { b.imagenDelEquipo(rd) }
	}
	if b.Asistir != nil {
		rd.r.ConAsistente(b.Asistir, string(b.P.Locale))
	}
	if b.Libreta != nil {
		rd.r.AlLibreta = func(c *CampoDirecciones) { b.verLibreta(rd, c) }
		rd.r.ConLibreta(true)
	}
	buscadores := map[*CampoDirecciones]func(string){}
	rd.r.AlBuscar = func(c *CampoDirecciones, q string) {
		if buscadores[c] == nil {
			buscadores[c] = b.sugeridor(c)
		}
		buscadores[c](q)
	}
	// El diálogo se mide al abrirse: si la columna crece, hay que volver a medirlo, o el
	// cuerpo se sale por debajo del pie.
	rd.r.AlCrecer = rd.d.Recolocar
	b.redaccion = rd
	rd.d.Mostrar(lienzo)
}

// menuAdjuntar son las dos vías de adjuntar: del equipo y de Drive.
//
// ponytail: «Adjuntar de Drive» se ofrece solo si la app tiene Drive (`ConDrive`).
func (b *Buzon) menuAdjuntar(rd *redaccion) *ui.Menu {
	t := b.P.T
	elementos := []fyne.CanvasObject{
		ui.NuevoElementoMenu(t.T("compose.attachFromDevice", nil), iconos.Mas, func() { b.adjuntarDelEquipo(rd) }),
	}
	if b.Explorador != nil {
		elementos = append(elementos,
			ui.NuevoElementoMenu(t.T("compose.attachFromDrive", nil), iconos.Rejilla, func() { b.elegirDeDrive(rd) }))
	}
	m := ui.NuevoMenu(elementos...)
	m.Rotulo, m.AlInicio, m.Arriba = t.T("compose.attach", nil), true, true
	return m
}

// adjuntarDelEquipo es `handleFilesChosen`: el panel de abrir del sistema, y cada
// fichero leído entero al borrador. Cancelar no dice nada. El mensaje que no cabe en
// el JSON de 1 MiB del servicio sale subido aparte (`api.Mail` lo decide al enviar).
//
// ponytail: sin el tope por fichero de la web (2 GB, `MAX_ATTACHMENT_MB`), que es el
// mismo `MAX_MESSAGE_MB` del servicio: lo dice el envío si se pasa.
func (b *Buzon) adjuntarDelEquipo(rd *redaccion) {
	if b.ElegirFicheros == nil {
		return
	}
	pedir(b, func(context.Context) ([]eml.Adjunto, error) {
		rutas, err := b.ElegirFicheros()
		if err != nil {
			return nil, err
		}
		return leerAdjuntos(rutas)
	}, func(as []eml.Adjunto, err error) {
		if err != nil {
			b.avisar(ui.AvisoPeligro, "compose.attachFailed", nil)
			return
		}
		rd.r.Adjuntar(as...)
	})
}

// imagenDelEquipo es «Desde este dispositivo» del panel de imagen: el selector del
// sistema filtrado a imágenes (`accept="image/*"`) y cada una elegida, dentro del texto
// donde está el cursor. Cancelar no dice nada.
//
// ponytail: sin el tope de `MAX_IMAGEN_EMBEBIDA`, que es el mismo de un adjunto (el
// de `adjuntarDelEquipo`): lo dice el envío si se pasa.
func (b *Buzon) imagenDelEquipo(rd *redaccion) {
	pedir(b, func(context.Context) ([]eml.Adjunto, error) {
		rutas, err := b.ElegirImagenes()
		if err != nil {
			return nil, err
		}
		return leerAdjuntos(rutas)
	}, func(as []eml.Adjunto, err error) {
		if err != nil {
			b.avisar(ui.AvisoPeligro, "compose.attachFailed", nil)
			return
		}
		if b.redaccion != rd {
			return // el redactor se cerró mientras
		}
		for _, a := range as {
			if strings.HasPrefix(a.Tipo, "image/") {
				rd.r.InsertarImagen(a)
			}
		}
	})
}

// leerAdjuntos lee cada ruta como `fileToAttachment`: su nombre, su tipo (por la
// extensión; si no se sabe, `application/octet-stream`) y su contenido.
func leerAdjuntos(rutas []string) ([]eml.Adjunto, error) { return leerAdjuntosConLimite(rutas, 0) }

func leerAdjuntosConLimite(rutas []string, limite int64) ([]eml.Adjunto, error) {
	as := make([]eml.Adjunto, 0, len(rutas))
	for _, r := range rutas {
		f, err := os.Open(r)
		if err != nil {
			return nil, err
		}
		var reader io.Reader = f
		if limite > 0 {
			reader = io.LimitReader(f, limite+1)
		}
		datos, err := io.ReadAll(reader)
		_ = f.Close()
		if limite > 0 && int64(len(datos)) > limite {
			return nil, errors.New("attachment_size")
		}
		if limite > 0 {
			limite -= int64(len(datos))
			if limite == 0 && len(as)+1 < len(rutas) {
				return nil, errors.New("attachment_size")
			}
		}
		if err != nil {
			return nil, err
		}
		tipo := mime.TypeByExtension(filepath.Ext(r))
		if tipo == "" {
			tipo = "application/octet-stream"
		}
		as = append(as, eml.Adjunto{Nombre: filepath.Base(r), Tipo: tipo, Datos: datos})
	}
	return as, nil
}

// cerrar retira el redactor sin más.
func (b *Buzon) cerrar(rd *redaccion) {
	if rd.pararAuto != nil {
		rd.pararAuto()
	}
	rd.d.Cerrar()
	if b.redaccion == rd {
		b.redaccion = nil
	}
}

// cerrarGuardando es `handleClose`: lo escrito se guarda como borrador y se cierra.
//
// No espera a que el guardado acabe, a diferencia de la web: el diálogo se va ya y el
// borrador se escribe detrás. Y si falla, SE DICE —la web calla en este camino—,
// porque cerrar dando por guardado lo que no está en ningún sitio es perderlo.
func (b *Buzon) cerrarGuardando(rd *redaccion) {
	if !b.redaccionVigente(rd) {
		return
	}
	if !rd.enviando && !rd.guardando && rd.r.B.ConContenido() {
		b.guardarBorrador(rd, false, false)
	}
	b.cerrar(rd)
}

// aspaRedactor es `handleCloseButtonClick`: lo único que ofrece NO guardar, así que
// pregunta. Descartar borra también el borrador que el autoguardado ya había dejado.
func (b *Buzon) aspaRedactor(rd *redaccion) {
	if !b.redaccionVigente(rd) {
		return
	}
	if rd.enviando || rd.guardando || !rd.r.B.ConContenido() {
		b.cerrar(rd)
		return
	}
	t := b.P.T
	lienzo := b.P.Lienzo()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(t.T("compose.discardBody", nil), tema.TextoSM, ui.Normal, "text", v).
		A(AnchoCuerpoDialogo(ui.DialogoSM, lienzo.Size().Width))
	var d *ui.Dialogo
	d = ui.Confirmacion(t.T("compose.discardTitle", nil), t.T("common.cancel", nil), t.T("compose.discardConfirm", nil), true,
		cuerpo, func() { d.Cerrar() }, func() {
			d.Cerrar()
			if !b.redaccionVigente(rd) {
				return
			}
			if id := rd.r.B.ID; id != "" {
				ctx := b.ctx
				b.Lanzar(func() {
					if ctx.Err() == nil {
						_ = b.S.Delete(ctx, id)
					}
				})
			}
			b.cerrar(rd)
		})
	d.RotuloCerrar = t.T("ui.close", nil)
	d.Mostrar(lienzo)
}

// descartar es la papelera del pie: cierra SIN guardar. El borrador que ya estuviera en
// Borradores se queda (la web tampoco lo borra desde aquí).
func (b *Buzon) descartar(rd *redaccion) { b.cerrar(rd) }

// programarAutoguardado es el autoguardado en vivo: dos segundos sin tocar nada y el
// borrador ya está en el servidor.
func (b *Buzon) programarAutoguardado(rd *redaccion) {
	if rd.pararAuto != nil {
		rd.pararAuto()
		rd.pararAuto = nil
	}
	if !b.redaccionVigente(rd) || rd.enviando || rd.guardando || !rd.r.B.ConContenido() {
		return
	}
	ctx, ciclo := b.ctx, b.ciclo
	rd.pararAuto = b.Esperar(EsperaAutoguardado, func() {
		// Stop no retira un callback que el reloj ya entregó al hilo de la interfaz.
		if ctx.Err() == nil && ciclo == b.ciclo && b.redaccionVigente(rd) && !rd.enviando {
			//nolint:contextcheck // La vida capturada ya está validada; guardarBorrador adopta ese mismo b.ctx mediante pedir.
			b.guardarBorrador(rd, true, false)
		}
	})
}

// redaccionVigente descarta eventos ya encolados de un diálogo retirado, incluso
// cuando otro redactor se abrió en la misma cuenta.
func (b *Buzon) redaccionVigente(rd *redaccion) bool {
	return rd != nil && b.redaccion == rd && b.ctx != nil && b.ctx.Err() == nil
}

// direccionesLaxas lee lo escrito sin exigir que sea válido: un borrador puede estar a
// medias (`parseAddresses` de la web).
func direccionesLaxas(s string) []string {
	fichas, actual := PartirDirecciones(s + ",")
	_ = actual
	out := make([]string, 0, len(fichas))
	for _, f := range fichas {
		if _, correo := leerFicha(f); correo != "" {
			out = append(out, correo)
		}
	}
	return out
}

// componer escribe el .eml. Un borrador admite direcciones a medias; un envío no.
func (b *Buzon) componer(br Borrador, borrador bool) (de string, s eml.Salida, crudo []byte, err error) {
	de = br.De
	if de == "" {
		de = b.principal
	}
	s = eml.Salida{
		De: de, Asunto: br.Asunto, Texto: TextoConDrive(b.P.T, br.Cuerpo, br.Drive), Adjuntos: br.Adjuntos,
		EnRespuestaA: br.EnRespuestaA, Referencias: br.Referencias, Borrador: borrador,
	}
	if br.HTML != "" {
		s.HTML = HTMLConDrive(b.P.T, br.HTML, br.Drive)
	}
	if borrador {
		s.Para, s.Cc, s.Cco = direccionesLaxas(br.Para), direccionesLaxas(br.Cc), direccionesLaxas(br.Cco)
	} else {
		for _, par := range []struct {
			de string
			a  *[]string
		}{{br.Para, &s.Para}, {br.Cc, &s.Cc}, {br.Cco, &s.Cco}} {
			if *par.a, err = eml.Direcciones(par.de); err != nil {
				return de, s, nil, errDireccion
			}
		}
	}
	crudo, err = eml.Componer(s, time.Now())
	return de, s, crudo, err
}

var errDireccion = errors.New("redactor: dirección no válida")

// guardarBorrador es `doSave`. Silencioso es el autoguardado: no interrumpe con un
// aviso por cada tecla, ya lo intentará el siguiente. Con `yCerrar`, es el botón
// «Guardar borrador»: avisa y cierra, y si falla se queda abierto.
func (b *Buzon) guardarBorrador(rd *redaccion, silencioso, yCerrar bool) {
	if !b.redaccionVigente(rd) || rd.guardando || rd.enviando {
		return
	}
	br := rd.r.B
	de, _, crudo, err := b.componer(br, true)
	if err != nil {
		if !silencioso {
			b.avisar(ui.AvisoPeligro, "compose.draftFailed", nil)
		}
		return
	}
	rd.guardando = true
	b.pintarBotonesRedactor(rd)
	pedir(b, func(ctx context.Context) (api.MailMessage, error) {
		return b.S.SaveDraft(ctx, br.ID, de, crudo)
	}, func(m api.MailMessage, err error) {
		rd.guardando = false
		b.pintarBotonesRedactor(rd)
		if err != nil {
			if !silencioso {
				b.avisar(ui.AvisoPeligro, "compose.draftFailed", nil)
			}
			return
		}
		rd.r.B.ID = m.ID // el servicio SUSTITUYE el borrador y le da otro id
		b.alGuardarBorrador()
		if yCerrar {
			b.avisar(ui.AvisoExito, "compose.draftSaved", nil)
			b.cerrar(rd)
		}
	})
}

// alGuardarBorrador es `onSaved`: Borradores ha cambiado. Se recarga la lista solo si
// es la que se está mirando —la web la recarga siempre, y con ello cierra el hilo que
// se estaba contestando—, y siempre los números del lateral.
func (b *Buzon) alGuardarBorrador() {
	if b.rolVista() == "drafts" {
		b.cargarLista()
	}
	b.refrescarContadores()
}

func (b *Buzon) pintarBotonesRedactor(rd *redaccion) {
	rd.guardar.Cargando, rd.guardar.Inerte = rd.guardando, rd.enviando
	rd.enviar.Cargando, rd.enviar.Inerte = rd.enviando, rd.guardando
	rd.descartar.Inerte = rd.enviando || rd.guardando
	for _, x := range []*ui.Boton{rd.guardar, rd.enviar, rd.descartar} {
		x.Refresh()
	}
}

// enviar es `handleSend`: valida «Para» en línea, pregunta si se menciona un adjunto
// que no va, y envía con la ventana de deshacer.
func (b *Buzon) enviar(rd *redaccion, sinAdjuntoConfirmado bool) {
	if !b.redaccionVigente(rd) || rd.enviando || rd.guardando {
		return
	}
	t := b.P.T
	br := rd.r.B
	if len(direccionesLaxas(br.Para)) == 0 {
		// Validación EN LÍNEA, con icono y texto en la fila, no un aviso suelto.
		rd.r.PonerError(t.T("compose.toRequired", nil))
		if c := fyne.CurrentApp().Driver().CanvasForObject(rd.r.para); c != nil {
			c.Focus(rd.r.para.Foco())
		}
		return
	}
	// Los de Drive también cuentan (`attachments.length + files.length` en la web).
	if !sinAdjuntoConfirmado && len(br.Adjuntos)+len(br.Drive) == 0 && MencionaAdjunto(br.Cuerpo) {
		b.preguntarSinAdjunto(rd)
		return
	}
	de, s, crudo, err := b.componer(br, false)
	if err != nil {
		b.avisarEnvio(err)
		return
	}
	if rd.pararAuto != nil {
		rd.pararAuto()
	}
	rd.enviando = true
	b.pintarBotonesRedactor(rd)
	pedir(b, func(ctx context.Context) (api.MailEnvio, error) {
		return b.S.Send(ctx, de, s.Destinatarios(), crudo, VentanaDeshacer)
	}, func(env api.MailEnvio, err error) {
		rd.enviando = false
		if err != nil {
			b.pintarBotonesRedactor(rd)
			b.avisarEnvio(err)
			return
		}
		if id := br.ID; id != "" { // el borrador reabierto ya no hace falta
			ctx := b.ctx
			b.Lanzar(func() {
				if ctx.Err() == nil {
					_ = b.S.Delete(ctx, id)
				}
			})
		}
		b.cerrar(rd)
		b.alGuardarBorrador()
		a := ui.NuevoAviso(ui.AvisoExito, t.T("compose.sent", nil), "")
		a.RotuloCerrar = t.T("ui.dismissNotification", nil)
		if env.ID != "" {
			a.Descripcion = t.T("compose.sendIn", i18n.Vars{"count": VentanaDeshacer})
			a.Duracion = VentanaDeshacer * time.Second
			a.RotuloAccion = t.T("compose.undo", nil)
			reabrir := br
			reabrir.ID = ""
			a.AlPulsarAccion = func() { b.deshacerEnvio(env.ID, reabrir) }
		}
		b.P.Notificar(a)
	})
}

// avisarEnvio dice por qué no salió, cuando el servicio lo dice: no poder enviar desde
// esa dirección y el mensaje inválido los arregla quien está delante en diez segundos.
func (b *Buzon) avisarEnvio(err error) {
	t := b.P.T
	var e *api.Error
	desc := ""
	switch {
	case errors.Is(err, errDireccion):
		desc = t.T("compose.sendInvalid", nil)
	case errors.As(err, &e) && e.Status == http.StatusForbidden:
		desc = t.T("compose.sendForbidden", nil)
	case errors.As(err, &e) && (e.Status == http.StatusBadRequest || e.Status == http.StatusUnprocessableEntity):
		desc = t.T("compose.sendInvalid", nil)
	}
	b.P.Avisar(ui.AvisoPeligro, t.T("compose.sendFailed", nil), desc)
}

// preguntarSinAdjunto es la confirmación de «Te adjunto…» sin adjunto.
func (b *Buzon) preguntarSinAdjunto(rd *redaccion) {
	t := b.P.T
	lienzo := b.P.Lienzo()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(t.T("compose.noAttachmentBody", nil), tema.TextoSM, ui.Normal, "text", v).
		A(AnchoCuerpoDialogo(ui.DialogoSM, lienzo.Size().Width))
	var d *ui.Dialogo
	d = ui.Confirmacion(t.T("compose.noAttachmentTitle", nil), t.T("common.cancel", nil), t.T("compose.noAttachmentSend", nil), false,
		cuerpo, func() { d.Cerrar() }, func() { d.Cerrar(); b.enviar(rd, true) })
	d.RotuloCerrar = t.T("ui.close", nil)
	d.Mostrar(lienzo)
}

// deshacerEnvio es `handleUndoSend`: saca el envío de la cola dentro de su ventana y
// reabre el redactor con lo que se acababa de enviar.
func (b *Buzon) deshacerEnvio(id string, br Borrador) {
	pedir(b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, b.S.CancelOutbound(ctx, id)
	}, func(_ struct{}, err error) {
		switch {
		case err == nil:
			b.abrirRedactor(br)
			b.avisar(ui.AvisoExito, "undo.done", nil)
		case errors.Is(err, api.ErrDemasiadoTarde):
			b.avisar(ui.AvisoPeligro, "error.undoSend", nil)
		default:
			b.avisar(ui.AvisoPeligro, "error.undoSendGone", nil)
		}
	})
}

// ---- La firma (`SignatureModal`) ----

// maxFirma es el `maxLength={2000}`: no llega a los 8 KiB que el servicio rechaza ni
// en el peor UTF-8, así que el rechazo del servidor queda fuera de lo que se teclea.
const maxFirma = 2000

// editarFirma abre «Firma»: un área de texto de seis filas con su ayuda. El diálogo
// solo se cierra cuando el servidor confirma, y mientras tanto el botón carga.
func (b *Buzon) editarFirma() {
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return
	}
	t := b.P.T
	ancho := AnchoCuerpoDialogo(ui.DialogoMD, lienzo.Size().Width)
	area := ui.NuevaAreaTexto(t.T("signature.placeholder", nil))
	area.Filas, area.Ancho = 6, ancho
	area.PonerTexto(b.firma)
	area.AlCambiar(func(s string) {
		if r := []rune(s); len(r) > maxFirma {
			area.PonerTexto(string(r[:maxFirma]))
		}
	})
	grupo := ui.GrupoCampo(t.T("signature.title", nil), false, area, t.T("signature.help", nil), "")
	var d *ui.Dialogo
	cancelar := ui.NuevoBoton(t.T("common.cancel", nil), func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	var guardar *ui.Boton
	guardar = ui.NuevoBoton(t.T("common.save", nil), func() {
		valor := area.Texto()
		guardar.Cargando, cancelar.Inerte = true, true
		guardar.Refresh()
		cancelar.Refresh()
		pedir(b, func(ctx context.Context) (struct{}, error) {
			return struct{}{}, b.S.SetSignature(ctx, valor)
		}, func(_ struct{}, err error) {
			guardar.Cargando, cancelar.Inerte = false, false
			guardar.Refresh()
			cancelar.Refresh()
			if err != nil {
				b.avisar(ui.AvisoPeligro, "error.signature", nil)
				return
			}
			b.firma = valor
			d.Cerrar()
		})
	}).Con(ui.Primario, ui.TallaMD)
	d = ui.NuevoDialogo(t.T("signature.title", nil), grupo).
		Con(ui.DialogoMD, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, guardar))
	d.RotuloCerrar = t.T("ui.close", nil)
	d.Foco = area
	d.Mostrar(lienzo)
}

// abrirAjuste atiende las entradas de «Ajustes» del lateral.
func (b *Buzon) abrirAjuste(clave string) {
	b.P.CerrarCajon()
	switch clave {
	case "external-mail":
		b.abrirCorreoExterno()
	case "signature":
		b.editarFirma()
	case "scheduled":
		b.verProgramados()
	case "vacation":
		b.verVacaciones()
	case "aliases":
		b.verAlias()
	case "rules":
		b.verReglas(nil)
	case "smime-anchor":
		b.bajarAncla()
	case "smime-identity":
		b.verSmime()
	case "shared":
		b.verCompartidos()
	case "mail-clients":
		if b.URLCuenta == "" || b.Abridor == nil {
			return
		}
		_ = api.AbrirURL(b.Abridor, "id", b.URLCuenta+"/account#app-passwords")
	}
}

// prefFormato es la preferencia de la barra de formato (`FORMATO_KEY` de la web).
const prefFormato = "mail_formato_visible"

// formatoVisible es si la barra sale desplegada: sí de serie, y lo último que se eligió.
func (b *Buzon) formatoVisible() bool {
	if a := fyne.CurrentApp(); a != nil {
		return a.Preferences().BoolWithFallback(prefFormato, true)
	}
	return true
}

func (b *Buzon) recordarFormato(si bool) {
	if a := fyne.CurrentApp(); a != nil {
		a.Preferences().SetBool(prefFormato, si)
	}
}

// pegar es `handlePaste`: el HTML del portapapeles del sistema, saneado, con su formato;
// si no lo hay (o no se puede leer aquí), el texto. Se lee fuera del hilo de la interfaz:
// en X11, si quien copió es esta misma ventana, su bucle tiene que poder contestar.
func (b *Buzon) pegar(rd *redaccion, texto string) {
	pedir(b, func(context.Context) (string, error) { return b.LeerHTML() }, func(h string, err error) {
		if b.redaccion != rd {
			return // el redactor se cerró mientras
		}
		if err != nil {
			rd.r.rico.PegarTexto(texto)
			return
		}
		// Las `data:` pasan a ser partes del mensaje (`cid:`), como las del equipo.
		h, partes := SacarImagenesEnLinea(h, nuevoCID)
		if d := DocumentoPegado(h); d != nil {
			rd.r.Adjuntar(partes...)
			rd.r.rico.Pegar(d)
		} else {
			rd.r.rico.PegarTexto(texto)
		}
		// Las que el saneado tira (`http:`, `data:`) no entran, como en la web: se dice.
		if n := ImagenesDescartadas(h); n > 0 {
			t := b.P.T
			clave := "compose.pastedImagesDropped"
			if n == 1 {
				clave = "compose.pastedImageDropped"
			}
			b.P.Avisar(ui.AvisoAdvertencia, t.T(clave, nil), t.T("compose.pastedImagesDroppedDescription", nil))
		}
	})
}
