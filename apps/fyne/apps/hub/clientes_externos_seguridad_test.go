// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"errors"
	"image/color"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

func coordenadasExternas() api.Coordenadas {
	return api.Coordenadas{
		Domain: "empresa.example", IMAP: api.ServidorCliente{Host: "imap.empresa.example", Port: 993},
		SMTP:      api.ServidorCliente{Host: "smtp.empresa.example", Port: 465},
		CalDAVURL: "https://calendar.empresa.example/dav/", CardDAVURL: "https://contacts.empresa.example/dav/",
		WebDAVURL: "https://drive.empresa.example/dav/",
	}
}

func filasExternas(objetos []fyne.CanvasObject) []*FilaCopia {
	var filas []*FilaCopia
	for _, objeto := range objetos {
		switch o := objeto.(type) {
		case *FilaCopia:
			filas = append(filas, o)
		case *Detalles:
			filas = append(filas, filasExternas(o.contenido.objetos())...)
		}
	}
	return filas
}

// Se recorre la UI real y se copian los valores servidos por esta instalación,
// incluidas las instrucciones plegadas; ninguna prueba abre programas ni usa la red.
func TestDispositivosSegurosSirveLosSieteClientesConDatosDeLaCuenta(t *testing.T) {
	for _, caso := range []struct {
		plataforma Plataforma
		valor      string
	}{
		{IPhone, "imap.empresa.example"},
		{Mac, "https://contacts.empresa.example/dav/"},
		{Outlook, "smtp.empresa.example"},
		{Thunderbird, "https://calendar.empresa.example/dav/"},
		{Android, "ana@empresa.example"},
		{UnidadDrive, "https://drive.empresa.example/dav/"},
		{APIs, "https://drive.empresa.example/compat/graph"},
	} {
		t.Run(string(caso.plataforma), func(t *testing.T) {
			d := NuevosClientesExternos(castellano(t))
			d.Estado, d.Coordenadas, d.Correo, d.Apex = Listo, coordenadasExternas(), "ana@empresa.example", "empresa.example"
			copiado := ""
			d.AlCopiar = func(valor, _ string) { copiado = valor }
			r := test.WidgetRenderer(d).(*clientesExternosRenderer)
			r.opciones[slices.Index(Plataformas, caso.plataforma)].Tapped(nil)
			r.siguiente.Tapped(nil)
			if d.Paso != 1 || d.Plataforma != caso.plataforma {
				t.Fatal("la elección no abre sus instrucciones")
			}
			for _, ancho := range []float32{360, 390, 1280} {
				d.Resize(fyne.NewSize(ancho, r.alto(ancho)))
				if r.siguiente.Position().X < 0 || r.siguiente.Position().X+r.siguiente.Size().Width > ancho {
					t.Fatalf("no se puede continuar con un ancho de %v", ancho)
				}
			}
			for _, fila := range filasExternas(r.cuerpo.objetos()) {
				if fila.Valor == caso.valor {
					fila.boton.Tapped(nil)
				}
			}
			if copiado != caso.valor {
				t.Fatalf("se copió %q, se esperaba %q", copiado, caso.valor)
			}
			r.siguiente.Tapped(nil)
			if d.Paso != 2 || len(r.comprobaciones(caso.plataforma)) == 0 {
				t.Fatal("falta la comprobación de la conexión")
			}
		})
	}
}

func clientesPreparados(t *testing.T, crear, perfil func(context.Context, string) (string, error), identidad func() auth.IdentitySnapshot) (*Vista, fyne.Clipboard) {
	t.Helper()
	v := NuevaVista(castellano(t), nil, time.Now()).ConCuenta(cuentaDoble{auth.Reclamos{Sub: "ana", Email: "ana@empresa.example"}})
	clip := test.NewClipboard()
	v.portapapeles = func() fyne.Clipboard { return clip }
	v.ConCoordenadasClientesExternos(func(context.Context) (api.Coordenadas, error) { return coordenadasExternas(), nil })
	if crear != nil {
		v.ConClientes(func(ctx context.Context, p Plataforma) (string, error) {
			return crear(ctx, map[Plataforma]string{Outlook: "Outlook", Thunderbird: "Thunderbird", Android: "Android"}[p])
		}, nil)
	}
	if perfil != nil {
		v.ConEnlacePerfilApple(perfil)
	}
	v.ConAutorizacionClientesExternos(identidad, nil)
	v.Ir(AClientesExternos)
	esperar(t, "cargar las coordenadas de los clientes", func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return v.controlClientes.pantalla.Estado == Listo
	})
	t.Cleanup(func() { v.Ir(AInicio); fyne.DoAndWait(func() {}) })
	return v, clip
}

