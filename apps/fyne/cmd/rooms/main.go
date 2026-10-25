// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command rooms abre Sytue Rooms en una ventana de verdad.
//
// Es SU PROPIA aplicación, como en la web es su propio subdominio: un binario
// instalable por producto (`docs/CONVENTIONS.md` §13.6). Ir a otro producto de la
// suite no es una pantalla de aquí, es invocar a su aplicación por el esquema de URL
// que declara su `FyneApp.toml`.
//
// Para VERLA hace falta una máquina con pantalla; el piloto no la tiene. Lo que sí
// corre en cualquier sitio, sin CGO y sin servidor gráfico, es
// `go test ./apps/rooms/...`, que pinta lo mismo con el pintor de software y compara
// los píxeles contra lo medido en producción.
package main

import (
	"context"
	"net/url"
	"path/filepath"
	"runtime"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/rooms"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/altavoz"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")

	// El idioma lo decide el SISTEMA, igual que en la web lo decide el middleware:
	// nunca se detecta dentro de una vista. Uno que la suite no sirva cae al inglés.
	l := s.Idioma()
	t, err := rooms.Traductor(l)
	if err != nil {
		panic(err) // el catálogo va embebido: si falla, es un error de compilación
	}

	p := rooms.NuevaPantalla(t, l)
	// Los datos: el mismo servicio que la web, con la sesión transversal. Se arranca
	// en cuanto hay alguien dentro; sin sesión, se abre Sytue ID en el navegador del
	// sistema, que es lo que hace la web al llegar sin cookie.
	base := api.NewRooms(s.URL("rooms"), s.Sesion, nil)
	var secure *rooms.Cifrado
	content := mx.NewRoomsService(base, func(ctx context.Context) (*mx.Client, error) { return secure.Cliente(ctx) })
	sala := rooms.NuevaSala(p, content, "")
	sala.Widget = s.CapturarWidget
	store, storeErr := ajustes.Carpeta()
	secure = &rooms.Cifrado{Sala: sala, API: base, Base: mx.Config{SytueApex: s.Apex(), StoreDir: filepath.Join(store, "rooms-matrix"), DeviceName: "Sytue Rooms · " + runtime.GOOS}, Abrir: func(target string) error { return api.AbrirFuera(a, target) }}
	if storeErr != nil {
		secure.Base.StoreDir = ""
	}
	secure.Login = func(ctx context.Context, cfg mx.Config) (*mx.Client, error) {
		if runtime.GOOS == "ios" || runtime.GOOS == "android" {
			return mx.LoginNative(ctx, cfg, base, s.MatrixBrowserReturn)
		}
		return mx.Login(ctx, cfg, func(target string) error {
			opened := make(chan error, 1)
			fyne.Do(func() { opened <- api.AbrirFuera(a, target) })
			select {
			case err := <-opened:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}
	secure.AlEvento = sala.EventoCifrado
	sala.Cifrado = secure
	p.Lateral.TieneCifrado = true
	sala.Apex = s.Apex()
	sala.Org = s.Sesion.Organizacion
	sala.CuentasApps = api.NewIntegraciones(s.URL("integrations"), s.Sesion, nil)
	sala.AutorizarApp = func(ctx context.Context, template, instance string) (string, error) {
		return s.AutorizarApp(ctx, sala.CuentasApps, template, instance, "native-rooms")
	}
	sala.AutorizarAppNueva = func(ctx context.Context, template string) (string, error) {
		return s.AutorizarAppNueva(ctx, sala.CuentasApps, template, "native-rooms")
	}
	sala.AutorizarConversaciones = func(ctx context.Context, provider, instance string) (string, error) {
		return s.AutorizarConversaciones(ctx, sala.CuentasApps, provider, instance)
	}
	sala.AutorizarReunion = func(ctx context.Context, provider, instance string) (string, error) {
		return s.AutorizarReunion(ctx, sala.CuentasApps, provider, instance)
	}
	sala.GeneracionSesion = func() string { return s.Estado().GeneracionSesion }
	p.Lateral.TieneConversaciones = true
	// Server AI has no access to encrypted Rooms message content.
	sala.Almacen = s.PorPersona()
	sala.Presencias = s.Hub.Presencias
	sala.PrefsAviso, sala.PonerPrefAviso = s.Hub.NotificationPrefs, s.Hub.SetNotificationPref
	sala.Ficheros = content
	sala.ElegirFicheros, sala.Revelar, sala.AbrirFichero = ficheros.Elegir, ficheros.Revelar, ficheros.Abrir
	// La llamada NATIVA (ADR 0046 enmendado): el mismo SFU y el mismo token que la web,
	// con la tarjeta de sonido del sistema. El micrófono se pide al abrir la antesala.
	// Sin ella (js/wasm, que solo se compila para comprobar tipos) Conectar se queda nil y
	// la llamada se abre en la web.
	if llamada.Disponible {
		sala.Conectar = func(u, token string, o llamada.Opciones) (rooms.EnCurso, error) {
			o.Audio = altavoz.Sistema{Micro: sala.MicroElegido, Altavoz: sala.AltavozElegido}
			return llamada.Entrar(u, token, o)
		}
		sala.AudioDe = func(micro, alt string) llamada.Dispositivo {
			return altavoz.Sistema{Micro: micro, Altavoz: alt}
		}
		sala.Dispositivos = func() (micros, altavoces, camaras []rooms.Opcion) {
			ms, as := altavoz.Dispositivos()
			for _, d := range ms {
				micros = append(micros, rooms.Opcion{ID: d.ID, Nombre: d.Nombre})
			}
			for _, d := range as {
				altavoces = append(altavoces, rooms.Opcion{ID: d.ID, Nombre: d.Nombre})
			}
			for _, d := range camara.Dispositivos() {
				camaras = append(camaras, rooms.Opcion{ID: d.ID, Nombre: d.Nombre})
			}
			return micros, altavoces, camaras
		}
		sala.Camara = func() (camara.Fuente, error) {
			p := camara.PedidoPorOmision
			p.ID = sala.CamaraElegida
			return camara.Abrir(p)
		}
		// Compartir la pantalla en escritorio y en Android (MediaProjection). En iOS pide
		// una extensión de ReplayKit que `fyne package` no construye (docs/roadmap.md), y
		// un botón que falla no se ofrece.
		if runtime.GOOS != "ios" {
			camara.TextoAvisoPantalla = t.T("meet.shareScreen", nil)
			sala.Pantalla = camara.AbrirPantalla
		}
	}
	sala.PedirMicro = altavoz.PedirMicro
	sala.AlPonerPresencia = func(ctx context.Context, e api.EstadoPresencia, texto string) {
		_ = s.PonerPresencia(ctx, e, texto) // el Hub la reparte; un fallo lo arregla el próximo latido
	}
	ctx := context.Background()
	arrancada := false
	identidad := ""
	secure.AlLista = func(sessionCtx context.Context) {
		if !arrancada {
			arrancada = true
			sala.Arrancar(sessionCtx)
			return
		}
		fyne.Do(func() {
			sala.RecargarMensajes()
			p.Refrescar()
		})
	}
	arrancar := func(e sesion.Estado) {
		p.Lateral.Presencia, p.Lateral.EstadoPersonal = rooms.PresenciaDe(e.Presencia), e.Mensaje
		if !e.Dentro() {
			if identidad != "" {
				secure.Salir() //nolint:contextcheck // Matrix revocation must finish after the Sytue session was canceled.
				content.Forget()
				sala.Olvidar()
			}
			identidad = ""
			arrancada = false
			p.Refrescar()
			return
		}
		actual := e.Sujeto + "\x00" + s.Sesion.Organizacion()
		if actual == identidad {
			p.Refrescar()
			return
		}
		secure.Cerrar() //nolint:contextcheck // Closing invalidates the previous device lifecycle before a new session starts.
		content.Forget()
		sala.Olvidar()
		identidad = actual
		sala.Yo = e.Sujeto
		arrancada = true
		sala.Arrancar(ctx)
		secure.Iniciar(ctx, e.Sujeto, s.Sesion.Organizacion())
		go func() {
			if r, err := s.Sesion.Reclamos(ctx); err == nil {
				admin := slices.Contains(r.Roles, "owner") || slices.Contains(r.Roles, "admin")
				fyne.Do(func() {
					if identidad == actual {
						sala.AdminOrg = admin
					}
				})
			}
		}()
	}
	s.AlCambiar(arrancar)
	// «Compartir en Rooms» de otra app (Drive, Mail…) llega por el esquema
	// (`sytue-rooms:/share?…`, `api.RutaCompartir`): es una ruta PROPIA, no un fichero
	// que abrir ni un destino que no existe.
	s.RutaPropia(func(u *url.URL) bool {
		_, ok := api.LeerCompartir(u)
		_, _, esCanal := api.LeerCanal(u)
		_, esReunion := api.LeerReunion(s.Apex(), u)
		_, esWhatsApp := api.LeerRetornoWhatsApp(u)
		return ok || esCanal || esReunion || api.LeerDenuncias(u) || esWhatsApp || api.LeerInvitacionesRooms(u)
	})
	s.AlAbrirURL(func(u *url.URL) {
		if instance, ok := api.LeerRetornoWhatsApp(u); ok {
			sala.ReanudarWhatsApp(instance)
		}
		if c, ok := api.LeerCompartir(u); ok {
			sala.Compartir(c)
		}
		// El aviso de una mención o de un mensaje directo abre el canal en el mensaje.
		if canal, mensaje, ok := api.LeerCanal(u); ok {
			fyne.Do(func() { sala.IrAEnlace(canal, mensaje) })
		}
		// El aviso de una denuncia abre la cola de quien modera.
		if api.LeerDenuncias(u) {
			fyne.Do(sala.AbrirDenunciasPorEnlace)
		}
		// El aviso de una invitación de otra organización abre la lista (COM-15).
		if api.LeerInvitacionesRooms(u) {
			fyne.Do(sala.AbrirInvitacionesPorEnlace)
		}
		// El enlace de una reunión entra en su llamada: por el App Link de Android, el
		// Universal Link de iOS o la página de la web, que abre la app en escritorio.
		// Con la app cerrada llega aquí igual (la sesión lo guarda hasta apuntarse).
		if id, ok := api.LeerReunion(s.Apex(), u); ok {
			clave := api.ClaveDeReunion(u)
			fyne.Do(func() { sala.EntrarPorEnlace(id, clave) })
		}
	})
	// El título de la ventana es el NOMBRE DEL PRODUCTO y sale de `FyneApp.toml`, no
	// del catálogo: una marca no se traduce y no tiene clave en la web.
	//
	// `Metadata()` devuelve un VALOR (`fyne.AppMetadata`), no un puntero: compararlo
	// con nil no compila. Se ve con `GOOS=js GOARCH=wasm go build ./cmd/...`, que
	// type-chequea sin CGO (no es un destino que sostengamos: es la forma barata de
	// mirar, porque el piloto no puede construir los `cmd/`).
	nombre := "Sytue Rooms"
	if m := fyne.CurrentApp().Metadata(); m.Name != "" {
		nombre = m.Name
	}
	w := a.NewWindow(nombre)
	w.SetPadded(false)
	w.SetContent(ui.NuevaBarraSuite(t, s, iconos.Rooms).Montar(p.Objeto()))
	sala.Atajos(w.Canvas())
	w.Resize(fyne.NewSize(1280, 744))
	s.Arrancar(ctx)
	if e := s.Estado(); e.Dentro() {
		arrancar(e)
	} else {
		go func() { _ = s.Entrar(ctx) }() // BLOQUEA hasta volver del navegador
	}
	w.SetOnClosed(func() {
		secure.Cerrar() //nolint:contextcheck // Closing invalidates the previous device lifecycle before a new session starts.
		content.Forget()
		sala.Olvidar()
	})
	w.ShowAndRun()
}
