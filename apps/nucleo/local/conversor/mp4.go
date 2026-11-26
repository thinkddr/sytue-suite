// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package conversor

import (
	"cmp"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"time"
)

// ErrNoEsMP4: el fichero no es un MP4/MOV que se pueda leer (CA-05).
var ErrNoEsMP4 = errors.New("conversor: no es un MP4/MOV legible")

// SondearMP4 lee, en Go y sin ffprobe, la duración, las dimensiones y los códecs de un
// MP4, M4V o MOV recorriendo sus cajas (ISO/IEC 14496-12): `moov/mvhd` da la duración,
// cada `trak` su tipo (`mdia/hdlr`), sus dimensiones (`tkhd`) y su códec (la primera
// entrada de `stbl/stsd`). Así la videoteca enseña bien un vídeo aunque el equipo no
// tenga ffmpeg; para reproducirlo sigue haciendo falta convertirlo.
func SondearMP4(ruta string) (Info, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Info{}, err
	}
	return leerMP4(f, st.Size())
}

type caja struct {
	tipo           string
	inicio, cuerpo int64 // inicio de la caja y de su contenido
	fin            int64
}

// cajas lista las cajas hijas entre desde y hasta.
func cajas(r io.ReaderAt, desde, hasta int64) ([]caja, error) {
	var out []caja
	var cab [16]byte
	for pos := desde; pos+8 <= hasta; {
		if _, err := r.ReadAt(cab[:8], pos); err != nil {
			return out, err
		}
		tam := int64(binary.BigEndian.Uint32(cab[:4]))
		tipo := string(cab[4:8])
		cuerpo := pos + 8
		switch tam {
		case 1: // tamaño de 64 bits
			if _, err := r.ReadAt(cab[8:16], pos+8); err != nil {
				return out, err
			}
			t := binary.BigEndian.Uint64(cab[8:16])
			if t > math.MaxInt64 {
				return out, ErrNoEsMP4
			}
			tam = int64(t)
			cuerpo = pos + 16
		case 0: // hasta el final
			tam = hasta - pos
		}
		if tam < cuerpo-pos || pos+tam > hasta {
			return out, ErrNoEsMP4
		}
		out = append(out, caja{tipo: tipo, inicio: pos, cuerpo: cuerpo, fin: pos + tam})
		pos += tam
		if len(out) > 10000 {
			return out, ErrNoEsMP4
		}
	}
	return out, nil
}

func hija(r io.ReaderAt, padre caja, tipo string) (caja, bool) {
	hs, _ := cajas(r, padre.cuerpo, padre.fin)
	for _, h := range hs {
		if h.tipo == tipo {
			return h, true
		}
	}
	return caja{}, false
}

func ruta(r io.ReaderAt, padre caja, tipos ...string) (caja, bool) {
	c := padre
	for _, t := range tipos {
		var ok bool
		if c, ok = hija(r, c, t); !ok {
			return caja{}, false
		}
	}
	return c, true
}

func leer(r io.ReaderAt, pos int64, n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := r.ReadAt(b, pos)
	return b, err
}

// codecs traduce el tipo de entrada de `stsd` al nombre de ffprobe.
var codecs = map[string]string{
	"avc1": "h264", "avc3": "h264", "hvc1": "hevc", "hev1": "hevc", "av01": "av1",
	"vp09": "vp9", "vp08": "vp8", "mp4v": "mpeg4", "apcn": "prores", "apch": "prores",
	"mp4a": "aac", "Opus": "opus", "ac-3": "ac3", "ec-3": "eac3", "fLaC": "flac",
	".mp3": "mp3", "alac": "alac", "lpcm": "pcm", "sowt": "pcm", "twos": "pcm",
}

