// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package sesion

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ErrSoloLectura es lo que recibe quien intenta escribir con `SYTUE_SOLO_LECTURA`.
var ErrSoloLectura = errors.New("sesion: solo lectura (SYTUE_SOLO_LECTURA), la petición no se envía")

// soloLectura corta ANTES de la red todo lo que no sea leer. Es para las tiradas de
// ventana real (`scripts/ventana-real.sh`), que recorren las apps con la cuenta de
// auditoría contra producción pulsando en coordenadas fijas: el 2026-10-05 una
// coordenada desplazada cayó en «Nueva nota» y el recorrido creaba, por diseño, un
// documento de plantilla en cada editor. Con esto un clic puede acabar en cualquier
// botón y no se escribe nada.
//
// Deja pasar el POST al `/token` del IdP: el canje y la renovación de la sesión, sin los
// que no hay ni lectura.
//
// ponytail: solo HTTP. Un WebSocket ya abierto (el chat de Rooms, la edición de un
// documento) sigue pudiendo escribir, pero para eso hace falta TECLEAR, no un clic; si
// el recorrido llega a escribir dentro de un canal o de un documento, se corta aquí la
// petición de `Upgrade`.
type soloLectura struct{ base http.RoundTripper }

func (s soloLectura) RoundTrip(r *http.Request) (*http.Response, error) {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return s.base.RoundTrip(r)
	case http.MethodPost:
		if strings.HasSuffix(r.URL.Path, "/token") {
			return s.base.RoundTrip(r)
		}
	}
	return nil, ErrSoloLectura
}

var unaVezSoloLectura sync.Once

// ponerSoloLectura envuelve el transporte por defecto, que es por donde salen todos los
// clientes de la suite: los de `api` (http.DefaultClient), el de la caché «red primero»
// y el de la sesión. Lo llama Abrir.
func ponerSoloLectura() {
	if os.Getenv("SYTUE_SOLO_LECTURA") == "" {
		return
	}
	unaVezSoloLectura.Do(func() {
		http.DefaultTransport = soloLectura{http.DefaultTransport}
		slog.Warn("SYTUE_SOLO_LECTURA: esta app no envía ninguna escritura")
	})
}

// Conexion arma el cliente HTTP de una app con la caché «red primero» de la suite y el
// monitor que ella alimenta. Sin monitor la pantalla nunca sabe que no hay red: se queda
// en «no se pudo cargar» y deja intentar mutaciones.
//
// Nació en `cmd/drive` y sube aquí porque es de todas las apps, como la sesión: lo único
// que cambia de una a otra es el producto y qué rutas NO se guardan (`privadas`). Lo que
// se guarda es lo que la web guarda (las reglas de `worker.ts`), por persona y en la
// caché del sistema. En Drive, el CONTENIDO de los ficheros, las miniaturas y los ZIP no:
// son descargas, no listados, y guardarlos sería copiar la unidad entera sin que nadie
// lo pida.
func (s *Suite) Conexion(producto string, privadas ...string) (*offline.Monitor, *http.Client) {
	red := &offline.Monitor{}
	// Recuperar la red es un disparador del widget: lo cambiado sin ella no llegó.
	red.Subscribe(func(e offline.State) {
		if e == offline.Online {
			s.AvisarWidget()
		}
	})
	t := &offline.Transport{
		Base:          api.Transporte, // con plazo (FYS-11)
		Owner:         s.dueñoCache,
		Product:       producto,
		Monitor:       red,
		Private:       privadas,
		CacheProducts: api.Productos(),
		Scope:         s.scopeCache,
	}
	var err error
	t.Store, err = Almacen()
	if err != nil {
		// El transporte pasa la red sin caché mientras no haya almacén. El resolver
		// permite adoptar el journal verificado tras completar la recuperación.
		slog.Warn("sin caché para trabajar sin conexión", "producto", producto, "error", err)
	}
	initial := t.Store
	t.ResolveStore = func(context.Context) (*offline.Store, error) {
		if current := s.cacheAlmacen.Load(); current != nil {
			return current.store, nil
		}
		return initial, nil
	}
	return red, &http.Client{Transport: t}
}

// scopeCache rejects the previous session before the cross-app watcher adopts it.
// It contains no token and is checked only around fallback/persistence, not per byte.
func (s *Suite) scopeCache() string {
	s.mu.Lock()
	generation := s.generacionSesion
	s.mu.Unlock()
	if generation == "" || ajustes.Leer().Sesion != generation {
		return ""
	}
	return generation
}

// Almacen abre el almacén local de la suite, en la caché del sistema: el mismo que
// usa la caché «red primero» de Conexion, y donde una app guarda lo que no puede
// perder sin red (los borradores y la cola de capturas de Notepads).
func Almacen() (*offline.Store, error) {
	return AlmacenConContexto(context.Background())
}

func AlmacenConContexto(ctx context.Context) (*offline.Store, error) {
	dir, err := directorioAlmacenActivo(ctx)
	if err != nil {
		return nil, err
	}
	return offline.OpenContext(ctx, dir)
}

func directorioAlmacen() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sytue", "offline"), nil
}

func contarAlmacen(espacios ...string) (int, error) {
	return contarAlmacenCon(context.Background(), espacios...)
}

func contarAlmacenCon(ctx context.Context, espacios ...string) (int, error) {
	dir, err := directorioAlmacenActivo(ctx)
	if err != nil {
		return 0, err
	}
	return offline.CountDirectory(dir, espacios...)
}

// vaciarAlmacen removes disposable responses and legacy drafts after logout.
// Durable private journals and source stages of every account remain protected;
// explicit current-scope discard is a separate action under its exclusive lease.
func (s *Suite) vaciarAlmacenCon(ctx context.Context) error {
	dir, err := s.directorioCacheActual(ctx)
	if err != nil {
		return err
	}
	moved, err := s.cacheReubicada(ctx)
	if err != nil {
		return err
	}
	namespaces := storage.AllPrivateJournalNamespaces()
	if moved {
		namespaces = append(namespaces, notas.EspacioCapturas, notas.EspacioBorradores)
	}
	for _, product := range storage.Products() {
		namespaces = append(namespaces, storage.PrivateStagingNamespaces(product)...)
	}
	return offline.ClearDirectoryKeepingContext(ctx, dir, namespaces...)
}

func directorioAlmacenActivo(ctx context.Context) (string, error) {
	source, err := directorioAlmacen()
	if err != nil || !almacenAplicacionSeparado {
		return source, err
	}
	app, err := productoAplicacionNativa(fyne.CurrentApp())
	if err != nil {
		return "", err
	}
	target, err := directorioAlmacenAplicacion(app)
	if err != nil {
		return "", err
	}
	retired, err := offline.JournalRelocationRetired(ctx, source, target)
	if err != nil {
		return "", err
	}
	if retired {
		return target, nil
	}
	return source, nil
}
