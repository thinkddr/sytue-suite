// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package rooms es Sytue Rooms en Fyne: el carril de canales, la cabecera del canal,
// el historial de mensajes y el compositor, con paridad a píxel con
// `apps/frontend/apps/rooms`.
//
// # Dónde acaba este paquete: el vídeo NO se pinta aquí
//
// Rooms es mensajería Y llamadas, y solo la primera mitad se porta. No es un recorte
// de alcance decidido aquí: está medido y escrito en el **ADR 0046**. No existe códec
// de vídeo en Go puro —`golang.org/x/image/vp8` rechaza todo interfotograma porque es
// el descodificador de WebP— y `pion/mediadevices` cubre 8 de 40 casillas y ninguna de
// móvil. El transporte sí funciona, así que las llamadas se harán con los SDK NATIVOS
// de LiveKit (Swift y Kotlin) **debajo de esta interfaz**.
//
// Lo que este paquete sí pinta de una llamada es el CROMO: la antesala («¿cámara?
// ¿micrófono?»), la barra de controles y el hueco donde va el vídeo, medido y
// reservado. Ver `llamada.go`.
//
// # De dónde salen los números
//
// De medir el DOM de producción (`corporate.sytue.eu/rooms/channels/…`, 1280×800,
// rem 14) con `test/e2e/medidas-web.mjs rooms`. Cada constante lleva escrito si es
// **medida** (salió de `getBoundingClientRect`) o de **hoja** (se deriva de las clases
// de Tailwind porque ninguna pantalla de producción la montaba).
//
// El barrido dio **521 elementos visibles** en la pantalla de canal. Ese número es lo
// que dice que la medida vale: con la cookie caducada el cromo se pinta igual y sale
// una fracción, que es el estado de error midiéndose a sí mismo.
//
// # Lo que este paquete NO construye
//
// Ni un widget genérico. El armazón, el carril, la sección, la fila, el botón, el
// campo, el avatar, el contador y el estado vacío son de `apps/fyne/ui` y se importan.
// Lo propio es lo que en la web también es propio de Rooms: la FILA DE MENSAJE —que no
// es el `ListItem` de la suite: agrupa por autor, pliega la cabecera y deja la hora en
// el canalón—, el separador de día, la caja de escribir y el cromo de la llamada.
package rooms