func elegirCliente(v *Vista, plataforma Plataforma) *clientesExternosRenderer {
	r := test.WidgetRenderer(v.controlClientes.pantalla).(*clientesExternosRenderer)
	r.opciones[slices.Index(Plataformas, plataforma)].Tapped(nil)
	r.siguiente.Tapped(nil)
	return r
}

func botonExterno(t *testing.T, r *clientesExternosRenderer, clave string) *Accion {
	t.Helper()
	for _, objeto := range r.cuerpo.objetos() {
		if b, ok := objeto.(*Accion); ok && b.Texto == r.d.T.T(clave, nil) {
			return b
		}
	}
	t.Fatalf("no está el botón %s", clave)
	return nil
}

func TestDispositivosSegurosNoInventaCoordenadasNiPermiteContinuarSinSesion(t *testing.T) {
	for _, caso := range []struct {
		nombre   string
		cuenta   Cuenta
		llamadas int
	}{
		{"sin-sesion", cuentaDoble{}, 0},
		{"error-servidor", cuentaDoble{auth.Reclamos{Sub: "ana", Email: "ana@empresa.example"}}, 1},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			var llamadas atomic.Int32
			v := NuevaVista(castellano(t), nil, time.Now()).ConCuenta(caso.cuenta)
			v.ConCoordenadasClientesExternos(func(context.Context) (api.Coordenadas, error) {
				llamadas.Add(1)
				return api.Coordenadas{}, errors.New("sin servicio")
			})
			v.Ir(AClientesExternos)
			esperar(t, "avisar de los datos no disponibles", func() bool { return v.controlClientes.pantalla.Estado == Fallo })
			d := v.controlClientes.pantalla
			r := test.WidgetRenderer(d).(*clientesExternosRenderer)
			if llamadas.Load() != int32(caso.llamadas) || d.Correo != "" || d.Coordenadas.Domain != "" || !r.siguiente.Inerte {
				t.Fatal("se ofrecieron datos o instrucciones sin una cuenta y coordenadas válidas")
			}
			v.Ir(AInicio)
		})
	}
}

