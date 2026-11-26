// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/apps/cast"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/video"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/conversor"
	"git.sytue.eu/sytue/one/apps/nucleo/local/videoteca"
)

// controlador es lo propio de Cast Office alrededor de las pantallas de siempre:
// importar, convertir, publicar, editar, póster, quitar y el ffmpeg del equipo.
type controlador struct {
	l     *local.App
	w     fyne.Window
	p     *cast.App
	teca  *videoteca.Videoteca
	herr  *herramientas
	t     func(string) string
	movil bool
	barra *fyne.Container
}

// conSistemaMovil es que la app corre en un teléfono con descodificador del sistema
// (Android: MediaCodec): lo que no es WebM se ve aquí si el teléfono sabe leerlo.
func (c *controlador) conSistemaMovil() bool { return c.movil && video.HaySistema() }

func (c *controlador) fallo(titulo string, err error) {
	if err != nil {
		local.Info(c.w, titulo, err.Error())
	}
}

// importar copia a la videoteca los vídeos que se elijan y abre el último.
func (c *controlador) importar() {
	go func() {
		rutas, err := ficheros.ElegirConTipos(c.t("importar.elegir"), videoteca.Extensiones...)
		if err != nil || len(rutas) == 0 {
			if err != nil && !errors.Is(err, ficheros.ErrNoDisponible) {
				fyne.Do(func() { c.fallo(c.t("abrir.error"), err) })
			}
			return
		}
		var ultimo string
		for _, r := range rutas {
			e, err := c.teca.Importar(context.Background(), r)
			if err != nil {
				fyne.Do(func() { c.fallo(c.t("abrir.error"), err) })
				continue
			}
			ultimo = e.ID
		}
		if ultimo != "" {
			fyne.Do(func() { c.p.AbrirVideo(ultimo) })
		}
	}()
}

func boton(texto string, f func()) *ui.Boton {
	return ui.NuevoBoton(texto, f).Con(ui.Secundario, ui.TallaSM)
}

func nota(texto, color string, v tema.Variante) fyne.CanvasObject {
	return ui.Texto(texto, tema.TextoSM, ui.Normal, tema.Color(color, v))
}

// mandos son los botones de la página de un vídeo, según en qué punto esté.
func (c *controlador) mandos(vid api.CastVideo) []fyne.CanvasObject {
	e, err := c.teca.Entrada(vid.ID)
	if err != nil {
		return nil
	}
	variante := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	est := c.teca.EstadoDe(vid.ID)
	_, errFF := c.herr.Actuales()
	var out []fyne.CanvasObject
	switch {
	case est.Convirtiendo:
		texto := c.t("preparando.sin")
		if f := est.Progreso.Fraccion(); f > 0 {
			texto = fmt.Sprintf(c.t("preparando"), int(f*100))
		}
		out = append(out, nota(texto, "text-muted", variante))
	case errors.Is(est.Fallo, videoteca.ErrNecesitaFFmpeg):
		corto := c.t("sinffmpeg.corto")
		if c.conSistemaMovil() {
			corto = c.t("movil.noLeeCorto")
		}
		out = append(out, nota(fmt.Sprintf(corto, formatoDe(e)), "danger-text", variante))
		if !c.movil {
			out = append(out, boton(c.t("ffmpeg"), c.ajustesFFmpeg))
		}
	case est.Fallo != nil:
		out = append(out, boton(c.t("reintentar"), func() {
			c.teca.Reintentar(vid.ID)
			c.p.AbrirVideo(vid.ID)
		}))
	}
	if errFF == nil && !c.movil && !errors.Is(est.Fallo, videoteca.ErrSinFichero) {
		out = append(out, boton(c.t("convertir"), func() { c.convertir(e) }))
	}
	out = append(out, boton(c.t("sistema"), func() {
		if err := ficheros.Abrir(e.Ruta); err != nil {
			c.fallo(c.t("abrir.error"), err)
		}
	}).ConIcono(iconos.Video, iconos.TallaSM))
	pub := c.t("publicar")
	if e.Publicado {
		pub = c.t("retirar")
	}
	out = append(out,
		boton(pub, func() {
			x := !e.Publicado
			c.cambiar(vid.ID, videoteca.Cambios{Publicado: &x})
		}),
		boton(c.t("editar"), func() { c.editar(e) }).ConIcono(iconos.Lapiz, iconos.TallaSM),
		boton(c.t("poster"), func() { c.poster(e) }),
		ui.NuevoBoton(c.t("quitar"), func() { c.quitar(e) }).Con(ui.Peligro, ui.TallaSM),
	)
	return out
}

