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
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// El trabajo `qualities`: pone lo generado al día con lo elegido. Genera en las
// dos escaleras las calidades que faltan (bajando el original, o la grabación
// de un directo, solo si falta algo), reescribe el master del HLS con las
// variantes que quedan y borra lo que sobra. Lo que ocupa el vídeo al final se
// MIDE en el almacén (no se estima): es lo que se declara a Drive.
//
// Es también lo que da la escalera WebM entera a un vídeo de antes de ella, que
// solo tenía webm/720p.webm: su elección es la recomendada (todas), así que le
// faltan 1080p (si el original llega) y 360p.

// plan es lo que un trabajo de calidades tiene que hacer.
type plan struct {
	objetivo               []int
	webmFaltan, webmSobran []int
	hlsFaltan, hlsSobran   []int
	conHLS                 bool
	webmAntiguo            bool
}

// planDe compara lo elegido con lo que hay. El HLS solo cuenta en los vídeos
// subidos (el de una grabación es el del directo).
func planDe(v domain.Video) plan {
	p := plan{
		objetivo:    domain.Resolver(elegidasDe(v), domain.OpcionesDe(v)),
		conHLS:      v.Kind == domain.KindUpload && v.HlsKey != "",
		webmAntiguo: v.WebmQualities == nil && v.WebmKey != "",
	}
	webm := v.CalidadesWebM()
	p.webmFaltan, p.webmSobran = diferencia(p.objetivo, webm), diferencia(webm, p.objetivo)
	if p.conHLS {
		hls := v.CalidadesHLS()
		p.hlsFaltan, p.hlsSobran = diferencia(p.objetivo, hls), diferencia(hls, p.objetivo)
	}
	return p
}

// diferencia son las de `a` que no están en `b`, en el orden de `a`.
func diferencia(a, b []int) []int {
	var out []int
	for _, q := range a {
		if !slices.Contains(b, q) {
			out = append(out, q)
		}
	}
	return out
}

func (w *trabajador) procesarCalidades(ctx context.Context, t tenancy.Tenant, v domain.Video, dir, salida string) (resultado, error) {
	if v.Width == nil || v.Height == nil {
		// Sin medir no se sabe qué permite el original: es un vídeo que aún no se
		// ha transcodificado, y su trabajo de subida ya usará la elección.
		return resultado{}, fmt.Errorf("%w: el vídeo aún no se ha medido", ErrSinEntrada)
	}
	p := planDe(v)
	w.logger.Info("transcoder: calidades", "video", v.ID, "objetivo", p.objetivo,
		"webm_faltan", p.webmFaltan, "webm_sobran", p.webmSobran, "hls_faltan", p.hlsFaltan, "hls_sobran", p.hlsSobran)
	pref := service.Prefijo(v)

	if err := w.generarFaltan(ctx, t, v, p, dir, salida); err != nil {
		return resultado{}, err
	}

	// El master se reescribe si cambia alguna variante: las que quedan más las
	// nuevas, que ffmpeg dejó en su propio master a medida.
	if len(p.hlsFaltan) > 0 || len(p.hlsSobran) > 0 {
		if err := w.reescribirMaster(ctx, t, v, p, salida); err != nil {
			return resultado{}, err
		}
	}

	// Primero lo nuevo (con el master que ya no nombra lo que sobra), después
	// borrar: quien esté viendo no se queda sin una variante que el master aún
	// anunciaba.
	if _, err := w.subirDirectorio(ctx, t, salida, pref); err != nil {
		return resultado{}, err
	}
	if err := w.borrarSobran(ctx, t, v, p); err != nil {
		return resultado{}, err
	}

	total, err := w.objetos.SizePrefix(ctx, t, pref)
	if err != nil {
		return resultado{}, fmt.Errorf("medir lo que ocupa: %w", err)
	}
	est := store.Estado{BytesTotal: ptr(max(total, v.BytesOriginal))}
	conWebM(&est, v, p.objetivo)
	if p.conHLS {
		est.HlsQualities = domain.AInt32(p.objetivo)
	}
	return resultado{estado: est, bytesDerivadas: total - v.BytesTotal}, nil
}