func TestDispositivosSegurosCreaUnaSolaClaveYLaOlvidaAlSalir(t *testing.T) {
	inicio, terminar := make(chan string, 1), make(chan struct{})
	var liberar sync.Once
	t.Cleanup(func() { liberar.Do(func() { close(terminar) }) })
	v, clip := clientesPreparados(t, func(ctx context.Context, etiqueta string) (string, error) {
		inicio <- etiqueta
		select {
		case <-terminar:
			return "clave-solo-una-vez", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}, nil, nil)
	r := elegirCliente(v, Thunderbird)
	botonExterno(t, r, "devices.oneClick.thunderbird").Tapped(nil)
	select {
	case etiqueta := <-inicio:
		if etiqueta != "Thunderbird" {
			t.Fatal("etiqueta incorrecta")
		}
	case <-time.After(time.Second):
		t.Fatal("no comenzó la creación")
	}
	botonExterno(t, r, "devices.oneClick.thunderbird").Tapped(nil)
	select {
	case <-inicio:
		t.Fatal("se creó dos veces durante la misma petición")
	default:
	}
	liberar.Do(func() { close(terminar) })
	esperar(t, "mostrar y copiar la clave creada", func() bool {
		return v.controlClientes.pantalla.Cliente.Clave == "clave-solo-una-vez" && clip.Content() == "clave-solo-una-vez"
	})
	r.siguiente.Tapped(nil)
	if v.controlClientes.pantalla.Cliente.Clave != "" {
		t.Fatal("la comprobación conserva la clave de un uso")
	}
	v.Ir(AInicio)
	if v.controlClientes.pantalla.Correo != "" || v.controlClientes.pantalla.Coordenadas.Domain != "" {
		t.Fatal("otra pantalla conserva los datos de la cuenta")
	}
}

func TestDispositivosSegurosDescartaClaveSiCambiaIdentidadAntesDeNotificarUI(t *testing.T) {
	var mu sync.Mutex
	identidad := auth.IdentitySnapshot{Subject: "ana", Organization: "empresa"}
	hecha := make(chan context.Context, 1)
	terminar := make(chan struct{})
	var liberar sync.Once
	t.Cleanup(func() { liberar.Do(func() { close(terminar) }) })
	v, clip := clientesPreparados(t, func(ctx context.Context, _ string) (string, error) {
		hecha <- ctx
		<-terminar
		return "clave-vieja", nil
	}, nil, func() auth.IdentitySnapshot { mu.Lock(); defer mu.Unlock(); return identidad })
	r := elegirCliente(v, Outlook)
	botonExterno(t, r, "devices.oneClick.outlook").Tapped(nil)
	var ctx context.Context
	select {
	case ctx = <-hecha:
	case <-time.After(time.Second):
		t.Fatal("no comenzó la petición")
	}
	mu.Lock()
	identidad = auth.IdentitySnapshot{Subject: "bea", Organization: "otra"}
	mu.Unlock()
	// El evento de UI aún no ha llegado; la comprobación de identidad ya debe protegerlo.
	liberar.Do(func() { close(terminar) })
	esperar(t, "terminar la petición", func() bool { return ctx.Err() != nil })
	fyne.DoAndWait(func() {})
	if v.controlClientes.pantalla.Cliente.Clave != "" || clip.Content() != "" {
		t.Fatal("se mostró o copió una clave de otra identidad")
	}
}

func TestDispositivosSegurosPerfilAppleUsaUnaAccionAutenticadaYNoUnGETAntiguo(t *testing.T) {
	peticiones := make(chan string, 1)
	v, _ := clientesPreparados(t, nil, func(_ context.Context, etiqueta string) (string, error) {
		peticiones <- etiqueta
		return "https://id.empresa.example/perfil/un-solo-uso", nil
	}, nil)
	abridor := &abridorDoble{}
	v.abrir = abridor
	r := elegirCliente(v, IPhone)
	if len(abridor.abiertas) != 0 {
		t.Fatal("navegar no debe crear ni descargar perfiles")
	}
	botonExterno(t, r, "devices.apple.download").Tapped(nil)
	esperar(t, "abrir el enlace de un uso", func() bool { return len(abridor.abiertas) == 1 })
	if <-peticiones != "Apple · iphone" || abridor.abiertas[0] != "https://id.empresa.example/perfil/un-solo-uso" {
		t.Fatal("no se abrió el perfil autenticado de esta instalación")
	}
}

func TestDispositivosSegurosNoAbreUnPerfilQueLlegaTrasCambiarDePaso(t *testing.T) {
	peticion, terminar := make(chan context.Context, 1), make(chan struct{})
	var liberar sync.Once
	t.Cleanup(func() { liberar.Do(func() { close(terminar) }) })
	v, _ := clientesPreparados(t, nil, func(ctx context.Context, _ string) (string, error) {
		peticion <- ctx
		<-terminar
		return "https://id.empresa.example/perfil/descartado", nil
	}, nil)
	abridor := &abridorDoble{}
	v.abrir = abridor
	r := elegirCliente(v, Mac)
	botonExterno(t, r, "devices.apple.download").Tapped(nil)
	var ctx context.Context
	select {
	case ctx = <-peticion:
	case <-time.After(time.Second):
		t.Fatal("no comenzó la petición del perfil")
	}
	r.atras.Tapped(nil)
	liberar.Do(func() { close(terminar) })
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("no terminó la petición del perfil")
	}
	fyne.DoAndWait(func() {})
	if len(abridor.abiertas) != 0 || v.controlClientes.pantalla.PerfilCreando {
		t.Fatal("se abrió un perfil de unas instrucciones que ya no están activas")
	}
}

func TestDispositivosSegurosElCambioDeSesionBorraLaClaveYRecargaLaCuenta(t *testing.T) {
	v, clip := clientesPreparados(t, func(context.Context, string) (string, error) { return "secreto-ana", nil }, nil, nil)
	r := elegirCliente(v, Outlook)
	botonExterno(t, r, "devices.oneClick.outlook").Tapped(nil)
	esperar(t, "mostrar la clave de Ana", func() bool { return v.controlClientes.pantalla.Cliente.Clave == "secreto-ana" })
	filasAnteriores := filasExternas(r.cuerpo.objetos())
	v.ConCuenta(cuentaDoble{auth.Reclamos{Sub: "bea", Email: "bea@empresa.example"}})
	v.RefrescarClientesExternos()
	esperar(t, "recargar la cuenta de Bea", func() bool { return v.controlClientes.pantalla.Correo == "bea@empresa.example" })
	d := v.controlClientes.pantalla
	if d.Cliente.Clave != "" || d.Plataforma != "" || d.Paso != 0 {
		t.Fatal("la cuenta nueva heredó el secreto o el paso de otra persona")
	}
	clip.SetContent("")
	for _, fila := range filasAnteriores {
		if fila.Valor == "secreto-ana" {
			fila.boton.Tapped(nil)
		}
	}
	if clip.Content() != "" {
		t.Fatal("una acción pendiente de la cuenta anterior volvió a copiar su secreto")
	}
	// Oculta no vuelve a pedir datos y mantiene borrada la información personal.
	v.Ir(AInicio)
	v.RefrescarClientesExternos()
	fyne.DoAndWait(func() {})
	if d.Correo != "" || d.Coordenadas.Domain != "" {
		t.Fatal("se recargó la cuenta en una pantalla oculta")
	}
}