// aviso es lo que dice el reproductor mientras el vídeo no se puede ver aquí: el
// porqué de verdad, en vez del «vuelve a subirlo» de la edición conectada.
func (c *controlador) aviso(vid api.CastVideo) string {
	e, err := c.teca.Entrada(vid.ID)
	if err != nil {
		return ""
	}
	est := c.teca.EstadoDe(vid.ID)
	switch {
	case est.Convirtiendo:
		return c.t("preparando.sin")
	case errors.Is(est.Fallo, videoteca.ErrNecesitaFFmpeg):
		if c.conSistemaMovil() {
			// El teléfono tiene descodificador (MediaCodec), pero no de este formato.
			return fmt.Sprintf(c.t("movil.noLee"), formatoDe(e))
		}
		if c.movil {
			return c.t("sinffmpeg.movil")
		}
		return fmt.Sprintf(c.t("sinffmpeg"), formatoDe(e))
	case est.Fallo != nil:
		return fmt.Sprintf(c.t("fallo"), est.Fallo.Error())
	}
	return ""
}

// formatoDe es lo que se dice del formato: los códecs si se saben; si no, la extensión.
func formatoDe(e videoteca.Entrada) string {
	var partes []string
	for _, x := range []string{e.Video, e.Audio} {
		if x != "" {
			partes = append(partes, strings.ToUpper(x))
		}
	}
	if len(partes) > 0 {
		return strings.Join(partes, " + ")
	}
	return strings.ToUpper(strings.TrimPrefix(filepath.Ext(e.Ruta), "."))
}

func (c *controlador) cambiar(id string, cambios videoteca.Cambios) {
	if _, err := c.teca.Actualizar(id, cambios); err != nil {
		c.fallo(c.t("abrir.error"), err)
		return
	}
	c.p.AbrirVideo(id)
}

func (c *controlador) editar(e videoteca.Entrada) {
	titulo := widget.NewEntry()
	titulo.SetText(e.Titulo)
	desc := widget.NewMultiLineEntry()
	desc.SetText(e.Descripcion)
	desc.SetMinRowsVisible(4)
	// La colección: una de las que hay o una nueva (CA-02).
	colecciones := c.teca.Colecciones()
	nombres := make([]string, 0, len(colecciones))
	for _, k := range colecciones {
		nombres = append(nombres, k.Nombre)
	}
	col := widget.NewSelectEntry(nombres)
	col.SetText(e.Coleccion)
	col.SetPlaceHolder(c.t("coleccion.ninguna"))
	local.Formulario(c.w, c.t("editar"), c.t("cancelar"), c.t("guardar"), []*widget.FormItem{
		widget.NewFormItem(c.t("editar.titulo"), titulo),
		widget.NewFormItem(c.t("editar.descripcion"), desc),
		widget.NewFormItem(c.t("coleccion"), col),
	}, func(d *ui.Dialogo) {
		d.Cerrar()
		tt, dd, cc := titulo.Text, desc.Text, col.Text
		c.cambiar(e.ID, videoteca.Cambios{Titulo: &tt, Descripcion: &dd, Coleccion: &cc})
	})
}

