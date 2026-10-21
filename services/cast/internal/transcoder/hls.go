// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// La grabación de un directo es el HLS que el egress dejó en S3: un
// `index.m3u8` con sus segmentos `.ts` al lado (CAST-ENCARGO §6, §7). Para sacar
// el WebM y el póster se baja al temporal con la misma disposición y se le da a
// ffmpeg el playlist local, cerrado con ENDLIST para que no lo trate como un
// directo que sigue creciendo.

// segmento es una entrada del playlist de medios: su URI relativa y su duración.
type segmento struct {
	URI      string
	Duracion float64
}

// Errores del playlist.
var (
	// ErrPlaylistInvalido: no es un playlist de medios que se pueda seguir.
	ErrPlaylistInvalido = errors.New("hls: playlist inválido")
)

// parsearPlaylist lee un playlist de MEDIOS (#EXTINF + URI). Rechaza un playlist
// maestro (#EXT-X-STREAM-INF) y las URI que salgan del directorio del playlist:
// aquí las claves las compone el egress, pero un `..` o una URL absoluta no
// tienen nada que hacer y no se siguen. Devuelve también si estaba cerrado.
func parsearPlaylist(texto string) (segs []segmento, cerrado bool, err error) {
	if !strings.HasPrefix(strings.TrimSpace(texto), "#EXTM3U") {
		return nil, false, fmt.Errorf("%w: no empieza por #EXTM3U", ErrPlaylistInvalido)
	}
	sc := bufio.NewScanner(strings.NewReader(texto))
	dur := -1.0
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		switch {
		case l == "":
			continue
		case strings.HasPrefix(l, "#EXT-X-STREAM-INF"):
			return nil, false, fmt.Errorf("%w: es un playlist maestro", ErrPlaylistInvalido)
		case l == "#EXT-X-ENDLIST":
			cerrado = true
		case strings.HasPrefix(l, "#EXTINF:"):
			v, _, _ := strings.Cut(strings.TrimPrefix(l, "#EXTINF:"), ",")
			d, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil || d < 0 {
				return nil, false, fmt.Errorf("%w: duración %q", ErrPlaylistInvalido, v)
			}
			dur = d
		case strings.HasPrefix(l, "#"):
			continue
		default:
			if dur < 0 {
				return nil, false, fmt.Errorf("%w: segmento %q sin #EXTINF", ErrPlaylistInvalido, l)
			}
			if !uriSegura(l) {
				return nil, false, fmt.Errorf("%w: segmento %q fuera del directorio", ErrPlaylistInvalido, l)
			}
			segs = append(segs, segmento{URI: l, Duracion: dur})
			dur = -1
		}
	}
	if err := sc.Err(); err != nil {
		return nil, false, fmt.Errorf("%w: %w", ErrPlaylistInvalido, err)
	}
	if len(segs) == 0 {
		return nil, false, fmt.Errorf("%w: sin segmentos", ErrPlaylistInvalido)
	}
	return segs, cerrado, nil
}

// uriSegura acepta solo rutas relativas que se queden bajo el playlist.
func uriSegura(u string) bool {
	if u == "" || strings.HasPrefix(u, "/") || strings.Contains(u, "://") || strings.Contains(u, "?") || strings.Contains(u, "\\") {
		return false
	}
	limpia := path.Clean(u)
	return limpia != "." && !strings.HasPrefix(limpia, "../") && limpia != ".."
}

// grabacion es el HLS bajado: el playlist local que se le da a ffmpeg y lo que
// ocupa en el almacén (playlist + segmentos), que cuenta en la cuota.
type grabacion struct {
	playlist string
	bytes    int64
	// duracion es la suma de los #EXTINF bajados.
	duracion float64
}

// descargarGrabacion baja el playlist `clave` y sus segmentos a `dir`. Con
// `hastaSegundos` > 0 baja solo lo que hace falta para llegar a ese instante (un
// póster de un directo de tres horas no necesita las tres horas) y escribe un
// playlist recortado; con 0, todo. El playlist local se cierra con ENDLIST.
func (w *trabajador) descargarGrabacion(ctx context.Context, t tenancy.Tenant, clave, dir string, hastaSegundos float64) (grabacion, error) {
	rc, err := w.objetos.Get(ctx, t, clave)
	if err != nil {
		return grabacion{}, fmt.Errorf("bajar el playlist %q: %w", clave, err)
	}
	texto, err := io.ReadAll(io.LimitReader(rc, 8<<20))
	_ = rc.Close()
	if err != nil {
		return grabacion{}, fmt.Errorf("leer el playlist %q: %w", clave, err)
	}
	segs, _, err := parsearPlaylist(string(texto))
	if err != nil {
		return grabacion{}, err
	}
	g := grabacion{bytes: int64(len(texto))}
	dirClave := path.Dir(clave)
	var lineas []string
	lineas = append(lineas, "#EXTM3U", "#EXT-X-VERSION:3", "#EXT-X-PLAYLIST-TYPE:VOD", "#EXT-X-TARGETDURATION:"+strconv.Itoa(objetivo(segs)))
	for _, s := range segs {
		if hastaSegundos > 0 && g.duracion > hastaSegundos {
			break
		}
		n, err := w.descargarA(ctx, t, path.Join(dirClave, s.URI), filepath.Join(dir, filepath.FromSlash(s.URI)))
		if err != nil {
			return grabacion{}, err
		}
		g.bytes += n
		g.duracion += s.Duracion
		lineas = append(lineas, "#EXTINF:"+strconv.FormatFloat(s.Duracion, 'f', 3, 64)+",", s.URI)
	}
	lineas = append(lineas, "#EXT-X-ENDLIST", "")
	g.playlist = filepath.Join(dir, "index.m3u8")
	if err := os.WriteFile(g.playlist, []byte(strings.Join(lineas, "\n")), 0o600); err != nil {
		return grabacion{}, fmt.Errorf("escribir el playlist local: %w", err)
	}
	return g, nil
}

// objetivo es el EXT-X-TARGETDURATION: el mayor segmento, redondeado hacia arriba.
func objetivo(segs []segmento) int {
	mayor := 0.0
	for _, s := range segs {
		mayor = max(mayor, s.Duracion)
	}
	n := int(mayor)
	if float64(n) < mayor {
		n++
	}
	return max(n, 1)
}

// descargarA copia el objeto `clave` al fichero `ruta` (creando su directorio) y
// devuelve cuántos bytes tenía.
func (w *trabajador) descargarA(ctx context.Context, t tenancy.Tenant, clave, ruta string) (int64, error) {
	rc, err := w.objetos.Get(ctx, t, clave)
	if err != nil {
		return 0, fmt.Errorf("bajar %q: %w", clave, err)
	}
	defer func() { _ = rc.Close() }()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o750); err != nil {
		return 0, fmt.Errorf("preparar %q: %w", filepath.Dir(ruta), err)
	}
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // G304: ruta dentro del temporal del trabajo, compuesta aquí
	if err != nil {
		return 0, fmt.Errorf("crear %q: %w", ruta, err)
	}
	n, err := io.Copy(f, rc)
	if errC := f.Close(); err == nil {
		err = errC
	}
	if err != nil {
		return 0, fmt.Errorf("guardar %q: %w", clave, err)
	}
	return n, nil
}
