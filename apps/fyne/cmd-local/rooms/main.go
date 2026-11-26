// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command rooms es Sytue Rooms de Sytue Office: la misma pantalla de la suite como
// CLIENTE de los servicios de mensajería que la persona configura, sin cuenta de Sytue,
// sin cifrado propio y sin servidor (apps/fyne/PLAN-EDICION-LOCAL.md §6.14).
//
// Comparado con `cmd/rooms`: no hay `sesion.Abrir` (ni OIDC, ni Hub, ni widget), ni
// `rooms.NuevoCifrado` (ni dispositivo Matrix verificado). El servicio es
// `local/chat.Agregador`, que junta:
//
//   - las Notas, siempre: canales de notas para uno mismo en `<Datos>/Datos/rooms/`,
//     en claro y sin red, así que la app abre y sirve sin configurar nada;
//   - las cuentas Matrix que se den de alta en «Cuentas de mensajería» (también Sytue
//     Rooms online), solo para salas SIN cifrado de extremo a extremo;
//   - los espacios de Slack, con la app propia del espacio (D-20);
//   - las cuentas de Microsoft Teams (chats y, con el consentimiento del administrador,
//     canales de los equipos) y de Google Chat (escribir; leer con el cliente OAuth propio
//     del Workspace, D-22), por OAuth en el navegador y con sondeo (RO-05, RO-06), con
//     directos nuevos, adjuntos y búsqueda en lo cargado; un chat o un espacio nuevo
//     aparece en el carril sin recargar.
//
// En todas: directos, adjuntos, crear canales (en la cuenta que se elija), explorar y
// unirse, «Hilos» con los hilos en los que se escribió, y añadir personas, donde el
// proveedor lo tiene.
//
// Sin llamadas: «Reuniones» no se pinta y `Conectar` se queda nil. «Llamar» de la
// cabecera solo sale con una cuenta de Teams o de Google, como «Reunión de Microsoft
// Teams» o «Reunión de Google Meet»: crea el enlace y lo publica en el canal, y se entra
// con la app oficial (D-21). OJO: la
// pantalla (`apps/rooms`) importa `apps/fyne/llamada` sin condición, así que este binario
// todavía ENLAZA LiveKit aunque no lo use; dejarlo fuera pide la etiqueta `edicionlocal`
// en ese paquete (§5.3), que es un cambio compartido pendiente.
package main