// poster es «Póster…»: una imagen del equipo o el fotograma de ahora. El fotograma sale
// de lo que enseña el reproductor (sin ffmpeg, CA-06); si aún no hay imagen, del
// instante en que está con ffmpeg o el descodificador del sistema.
func (c *controlador) poster(e videoteca.Entrada) {
	var d *ui.Dialogo
	listo := func(err error) {
		fyne.Do(func() {
			if err != nil {
				c.fallo(c.t("abrir.error"), err)
				return
			}
			c.p.TirarPoster(e.ID)
			c.p.AbrirVideo(e.ID)
		})
	}
	imagen := widget.NewButton(c.t("poster.imagen"), func() {
		d.Cerrar()
		go func() {
			fs, err := ficheros.ElegirConTipos(c.t("poster.elegir"), "jpg", "jpeg", "png", "webp")
			if err != nil || len(fs) == 0 {
				return
			}
			datos, err := os.ReadFile(fs[0])
			if err == nil {
				_, err = c.teca.PonerPoster(e.ID, datos, filepath.Ext(fs[0]))
			}
			listo(err)
		}()
	})
	cuerpo := container.NewVBox(imagen)
	img, en, hayImagen := c.p.FotogramaDeAhora(e.ID)
	_, errFF := c.herr.Actuales()
	if !hayImagen {
		en = time.Duration(e.DuracionMS) * time.Millisecond / 3
	}
	if hayImagen || errFF == nil || video.HaySistema() || strings.EqualFold(filepath.Ext(e.Ruta), ".webm") {
		cuerpo.Add(widget.NewButton(c.t("poster.fotograma"), func() {
			d.Cerrar()
			go func() {
				if hayImagen {
					jpg, err := codificarJPEG(img)
					if err == nil {
						_, err = c.teca.PonerFotograma(e.ID, jpg)
					}
					listo(err)
					return
				}
				ctx, fin := context.WithTimeout(context.Background(), time.Minute)
				defer fin()
				_, err := c.teca.PosterDeFotograma(ctx, e.ID, en)
				listo(err)
			}()
		}))
	}
	d = local.Ventana(c.w, c.t("poster"), cuerpo, nil)
}

func (c *controlador) quitar(e videoteca.Entrada) {
	borrar := widget.NewCheck(c.t("quitar.fichero"), nil)
	var cuerpo fyne.CanvasObject = borrar
	if !e.Copiado {
		aviso := widget.NewLabel(c.t("quitar.apuntado"))
		aviso.Wrapping = fyne.TextWrapWord
		cuerpo = aviso
	}
	var d *ui.Dialogo
	d = ui.Confirmacion(fmt.Sprintf(c.t("quitar.pregunta"), e.Titulo), c.t("cancelar"), c.t("quitar"), true, cuerpo, nil, func() {
		d.Cerrar()
		if err := c.teca.Borrar(e.ID, borrar.Checked); err != nil {
			c.fallo(c.t("abrir.error"), err)
			return
		}
		c.p.Ir(cast.RutaInicio)
	})
	d.Mostrar(c.w.Canvas())
}

// ---- Convertir a… ----

// salidaLibre propone dónde dejar la conversión: al lado del original, con un nombre que
// no pise nada (ni el propio original).
func salidaLibre(origen, ext string) string {
	dir := filepath.Dir(origen)
	base := strings.TrimSuffix(filepath.Base(origen), filepath.Ext(origen))
	candidata := filepath.Join(dir, base+ext)
	for i := 1; ; i++ {
		if _, err := os.Lstat(candidata); errors.Is(err, os.ErrNotExist) {
			return candidata
		}
		sufijo := " (convertido)"
		if i > 1 {
			sufijo = fmt.Sprintf(" (convertido %d)", i)
		}
		candidata = filepath.Join(dir, base+sufijo+ext)
	}
}