import (
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Traductor es el de la suite con el catálogo común y el de Rooms encima, que es el
// orden de `createAppTranslator` en la web.
func Traductor(l i18n.Locale) (*i18n.Translator, error) {
	return i18n.New(l, i18n.Common(), catalogos.Rooms())
}

// Estado son los SEIS estados en los que puede estar una vista. No es una
// simplificación: la web los tiene todos y cada uno pinta algo distinto —el esqueleto
// no es el vacío, y el vacío no es el error—. En una sala de conversación confundir
// «no hay mensajes» con «no he podido preguntar» es lo que hace que alguien conteste
// creyendo que nadie ha escrito.
type Estado uint8

// Los seis.
const (
	// Cargando es el esqueleto con la geometría real de la fila, para que no salte el
	// layout al llegar los datos.
	Cargando Estado = iota
	// Listo hay datos y se pintan.
	Listo
	// Vacio la consulta fue bien y no hay nada.
	Vacio
	// Error la consulta falló; se ofrece reintentar.
	Error
	// SinConexion no hay red: lo que haya en memoria se enseña en SOLO LECTURA. En
	// Rooms además se apaga el compositor, porque un mensaje escrito sin socket no
	// llega y la persona se queda creyendo que sí.
	SinConexion
	// SinPermiso el servidor contestó 403 (un canal privado del que te sacaron).
	SinPermiso
)

// Clase es el `kind` de un canal.
type Clase uint8

// Las tres clases de canal de la web.
const (
	// Publico es un canal abierto de la organización.
	Publico Clase = iota
	// Privado lleva la insignia de `channel.privateBadge`.
	Privado
	// Directo es un mensaje directo: en la cabecera va el avatar y no el «#».
	Directo
)

// Aviso es el `notifyLevel` de un canal, que decide qué suma al contador de su
// categoría al plegarla.
type Aviso uint8

// Los tres niveles.
const (
	// AvisarTodo es el defecto: suman todos los no leídos.
	AvisarTodo Aviso = iota
	// AvisarMenciones suma solo las @menciones.
	AvisarMenciones
	// AvisarNada no suma nada, igual que silenciado.
	AvisarNada
)

// Canal es un `Channel` del SDK, con lo que la fila del carril necesita.
type Canal struct {
	ID, Nombre string
	Clase      Clase
	// Tema es el `topic`: la línea bajo el nombre en la cabecera.
	Tema string
	// Proposito es el `purpose`: de qué va el canal, lo que se lee al explorar.
	Proposito string
	// CategoriaID es la categoría personalizada a la que se movió; vacío, cae en su
	// sección por defecto.
	CategoriaID string
	// Orden es el `sortOrder`; a igualdad manda el nombre.
	Orden int
	// SinLeer y Menciones alimentan la pastilla de la fila y el contador de la
	// categoría plegada.
	SinLeer, Menciones int
	Favorito           bool
	Silenciado         bool
	Nivel              Aviso
	// EnLlamada pinta el punto vivo de «hay una llamada» dentro de la fila.
	EnLlamada bool
	// Externo es un canal de OTRA organización (ADR 0040, `@usuario`): lleva el
	// edificio. La conversación vive en la organización ANFITRIONA y allí no tienes
	// tus roles; esta bandera solo dice que lo es, no decide nada de permisos.
	Externo bool
	// Presencia y EstadoPar son los del otro de un directo 1:1 (`activePeerPresence`):
	// el punto de su avatar y la línea de debajo del nombre. Personas es cuántos hay en
	// un directo de grupo, contándote; cero en un 1:1.
	Presencia ui.Presencia
	EstadoPar string
	Personas  int
}

// Categoria es una categoría personalizada del carril.
type Categoria struct {
	ID, Nombre string
	Orden      int
	Plegada    bool
}

// Autor es quien firma un mensaje.
type Autor struct {
	ID, Nombre string
	// Bot marca los mensajes de webhooks e integraciones. Se anuncia con PALABRAS
	// («BOT»), no solo con color.
	Bot bool
}

// TipoMensaje distingue el texto de los avisos del sistema.
type TipoMensaje uint8

// Los tipos que el historial pinta distinto.
const (
	// Texto es un mensaje corriente.
	Texto TipoMensaje = iota
	// Llamada es la tarjeta de «X inició una llamada» / «Llamada finalizada · N min».
	Llamada
)

// Prioridad es el `priority` de un mensaje (Ola 12 de la web).
type Prioridad uint8

// Las tres.
const (
	Normal Prioridad = iota
	Importante
	Urgente
)

// Reaccion es un emoji con cuántas veces se puso y si estás dentro.
type Reaccion struct {
	Emoji string
	Veces int
	Mia   bool
	// Quien es «quién reaccionó» ya escrito, o "" si el servicio no dio quiénes.
	Quien string
}

// Mensaje es un `ChatMessage`.
type Mensaje struct {
	ID string
	// CanalID es el canal del mensaje: los paneles de guardados y búsqueda mezclan
	// mensajes de varios.
	CanalID string
	Autor   Autor
	Cuerpo  string
	Creado  time.Time
	Tipo    TipoMensaje
	// Segundos es la duración de una llamada YA terminada; cero mientras dura.
	Segundos int
	// Terminada distingue «inició una llamada» de «llamada finalizada»: una llamada
	// de menos de un minuto tiene Segundos 0 y sigue estando terminada.
	Terminada bool

	// Menciones son los NOMBRES de las personas @mencionadas, para sus chips.
	Menciones []string

	Prioridad Prioridad
	// PideAcuse es que el autor pide acuse de recibo; Acuses, cuántos lo han dado, y
	// Acusado, si quien mira ya lo dio.
	PideAcuse bool
	Acuses    int
	Acusado   bool

	Editado    bool
	Enviando   bool
	Fallido    bool
	Reacciones []Reaccion
	// Respuestas es el `threadCount`: cuántas hay colgando en el hilo.
	Respuestas int
	// Siguiendo dice que quien mira sigue el hilo de este mensaje (`followedIds`).
	Siguiendo bool
	// Tarjeta es la encuesta o la tarea del mensaje, o nil.
	Tarjeta *Tarjeta
	// Adjuntos son sus ficheros de Drive.
	Adjuntos []Adjunto
}

// AgruparCanales es `groupChannels` de `lib/categories.ts`, sección por sección.
//
// Un canal asignado a una categoría personalizada CONOCIDA va solo en ella y gana
// sobre la sección por defecto; si su categoría ya no existe —recién borrada, la lista
// aún sin refrescar— recae en la que le toque por clase. Favoritos es ortogonal (la
// estrella), no una categoría.
func AgruparCanales(canales []Canal, categorias []Categoria) []Seccion {
	conocidas := make(map[string]bool, len(categorias))
	for _, c := range categorias {
		conocidas[c.ID] = true
	}
	propias := map[string][]Canal{}
	var favoritos, publicos, directos []Canal
	for _, c := range canales {
		switch {
		case c.CategoriaID != "" && conocidas[c.CategoriaID]:
			propias[c.CategoriaID] = append(propias[c.CategoriaID], c)
		case c.Favorito:
			favoritos = append(favoritos, c)
		case c.Clase == Directo:
			directos = append(directos, c)
		default:
			publicos = append(publicos, c)
		}
	}

	var out []Seccion
	if len(favoritos) > 0 {
		out = append(out, nuevaSeccion(ClaveFavoritos, "sidebar.favourites", favoritos))
	}
	out = append(out, nuevaSeccion(ClaveCanales, "sidebar.channels", publicos))
	ordenadas := append([]Categoria(nil), categorias...)
	ordenarCategorias(ordenadas)
	for _, cat := range ordenadas {
		s := nuevaSeccion(cat.ID, "", propias[cat.ID])
		s.Titulo, s.Categoria = cat.Nombre, &cat
		out = append(out, s)
	}
	return append(out, nuevaSeccion(ClaveDirectos, "sidebar.directMessages", directos))
}

// Las claves estables de las tres secciones por defecto. No son categorías del
// servidor: son `DEFAULT_*` de `lib/categories.ts`.
const (
	ClaveFavoritos = "favorites"
	ClaveCanales   = "channels"
	ClaveDirectos  = "dms"
)

// Seccion es un `SidebarGroup`: un tramo del carril con su rótulo y sus canales.
type Seccion struct {
	// Clave es el id de la categoría o una de las `Clave*`.
	Clave string
	// Titulo solo lo traen las categorías PERSONALIZADAS (lo escribió una persona).
	Titulo string
	// ClaveTitulo es la del catálogo, para las secciones por defecto.
	ClaveTitulo string
	// Categoria es la categoría de la que sale; nil en las por defecto.
	Categoria *Categoria
	Canales   []Canal
	// SinLeer es el agregado que enseña la pastilla al plegar la sección.
	SinLeer int
}

func nuevaSeccion(clave, claveTitulo string, canales []Canal) Seccion {
	ordenarCanales(canales)
	return Seccion{
		Clave: clave, ClaveTitulo: claveTitulo,
		Canales: canales, SinLeer: sinLeerDe(canales),
	}
}

// sinLeerDe es `unreadOf`: respeta el nivel de aviso de cada canal, porque un contador
// que suma lo silenciado convierte el plegado en una alarma permanente.
func sinLeerDe(canales []Canal) int {
	n := 0
	for _, c := range canales {
		switch {
		case c.Nivel == AvisarNada || c.Silenciado:
		case c.Nivel == AvisarMenciones:
			n += c.Menciones
		default:
			n += c.SinLeer
		}
	}
	return n
}

// ordenarCanales es `byOrderThenName`.
func ordenarCanales(c []Canal) {
	ordenarPor(c, func(a, b Canal) int {
		if a.Orden != b.Orden {
			return a.Orden - b.Orden
		}
		return strings.Compare(a.Nombre, b.Nombre)
	})
}

func ordenarCategorias(c []Categoria) {
	ordenarPor(c, func(a, b Categoria) int {
		if a.Orden != b.Orden {
			return a.Orden - b.Orden
		}
		return strings.Compare(a.Nombre, b.Nombre)
	})
}

// ordenarPor es una inserción ESTABLE. No es `slices.SortFunc`, que no lo es: la web
// usa `Array.prototype.sort`, que sí garantiza estabilidad desde ES2019, y en una lista
// de canales con el mismo `sortOrder` y el mismo nombre eso es la diferencia entre que
// el carril se quede quieto o baile a cada repintado. Las listas son de decenas.
func ordenarPor[T any](xs []T, cmp func(a, b T) int) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && cmp(xs[j], xs[j-1]) < 0; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// IndiceSeparadorNoLeidos es `unreadDividerIndex`: ante qué fila va el separador
// «Nuevos mensajes», dado el contador capturado AL ABRIR el canal y cuántos mensajes
// hay cargados. −1 es «sin separador».
//
// Los no leídos son los `cuantos` más recientes, así que el primero está en
// `total − cuantos`. Con más no leídos que mensajes cargados se ancla arriba: todo lo
// visible es nuevo.
//
// ponytail: se deriva del CONTADOR y no de un `last_read` por mensaje, que el servidor
// no expone al cliente; si el contador excluyera los mensajes propios el separador
// podría quedar unas filas corrido. Es un marcador visual de esta apertura, no un
// estado persistido — se afina cuando el servidor dé la marca por mensaje.
func IndiceSeparadorNoLeidos(cuantos, total int) int {
	switch {
	case cuantos <= 0 || total <= 0:
		return -1
	case cuantos >= total:
		return 0
	}
	return total - cuantos
}

// Agrupado dice si el mensaje `i` continúa al anterior: mismo autor, mismo día y el
// anterior no es una tarjeta de llamada.
//
// Lo último no es un detalle: una tarjeta de llamada es un aviso del SISTEMA, sin
// avatar ni nombre, y el mensaje que venía detrás del mismo autor salía huérfano —solo
// el texto, sin quién ni cuándo— porque «continuaba» a la tarjeta. Es el comentario
// literal de `MessageView.tsx`.
func Agrupado(mensajes []Mensaje, i int) bool {
	if i <= 0 || i >= len(mensajes) || len(mensajes) < 2 {
		return false
	}
	previo, actual := mensajes[i-1 : i][0], mensajes[i]
	if CambiaElDia(mensajes, i) || previo.Tipo == Llamada {
		return false
	}
	return previo.Autor.ID == actual.Autor.ID
}

// CambiaElDia dice si ante el mensaje `i` va el separador de día: siempre ante el
// primero, y después cada vez que la fecha LOCAL cambia.
func CambiaElDia(mensajes []Mensaje, i int) bool {
	if i < 0 || i >= len(mensajes) {
		return false
	}
	if i == 0 {
		return true
	}
	return !mismoDia(mensajes[i-1 : i][0].Creado, mensajes[i].Creado)
}

func mismoDia(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// RotuloDia es `dayLabel`: «Hoy», «Ayer» o la fecha larga con el día de la semana.
//
// La mayúscula inicial NO se fuerza, y eso es deliberado: en alemán y en inglés el día
// ya va en mayúscula y en español, francés e italiano va en minúscula a propósito.
// Capitalizarla a mano sería una falta de ortografía en tres de las cinco lenguas.
//
// `ahora` se pasa para que el guardián no dependa del reloj.
func RotuloDia(t *i18n.Translator, l i18n.Locale, cuando, ahora time.Time) string {
	switch {
	case mismoDia(cuando, ahora):
		return t.T("day.today", nil)
	case mismoDia(cuando, ahora.AddDate(0, 0, -1)):
		return t.T("day.yesterday", nil)
	}
	return i18n.FormatWeekdayDayMonth(l, cuando)
}

// RotuloHora es `timeLabel`: la hora y el minuto de la fila.
func RotuloHora(l i18n.Locale, cuando time.Time) string {
	if cuando.IsZero() {
		return ""
	}
	return i18n.FormatTime(l, cuando)
}

// RotuloFechaCompleta es `fullDateLabel`: lo que la web pone en el `title` de una hora
// truncada, y aquí en la pista.
func RotuloFechaCompleta(l i18n.Locale, cuando time.Time) string {
	if cuando.IsZero() {
		return ""
	}
	return i18n.FormatDateTime(l, cuando)
}

// DuracionLlamada es `formatCallDuration`: los cuatro escalones de `call.duration.*`.
// Menos de un minuto tiene su propia cadena porque «0 min» se lee como un fallo.
func DuracionLlamada(t *i18n.Translator, segundos int) string {
	minutos := segundos / 60
	switch {
	case minutos < 1:
		return t.T("call.duration.underMinute", nil)
	case minutos < 60:
		return t.T("call.duration.minutes", i18n.Vars{"count": minutos})
	}
	horas, resto := minutos/60, minutos%60
	if resto == 0 {
		return t.T("call.duration.hours", i18n.Vars{"count": horas})
	}
	return t.T("call.duration.hoursMinutes", i18n.Vars{"hours": horas, "minutes": resto})
}

// RotuloLlamada es el texto de una tarjeta de llamada, en sus dos formas.
func RotuloLlamada(t *i18n.Translator, m Mensaje) string {
	if m.Terminada {
		return t.T("call.ended", i18n.Vars{"duration": DuracionLlamada(t, m.Segundos)})
	}
	return t.T("call.started", i18n.Vars{"name": m.Autor.Nombre})
}

// Contador es lo que se escribe en la pastilla de no leídos: la web corta en 99+.
func Contador(n int) string {
	if n > 99 {
		return "99+"
	}
	return strconv.Itoa(n)
}

// RotuloEscribiendo es `typingLabel`: «X está escribiendo…», «X e Y…» o «varias
// personas…». Son tres PLANTILLAS enteras del catálogo y no trozos pegados, porque en
// varias lenguas el verbo concuerda y el orden cambia.
func RotuloEscribiendo(t *i18n.Translator, nombres []string) string {
	switch len(nombres) {
	case 0:
		return ""
	case 1:
		return t.T("typing.one", i18n.Vars{"name": nombres[0]})
	case 2:
		return t.T("typing.two", i18n.Vars{"first": nombres[0], "second": nombres[1]})
	}
	return t.T("typing.many", nil)
}
