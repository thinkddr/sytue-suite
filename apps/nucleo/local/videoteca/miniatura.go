// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package videoteca

import (
	"bufio"
	"context"
	"crypto/md5" //nolint:gosec // G501: el nombre de la miniatura lo fija freedesktop (MD5 del URI), no es seguridad
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// # La miniatura del sistema (CA-06)
//
// Sin ffmpeg ni un descodificador que lea el vídeo, el póster por defecto es la
// miniatura que ya sabe hacer el sistema:
//
//   - Linux y los BSD (freedesktop): primero la que ya esté en la caché de miniaturas
//     (`~/.cache/thumbnails/{xx-large,x-large,large,normal}/<md5 del URI>.png`, la que
//     dejó el gestor de ficheros); si no, la hace el miniaturizador instalado para su
//     tipo (`/usr/share/thumbnailers/*.thumbnailer`: ffmpegthumbnailer, totem…). Si no
//     hay ninguno, no hay miniatura y se queda sin póster (siempre se puede elegir una
//     imagen).
//   - macOS: Quick Look (`qlmanage -t`).
//   - Android: `MediaMetadataRetriever`, por el puente Java de la APK
//     (miniatura_android.go), en JPEG.
//   - Windows: la del Explorador (`IShellItemImageFactory::GetImage`, por COM en Go puro:
//     miniatura_explorador.go y miniatura_windows.go). Compilada en cruzado; sin probar
//     aún en un Windows de verdad.

// ErrSinMiniatura: el sistema no da miniatura de este vídeo.
var ErrSinMiniatura = errors.New("videoteca: el sistema no da miniatura de este vídeo")

// TamanoMiniatura es el lado que se pide a un miniaturizador.
const TamanoMiniatura = 512

// MiniaturaDelSistema devuelve la miniatura que da el sistema para el vídeo de ruta y su
// extensión («png»; «jpg» en Android).
func MiniaturaDelSistema(ctx context.Context, ruta string) ([]byte, string, error) {
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, "", err
	}
	return miniaturaDelSistema(ctx, abs)
}

// ---- freedesktop ----

// uriFichero es el URI con el que freedesktop nombra un fichero: `file://` y la ruta
// con lo que no es seguro escapado (las barras no).
func uriFichero(abs string) string {
	partes := strings.Split(filepath.ToSlash(abs), "/")
	for i, p := range partes {
		partes[i] = url.PathEscape(p)
	}
	return "file://" + strings.Join(partes, "/")
}

// enCacheFreedesktop busca la miniatura ya hecha, más nueva que el vídeo.
func enCacheFreedesktop(abs string) ([]byte, bool) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		casa, err := os.UserHomeDir()
		if err != nil {
			return nil, false
		}
		base = filepath.Join(casa, ".cache")
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, false
	}
	suma := md5.Sum([]byte(uriFichero(abs))) //nolint:gosec // G401: ver el import
	nombre := hex.EncodeToString(suma[:]) + ".png"
	for _, t := range []string{"xx-large", "x-large", "large", "normal"} {
		// G703: la caché de miniaturas de freedesktop de quien ejecuta la app, con el nombre
		// que manda la especificación (un md5 en hexadecimal).
		r := filepath.Join(base, "thumbnails", t, nombre)
		mi, err := os.Stat(r) //nolint:gosec // G703: ver arriba
		if err != nil || mi.ModTime().Before(fi.ModTime()) {
			continue
		}
		if datos, err := os.ReadFile(r); err == nil && len(datos) > 0 { //nolint:gosec // G703: ver arriba
			return datos, true
		}
	}
	return nil, false
}

// miniaturizador es una entrada de `*.thumbnailer`.
type miniaturizador struct {
	probar string
	exec   []string
	tipos  []string
}

// leerMiniaturizador lee un fichero `.thumbnailer` (formato de escritorio).
func leerMiniaturizador(ruta string) (miniaturizador, bool) {
	f, err := os.Open(ruta)
	if err != nil {
		return miniaturizador{}, false
	}
	defer f.Close()
	var m miniaturizador
	dentro := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(l, "[") {
			dentro = l == "[Thumbnailer Entry]"
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !dentro || !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "TryExec":
			m.probar = strings.TrimSpace(v)
		case "Exec":
			m.exec = partirExec(strings.TrimSpace(v))
		case "MimeType":
			for _, t := range strings.Split(v, ";") {
				if t = strings.TrimSpace(t); t != "" {
					m.tipos = append(m.tipos, t)
				}
			}
		}
	}
	return m, len(m.exec) > 0 && len(m.tipos) > 0
}