func (c *controlador) convertir(e videoteca.Entrada) {
	h, err := c.herr.Actuales()
	if err != nil {
		c.ajustesFFmpeg()
		return
	}
	cods := c.herr.Codificadores()
	var posibles []conversor.Destino
	var nombres []string
	for _, d := range conversor.Destinos() {
		if d.SoloAudio && e.Medido && e.Audio == "" {
			continue // no hay sonido que sacar
		}
		if d.Posible(cods) {
			posibles = append(posibles, d)
			nombres = append(nombres, d.Descripcion)
		}
	}
	if len(posibles) == 0 {
		local.Info(c.w, c.t("convertir"), c.t("convertir.ninguno"))
		return
	}
	elegido := posibles[0]
	destino := widget.NewEntry()
	destino.SetText(salidaLibre(e.Ruta, elegido.Extension))
	formato := widget.NewSelect(nombres, func(s string) {
		for _, d := range posibles {
			if d.Descripcion == s {
				elegido = d
				actual := destino.Text
				destino.SetText(strings.TrimSuffix(actual, filepath.Ext(actual)) + d.Extension)
			}
		}
	})
	formato.SetSelected(nombres[0])
	destino.SetText(salidaLibre(e.Ruta, elegido.Extension))
	carpeta := widget.NewButton(c.t("convertir.carpeta"), func() {
		go func() {
			dir, err := ficheros.ElegirCarpeta(c.t("convertir.carpeta"))
			if err != nil || dir == "" {
				return
			}
			fyne.Do(func() { destino.SetText(filepath.Join(dir, filepath.Base(destino.Text))) })
		}()
	})
	local.Formulario(c.w, fmt.Sprintf(c.t("convertir.titulo"), e.Titulo), c.t("cancelar"), c.t("convertir.empezar"), []*widget.FormItem{
		widget.NewFormItem(c.t("convertir.formato"), formato),
		// La ruta a todo el ancho y el botón debajo: al lado, en los idiomas de rótulos
		// largos, la ruta se quedaba en unas pocas letras.
		widget.NewFormItem(c.t("convertir.destino"), container.NewVBox(destino, container.NewHBox(carpeta))),
	}, func(d *ui.Dialogo) {
		sal := strings.TrimSpace(destino.Text)
		if _, err := os.Lstat(sal); err == nil {
			// Nunca se escribe encima de un fichero que ya existe (R-NO-CORROMPE).
			local.Info(c.w, c.t("convertir"), c.t("convertir.existe"))
			return
		}
		d.Cerrar()
		c.lanzarConversion(h, e, sal, elegido)
	})
}

func (c *controlador) lanzarConversion(h conversor.Herramientas, e videoteca.Entrada, sal string, d conversor.Destino) {
	ctx, cancelar := context.WithCancel(context.Background())
	barra := widget.NewProgressBar()
	detalle := widget.NewLabel("")
	prog := ui.NuevoDialogo(fmt.Sprintf(c.t("convertir.en"), e.Titulo), container.NewVBox(barra, detalle)).Con(ui.DialogoSM,
		widget.NewButton(c.t("cancelar"), cancelar))
	// Cerrar el diálogo (aspa, Escape, «atrás») cancela la conversión.
	prog.AlCerrar = func() { cancelar(); prog.Cerrar() }
	prog.Mostrar(c.w.Canvas())
	go func() {
		defer cancelar()
		total := time.Duration(e.DuracionMS) * time.Millisecond
		err := h.Convertir(ctx, e.Ruta, sal, d.Formato, total, func(p conversor.Progreso) {
			fyne.Do(func() {
				barra.SetValue(p.Fraccion())
				detalle.SetText(strings.TrimSpace(fmt.Sprintf("%s / %s  %s", p.Hecho.Truncate(time.Second), total.Truncate(time.Second), p.Velocidad)))
			})
		})
		//nolint:contextcheck // lo que ofrece el aviso de «listo» lo pulsa la persona después, ya sin la conversión
		fyne.Do(func() {
			prog.Cerrar()
			if errors.Is(err, context.Canceled) {
				return
			}
			if err != nil {
				c.fallo(c.t("convertir"), err)
				return
			}
			c.hecho(sal, d)
		})
	}()
}