func TestDispositivosSegurosFalloDeCredencialNoCopiaNiAbreNada(t *testing.T) {
	v, clip := clientesPreparados(t, func(context.Context, string) (string, error) { return "", errors.New("rechazada") },
		func(context.Context, string) (string, error) { return "", errors.New("rechazada") }, nil)
	abridor := &abridorDoble{}
	v.abrir = abridor
	r := elegirCliente(v, Android)
	botonExterno(t, r, "devices.oneClick.android").Tapped(nil)
	esperar(t, "mostrar el fallo de la clave", func() bool { return v.controlClientes.pantalla.Cliente.Fallo })
	r.atras.Tapped(nil)
	r.opciones[slices.Index(Plataformas, Mac)].Tapped(nil)
	r.siguiente.Tapped(nil)
	botonExterno(t, r, "devices.apple.download").Tapped(nil)
	esperar(t, "mostrar el fallo del perfil", func() bool { return v.controlClientes.pantalla.PerfilFallo })
	if clip.Content() != "" || len(abridor.abiertas) != 0 {
		t.Fatal("un fallo copió credenciales o abrió una URL")
	}
}

func TestDispositivosSegurosNoAbrePerfilesFueraDeHTTPS(t *testing.T) {
	for _, enlace := range []string{"http://id.empresa.example/perfil/credencial", "file:///tmp/perfil.mobileconfig", "https://usuario:secreto@id.empresa.example/perfil"} {
		v, _ := clientesPreparados(t, nil, func(context.Context, string) (string, error) { return enlace, nil }, nil)
		abridor := &abridorDoble{}
		v.abrir = abridor
		r := elegirCliente(v, IPhone)
		botonExterno(t, r, "devices.apple.download").Tapped(nil)
		esperar(t, "rechazar un enlace de perfil inseguro", func() bool { return v.controlClientes.pantalla.PerfilFallo })
		if len(abridor.abiertas) != 0 {
			t.Fatal("se abrió un enlace inseguro que contiene una credencial")
		}
		v.Ir(AInicio)
	}
}

func TestDispositivosSegurosCancelaAntesDeLiberarElProveedor(t *testing.T) {
	for _, perfil := range []bool{false, true} {
		for _, salida := range []string{"atras", "plataforma", "ruta"} {
			t.Run(salida+map[bool]string{false: "/clave", true: "/perfil"}[perfil], func(t *testing.T) {
				peticion, liberar := make(chan context.Context, 1), make(chan struct{})
				var once sync.Once
				t.Cleanup(func() { once.Do(func() { close(liberar) }) })
				proveedor := func(ctx context.Context, _ string) (string, error) {
					peticion <- ctx
					<-liberar
					return "", ctx.Err()
				}
				v, _ := clientesPreparados(t, proveedor, proveedor, nil)
				plataforma, clave := Outlook, "devices.oneClick.outlook"
				if perfil {
					plataforma, clave = Mac, "devices.apple.download"
				}
				r := elegirCliente(v, plataforma)
				botonExterno(t, r, clave).Tapped(nil)
				var ctx context.Context
				select {
				case ctx = <-peticion:
				case <-time.After(time.Second):
					t.Fatal("no comenzó la operación")
				}
				switch salida {
				case "atras":
					r.atras.Tapped(nil)
				case "plataforma":
					r.opciones[slices.Index(Plataformas, Android)].Tapped(nil)
				case "ruta":
					v.Ir(AInicio)
				}
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
					t.Fatal("la operación sólo se invalida, pero no se cancela al salir")
				}
				// El proveedor continúa bloqueado: la cancelación viene de la UI.
				once.Do(func() { close(liberar) })
			})
		}
	}
}