import (
	"cmp"
	"context"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/rooms"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/chat"
	"git.sytue.eu/sytue/one/apps/nucleo/local/cuentas"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// La pantalla habla con el agregador como con el servicio de Rooms, y le pide las
// reuniones de Teams y de Meet (RO-05, RO-06).
var (
	_ rooms.Servicio         = (*chat.Agregador)(nil)
	_ rooms.CreadorReuniones = (*chat.Agregador)(nil)
	_ rooms.Presencias       = (*chat.Agregador)(nil).Presencias
)

func main() {
	a := local.NuevaApp("")
	l, err := local.Abrir(a, "rooms")
	if err != nil {
		slog.Error("rooms", "error", err)
		os.Exit(1)
	}
	tema.ElegirCromo(tema.CromoPapel)
	idioma := l.Idioma()
	t, err := rooms.Traductor(idioma)
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}
	tx := textosDe(string(idioma))

	carpeta := l.Espacio.Rutas.Producto("rooms")
	notas, err := chat.AbrirNotas(filepath.Join(carpeta, "notas"), tx.notas, tx.primerCanal, tx.primerAsunto)
	if err != nil {
		slog.Error("no se pudieron abrir las notas de Rooms", "error", err)
		os.Exit(1)
	}
	notas.Autor = cmp.Or(l.Espacio.Perfil.Nombre, tx.yo)
	agg := chat.NuevoAgregador(filepath.Join(carpeta, "preferencias.json"))
	agg.Poner(chat.CuentaNotas, notas)
	reg, err := chat.AbrirRegistro(l.Espacio.Ajuste("chat-cuentas.json"), l.Secretos)
	if err != nil {
		slog.Error("no se pudieron leer las cuentas de mensajería", "error", err)
		os.Exit(1)
	}
	// Las cuentas hablan por el portero (§5.8): solo con sus servidores; los tokens de
	// Teams y de Google, con el propósito `oauth`.
	reg.Cliente = l.Red.Cliente(red.Chat)
	reg.ClienteOAuth = l.Red.Cliente(red.OAuth)
	reg.Clientes = l.Nube.Clientes
	reg.Avisos = tx.avisos()
	for _, c := range reg.Cuentas() {
		p, err := reg.Abrir(c)
		if err != nil {
			slog.Warn("rooms: una cuenta no se pudo abrir", "cuenta", c.Rotulo, "error", err)
			continue
		}
		agg.Poner(c.ID, p)
	}

	// Los adjuntos elegidos esperan en la caché hasta que se manda el mensaje; «Guardar
	// en Drive» los deja en la carpeta de Drive de la edición.
	agg.CarpetaSubidas = filepath.Join(l.Espacio.Rutas.Cache, "rooms-subidas")
	agg.GuardarEn = filepath.Join(l.Espacio.Rutas.Datos, "Drive")
	agg.RotuloCuenta = tx.cuenta

	p := rooms.NuevaPantalla(t, idioma)
	sala := rooms.NuevaSala(p, agg, chat.Yo)
	sala.Almacen = l.Personal()
	// Adjuntos (RO-04): Matrix sin cifrar, ficheros de Slack y los de las notas. Van por
	// el agregador, que cumple lo que la pantalla le pide a Drive.
	sala.Ficheros = agg
	sala.ElegirFicheros, sala.Revelar, sala.AbrirFichero = ficheros.Elegir, ficheros.Revelar, ficheros.Abrir
	sala.SinPresencia = true
	// La presencia de quien la reparte (Teams); de los demás, nada inventado.
	sala.Presencias = agg.Presencias
	// «Ponme al día» con la IA de la persona (RO-08): aquí el contenido no va cifrado
	// por nosotros, así que se puede resumir. Sin IA configurada, nil.
	sala.IA = l.Asistir()
	l.AlCambiarIA(func() { fyne.Do(func() { sala.IA = l.Asistir() }) })
	p.Lateral.SinReuniones = true
	p.Canal.Cabecera.SinLlamar = true
	// Sin Hub no hay presencia propia que enseñar; con varias cuentas, cada una es su
	// categoría y «Canales» se queda sin los suyos sin estar vacío.
	p.Lateral.SinPresenciaPropia = true
	p.Lateral.CanalesPorCuenta = len(agg.Cuentas()) > 1
	// Una cuenta que no contestaba al abrir (sin red) trae sus canales cuando vuelve; un
	// chat de Teams o un espacio de Google Chat que se abre en otro sitio aparece en el
	// carril sin recargar.
	agg.AlRecuperar = func(string) { fyne.Do(p.Lateral.AlReintentar) }
	agg.AlCambiarCanales = func(string) { fyne.Do(sala.RefrescarCanales) }
	// Los adjuntos de Teams se bajan de los servidores de SharePoint a los que Graph
	// redirige: el portero los deja pasar solo con una cuenta de Teams con adjuntos.
	l.Red.Fuente(reg.DestinosDescarga)

	w := a.NewWindow(nombreVentana(a))
	w.SetPadded(false)
	p.Lateral.RotuloCuentas = tx.cuentas
	p.Lateral.AlCuentas = func() {
		mostrarCuentas(w, l, reg, agg, tx, func() {
			p.Lateral.CanalesPorCuenta = len(agg.Cuentas()) > 1
			p.Lateral.AlReintentar()
		})
	}
	// Ajustes → Cuentas de cualquier app de la edición (y su enlace
	// `sytueoffice-rooms:/cuentas?accion=…`): añadir abre estas cuentas; quitar pregunta.
	gestionar := func(accion, id string) {
		listo := func() {
			p.Lateral.CanalesPorCuenta = len(agg.Cuentas()) > 1
			p.Lateral.AlReintentar()
		}
		switch accion {
		case local.AccionQuitar:
			quitarCuenta(w, l, reg, agg, tx, id, listo)
		case local.AccionReconectar:
			reconectarCuenta(w, l, reg, agg, tx, id, listo)
		default:
			mostrarCuentas(w, l, reg, agg, tx, listo)
		}
	}
	l.GestionarCuentas(cuentas.OrigenChat, local.GestorCuentas{
		Anadir:     func() { gestionar(local.AccionAnadir, "") },
		Reconectar: func(id string) { gestionar(local.AccionReconectar, id) },
		Quitar:     func(id string) { gestionar(local.AccionQuitar, id) },
	})
	l.AlAbrirEnlace(func(u *url.URL) {
		if accion, id, ok := local.LeerEnlaceCuentas(u); ok {
			gestionar(accion, id)
		}
	})

	w.SetContent(l.Montar(w, iconos.Rooms, p.Objeto()))
	sala.Atajos(w.Canvas())
	soltar.AlSoltar(w, func(x soltar.Soltado) { sala.SoltarTexto(x.ComoTexto(), x.Posicion) })
	w.Resize(fyne.NewSize(1280, 800))

	ctx, cancelar := context.WithCancel(context.Background())
	l.AlCerrar(func() {
		cancelar()
		sala.Parar()
	})
	sala.Arrancar(ctx)
	w.ShowAndRun()
}

func nombreVentana(a fyne.App) string {
	if m := a.Metadata(); m.Name != "" {
		return m.Name
	}
	return "Sytue Rooms Office"
}