// hecho avisa de una conversión terminada y ofrece abrirla o añadirla a la videoteca.
func (c *controlador) hecho(sal string, d conversor.Destino) {
	texto := widget.NewLabel(fmt.Sprintf(c.t("convertir.listo"), sal))
	texto.Wrapping = fyne.TextWrapWord
	var dlg *ui.Dialogo
	botones := container.NewHBox(
		widget.NewButton(c.t("carpeta"), func() { _ = ficheros.Abrir(filepath.Dir(sal)) }),
		widget.NewButton(c.t("sistema"), func() { _ = ficheros.Abrir(sal) }),
	)
	if !d.SoloAudio && d.Formato != conversor.GIF {
		botones.Add(widget.NewButton(c.t("convertir.anadir"), func() {
			dlg.Cerrar()
			go func() {
				e, err := c.teca.Apuntar(context.Background(), sal)
				fyne.Do(func() {
					if err != nil {
						c.fallo(c.t("abrir.error"), err)
						return
					}
					c.p.AbrirVideo(e.ID)
				})
			}()
		}))
	}
	dlg = local.Ventana(c.w, c.t("convertir"), container.NewVBox(texto, botones), nil)
}

// ---- El ffmpeg del equipo ----

func (c *controlador) estadoFFmpeg() string {
	h, err := c.herr.Actuales()
	if err != nil {
		return c.t("ffmpeg.noencontrado") + " " + c.t("ffmpeg.instalar")
	}
	s := fmt.Sprintf(c.t("ffmpeg.encontrado"), h.FFmpeg)
	if h.Version != "" {
		s += "\n" + h.Version
	}
	if h.FFprobe == "" {
		s += "\n" + c.t("ffmpeg.sinprobe")
	}
	return s
}

// ajustesFFmpeg enseña qué ffmpeg se usa y deja poner otro.
func (c *controlador) ajustesFFmpeg() {
	cuerpo, guardar := c.panelFFmpeg()
	var d *ui.Dialogo
	d = ui.Confirmacion(c.t("ffmpeg.titulo"), c.t("cerrar"), c.t("guardar"), false, cuerpo, nil, func() {
		d.Cerrar()
		guardar()
	})
	d.Mostrar(c.w.Canvas())
}

// seccionFFmpeg es la pestaña de Cast en Ajustes de la cáscara: lo mismo que el diálogo,
// con su botón de guardar.
func (c *controlador) seccionFFmpeg(fyne.Window) fyne.CanvasObject {
	cuerpo, guardar := c.panelFFmpeg()
	boton := widget.NewButton(c.t("guardar"), guardar)
	boton.Importance = widget.HighImportance
	return container.NewVBox(cuerpo, container.NewHBox(boton))
}

// panelFFmpeg es el contenido de los ajustes del ffmpeg y lo que hace guardarlo.
func (c *controlador) panelFFmpeg() (fyne.CanvasObject, func()) {
	ayuda := widget.NewLabel(c.t("ffmpeg.ayuda"))
	ayuda.Wrapping = fyne.TextWrapWord
	ruta := widget.NewEntry()
	ruta.SetPlaceHolder("ffmpeg")
	ruta.SetText(c.herr.Configurada())
	estado := widget.NewLabel(c.estadoFFmpeg())
	estado.Wrapping = fyne.TextWrapWord
	buscar := widget.NewButton(c.t("ffmpeg.buscar"), func() {
		go func() {
			fs, err := ficheros.Elegir()
			if err != nil || len(fs) == 0 {
				return
			}
			fyne.Do(func() { ruta.SetText(fs[0]) })
		}()
	})
	cuerpo := container.NewVBox(ayuda, widget.NewLabel(c.t("ffmpeg.ruta")), container.NewBorder(nil, nil, nil, buscar, ruta), estado)
	return cuerpo, func() {
		_, err := c.herr.Configurar(ruta.Text)
		estado.SetText(c.estadoFFmpeg())
		for _, e := range c.teca.Lista() {
			c.teca.Reintentar(e.ID)
		}
		c.ponerBarra()
		if err != nil {
			local.Info(c.w, c.t("ffmpeg.titulo"), c.estadoFFmpeg())
		}
		if c.p.Ruta() == cast.RutaVer {
			c.p.Repintar()
		}
	}
}