func leerMP4(r io.ReaderAt, tam int64) (Info, error) {
	raiz, err := cajas(r, 0, tam)
	if err != nil && len(raiz) == 0 {
		return Info{}, ErrNoEsMP4
	}
	var ftyp string
	var moov caja
	hayMoov := false
	for _, c := range raiz {
		switch c.tipo {
		case "ftyp":
			if b, err := leer(r, c.cuerpo, 4); err == nil {
				ftyp = string(b)
			}
		case "moov":
			moov, hayMoov = c, true
		}
	}
	if !hayMoov {
		return Info{}, ErrNoEsMP4
	}
	info := Info{Contenedor: "mov,mp4,m4a,3gp,3g2,mj2"}
	if ftyp == "qt  " {
		info.Contenedor = "mov,mp4,m4a,3gp,3g2,mj2" // el mismo nombre que da ffprobe a los dos
	}
	if mvhd, ok := hija(r, moov, "mvhd"); ok {
		info.Duracion = duracionMvhd(r, mvhd)
	}
	trs, _ := cajas(r, moov.cuerpo, moov.fin)
	for _, t := range trs {
		if t.tipo != "trak" {
			continue
		}
		manejo, codec, ok := pista(r, t)
		if !ok {
			continue
		}
		switch manejo {
		case "vide":
			if info.Video != "" {
				continue
			}
			info.Video = codec
			if tkhd, ok := hija(r, t, "tkhd"); ok {
				info.Ancho, info.Alto = dimensionesTkhd(r, tkhd)
			}
		case "soun":
			if info.Audio == "" {
				info.Audio = codec
			}
		}
	}
	if info.Video == "" && info.Audio == "" {
		return Info{}, ErrNoEsMP4
	}
	return info, nil
}

// pista dice de qué es una pista (`vide`, `soun`…, de su `hdlr`) y su códec con el
// nombre de ffprobe, o el tipo de `stsd` tal cual si no se conoce.
func pista(r io.ReaderAt, t caja) (manejo, codec string, ok bool) {
	hdlr, ok := ruta(r, t, "mdia", "hdlr")
	if !ok {
		return "", "", false
	}
	b, err := leer(r, hdlr.cuerpo+8, 4) // versión+flags (4) y pre_defined (4)
	if err != nil {
		return "", "", false
	}
	if stsd, ok := ruta(r, t, "mdia", "minf", "stbl", "stsd"); ok {
		if e, err := leer(r, stsd.cuerpo+12, 4); err == nil { // versión+flags, número, tamaño
			codec = cmp.Or(codecs[string(e)], string(e))
		}
	}
	return string(b), codec, true
}

func duracionMvhd(r io.ReaderAt, c caja) time.Duration {
	v, err := leer(r, c.cuerpo, 1)
	if err != nil {
		return 0
	}
	var escala, dur uint64
	if v[0] == 1 {
		b, err := leer(r, c.cuerpo+4+16, 12)
		if err != nil {
			return 0
		}
		escala, dur = uint64(binary.BigEndian.Uint32(b[:4])), binary.BigEndian.Uint64(b[4:12])
	} else {
		b, err := leer(r, c.cuerpo+4+8, 8)
		if err != nil {
			return 0
		}
		escala, dur = uint64(binary.BigEndian.Uint32(b[:4])), uint64(binary.BigEndian.Uint32(b[4:8]))
	}
	if escala == 0 || dur == ^uint64(0) || dur == 0xffffffff {
		return 0
	}
	return time.Duration(float64(dur) / float64(escala) * float64(time.Second))
}

// dimensionesTkhd: ancho y alto son los dos últimos campos de 16.16 de la caja.
func dimensionesTkhd(r io.ReaderAt, c caja) (int, int) {
	if c.fin-c.cuerpo < 8 {
		return 0, 0
	}
	b, err := leer(r, c.fin-8, 8)
	if err != nil {
		return 0, 0
	}
	return int(binary.BigEndian.Uint32(b[:4]) >> 16), int(binary.BigEndian.Uint32(b[4:]) >> 16)
}
