// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"errors"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/fyne/anfitrion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/avisos"
	"git.sytue.eu/sytue/one/apps/nucleo/local/copia"
	"git.sytue.eu/sytue/one/apps/nucleo/local/cuentas"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// Los cimientos transversales de la cáscara: el portero de red (§5.8), las cuentas de
// toda la edición (§5.10), la copia de seguridad (§5.13) y los avisos programados
// (§5.12). Los monta Abrir.

// instalarPortero deja el portero a cargo de TODA la red del proceso: los clientes de la
// IA y de la nube salen por él, y lo que no pase por él (http.DefaultTransport,
// api.Transporte) falla cerrado.
func (l *App) instalarPortero() {
	l.Red = red.Abrir(l.Espacio.Ajuste("red.json"))
	l.Cuentas = cuentas.Abrir(l.Espacio.Rutas.Ajustes, l.Espacio.Rutas.Datos)

	// La IA: la dirección del proveedor elegido, si hay uno.
	l.Red.Fuente(func() []red.Destino {
		dir := l.IA.Direccion()
		if dir == "" {
			return nil
		}
		d, err := red.DestinoDeURL(red.IA, dir, l.DestinoIA())
		if err != nil {
			return nil
		}
		return []red.Destino{d}
	})
	// La nube: los servidores de cada cuenta conectada.
	l.Red.Fuente(func() []red.Destino {
		var ds []red.Destino
		for _, c := range l.Nube.Cuentas() {
			for _, h := range nube.Hosts(c) {
				ds = append(ds, red.Destino{Proposito: red.Nube, Host: h, Motivo: l.T("red.motivo.nube") + " " + c.Rotulo})
			}
		}
		return ds
	})
	// Correo y mensajería: lo que guardan Mail y Rooms (cuentas).
	l.Red.Fuente(l.Cuentas.Destinos)

	l.IA.HTTP = &http.Client{Transport: l.Red.Transporte(red.IA), Timeout: 125 * time.Second}
	l.Nube.HTTP = &http.Client{Transport: l.Red.Transporte(red.Nube), Timeout: 60 * time.Second}

	http.DefaultTransport = red.Denegar{}
	http.DefaultClient = &http.Client{Transport: red.Denegar{}}
	api.Transporte = red.Denegar{}
}

// prepararCopia y prepararAvisos se hacen al abrir; no tocan la red.
func (l *App) prepararCopia() {
	l.Copia = copia.Nueva(l.Espacio.Rutas.Datos, l.Espacio.Rutas.Ajustes, l.Producto)
}

func (l *App) prepararAvisos() {
	var s avisos.Sistema
	switch runtime.GOOS {
	case "windows":
		s = avisos.Windows{AppID: l.A.Metadata().ID}
	case "darwin", "ios", "android":
		s = avisosFyne{l.A}
	default:
		// Linux y el resto: un reloj del proceso, que avisa por el aviso del sistema (con
		// clic que abre el elemento) mientras la app siga abierta.
		s = &avisos.Temporizador{Avisar: l.avisarAhora}
	}
	l.Avisos = avisos.Abrir(l.Espacio.Ajuste("avisos-"+l.Producto+".json"), s)
}

// avisosFyne es el [avisos.Sistema] del `ScheduleNotification` de Fyne: en macOS e iOS
// `UNUserNotificationCenter`, en Android `AlarmManager`.
type avisosFyne struct{ a fyne.App }

func (f avisosFyne) Programar(_ string, a avisos.Aviso) (string, error) {
	return anfitrion.ProgramarAviso(f.a, a.Titulo, a.Cuerpo, a.Cuando)
}

func (f avisosFyne) Cancelar(id string) error { return anfitrion.CancelarAviso(f.a, id) }

func (l *App) avisarAhora(a avisos.Aviso) {
	m := l.A.Metadata()
	err := escritorio.Avisar(escritorio.Aviso{
		ID: m.ID, Nombre: m.Name, Esquema: espacio.Esquema(l.Producto), Titulo: a.Titulo, Cuerpo: a.Cuerpo,
		AlPulsar: func() {
			if a.Enlace != "" {
				fyne.Do(func() { l.recibirTexto(a.Enlace) })
			}
		},
	})
	if err != nil {
		anfitrion.AvisoDeRespaldo(l.A, a.Titulo, a.Cuerpo)
	}
}

// Recordar programa los avisos de un origen («calendar») para los próximos 30 días,
// sustituyendo a los de antes. Es lo que llaman las apps al abrir y al cambiar algo.
func (l *App) Recordar(origen string, lista []avisos.Aviso) error {
	return l.Avisos.Reprogramar(origen, lista, time.Now())
}

// ----- Elementos que llegan por enlace -----

// AlAbrirElemento registra cómo abre esta app un elemento suyo que llega por enlace
// (`sytueoffice-<producto>:/abrir?clase=<clase>&id=<id>`: el Hub, un aviso, otra app).
// Lo que llegó antes de registrarlo (o antes de que la app arrancara) se entrega en
// cuanto se pueda, en orden.
func (l *App) AlAbrirElemento(f func(clase, id string)) {
	l.mu.Lock()
	l.abrirElemento = f
	l.mu.Unlock()
	l.vaciarPendientes()
}