func TestDispositivosSegurosTemaYPresenciaConservanPasoPeticionYResultado(t *testing.T) {
	tema.Elegir("claro")
	tema.ElegirForma("carril")
	t.Cleanup(func() { tema.Elegir(""); tema.FormaDeLosGuardianes() })
	peticion, liberar := make(chan context.Context, 1), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(liberar) }) })
	identidad := auth.IdentitySnapshot{Issuer: "https://id.empresa.example", Client: "native", Subject: "ana", Organization: "empresa"}
	v, clip := clientesPreparados(t, func(ctx context.Context, _ string) (string, error) {
		peticion <- ctx
		<-liberar
		return "secreto-conservado", nil
	}, nil, func() auth.IdentitySnapshot { return identidad })
	r := elegirCliente(v, Outlook)
	botonExterno(t, r, "devices.oneClick.outlook").Tapped(nil)
	var ctx context.Context
	select {
	case ctx = <-peticion:
	case <-time.After(time.Second):
		t.Fatal("no comenzó la operación")
	}
	revision := v.controlClientes.revision
	tema.Elegir("oscuro")
	tema.ElegirForma("marco")
	v.RefrescarSesionClientesExternos() // AlCambiar también avisa de tema y presencia.
	fyne.DoAndWait(func() {})
	d := v.controlClientes.pantalla
	if d.Paso != 1 || d.Plataforma != Outlook || !d.Cliente.Creando || v.controlClientes.revision != revision {
		t.Fatal("un repintado canceló o reinició el asistente")
	}
	select {
	case <-ctx.Done():
		t.Fatal("un repintado canceló la petición")
	default:
	}
	if r.clave.variante != tema.Oscuro || !r.clave.marco {
		t.Fatal("el cuerpo mantuvo su identidad visual anterior")
	}
	var circulo *canvas.Circle
	for _, objeto := range r.cuerpo.objetos() {
		if c, ok := objeto.(*canvas.Circle); ok {
			circulo = c
			break
		}
	}
	if circulo == nil || color.NRGBAModel.Convert(circulo.FillColor) != tema.Color("bg-subtle", tema.Oscuro) {
		t.Fatal("las instrucciones conservaron colores del tema anterior")
	}
	once.Do(func() { close(liberar) })
	esperar(t, "conservar el resultado tras repintar", func() bool { return d.Cliente.Clave == "secreto-conservado" && clip.Content() == "secreto-conservado" })
	v.RefrescarSesionClientesExternos()
	fyne.DoAndWait(func() {})
	if d.Cliente.Clave != "secreto-conservado" || d.Paso != 1 {
		t.Fatal("la presencia borró el resultado ya mostrado")
	}
	identidad = auth.IdentitySnapshot{} // Logout sí limpia, aunque la vista siga abierta.
	v.RefrescarSesionClientesExternos()
	fyne.DoAndWait(func() {})
	if d.Cliente.Clave != "" || d.Correo != "" || d.Paso != 0 {
		t.Fatal("el cierre de sesión conservó información de la cuenta")
	}
}

func TestDispositivosSegurosOperacionNoHeredaElDeadlineDeCarga(t *testing.T) {
	carga := make(chan context.Context, 1)
	operacion := make(chan context.Context, 1)
	v := NuevaVista(castellano(t), nil, time.Now()).ConCuenta(cuentaDoble{auth.Reclamos{Sub: "ana", Email: "ana@empresa.example"}})
	v.ConCoordenadasClientesExternos(func(ctx context.Context) (api.Coordenadas, error) { carga <- ctx; return coordenadasExternas(), nil })
	v.ConClientes(func(ctx context.Context, _ Plataforma) (string, error) { operacion <- ctx; return "clave-nueva", nil }, nil)
	v.Ir(AClientesExternos)
	t.Cleanup(func() { v.Ir(AInicio); fyne.DoAndWait(func() {}) })
	esperar(t, "terminar la carga inicial", func() bool { return v.controlClientes.pantalla.Estado == Listo })
	ctxCarga := <-carga
	select {
	case <-ctxCarga.Done():
	case <-time.After(time.Second):
		t.Fatal("la carga no liberó su contexto hijo")
	}
	if _, tieneDeadline := v.controlClientes.contexto.Deadline(); tieneDeadline || v.controlClientes.contexto.Err() != nil {
		t.Fatal("la vida del asistente hereda los 15 segundos de carga")
	}
	r := elegirCliente(v, Thunderbird)
	botonExterno(t, r, "devices.oneClick.thunderbird").Tapped(nil)
	var ctxOperacion context.Context
	select {
	case ctxOperacion = <-operacion:
	case <-time.After(time.Second):
		t.Fatal("la nueva operación heredó la cancelación de la carga")
	}
	deadline, ok := ctxOperacion.Deadline()
	if !ok || time.Until(deadline) < 20*time.Second {
		t.Fatal("la operación no tiene su propio plazo de 30 segundos")
	}
	esperar(t, "mostrar la nueva clave", func() bool { return v.controlClientes.pantalla.Cliente.Clave == "clave-nueva" })
}

type transporteDispositivosSeguros func(*http.Request) (*http.Response, error)