// generarFaltan baja la entrada y genera bajo `salida` lo que falta de cada
// escalera. Sin nada que generar no baja nada.
func (w *trabajador) generarFaltan(ctx context.Context, t tenancy.Tenant, v domain.Video, p plan, dir, salida string) error {
	if len(p.webmFaltan) == 0 && len(p.hlsFaltan) == 0 {
		return nil
	}
	e, s, err := w.entradaDe(ctx, t, v, dir)
	if err != nil {
		return err
	}
	if len(p.hlsFaltan) > 0 {
		esc := escaleraDe(p.hlsFaltan)
		for _, c := range esc {
			if err := os.MkdirAll(filepath.Join(salida, "hls", c.Nombre), 0o750); err != nil {
				return fmt.Errorf("preparar hls/%s: %w", c.Nombre, err)
			}
		}
		if err := w.ejecutar(ctx, "hls", argsHLS(e, salida, esc, s, w.cfg.TranscoderThreads)); err != nil {
			return err
		}
	}
	if len(p.webmFaltan) > 0 {
		return w.webm(ctx, e, s, escaleraDe(p.webmFaltan), salida)
	}
	return nil
}

// borrarSobran borra del almacén los WebM y las variantes HLS que ya no se eligen.
func (w *trabajador) borrarSobran(ctx context.Context, t tenancy.Tenant, v domain.Video, p plan) error {
	pref := service.Prefijo(v)
	for _, q := range p.webmSobran {
		clave := pref + media.RutaWebMDe(q)
		if p.webmAntiguo {
			clave = v.WebmKey
		}
		if err := w.objetos.Delete(ctx, t, clave); err != nil && !errors.Is(err, objectstore.ErrNotFound) {
			return fmt.Errorf("borrar %q: %w", clave, err)
		}
	}
	for _, q := range p.hlsSobran {
		if err := w.objetos.DeletePrefix(ctx, t, pref+"hls/"+strconv.Itoa(q)+"p/"); err != nil {
			return fmt.Errorf("borrar hls/%dp: %w", q, err)
		}
	}
	return nil
}

// entradaDe baja y mide de dónde se generan las calidades: el original o, en un
// directo grabado, su HLS.
func (w *trabajador) entradaDe(ctx context.Context, t tenancy.Tenant, v domain.Video, dir string) (entrada, Sonda, error) {
	var e entrada
	switch {
	case v.OriginalKey != "":
		e = entrada{ruta: filepath.Join(dir, ficheroEntrada)}
		if _, err := w.descargarA(ctx, t, v.OriginalKey, e.ruta); err != nil {
			return entrada{}, Sonda{}, err
		}
	case v.Kind == domain.KindLive && v.HlsKey != "":
		g, err := w.descargarGrabacion(ctx, t, v.HlsKey, filepath.Join(dir, subdirGrabacion), 0)
		if err != nil {
			return entrada{}, Sonda{}, err
		}
		e = entrada{ruta: g.playlist, hls: true}
	default:
		return entrada{}, Sonda{}, ErrSinEntrada
	}
	s, err := w.sondear(ctx, e)
	if err != nil {
		return entrada{}, Sonda{}, err
	}
	return e, s, nil
}

