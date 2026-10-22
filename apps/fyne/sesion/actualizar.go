// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/firma"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// # El actualizador propio de las apps de escritorio
//
// El Hub dice qué versión hay publicada de cada app, con su SHA-256
// (`GET /api/v1/instaladores`, con el token de la sesión: tenerla ya es tener derecho,
// como en la página de descargas del portal). Aquí se decide si es posterior a la que
// corre, se baja, se comprueba la suma y se le pasa al sistema
// (`escritorio.Instalar`). Sin suma no se instala nada: no habría manera de saber que
// lo bajado es lo que se publicó.
//
// Lo hace el Hub, que es la app que se queda en la bandeja, y pone al día a la vez a sus
// hermanas: las apps de la suite instaladas junto a él (`escritorio.Hermanas`, en macOS
// y Linux; en Windows el instalador de su bundle las pone todas).
//
// En escritorio se publica un instalador por BUNDLE, no por app: el Hub se pone al día
// con el de «Sytue One» y Drive con el de «Sytue Drive». El catálogo del Hub ya lo
// cuenta así —una entrada por cada app que lleva un bundle, con el fichero de ese
// bundle (services/hub/internal/instaladores)—, de modo que aquí se sigue eligiendo por
// app y lo único nuevo es no bajar el mismo bundle una vez por cada una.

// cadaCuanto es cada cuánto vuelve a mirar el Hub residente, que puede pasar semanas
// abierto en la bandeja.
const cadaCuanto = 12 * time.Hour

// errSuma es que lo bajado no es lo publicado.
var errSuma = errors.New("sesion: el instalador bajado no tiene la suma publicada")

// errSumaFirmada es que la suma que da el Hub no es la del SHA256SUMS firmado.
var errSumaFirmada = errors.New("sesion: el instalador no está en el SHA256SUMS firmado de su versión, o con otra suma")

