// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp" // las imágenes de producto pueden ser WebP, como en la web

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
)

// # Las imágenes de los productos y «Crear presupuesto» (ERP-09)
//
// Como `PuntoVenta.tsx`: el servicio dice qué adjunto es la imagen de cada producto
// (`ImagenesProductos`, en tandas de 200) y sus bytes bajan por la descarga autenticada
// de los adjuntos. Aquí se reducen al bajar (`miniatura`, una imagen a la vez en el
// proceso): una tesela mide unos 170 px y la foto del móvil, doce megapíxeles. Subir una
// es un adjunto más del producto, JPEG, PNG o WebP de hasta 10 MB (`pos.imageHint`).

// LadoImagenTPV es el lado máximo al que se reduce la imagen de un producto: el doble
// de la tesela más ancha, para una pantalla de densidad 2.
const LadoImagenTPV = 480

// MaxImagenTPV es el tope de la imagen que se sube, el de la web.
const MaxImagenTPV = 10 << 20

// tipoImagenTPV es el tipo de la imagen por su extensión; solo los tres de la web.
func tipoImagenTPV(ruta string) (string, bool) {
	switch strings.ToLower(filepath.Ext(ruta)) {
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".png":
		return "image/png", true
	case ".webp":
		return "image/webp", true
	}
	return "", false
}

// engancharTPVExtras conecta el presupuesto y las imágenes con el servicio.
func (p *Pantalla) engancharTPVExtras() {
	t := p.tpv
	t.AlPresupuesto = p.presupuestoTPV
	t.AlSubirImagen = p.subirImagenTPV
}

// presupuestoTPV crea el presupuesto y lo abre, como `onPresupuesto` de la web.
func (p *Pantalla) presupuestoTPV(pr api.ERPPresupuestoTPV) {
	t := p.tpv
	if p.svc == nil {
		t.Presupuestado(errSinServicio, p.t("error.generic"))
		p.Repintar()
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		r, err := p.svc.PresupuestoTPV(ctx, pr)
		enHilo(func() {
			t.Presupuestado(err, MensajeDeError(err, p.tr))
			if err == nil {
				p.revision++
				p.olvidar(r.Modelo)
				p.abrirRegistro(r.Modelo, r.ID)
			}
			p.Repintar()
		})
	})
}

// cargarImagenesTPV pide qué productos tienen imagen y baja las que aún no están (o
// cambiaron de adjunto). Lo que falle se queda sin imagen: la tesela de siempre.
func (p *Pantalla) cargarImagenesTPV() {
	t := p.tpv
	if p.svc == nil || len(t.Productos) == 0 {
		return
	}
	ids := make([]string, 0, len(t.Productos))
	for _, r := range t.Productos {
		ids = append(ids, r.ID)
	}
	conocidas := make(map[string]string, len(t.adjuntosImagen))
	for k, v := range t.adjuntosImagen {
		conocidas[k] = v
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		adjuntos, err := p.svc.ImagenesProductos(ctx, ids)
		if err != nil {
			return
		}
		nuevas := map[string]image.Image{}
		for producto, adjunto := range adjuntos {
			if conocidas[producto] == adjunto {
				continue
			}
			if img := p.bajarImagenTPV(ctx, adjunto); img != nil {
				nuevas[producto] = img
			}
		}
		enHilo(func() {
			if t.Imagenes == nil {
				t.Imagenes = map[string]image.Image{}
			}
			if t.adjuntosImagen == nil {
				t.adjuntosImagen = map[string]string{}
			}
			for producto, img := range nuevas {
				t.Imagenes[producto], t.adjuntosImagen[producto] = img, adjuntos[producto]
			}
			if len(nuevas) > 0 {
				p.Repintar()
			}
		})
	})
}

// bajarImagenTPV baja el adjunto y lo reduce; nil si no se puede.
func (p *Pantalla) bajarImagenTPV(ctx context.Context, adjunto string) image.Image {
	rc, err := p.svc.Adjunto(ctx, adjunto)
	if err != nil {
		return nil
	}
	defer func() { _ = rc.Close() }()
	datos, err := io.ReadAll(io.LimitReader(rc, MaxImagenTPV+1))
	if err != nil || len(datos) > MaxImagenTPV {
		return nil
	}
	return reducirImagenTPV(ctx, datos)
}

// reducirImagenTPV decodifica la imagen con las guardas de `miniatura` (la cabecera
// decide si cabe antes de gastar memoria) y la deja en LadoImagenTPV.
func reducirImagenTPV(ctx context.Context, datos []byte) image.Image {
	liberar, err := miniatura.TurnoConContexto(ctx)
	if err != nil {
		return nil
	}
	defer liberar()
	img, err := miniatura.Decodificar(datos)
	if err != nil {
		return nil
	}
	return miniatura.Reducir(img, LadoImagenTPV)
}

// subirImagenTPV elige un fichero del sistema y lo sube como imagen del producto.
func (p *Pantalla) subirImagenTPV(producto string) {
	t := p.tpv
	if p.ElegirFicheros == nil || p.svc == nil || t.SubiendoImagen != "" {
		return
	}
	p.ElegirFicheros(func(rutas []string) {
		if len(rutas) == 0 {
			return
		}
		ruta := rutas[0]
		tipo, ok := tipoImagenTPV(ruta)
		info, err := os.Stat(ruta)
		if !ok || err != nil || info.Size() > MaxImagenTPV {
			t.Fallo = p.t("pos.imageHint")
			p.Repintar()
			return
		}
		t.SubiendoImagen, t.Fallo = producto, ""
		p.Repintar()
		p.fondo(func() {
			datos, err := os.ReadFile(ruta) //nolint:gosec // la ruta la eligió la persona en el selector del sistema
			if err == nil {
				ctx, cancelar := p.ctx()
				_, err = p.svc.SubirAdjunto(ctx, "producto", producto, filepath.Base(ruta), tipo, datos)
				cancelar()
			}
			enHilo(func() {
				t.SubiendoImagen = ""
				if err != nil {
					t.Fallo = MensajeDeError(err, p.tr)
				} else {
					delete(t.adjuntosImagen, producto)
				}
				p.Repintar()
				if err == nil {
					p.cargarImagenesTPV()
				}
			})
		})
	})
}