// reescribirMaster deja en salida/hls/master.m3u8 el master con las variantes
// que quedan del actual y las que ffmpeg acaba de generar.
func (w *trabajador) reescribirMaster(ctx context.Context, t tenancy.Tenant, v domain.Video, p plan, salida string) error {
	rc, err := w.objetos.Get(ctx, t, v.HlsKey)
	if err != nil {
		return fmt.Errorf("bajar el master: %w", err)
	}
	actual, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	_ = rc.Close()
	if err != nil {
		return fmt.Errorf("leer el master: %w", err)
	}
	ruta := filepath.Join(salida, "hls", "master.m3u8")
	var nuevo []byte
	if len(p.hlsFaltan) > 0 {
		nuevo, err = os.ReadFile(ruta) //nolint:gosec // G304: ruta dentro del temporal del trabajo
		if err != nil {
			return fmt.Errorf("leer el master nuevo: %w", err)
		}
	}
	fusionado, err := fusionarMaster(string(actual), string(nuevo), p.objetivo)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(ruta), 0o750); err != nil {
		return err
	}
	return os.WriteFile(ruta, []byte(fusionado), 0o600) //nolint:gosec // G703: ruta dentro del temporal del trabajo
}

// variante es una entrada de un master: su #EXT-X-STREAM-INF, su URI y la
// calidad (la carpeta de la URI, «720p/index.m3u8» → 720).
type variante struct {
	inf, uri string
	calidad  int
	banda    int64
}

// parsearMaster separa la cabecera (todo lo que va antes de la primera
// variante) de las variantes.
func parsearMaster(texto string) (cabecera []string, vs []variante, err error) {
	if !strings.HasPrefix(strings.TrimSpace(texto), "#EXTM3U") {
		return nil, nil, fmt.Errorf("%w: el master no empieza por #EXTM3U", ErrPlaylistInvalido)
	}
	sc := bufio.NewScanner(strings.NewReader(texto))
	inf := ""
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		switch {
		case l == "":
		case strings.HasPrefix(l, "#EXT-X-STREAM-INF"):
			inf = l
		case inf != "" && !strings.HasPrefix(l, "#"):
			carpeta, _, ok := strings.Cut(l, "/")
			q, errQ := strconv.Atoi(strings.TrimSuffix(carpeta, "p"))
			if !ok || errQ != nil || strings.Contains(l, "..") {
				return nil, nil, fmt.Errorf("%w: variante %q", ErrPlaylistInvalido, l)
			}
			vs = append(vs, variante{inf: inf, uri: l, calidad: q, banda: banda(inf)})
			inf = ""
		case len(vs) == 0 && inf == "":
			cabecera = append(cabecera, l)
		}
	}
	return cabecera, vs, sc.Err()
}

// banda es el BANDWIDTH de un #EXT-X-STREAM-INF (0 si no lo lleva).
func banda(inf string) int64 {
	_, attrs, _ := strings.Cut(inf, ":")
	for _, a := range strings.Split(attrs, ",") {
		if v, ok := strings.CutPrefix(a, "BANDWIDTH="); ok {
			n, _ := strconv.ParseInt(v, 10, 64)
			return n
		}
	}
	return 0
}

// fusionarMaster compone el master de las calidades `quedan`: de cada una, la
// variante del master nuevo si la trae (la acaba de generar ffmpeg) o la del
// actual. De mayor a menor tasa, que es como lo escribe ffmpeg y como lo
// esperan los reproductores que empiezan por la primera.
func fusionarMaster(actual, nuevo string, quedan []int) (string, error) {
	cab, viejas, err := parsearMaster(actual)
	if err != nil {
		return "", err
	}
	var nuevas []variante
	if strings.TrimSpace(nuevo) != "" {
		if _, nuevas, err = parsearMaster(nuevo); err != nil {
			return "", err
		}
	}
	elegida := map[int]variante{}
	for _, v := range viejas {
		elegida[v.calidad] = v
	}
	for _, v := range nuevas {
		elegida[v.calidad] = v
	}
	var out []variante
	for _, q := range quedan {
		v, ok := elegida[q]
		if !ok {
			return "", fmt.Errorf("%w: falta la variante %dp", ErrPlaylistInvalido, q)
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].banda > out[j].banda })
	var b strings.Builder
	for _, l := range cab {
		b.WriteString(l + "\n")
	}
	for _, v := range out {
		b.WriteString(v.inf + "\n" + v.uri + "\n")
	}
	return b.String(), nil
}