// conFirma comprueba, con la clave de publicación embebida, que el SHA256SUMS firmado
// es de la versión del instalador y le da a ese fichero la misma suma que el Hub. Así
// la suma que luego se compara con lo bajado es la firmada, no la que diga el servidor
// (auditoría 2026-10-06, CI-05).
func conFirma(pub, sumas []byte, sig string, i api.Instalador) error {
	lista, err := firma.VerificarSumas(pub, sumas, sig, i.Version)
	if err != nil {
		return err
	}
	suma, ok := lista[filepath.Base(strings.ReplaceAll(i.Fichero, `\`, "/"))]
	if !ok || !strings.EqualFold(suma, i.SHA256) {
		return errSumaFirmada
	}
	return nil
}

// Pendiente es una actualización por poner: la de esta app o la de una hermana
// (`escritorio.Hermanas`).
type Pendiente struct {
	api.Instalador
	// Destino es dónde está instalada la hermana; vacío, es esta app.
	Destino string
}

// BuscarActualizaciones pregunta al Hub qué hay publicado para este sistema y devuelve
// lo que es posterior a lo instalado: las hermanas primero y, la ÚLTIMA, esta app, que
// es la que se cierra al ponerse. No hay nada que buscar en el móvil, en un `go run` ni
// en una app sin versión grabada.
func (s *Suite) BuscarActualizaciones(ctx context.Context) ([]Pendiente, error) {
	exe, _ := os.Executable()
	plataforma, app, actual := escritorio.Plataforma(), escritorio.AppDeEjecutable(exe), s.app.Metadata().Version
	if plataforma == "" || app == "" || actual == "" || s.app.Driver().Device().IsMobile() {
		return nil, nil
	}
	lista, err := s.Hub.Instaladores(ctx, plataforma)
	if err != nil {
		return nil, err
	}
	return pendientesDe(lista, runtime.GOARCH, app, actual, escritorio.Hermanas()), nil
}

// pendientesDe decide, de lo publicado, qué hay que poner. Una hermana cuya versión no
// dice el sistema (Linux) se supone de la misma que esta: se instalaron juntas.
func pendientesDe(lista []api.Instalador, arq, app, actual string, hermanas []escritorio.Hermana) []Pendiente {
	var out []Pendiente
	for _, h := range hermanas {
		if i, ok := elegirActualizacion(lista, arq, h.App, cmp.Or(h.Version, actual)); ok {
			out = append(out, Pendiente{Instalador: i, Destino: h.Destino})
		}
	}
	if i, ok := elegirActualizacion(lista, arq, app, actual); ok {
		out = append(out, Pendiente{Instalador: i})
	}
	return out
}

// elegirActualizacion es el instalador de `app` posterior a `actual` para el procesador
// `arq` (`runtime.GOARCH`), si lo hay y trae su suma. Vale el de `arq` o un universal, y
// NADA MÁS: si falta el de este procesador no se cae a otro —un x86_64 en un ARM no
// arranca, y en Windows ARM la emulación es lenta y no es lo que se instaló—, sino que
// para esta máquina no hay actualización. Uno sin arquitectura (un Hub anterior) tampoco.
func elegirActualizacion(lista []api.Instalador, arq, app, actual string) (api.Instalador, bool) {
	for _, i := range lista {
		if i.App == app && (i.Arquitectura == arq || i.Arquitectura == "universal") &&
			i.SHA256 != "" && escritorio.MasNueva(actual, i.Version) {
			return i, true
		}
	}
	return api.Instalador{}, false
}

// Actualizar baja cada instalador, comprueba su SHA-256 y lo pone: las hermanas en su
// sitio, y esta app la última. Si esta se ha puesto, la CIERRA: el instalador, o la
// copia nueva que se abre sola, termina el trabajo. Una hermana que falla no para a las
// demás; lo que haya fallado vuelve junto.
func (s *Suite) Actualizar(ctx context.Context, pendientes []Pendiente) error {
	base, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(base, "sytue", "actualizaciones")
	// LA FIRMA. Con clave embebida, sin SHA256SUMS firmado no se instala nada. Sin
	// ella (hasta que la publicación firme), se sigue con la suma del Hub y se dice.
	pub := firma.Publica()
	var (
		sumas []byte
		sig   string
	)
	if pub == nil {
		slog.Warn("sesion: este binario no trae clave de publicación; se instala con la suma del Hub")
	} else if sumas, sig, err = s.Hub.SumasInstaladores(ctx); err != nil {
		return fmt.Errorf("sesion: sin SHA256SUMS firmado no se actualiza: %w", err)
	}
	var fallos []error
	// Cada bundle se baja UNA vez, y si falla no se reintenta por cada app que lleva.
	type bajada struct {
		ruta string
		err  error
	}
	bajados := map[string]bajada{}
	for _, p := range pendientes {
		if pub != nil {
			if err := conFirma(pub, sumas, sig, p.Instalador); err != nil {
				fallos = append(fallos, fmt.Errorf("%s: %w", p.App, err))
				continue
			}
		}
		b, ok := bajados[p.Fichero]
		if !ok {
			b.ruta, b.err = bajarComprobado(ctx, s.Hub, p.Instalador, dir)
			bajados[p.Fichero] = b
		}
		ruta, err := b.ruta, b.err
		switch {
		case err != nil:
		case p.Destino != "":
			err = escritorio.InstalarHermana(ctx, ruta, p.Destino)
		default:
			if err = escritorio.Instalar(ctx, ruta); err == nil {
				fyne.Do(s.app.Quit)
			}
		}
		if err != nil {
			fallos = append(fallos, fmt.Errorf("%s: %w", p.App, err))
		}
	}
	return errors.Join(fallos...)
}

// bajarComprobado deja el instalador en `dir` con su nombre y solo si su SHA-256 es la
// publicada; si no, no deja nada. Se escribe primero con otro nombre, así que un
// instalador a medias nunca se confunde con uno bueno.
func bajarComprobado(ctx context.Context, hub *api.Hub, i api.Instalador, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	nombre := filepath.Base(strings.ReplaceAll(i.Fichero, `\`, "/"))
	f, err := os.CreateTemp(dir, nombre+".*.parte")
	if err != nil {
		return "", err
	}
	suma := sha256.New()
	err = hub.BajarInstalador(ctx, i, io.MultiWriter(f, suma))
	err = errors.Join(err, f.Close())
	if err == nil && !strings.EqualFold(hex.EncodeToString(suma.Sum(nil)), i.SHA256) {
		err = errSuma
	}
	if err != nil {
		return "", errors.Join(err, os.Remove(f.Name()))
	}
	final := filepath.Join(dir, nombre)
	if err := os.Rename(f.Name(), final); err != nil {
		return "", errors.Join(err, os.Remove(f.Name()))
	}
	return final, nil
}

// ElementoActualizar es «Buscar actualizaciones» para el menú de la bandeja. Lo mira
// también al arrancar y cada doce horas; si hay versión nueva, lo avisa en el sistema
// (pulsar el aviso la instala) y el elemento pasa a «Actualizar a la versión X».
func (s *Suite) ElementoActualizar(t *i18n.Translator, menu *fyne.Menu) *fyne.MenuItem {
	item := fyne.NewMenuItem(t.T("shell.update.check", nil), nil)
	nombre := s.app.Metadata().Name
	var (
		mu      sync.Mutex
		avisada string // la versión de la que ya se ha avisado, para no repetirlo
	)
	instalar := func(pendientes []Pendiente) {
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), 30*time.Minute)
			defer parar()
			if err := s.Actualizar(ctx, pendientes); err != nil {
				slog.Warn("sesion: no se ha podido actualizar", "error", err)
				s.avisarSistema(t.T("shell.update.failed", i18n.Vars{"app": nombre}), t.T("shell.update.failedBody", nil), nil)
			}
		}()
	}
	buscar := func(aMano bool) {
		ctx, parar := context.WithTimeout(context.Background(), 30*time.Second)
		defer parar()
		pendientes, err := s.BuscarActualizaciones(ctx)
		switch {
		case err != nil:
			slog.Info("sesion: no se ha podido buscar actualizaciones", "error", err)
			if aMano {
				s.avisarSistema(t.T("shell.update.failed", i18n.Vars{"app": nombre}), t.T("shell.update.failedBody", nil), nil)
			}
		case len(pendientes) == 0:
			if aMano {
				s.avisarSistema(t.T("shell.update.upToDate", i18n.Vars{"app": nombre}),
					t.T("shell.update.current", i18n.Vars{"version": s.app.Metadata().Version}), nil)
			}
		default:
			// La versión que se enseña es la de la última: esta app si tiene, o una hermana.
			version := pendientes[len(pendientes)-1].Version
			fyne.Do(func() {
				item.Label = t.T("shell.update.install", i18n.Vars{"version": version})
				item.Action = func() { instalar(pendientes) }
				menu.Refresh()
			})
			mu.Lock()
			repetido := avisada == version
			avisada = version
			mu.Unlock()
			if !repetido {
				s.avisarEnLaCampana(nombre, version)
			}
			if aMano || !repetido {
				s.avisarSistema(t.T("shell.update.available", i18n.Vars{"app": nombre}),
					t.T("shell.update.availableBody", i18n.Vars{"version": version}), func() { instalar(pendientes) })
			}
		}
	}
	item.Action = func() { go buscar(true) }
	// Al arrancar y cada doce horas, pero solo con sesión: sin ella el Hub diría 401, y
	// quien acaba de entrar tiene que enterarse ya, no dentro de doce horas.
	go func() {
		for {
			if s.Sesion.Subject() == "" {
				time.Sleep(time.Minute)
				continue
			}
			buscar(false)
			time.Sleep(cadaCuanto)
		}
	}()
	return item
}

// avisarEnLaCampana deja la versión nueva también en la campana de la suite, que se
// queda hasta que se lee: el aviso del sistema se va si nadie lo mira. Una vez por
// versión aunque la app se reinicie; si el Hub no contesta, se intenta en la próxima
// búsqueda.
func (s *Suite) avisarEnLaCampana(nombre, version string) {
	const clave = "actualizacion.enLaCampana"
	p := s.app.Preferences()
	if s.Hub == nil || p.String(clave) == version {
		return
	}
	ctx, parar := context.WithTimeout(context.Background(), 10*time.Second)
	defer parar()
	err := s.Hub.AvisarAMi(ctx, api.HubAvisoPropio{
		Source: "one", Type: "one.update.available",
		Title:  nombre + " " + version + " disponible",
		URL:    api.AppURL(s.Apex(), "one", s.Organizacion(), "/descargas"),
		Params: map[string]string{"app": nombre, "version": version},
	})
	if err != nil {
		slog.Info("sesion: no se ha podido dejar la actualización en la campana", "error", err)
		return
	}
	p.SetString(clave, version)
}

// avisarSistema enseña un aviso de la app en el sistema; pulsarlo llama a `alPulsar`.
// Donde el sistema no sabe avisar del clic sale el de Fyne, sin él.
func (s *Suite) avisarSistema(titulo, cuerpo string, alPulsar func()) {
	m := s.app.Metadata()
	if err := escritorio.Avisar(escritorio.Aviso{
		ID: m.ID, Nombre: m.Name, Esquema: m.Custom["esquema"], Titulo: titulo, Cuerpo: cuerpo, AlPulsar: alPulsar,
	}); err != nil {
		AvisoDeRespaldo(s.app, titulo, cuerpo)
	}
}
