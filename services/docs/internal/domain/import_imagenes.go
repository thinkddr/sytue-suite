// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"

	"golang.org/x/image/tiff"
	"golang.org/x/image/vector"
)

const (
	// Topes defensivos al convertir imágenes binarias (TIFF/EMF) a PNG en memoria.
	maxImagenConversionBytes   = 16 << 20 // 16 MiB
	maxImagenConversionPixeles = 16_000_000
	maxImagenConversionLado    = 960
	maxRegistrosEMF            = 50_000
	maxPuntosEMF               = 100_000
)

// tiffADataURL decodifica un TIFF acotado y lo devuelve como data:image/png;base64,...
// Si el fichero supera los topes o está dañado, devuelve "".
func tiffADataURL(raw []byte) string {
	if len(raw) == 0 || len(raw) > maxImagenConversionBytes {
		return ""
	}
	cfg, err := tiff.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxImagenConversionPixeles {
		return ""
	}
	img, err := tiff.Decode(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	return imagenAPNGDataURL(achicarImagen(img, maxImagenConversionLado))
}

func achicarImagen(src image.Image, maxLado int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || (w <= maxLado && h <= maxLado) {
		return src
	}
	escala := float64(maxLado) / float64(max(w, h))
	nw := max(1, int(math.Round(float64(w)*escala)))
	nh := max(1, int(math.Round(float64(h)*escala)))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + (y*h)/nh
		for x := 0; x < nw; x++ {
			sx := b.Min.X + (x*w)/nw
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func imagenAPNGDataURL(img image.Image) string {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

type emfXForm struct {
	m11, m12, m21, m22, dx, dy float64
}

func emfIdentidad() emfXForm {
	return emfXForm{m11: 1, m22: 1}
}

func emfMultiplicar(a, b emfXForm) emfXForm {
	return emfXForm{
		m11: a.m11*b.m11 + a.m12*b.m21,
		m12: a.m11*b.m12 + a.m12*b.m22,
		m21: a.m21*b.m11 + a.m22*b.m21,
		m22: a.m21*b.m12 + a.m22*b.m22,
		dx:  a.dx*b.m11 + a.dy*b.m21 + b.dx,
		dy:  a.dx*b.m12 + a.dy*b.m22 + b.dy,
	}
}

type emfPluma struct {
	estilo uint32
	ancho  float64
	color  color.NRGBA
}

type emfPincel struct {
	estilo uint32
	color  color.NRGBA
}

type emfObjeto struct {
	esPluma bool
	pluma   emfPluma
	pincel  emfPincel
}

type emfDC struct {
	xf               emfXForm
	winOrgX, winOrgY float64
	winExtX, winExtY float64
	vpOrgX, vpOrgY   float64
	vpExtX, vpExtY   float64
	mapMode          uint32
	curX, curY       float64
	pluma            emfPluma
	pincel           emfPincel
}

type emfPunto struct {
	x, y float32
}

type emfSubcamino struct {
	pts     []emfPunto
	cerrado bool
}

func leI32(b []byte) int32 {
	return int32(binary.LittleEndian.Uint32(b[:4])) //nolint:gosec // G115: campo entero con signo de 32 bits ([MS-EMF]/[MS-WMF])
}

func leI16(b []byte) int16 {
	return int16(binary.LittleEndian.Uint16(b[:2])) //nolint:gosec // G115: campo entero con signo de 16 bits ([MS-EMF])
}

// emfADataURL rasteriza un metarchivo Enhanced Metafile ([MS-EMF]) acotado a PNG.
// Si tiene un rectángulo de bordes vacío (como los comodines 0,0..-1,-1) o no es un EMF
// válido, devuelve "".
//
//nolint:gocognit,gocyclo // intérprete de registros binarios EMF: un caso por código de registro ([MS-EMF])
func emfADataURL(raw []byte) string {
	if len(raw) < 88 || len(raw) > maxImagenConversionBytes {
		return ""
	}
	if binary.LittleEndian.Uint32(raw[0:4]) != 1 || binary.LittleEndian.Uint32(raw[4:8]) < 88 {
		return ""
	}
	if binary.LittleEndian.Uint32(raw[40:44]) != 0x464D4520 { // " EMF"
		return ""
	}
	bl := leI32(raw[8:12])
	bt := leI32(raw[12:16])
	br := leI32(raw[16:20])
	bb := leI32(raw[20:24])
	bw := float64(br) - float64(bl)
	bh := float64(bb) - float64(bt)
	if bw <= 0 || bh <= 0 || bw > 1e7 || bh > 1e7 {
		return ""
	}

	escala := 1.0
	if bw > maxImagenConversionLado || bh > maxImagenConversionLado {
		escala = float64(maxImagenConversionLado) / math.Max(bw, bh)
	}
	outW := max(1, int(math.Ceil(bw*escala)))
	outH := max(1, int(math.Ceil(bh*escala)))
	if int64(outW)*int64(outH) > maxImagenConversionPixeles {
		return ""
	}
	escalaX := float64(outW) / bw
	escalaY := float64(outH) / bh

	lienzo := image.NewRGBA(image.Rect(0, 0, outW, outH))
	dc := emfDC{
		xf:      emfIdentidad(),
		winExtX: 1, winExtY: 1,
		vpExtX: 1, vpExtY: 1,
		mapMode: 1,
		pluma:   emfPluma{estilo: 0, ancho: 1, color: color.NRGBA{A: 255}},
		pincel:  emfPincel{estilo: 0, color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
	}
	var pilaDC []emfDC
	objetos := map[uint32]emfObjeto{}
	var camino []emfSubcamino
	enCamino := false
	dibujado := false

	aLienzo := func(lx, ly float64) emfPunto {
		px := lx*dc.xf.m11 + ly*dc.xf.m21 + dc.xf.dx
		py := lx*dc.xf.m12 + ly*dc.xf.m22 + dc.xf.dy
		sx, sy := 1.0, 1.0
		if (dc.mapMode == 7 || dc.mapMode == 8) && dc.winExtX != 0 && dc.winExtY != 0 {
			sx = dc.vpExtX / dc.winExtX
			sy = dc.vpExtY / dc.winExtY
		}
		dx := (px-dc.winOrgX)*sx + dc.vpOrgX
		dy := (py-dc.winOrgY)*sy + dc.vpOrgY
		return emfPunto{
			x: float32((dx - float64(bl)) * escalaX),
			y: float32((dy - float64(bt)) * escalaY),
		}
	}

	rellenarCamino := func(subs []emfSubcamino) {
		if dc.pincel.estilo == 1 || dc.pincel.color.A == 0 || len(subs) == 0 {
			return
		}
		rast := vector.NewRasterizer(outW, outH)
		rast.DrawOp = draw.Over
		tiene := false
		for _, s := range subs {
			if len(s.pts) < 2 {
				continue
			}
			tiene = true
			rast.MoveTo(s.pts[0].x, s.pts[0].y)
			for _, p := range s.pts[1:] {
				rast.LineTo(p.x, p.y)
			}
			rast.ClosePath()
		}
		if !tiene {
			return
		}
		rast.Draw(lienzo, lienzo.Bounds(), image.NewUniform(dc.pincel.color), image.Point{})
		dibujado = true
	}

	trazarSegmento := func(rast *vector.Rasterizer, a, b emfPunto, r float32) {
		dx := b.x - a.x
		dy := b.y - a.y
		l := float32(math.Hypot(float64(dx), float64(dy)))
		if l < 0.01 {
			return
		}
		nx := -dy / l * r
		ny := dx / l * r
		rast.MoveTo(a.x+nx, a.y+ny)
		rast.LineTo(b.x+nx, b.y+ny)
		rast.LineTo(b.x-nx, b.y-ny)
		rast.LineTo(a.x-nx, a.y-ny)
		rast.ClosePath()
	}

	trazarCamino := func(subs []emfSubcamino) {
		if (dc.pluma.estilo&0xFF) == 5 || dc.pluma.color.A == 0 || len(subs) == 0 {
			return
		}
		ancho := dc.pluma.ancho * math.Hypot(dc.xf.m11, dc.xf.m12) * escalaX
		r := float32(math.Max(0.6, ancho*0.5))
		rast := vector.NewRasterizer(outW, outH)
		rast.DrawOp = draw.Over
		tiene := false
		for _, s := range subs {
			if len(s.pts) < 2 {
				continue
			}
			tiene = true
			for i := 0; i+1 < len(s.pts); i++ {
				trazarSegmento(rast, s.pts[i], s.pts[i+1], r)
			}
			if s.cerrado && len(s.pts) > 2 {
				trazarSegmento(rast, s.pts[len(s.pts)-1], s.pts[0], r)
			}
		}
		if !tiene {
			return
		}
		rast.Draw(lienzo, lienzo.Bounds(), image.NewUniform(dc.pluma.color), image.Point{})
		dibujado = true
	}

	asegurarSub := func() int {
		if len(camino) == 0 || camino[len(camino)-1].cerrado {
			camino = append(camino, emfSubcamino{pts: []emfPunto{aLienzo(dc.curX, dc.curY)}})
		}
		return len(camino) - 1
	}

	agregarCubic := func(p0, p1, p2, p3 emfPunto, dst *[]emfPunto) {
		const pasos = 8
		for i := 1; i <= pasos; i++ {
			t := float32(i) / pasos
			u := 1 - t
			x := u*u*u*p0.x + 3*u*u*t*p1.x + 3*u*t*t*p2.x + t*t*t*p3.x
			y := u*u*u*p0.y + 3*u*u*t*p1.y + 3*u*t*t*p2.y + t*t*t*p3.y
			*dst = append(*dst, emfPunto{x: x, y: y})
		}
	}

	off := 0
	for nRec := 0; off+8 <= len(raw) && nRec < maxRegistrosEMF; nRec++ {
		tipo := binary.LittleEndian.Uint32(raw[off : off+4])
		tam := int(binary.LittleEndian.Uint32(raw[off+4 : off+8]))
		if tam < 8 || off+tam > len(raw) {
			break
		}
		rec := raw[off : off+tam]
		off += tam

		u32 := func(o int) uint32 { return binary.LittleEndian.Uint32(rec[o : o+4]) }
		i32 := func(o int) int32 { return leI32(rec[o : o+4]) }
		i16 := func(o int) int16 { return leI16(rec[o : o+2]) }
		f32 := func(o int) float64 { return float64(math.Float32frombits(u32(o))) }
		clr := func(o int) color.NRGBA {
			return color.NRGBA{R: rec[o], G: rec[o+1], B: rec[o+2], A: 255}
		}

		switch tipo {
		case 14: // EMR_EOF
			off = len(raw)
		case 9: // EMR_SETWINDOWEXTEX
			if tam >= 16 {
				dc.winExtX, dc.winExtY = float64(i32(8)), float64(i32(12))
			}
		case 10: // EMR_SETWINDOWORGEX
			if tam >= 16 {
				dc.winOrgX, dc.winOrgY = float64(i32(8)), float64(i32(12))
			}
		case 11: // EMR_SETVIEWPORTEXTEX
			if tam >= 16 {
				dc.vpExtX, dc.vpExtY = float64(i32(8)), float64(i32(12))
			}
		case 12: // EMR_SETVIEWPORTORGEX
			if tam >= 16 {
				dc.vpOrgX, dc.vpOrgY = float64(i32(8)), float64(i32(12))
			}
		case 17: // EMR_SETMAPMODE
			if tam >= 12 {
				dc.mapMode = u32(8)
			}
		case 33: // EMR_SAVEDC
			if len(pilaDC) < 64 {
				pilaDC = append(pilaDC, dc)
			}
		case 34: // EMR_RESTOREDC
			if tam >= 12 && len(pilaDC) > 0 {
				rel := int(i32(8))
				idx := len(pilaDC) - 1
				if rel < 0 && len(pilaDC)+rel >= 0 {
					idx = len(pilaDC) + rel
				}
				dc = pilaDC[idx]
				pilaDC = pilaDC[:idx]
			}
		case 35: // EMR_SETWORLDTRANSFORM
			if tam >= 32 {
				dc.xf = emfXForm{m11: f32(8), m12: f32(12), m21: f32(16), m22: f32(20), dx: f32(24), dy: f32(28)}
			}
		case 36: // EMR_MODIFYWORLDTRANSFORM
			if tam >= 36 {
				g := emfXForm{m11: f32(8), m12: f32(12), m21: f32(16), m22: f32(20), dx: f32(24), dy: f32(28)}
				switch u32(32) {
				case 1:
					dc.xf = emfIdentidad()
				case 2:
					dc.xf = emfMultiplicar(g, dc.xf)
				case 3:
					dc.xf = emfMultiplicar(dc.xf, g)
				case 4:
					dc.xf = g
				}
			}
		case 37: // EMR_SELECTOBJECT
			if tam >= 12 {
				ih := u32(8)
				switch ih {
				case 0x80000000:
					dc.pincel = emfPincel{estilo: 0, color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}
				case 0x80000001:
					dc.pincel = emfPincel{estilo: 0, color: color.NRGBA{R: 192, G: 192, B: 192, A: 255}}
				case 0x80000002:
					dc.pincel = emfPincel{estilo: 0, color: color.NRGBA{R: 128, G: 128, B: 128, A: 255}}
				case 0x80000003:
					dc.pincel = emfPincel{estilo: 0, color: color.NRGBA{R: 64, G: 64, B: 64, A: 255}}
				case 0x80000004:
					dc.pincel = emfPincel{estilo: 0, color: color.NRGBA{A: 255}}
				case 0x80000005:
					dc.pincel = emfPincel{estilo: 1}
				case 0x80000006:
					dc.pluma = emfPluma{estilo: 0, ancho: 1, color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}
				case 0x80000007:
					dc.pluma = emfPluma{estilo: 0, ancho: 1, color: color.NRGBA{A: 255}}
				case 0x80000008:
					dc.pluma = emfPluma{estilo: 5}
				default:
					if obj, ok := objetos[ih]; ok {
						if obj.esPluma {
							dc.pluma = obj.pluma
						} else {
							dc.pincel = obj.pincel
						}
					}
				}
			}
		case 38: // EMR_CREATEPEN
			if tam >= 28 && len(objetos) < 4096 {
				objetos[u32(8)] = emfObjeto{
					esPluma: true,
					pluma:   emfPluma{estilo: u32(12) & 0xFF, ancho: math.Max(1, float64(i32(16))), color: clr(24)},
				}
			}
		case 95: // EMR_EXTCREATEPEN
			if tam >= 44 && len(objetos) < 4096 {
				est := u32(28) & 0xFF
				if u32(36) == 1 { // BS_NULL
					est = 5
				}
				objetos[u32(8)] = emfObjeto{
					esPluma: true,
					pluma:   emfPluma{estilo: est, ancho: math.Max(1, float64(u32(32))), color: clr(40)},
				}
			}
		case 39: // EMR_CREATEBRUSHINDIRECT
			if tam >= 24 && len(objetos) < 4096 {
				objetos[u32(8)] = emfObjeto{
					pincel: emfPincel{estilo: u32(12), color: clr(16)},
				}
			}
		case 40: // EMR_DELETEOBJECT
			if tam >= 12 {
				delete(objetos, u32(8))
			}
		case 59: // EMR_BEGINPATH
			enCamino = true
			camino = nil
		case 60: // EMR_ENDPATH
			enCamino = false
		case 61: // EMR_CLOSEFIGURE
			if len(camino) > 0 {
				camino[len(camino)-1].cerrado = true
			}
		case 62: // EMR_FILLPATH
			rellenarCamino(camino)
			camino = nil
		case 63: // EMR_STROKEANDFILLPATH
			rellenarCamino(camino)
			trazarCamino(camino)
			camino = nil
		case 64: // EMR_STROKEPATH
			trazarCamino(camino)
			camino = nil
		case 27: // EMR_MOVETOEX
			if tam >= 16 {
				dc.curX, dc.curY = float64(i32(8)), float64(i32(12))
				if enCamino {
					camino = append(camino, emfSubcamino{pts: []emfPunto{aLienzo(dc.curX, dc.curY)}})
				}
			}
		case 54: // EMR_LINETO
			if tam >= 16 {
				nx, ny := float64(i32(8)), float64(i32(12))
				if enCamino {
					idx := asegurarSub()
					camino[idx].pts = append(camino[idx].pts, aLienzo(nx, ny))
				} else {
					trazarCamino([]emfSubcamino{{pts: []emfPunto{aLienzo(dc.curX, dc.curY), aLienzo(nx, ny)}}})
				}
				dc.curX, dc.curY = nx, ny
			}
		case 42, 43: // EMR_ELLIPSE, EMR_RECTANGLE
			if tam >= 24 {
				l, t, r, b := float64(i32(8)), float64(i32(12)), float64(i32(16)), float64(i32(20))
				var pts []emfPunto
				if tipo == 43 {
					pts = []emfPunto{aLienzo(l, t), aLienzo(r, t), aLienzo(r, b), aLienzo(l, b)}
				} else {
					cx, cy := (l+r)*0.5, (t+b)*0.5
					rx, ry := (r-l)*0.5, (b-t)*0.5
					for k := 0; k < 24; k++ {
						ang := float64(k) * 2 * math.Pi / 24
						pts = append(pts, aLienzo(cx+rx*math.Cos(ang), cy+ry*math.Sin(ang)))
					}
				}
				sub := emfSubcamino{pts: pts, cerrado: true}
				if enCamino {
					camino = append(camino, sub)
				} else {
					rellenarCamino([]emfSubcamino{sub})
					trazarCamino([]emfSubcamino{sub})
				}
			}
		case 2, 3, 4, 5, 6, 85, 86, 87, 88, 89:
			// Polilíneas, polígonos y Béziers (32 bits y 16 bits).
			if tam < 28 {
				continue
			}
			n := int(u32(24))
			if n <= 0 || n > maxPuntosEMF {
				continue
			}
			es16 := tipo >= 85
			paso := 8
			if es16 {
				paso = 4
			}
			if 28+n*paso > tam {
				continue
			}
			leerPt := func(k int) (float64, float64) {
				p := 28 + k*paso
				if es16 {
					return float64(i16(p)), float64(i16(p + 2))
				}
				return float64(i32(p)), float64(i32(p + 4))
			}
			switch tipo {
			case 3, 86, 4, 87: // POLYGON / POLYLINE
				pts := make([]emfPunto, 0, n)
				for k := 0; k < n; k++ {
					x, y := leerPt(k)
					pts = append(pts, aLienzo(x, y))
				}
				cerrado := tipo == 3 || tipo == 86
				sub := emfSubcamino{pts: pts, cerrado: cerrado}
				if enCamino {
					camino = append(camino, sub)
				} else {
					if cerrado {
						rellenarCamino([]emfSubcamino{sub})
					}
					trazarCamino([]emfSubcamino{sub})
				}
			case 6, 89: // POLYLINETO
				if enCamino {
					idx := asegurarSub()
					for k := 0; k < n; k++ {
						x, y := leerPt(k)
						camino[idx].pts = append(camino[idx].pts, aLienzo(x, y))
						dc.curX, dc.curY = x, y
					}
				} else {
					pts := make([]emfPunto, 0, n+1)
					pts = append(pts, aLienzo(dc.curX, dc.curY))
					for k := 0; k < n; k++ {
						x, y := leerPt(k)
						pts = append(pts, aLienzo(x, y))
						dc.curX, dc.curY = x, y
					}
					trazarCamino([]emfSubcamino{{pts: pts}})
				}
			case 2, 85: // POLYBEZIER
				if n < 4 {
					continue
				}
				x0, y0 := leerPt(0)
				pts := []emfPunto{aLienzo(x0, y0)}
				for k := 1; k+2 < n; k += 3 {
					x1, y1 := leerPt(k)
					x2, y2 := leerPt(k + 1)
					x3, y3 := leerPt(k + 2)
					agregarCubic(pts[len(pts)-1], aLienzo(x1, y1), aLienzo(x2, y2), aLienzo(x3, y3), &pts)
				}
				if enCamino {
					camino = append(camino, emfSubcamino{pts: pts})
				} else {
					trazarCamino([]emfSubcamino{{pts: pts}})
				}
			case 5, 88: // POLYBEZIERTO
				if enCamino {
					idx := asegurarSub()
					for k := 0; k+2 < n; k += 3 {
						x1, y1 := leerPt(k)
						x2, y2 := leerPt(k + 1)
						x3, y3 := leerPt(k + 2)
						p0 := camino[idx].pts[len(camino[idx].pts)-1]
						agregarCubic(p0, aLienzo(x1, y1), aLienzo(x2, y2), aLienzo(x3, y3), &camino[idx].pts)
						dc.curX, dc.curY = x3, y3
					}
				} else {
					pts := []emfPunto{aLienzo(dc.curX, dc.curY)}
					for k := 0; k+2 < n; k += 3 {
						x1, y1 := leerPt(k)
						x2, y2 := leerPt(k + 1)
						x3, y3 := leerPt(k + 2)
						agregarCubic(pts[len(pts)-1], aLienzo(x1, y1), aLienzo(x2, y2), aLienzo(x3, y3), &pts)
						dc.curX, dc.curY = x3, y3
					}
					trazarCamino([]emfSubcamino{{pts: pts}})
				}
			}
		case 7, 8, 90, 91:
			// POLYPOLYLINE / POLYPOLYGON (32 bits y 16 bits).
			if tam < 32 {
				continue
			}
			nPolys := int(u32(24))
			totalPts := int(u32(28))
			if nPolys <= 0 || nPolys > 10_000 || totalPts <= 0 || totalPts > maxPuntosEMF {
				continue
			}
			es16 := tipo >= 90
			paso := 8
			if es16 {
				paso = 4
			}
			offPts := 32 + nPolys*4
			if offPts+totalPts*paso > tam {
				continue
			}
			cerrado := tipo == 8 || tipo == 91
			var subs []emfSubcamino
			posPt := offPts
			for p := 0; p < nPolys; p++ {
				cnt := int(u32(32 + p*4))
				if cnt <= 0 || posPt+cnt*paso > tam {
					break
				}
				pts := make([]emfPunto, 0, cnt)
				for k := 0; k < cnt; k++ {
					var x, y float64
					if es16 {
						x, y = float64(i16(posPt)), float64(i16(posPt+2))
					} else {
						x, y = float64(i32(posPt)), float64(i32(posPt+4))
					}
					posPt += paso
					pts = append(pts, aLienzo(x, y))
				}
				subs = append(subs, emfSubcamino{pts: pts, cerrado: cerrado})
			}
			if enCamino {
				camino = append(camino, subs...)
			} else {
				if cerrado {
					rellenarCamino(subs)
				}
				trazarCamino(subs)
			}
		case 76, 81: // EMR_BITBLT, EMR_STRETCHDIBITS
			var xDest, yDest, cxDest, cyDest float64
			var offBmi, cbBmi, offBits, cbBits int
			if tipo == 81 && tam >= 80 {
				xDest, yDest = float64(i32(24)), float64(i32(28))
				cxDest, cyDest = float64(i32(68)), float64(i32(72))
				offBmi, cbBmi = int(u32(48)), int(u32(52))
				offBits, cbBits = int(u32(56)), int(u32(60))
			} else if tipo == 76 && tam >= 100 {
				xDest, yDest = float64(i32(24)), float64(i32(28))
				cxDest, cyDest = float64(i32(32)), float64(i32(36))
				offBmi, cbBmi = int(u32(84)), int(u32(88))
				offBits, cbBits = int(u32(92)), int(u32(96))
			}
			if cbBmi >= 40 && cbBits > 0 && offBmi >= 0 && offBits >= 0 && offBmi+cbBmi <= tam && offBits+cbBits <= tam {
				if dib := decodificarDIB(rec[offBmi:offBmi+cbBmi], rec[offBits:offBits+cbBits]); dib != nil {
					p0 := aLienzo(xDest, yDest)
					p1 := aLienzo(xDest+cxDest, yDest+cyDest)
					x0, x1 := int(math.Min(float64(p0.x), float64(p1.x))), int(math.Max(float64(p0.x), float64(p1.x)))
					y0, y1 := int(math.Min(float64(p0.y), float64(p1.y))), int(math.Max(float64(p0.y), float64(p1.y)))
					if x1 > x0 && y1 > y0 {
						sb := dib.Bounds()
						for y := max(0, y0); y < min(outH, y1); y++ {
							sy := sb.Min.Y + ((y-y0)*sb.Dy())/(y1-y0)
							for x := max(0, x0); x < min(outW, x1); x++ {
								sx := sb.Min.X + ((x-x0)*sb.Dx())/(x1-x0)
								lienzo.Set(x, y, dib.At(sx, sy))
							}
						}
						dibujado = true
					}
				}
			}
		}
	}

	if !dibujado {
		return ""
	}
	return imagenAPNGDataURL(lienzo)
}

func decodificarDIB(bmi, bits []byte) image.Image {
	if len(bmi) < 40 {
		return nil
	}
	w := int(leI32(bmi[4:8]))
	h := int(leI32(bmi[8:12]))
	abajoArriba := true
	if h < 0 {
		h = -h
		abajoArriba = false
	}
	bpp := int(binary.LittleEndian.Uint16(bmi[14:16]))
	comp := binary.LittleEndian.Uint32(bmi[16:20])
	if w <= 0 || h <= 0 || int64(w)*int64(h) > maxImagenConversionPixeles || comp != 0 || (bpp != 24 && bpp != 32) {
		return nil
	}
	stride := ((w*bpp + 31) / 32) * 4
	if stride*h > len(bits) {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bytesPx := bpp / 8
	for y := 0; y < h; y++ {
		dy := y
		if abajoArriba {
			dy = h - 1 - y
		}
		fila := bits[y*stride:]
		for x := 0; x < w; x++ {
			px := fila[x*bytesPx:]
			a := uint8(255)
			if bpp == 32 && px[3] != 0 {
				a = px[3]
			}
			img.SetRGBA(x, dy, color.RGBA{R: px[2], G: px[1], B: px[0], A: a})
		}
	}
	return img
}