// partirExec parte una línea Exec en argumentos, con las comillas dobles de la
// especificación de entradas de escritorio. No pasa por ningún intérprete.
func partirExec(s string) []string {
	var out []string
	var b strings.Builder
	comillas, hay := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && comillas && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		case c == '"':
			comillas, hay = !comillas, true
		case (c == ' ' || c == '\t') && !comillas:
			if hay || b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
				hay = false
			}
		default:
			b.WriteByte(c)
		}
	}
	if hay || b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// tipoMIME es el tipo de un vídeo por su extensión (los miniaturizadores se eligen por
// tipo).
func tipoMIME(ruta string) string {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(ruta), ".")) {
	case "mp4":
		return "video/mp4"
	case "m4v":
		return "video/x-m4v"
	case "mov":
		return "video/quicktime"
	case "mkv":
		return "video/x-matroska"
	case "webm":
		return "video/webm"
	case "avi":
		return "video/x-msvideo"
	case "mpg", "mpeg":
		return "video/mpeg"
	case "ogv":
		return "video/ogg"
	case "3gp":
		return "video/3gpp"
	case "ts", "mts", "m2ts":
		return "video/mp2t"
	case "wmv":
		return "video/x-ms-wmv"
	case "flv":
		return "video/x-flv"
	}
	return ""
}

// carpetasMiniaturizadores son las de `thumbnailers` de XDG_DATA_DIRS (y la del
// usuario).
func carpetasMiniaturizadores() []string {
	dirs := os.Getenv("XDG_DATA_DIRS")
	if dirs == "" {
		dirs = "/usr/local/share:/usr/share"
	}
	var out []string
	if casa, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(casa, ".local", "share", "thumbnailers"))
	}
	for _, d := range filepath.SplitList(dirs) {
		out = append(out, filepath.Join(d, "thumbnailers"))
	}
	return out
}

// conMiniaturizador hace la miniatura con el miniaturizador instalado para su tipo.
func conMiniaturizador(ctx context.Context, abs string, carpetas []string) ([]byte, error) {
	tipo := tipoMIME(abs)
	if tipo == "" {
		return nil, ErrSinMiniatura
	}
	for _, c := range carpetas {
		fs, _ := filepath.Glob(filepath.Join(c, "*.thumbnailer"))
		for _, f := range fs {
			m, ok := leerMiniaturizador(f)
			if !ok || !casaTipo(m.tipos, tipo) {
				continue
			}
			if m.probar != "" {
				if _, err := exec.LookPath(m.probar); err != nil {
					continue
				}
			}
			if datos, err := ejecutarMiniaturizador(ctx, m, abs); err == nil {
				return datos, nil
			}
		}
	}
	return nil, ErrSinMiniatura
}

func casaTipo(tipos []string, tipo string) bool {
	for _, t := range tipos {
		if t == tipo || t == "video/*" || (strings.HasSuffix(t, "/*") && strings.HasPrefix(tipo, strings.TrimSuffix(t, "*"))) {
			return true
		}
	}
	return false
}

func ejecutarMiniaturizador(ctx context.Context, m miniaturizador, abs string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "sytue-miniatura-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	salida := filepath.Join(dir, "miniatura.png")
	args := make([]string, 0, len(m.exec))
	for _, a := range m.exec {
		a = strings.NewReplacer("%%", "%", "%i", abs, "%u", uriFichero(abs), "%o", salida, "%s", strconv.Itoa(TamanoMiniatura)).Replace(a)
		args = append(args, a)
	}
	c, fin := context.WithTimeout(ctx, 30*time.Second)
	defer fin()
	cmd := exec.CommandContext(c, args[0], args[1:]...) //nolint:gosec // G204: el miniaturizador lo instaló el sistema; sin intérprete
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	datos, err := os.ReadFile(salida)
	if err != nil || len(datos) == 0 {
		return nil, ErrSinMiniatura
	}
	return datos, nil
}