func (f transporteDispositivosSeguros) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestDispositivosSegurosIntencionMostradaNoCreaCredencialesEnLaCuentaSiguiente(t *testing.T) {
	for _, perfil := range []bool{false, true} {
		t.Run(map[bool]string{false: "clave", true: "perfil"}[perfil], func(t *testing.T) {
			tokens := auth.Tokens{Issuer: "https://id.empresa.example", Cliente: "native", Subject: "ana", Organizacion: "empresa", Access: "synthetic-a", Refresh: "synthetic-refresh-a", Expiry: time.Now().Add(time.Hour)}
			s := auth.NewSession(tokens, nil, nil)
			var posts atomic.Int32
			cuenta := api.NewAccount("https://id.empresa.example", s, &http.Client{Transport: transporteDispositivosSeguros(func(*http.Request) (*http.Response, error) { posts.Add(1); return nil, errors.New("POST inesperado") })})
			peticion, liberar := make(chan context.Context, 1), make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(liberar) }) })
			crear := func(ctx context.Context, etiqueta string) (string, error) {
				peticion <- ctx
				<-liberar // La validación de UI ya pasó; aún no se ha pedido token.
				if perfil {
					return cuenta.EnlacePerfilApple(ctx, etiqueta)
				}
				clave, err := cuenta.CreateAppPassword(ctx, etiqueta)
				return clave.Password, err
			}
			v := NuevaVista(castellano(t), nil, time.Now()).ConCuenta(cuentaDoble{auth.Reclamos{Sub: "ana", Email: "ana@empresa.example"}})
			v.ConCoordenadasClientesExternos(func(context.Context) (api.Coordenadas, error) { return coordenadasExternas(), nil })
			v.ConClientes(func(ctx context.Context, _ Plataforma) (string, error) { return crear(ctx, "Outlook") }, nil)
			v.ConEnlacePerfilApple(crear)
			v.ConAutorizacionClientesExternos(s.SnapshotIdentity, s.WithAuthorizationIntent)
			v.Ir(AClientesExternos)
			t.Cleanup(func() { v.Ir(AInicio); fyne.DoAndWait(func() {}) })
			esperar(t, "mostrar la cuenta A", func() bool { return v.controlClientes.pantalla.Estado == Listo })
			mostrada := auth.RequestAuthorizationIntent(v.controlClientes.contexto)
			plataforma, clave := Outlook, "devices.oneClick.outlook"
			if perfil {
				plataforma, clave = Mac, "devices.apple.download"
			}
			r := elegirCliente(v, plataforma)
			botonExterno(t, r, clave).Tapped(nil)
			var ctx context.Context
			select {
			case ctx = <-peticion:
			case <-time.After(time.Second):
				t.Fatal("no comenzó la intención del clic")
			}
			if mostrada == nil || auth.RequestAuthorizationIntent(ctx) != mostrada {
				t.Fatal("el clic recapturó autorización en vez de heredar la mostrada")
			}
			tokens.Subject, tokens.Organizacion, tokens.Access, tokens.Refresh = "bea", "otra", "synthetic-b", "synthetic-refresh-b"
			s.Sincronizar(tokens)
			once.Do(func() { close(liberar) })
			esperar(t, "terminar la autorización rechazada", func() bool { return ctx.Err() != nil })
			fyne.DoAndWait(func() {})
			if posts.Load() != 0 || v.controlClientes.pantalla.Cliente.Clave != "" {
				t.Fatal("una intención de A fabricó una credencial para B")
			}
		})
	}
}