func (l *App) entregarElemento(clase, id string) {
	l.mu.Lock()
	f := l.abrirElemento
	if f == nil || !l.arrancada {
		l.elementos = append(l.elementos, [2]string{clase, id})
		l.mu.Unlock()
		return
	}
	l.mu.Unlock()
	fyne.Do(func() { f(clase, id) })
}

// ----- Otros enlaces que llegan -----

// AlAbrirEnlace registra qué hace esta app con un enlace que no es un fichero
// (`?local=`) ni un elemento (`?clase=…&id=…`): `mailto:` y `sytueoffice-mail:/redactar?…`
// en Mail, `sytueoffice-calendar:/events/new?action=create…` en Calendar. Llegan por
// `SetOnOpenURL` (macOS, iOS, Android) o por los argumentos (Linux, Windows). El enlace es
// un dato no confiable: quien lo recibe lo lee estricto. Lo que llegó antes de
// registrarlo (o antes de que la app arrancara) se entrega en cuanto se pueda, en orden,
// en el hilo de la interfaz.
func (l *App) AlAbrirEnlace(f func(*url.URL)) {
	l.mu.Lock()
	l.abrirEnlace = f
	l.mu.Unlock()
	l.vaciarPendientes()
}

func (l *App) entregarEnlace(u *url.URL) {
	l.mu.Lock()
	f := l.abrirEnlace
	if f == nil || !l.arrancada {
		if len(l.enlaces) < 32 { // nadie lo atiende aún; no se acumulan sin fin
			l.enlaces = append(l.enlaces, u)
		}
		l.mu.Unlock()
		return
	}
	l.mu.Unlock()
	fyne.Do(func() { f(u) })
}

// alArrancar es el SetOnStarted de la app: desde ahora se entrega lo que espera.
func (l *App) alArrancar() {
	l.mu.Lock()
	l.arrancada = true
	l.mu.Unlock()
	l.vaciarPendientes()
}

// vaciarPendientes entrega, si ya se puede, los elementos y enlaces que esperan.
func (l *App) vaciarPendientes() {
	l.mu.Lock()
	if !l.arrancada {
		l.mu.Unlock()
		return
	}
	fe, fu := l.abrirElemento, l.abrirEnlace
	var elementos [][2]string
	var enlaces []*url.URL
	if fe != nil {
		elementos, l.elementos = l.elementos, nil
	}
	if fu != nil {
		enlaces, l.enlaces = l.enlaces, nil
	}
	l.mu.Unlock()
	for _, e := range elementos {
		fyne.Do(func() { fe(e[0], e[1]) })
	}
	for _, u := range enlaces {
		fyne.Do(func() { fu(u) })
	}
}

// ----- Abrir entre apps (§5.11) -----

// errSinApp: el esquema de esa app no lo atiende nadie en este equipo.
var errSinApp = errors.New("local: la app de Sytue Office no está instalada")

// AbrirEn abre el fichero ruta en la app de Sytue Office de producto, por su esquema
// (`sytueoffice-<producto>:/abrir?local=file:///…`), y, si no está instalada o producto
// va vacío, con la app predeterminada del sistema. NUNCA cae a la web.
func AbrirEn(a fyne.App, producto, ruta string) error {
	if producto != "" {
		err := errSinApp
		if hay, sabido := hayAppPara(api.EsquemaOffice + producto); hay || !sabido {
			err = api.AbrirFicheroEnOffice(a, producto, storage.NewFileURI(ruta).String())
		}
		if err == nil {
			return nil
		}
	}
	return ficheros.Abrir(ruta)
}

// AbrirEn es [AbrirEn] desde esta app.
func (l *App) AbrirEn(producto, ruta string) error { return AbrirEn(l.A, producto, ruta) }

// HayApp dice si la app de Sytue Office de producto está instalada en este equipo (si
// alguien atiende su esquema), y sabido si se ha podido saber: donde no (macOS, el
// móvil, un Linux sin `xdg-mime`), sabido es false y quien pregunta la da por instalada,
// como AbrirEn. Puede lanzar un proceso: fuera del hilo de la interfaz.
func HayApp(producto string) (hay, sabido bool) {
	return hayAppPara(api.EsquemaOffice + producto)
}

// AbrirEnlace da al sistema un enlace a otra app de Sytue Office
// (`sytueoffice-<producto>:/abrir?clase=…&id=…`), sin caer a la web.
func (l *App) AbrirEnlace(enlace string) error { return api.AbrirEnOffice(l.A, enlace) }

// HayApp es [HayApp] desde esta app, en un solo valor: false solo si el sistema dice que
// nadie atiende el esquema; donde no se sabe, true. Fuera del hilo de la interfaz.
func (l *App) HayApp(producto string) bool {
	hay, sabido := HayApp(producto)
	return hay || !sabido
}