func TestDispositivosSegurosRecargaTrasRenovacionDeOtraLlamadaSinRepetirElClic(t *testing.T) {
	tokens := auth.Tokens{Issuer: "https://id.empresa.example", Cliente: "native", Subject: "ana", Organizacion: "empresa", Access: "synthetic-old", Refresh: "synthetic-refresh", Expiry: time.Now().Add(time.Hour)}
	s := auth.NewSession(tokens, func(context.Context, auth.Tokens) (auth.Tokens, error) {
		renovados := tokens
		renovados.Access = "synthetic-renewed"
		return renovados, nil
	}, nil)
	var posts atomic.Int32
	cuenta := api.NewAccount("https://id.empresa.example", s, &http.Client{Transport: transporteDispositivosSeguros(func(r *http.Request) (*http.Response, error) {
		posts.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"p","password":"clave-nueva"}`)), Request: r}, nil
	})})
	v := NuevaVista(castellano(t), nil, time.Now()).ConCuenta(cuentaDoble{auth.Reclamos{Sub: "ana", Email: "ana@empresa.example"}})
	v.ConCoordenadasClientesExternos(func(context.Context) (api.Coordenadas, error) { return coordenadasExternas(), nil })
	v.ConAutorizacionClientesExternos(s.SnapshotIdentity, s.WithAuthorizationIntent)
	v.ConClientes(func(ctx context.Context, _ Plataforma) (string, error) {
		nueva, err := cuenta.CreateAppPassword(ctx, "Outlook")
		return nueva.Password, err
	}, nil)
	v.Ir(AClientesExternos)
	t.Cleanup(func() { v.Ir(AInicio); fyne.DoAndWait(func() {}) })
	esperar(t, "cargar antes de la renovación", func() bool { return v.controlClientes.pantalla.Estado == Listo })
	r := elegirCliente(v, Outlook)
	mostrada := auth.RequestAuthorizationIntent(v.controlClientes.contexto)
	s.Invalidate(tokens.Access)
	if _, err := s.AccessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mostrada.Current() {
		t.Fatal("el test no retiró el witness de la llamada anterior")
	}
	botonExterno(t, r, "devices.oneClick.outlook").Tapped(nil)
	esperar(t, "recargar sin reproducir la escritura", func() bool { return v.controlClientes.pantalla.Estado == Listo && v.controlClientes.pantalla.Paso == 0 })
	if posts.Load() != 0 || auth.RequestAuthorizationIntent(v.controlClientes.contexto) == mostrada {
		t.Fatal("se reprodujo una intención vieja o no se renovó la vista")
	}
	r = elegirCliente(v, Outlook)
	botonExterno(t, r, "devices.oneClick.outlook").Tapped(nil)
	esperar(t, "aceptar un clic nuevo tras la recarga", func() bool { return v.controlClientes.pantalla.Cliente.Clave == "clave-nueva" })
	if posts.Load() != 1 {
		t.Fatal("la vista quedó bloqueada o duplicó la escritura tras renovar")
	}
}

func TestDispositivosSegurosNoInstalaUnPerfilNativoDeOtraIntencion(t *testing.T) {
	for _, cambiaCuenta := range []bool{false, true} {
		identidad := auth.IdentitySnapshot{Subject: "ana", Organization: "empresa"}
		v, _ := clientesPreparados(t, nil, nil, func() auth.IdentitySnapshot { return identidad })
		peticion, liberar := make(chan context.Context, 1), make(chan struct{})
		var once sync.Once
		t.Cleanup(func() { once.Do(func() { close(liberar) }) })
		instalaciones := 0
		v.ConPerfilApple(Mac, func(ctx context.Context) (func() error, error) {
			peticion <- ctx
			<-liberar
			return func() error { instalaciones++; return nil }, nil
		})
		r := elegirCliente(v, Mac)
		botonExterno(t, r, "devices.apple.download").Tapped(nil)
		var ctx context.Context
		select {
		case ctx = <-peticion:
		case <-time.After(time.Second):
			t.Fatal("no comenzó la preparación del perfil")
		}
		if cambiaCuenta {
			identidad = auth.IdentitySnapshot{Subject: "bea", Organization: "otra"}
		} else {
			r.atras.Tapped(nil)
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("cambiar de paso no canceló la preparación")
			}
		}
		once.Do(func() { close(liberar) })
		if cambiaCuenta {
			esperar(t, "descartar el perfil antes de instalarlo", func() bool { return ctx.Err() != nil })
		}
		fyne.DoAndWait(func() {})
		if instalaciones != 0 {
			t.Fatal("se instaló un perfil de una intención que ya no está visible")
		}
	}
}

func TestDispositivosSegurosCancelaMontajeYLogoutSinEsperarAlProveedor(t *testing.T) {
	for _, logout := range []bool{false, true} {
		identidad := auth.IdentitySnapshot{Subject: "ana", Organization: "empresa"}
		v, _ := clientesPreparados(t, nil, nil, func() auth.IdentitySnapshot { return identidad })
		peticion, liberar := make(chan context.Context, 1), make(chan struct{})
		var once sync.Once
		t.Cleanup(func() { once.Do(func() { close(liberar) }) })
		v.ConMontaje(func(ctx context.Context, _ string) error { peticion <- ctx; <-liberar; return ctx.Err() })
		r := elegirCliente(v, UnidadDrive)
		botonExterno(t, r, "shell.mountDrive").Tapped(nil)
		var ctx context.Context
		select {
		case ctx = <-peticion:
		case <-time.After(time.Second):
			t.Fatal("no comenzó el montaje")
		}
		if logout {
			identidad = auth.IdentitySnapshot{}
			v.RefrescarSesionClientesExternos()
			fyne.DoAndWait(func() {})
		} else {
			r.atras.Tapped(nil)
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("el cambio de contexto no canceló el montaje")
		}
		once.Do(func() { close(liberar) })
		if logout && (v.controlClientes.pantalla.Correo != "" || v.controlClientes.pantalla.Coordenadas.Domain != "") {
			t.Fatal("logout dejó información de la cuenta")
		}
	}
}

func TestDispositivosSegurosAbrirProgramaNoBloqueaLaColaUINiRepiteLaCredencial(t *testing.T) {
	creada := make(chan context.Context, 1)
	var creaciones atomic.Int32
	v, clip := clientesPreparados(t, func(ctx context.Context, _ string) (string, error) {
		creaciones.Add(1)
		creada <- ctx
		return "clave-antes-de-abrir", nil
	}, nil, nil)
	abriendo, liberar := make(chan string, 1), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(liberar) }) })
	var termino atomic.Bool
	v.controlClientes.abrir = func(Plataforma) error {
		defer termino.Store(true)
		abriendo <- clip.Content()
		select {
		case <-liberar:
			return nil
		case <-time.After(2 * time.Second):
			return errors.New("el test no pudo usar UI con el abridor bloqueado")
		}
	}
	r := elegirCliente(v, Thunderbird)
	botonExterno(t, r, "devices.oneClick.thunderbird").Tapped(nil)
	var ctx context.Context
	select {
	case ctx = <-creada:
	case <-time.After(time.Second):
		t.Fatal("no comenzó la creación")
	}
	valorAlAbrir := ""
	esperar(t, "abrir después de copiar sin bloquear UI", func() bool {
		select {
		case valorAlAbrir = <-abriendo:
			return true
		default:
			return false
		}
	})
	if valorAlAbrir != "clave-antes-de-abrir" || termino.Load() {
		t.Fatal("abrir bloqueó la copia/UI o se ejecutó antes de copiar")
	}
	procesada := false
	fyne.Do(func() { procesada = true })
	fyne.DoAndWait(func() {})
	if !procesada || termino.Load() {
		t.Fatal("la cola UI no procesa otras tareas mientras espera el programa")
	}
	botonExterno(t, r, "devices.oneClick.thunderbird").Tapped(nil)
	if creaciones.Load() != 1 {
		t.Fatal("esperar la apertura creó otra credencial")
	}
	r.atras.Tapped(nil)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("UI no pudo cancelar mientras esperaba el SO")
	}
	if termino.Load() {
		t.Fatal("el abridor terminó antes de comprobar la cancelación de UI")
	}
	once.Do(func() { close(liberar) })
	esperar(t, "terminar la apertura cancelada", termino.Load)
	fyne.DoAndWait(func() {})
	if v.controlClientes.pantalla.Cliente.Clave != "" || v.controlClientes.pantalla.Paso != 0 {
		t.Fatal("el resultado tardío de abrir restauró el paso o su clave")
	}
}

func TestClientesExternosEsSecundarioYConservaLosControlesDeOne(t *testing.T) {
	v, _ := clientesPreparados(t, nil, nil, nil)
	v.Ir(ADispositivos)
	drive, oauth := 0, 0
	v.disp.AlAbrirDrive = func() { drive++ }
	v.disp.AlConectarApps = func() { oauth++ }
	v.disp.PuedeConectarApps = true
	v.disp.Refresh()
	r := test.WidgetRenderer(v.disp).(*dispositivosRenderer)
	r.drive.Tapped(nil)
	r.apps.Tapped(nil)
	r.externos.Tapped(nil)
	esperar(t, "abrir el asistente externo", func() bool { return v.Destino() == AClientesExternos && v.controlClientes.pantalla.Estado == Listo })
	if drive != 1 || oauth != 1 || !v.disp.DriveEnSistema || slices.Contains(Portadas(), AClientesExternos) {
		t.Fatal("el acceso secundario alteró Drive/OAuth o el menú principal")
	}
	if d, ok := DestinoDeRuta("/one/dispositivos/clientes/"); !ok || d != AClientesExternos {
		t.Fatal("el enlace profundo no abre el asistente secundario")
	}
	elegirCliente(v, Thunderbird)
	if !v.atras() || v.controlClientes.pantalla.Paso != 0 || v.Destino() != AClientesExternos {
		t.Fatal("atrás no retrocede el paso del asistente")
	}
	if !v.atras() || v.Destino() != ADispositivos || v.controlClientes.pantalla.Visible() {
		t.Fatal("el asistente no vuelve a la integración One")
	}
	if !v.disp.Visible() || v.controlClientes.pantalla.Correo != "" {
		t.Fatal("volver no mostró One o conservó datos personales del asistente")
	}
}
