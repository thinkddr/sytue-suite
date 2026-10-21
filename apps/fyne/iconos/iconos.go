// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package iconos es la iconografía de la suite para Fyne: los iconos de trazo del
// cromo y las marcas de producto, los mismos que pinta la web.
//
// # Conjunto CERRADO, comprobado por el COMPILADOR
//
// Un icono se pide con una constante tipada (`iconos.Buscar`), no con una cadena. Es
// la diferencia que cuesta dinero: en Gio los iconos y los tokens eran literales y un
// `sidebar-hover` por `sidebar-hover-bg` no se veía hasta PINTAR la pantalla, donde
// entraba en pánico. Aquí `iconos.Busqar` no compila.
//
// Lo mismo con las marcas (`iconos.Mail`). El color sí sigue siendo un token en cadena
// porque es de `tema`, pero se resuelve al CONSTRUIR el icono y no al pintarlo, así
// que un token inventado revienta cuando se arma el widget y no en la primera vez que
// esa pantalla se abre.
//
// # De dónde salen las rutas
//
// Verbatim de `apps/frontend/packages/ui/src/components/Icon.tsx`, que es el catálogo
// de la web: rejilla de 24, remates y uniones redondos, y el grosor y el relleno de
// `Glifo.tsx` (ver «El estilo del trazo» en pintar.go).
// `TestElCatalogoEsElDeLaWeb` lee ese .tsx y compara ruta a ruta, en los dos sentidos:
// una que cambie allí y una que sobre aquí ponen esto en rojo.
//
// # Los glifos PROPIOS de una app
//
// Algunos productos dibujan glifos que el catálogo compartido no tiene, y en la web
// tampoco: viven en `apps/frontend/apps/<app>/src/components/glyphs.tsx`. Aquí entran
// con su constante y su entrada, al final y detrás de una frontera declarada en
// `propios`, y el mismo guardián los compara contra ESE .tsx. No se mezclan con los
// compartidos —van detrás y no intercalados— porque las
// claves CHOCAN —`trash`, `clock` y `tag` existen en los dos con rutas distintas—, y
// porque de una app se porta lo que se usa, no el fichero entero.
//
// # Cómo se pinta (y por qué no con canvas.Image)
//
// Ver `pintar.go`: Fyne rasteriza el SVG al tamaño que le dice `CanvasForObject`, que
// en una pantalla HiDPI no es el de dispositivo, y `oksvg` no escala el grosor del
// trazo con el viewBox. Las dos cosas están medidas allí.
package iconos

// paridad-web: apps/frontend/packages/ui/src/components/Icon.tsx

// Icono es un icono de trazo del catálogo compartido. El cero es Ninguno, y los demás
// valores válidos son las constantes de abajo y solo esas.
type Icono uint8

// El catálogo, en el orden de Icon.tsx. El comentario de cada uno es su clave en la
// web, que es por la que hay que buscarlo allí.
const (
	// Ninguno es el CERO del tipo, y es «no hay icono». Está el primero a propósito:
	// sin él, un campo de estructura sin inicializar pintaría el primer icono de la
	// lista —una lupa— sin que nadie lo pidiera. Con él, el cero es el vacío y quien
	// quiera un icono tiene que nombrarlo.
	Ninguno             Icono = iota
	Buscar                    // search
	Correo                    // mail
	Campana                   // bell
	Cerrar                    // close
	Tic                       // check
	ChevronAbajo              // chevronDown
	ChevronArriba             // chevronUp
	ChevronDerecha            // chevronRight
	ChevronIzquierda          // chevronLeft
	Menu                      // menu
	Rejilla                   // grid
	Usuario                   // user
	Ajustes                   // settings
	Salir                     // logout
	Informacion               // info
	Aviso                     // warning
	Peligro                   // danger
	Mas                       // plus
	Edificio                  // building
	Usuarios                  // users
	BaseDeDatos               // database
	Cubo                      // bucket
	Cohete                    // rocket
	Nube                      // cloud
	Auditoria                 // audit
	Candado                   // lock
	Globo                     // globe
	Tabla                     // table
	IA                        // ai
	Puntos                    // ellipsis
	Codigo                    // code
	Terminal                  // terminal
	Grafico                   // chart
	Reloj                     // clock
	Recibo                    // receipt
	Llave                     // key
	Escudo                    // shield
	Dispositivo               // device
	UsuarioVisto              // userCheck
	Etiqueta                  // tag
	Mano                      // hand
	Rotulador                 // highlighter
	Subrayado                 // underline
	Tachado                   // strikethrough
	Comentario                // comment
	Lapiz                     // pen
	Cuadrado                  // square
	Circulo                   // circle
	Linea                     // line
	FlechaArribaDerecha       // arrowUpRight
	Regla                     // ruler
	Texto                     // text
	Negrita                   // bold
	Cursiva                   // italic
	Enlace                    // link
	Cita                      // quote
	Lista                     // list
	Sello                     // stamp
	CampoDeFormulario         // formField
	Censura                   // redact
	Menos                     // minus
	AjustarAncho              // fitWidth
	AjustarPagina             // fitPage
	DosPaginas                // dosPaginas
	Camara                    // camera
	Microfono                 // microfono
	MicrofonoApagado          // microfonoOff
	Pantalla                  // pantalla
	Corazon                   // corazon
	Imagen                    // image
	Album                     // album
	Rotar                     // rotate
	VoltearH                  // flipH
	VoltearV                  // flipV
	Deshacer                  // undo
	Guardar                   // save
	Descargar                 // download
	Subir                     // upload
	Intercambiar              // swap
	Calendario                // calendar
	Copias                    // copies
	Papelera                  // trash
	Recorte                   // recorte
	Voz                       // voz
	Emitir                    // emitir
	Explorar                  // explorar
	Inicio                    // home
	Maximizar                 // maximizar
	Minimizar                 // minimizar
	Ojo                       // ojo
	Pausa                     // pause
	ImagenEnImagen            // pip
	Reproducir                // play
	Video                     // video
	VozApagada                // vozOff
	OjoTachado                // ojoOff

	// --- Los glifos PROPIOS de Sytue Drive (`apps/drive/.../glyphs.tsx`) ---
	//
	// Mismo criterio que los de Mail, y el prefijo hace aquí la misma falta: `folder`
	// y `star` existen también en el glyphs.tsx de Mail con OTRO dibujo, y `trash` en
	// los tres sitios. Se portan los que las pantallas de Drive usan.
	DriveCarpeta      // folder
	DriveFichero      // file
	DriveDocumento    // document
	DriveHoja         // spreadsheet
	DrivePresentacion // presentation
	DriveEstrella     // star

	// numIconos va SIEMPRE el último y no es un icono: es cuántos hay.
	//
	// Sin él, `TestElCatalogoNoTieneHuecos` se acotaba con la última constante que
	// conocía, así que una AÑADIDA DESPUÉS no la miraba nadie y se quedaba sin entrada
	// en `catalogo`. Comprobado a mano el 2026-09-21: el guardián salía en verde con un
	// icono sin ruta.
	numIconos
)

// Fuente es el `.tsx` del que sale un icono, relativo a la raíz del repositorio.
// `TestElCatalogoEsElDeLaWeb` compara cada grupo contra SU fichero.
const (
	FuenteComun = "apps/frontend/packages/ui/src/components/Icon.tsx"
	FuenteDrive = "apps/frontend/apps/drive/src/components/glyphs.tsx"
)

// propios marca dónde empiezan los glifos de cada app: de esa constante en adelante
// —hasta la siguiente frontera o hasta `numIconos`— el icono sale de ese `.tsx` y no
// del catálogo compartido. La app que porte los suyos añade UNA línea aquí.
//
// Es un rango y no un campo por entrada para no tocar las ochenta y tres líneas del
// catálogo compartido. Si alguien mete un icono común dentro del rango, el guardián
// lo busca en `glyphs.tsx`, no lo encuentra y se pone rojo en el acto.
var propios = []struct {
	desde  Icono
	fuente string
}{
	{DriveCarpeta, FuenteDrive},
}

// Origen es el `.tsx` del que sale este icono.
func (i Icono) Origen() string {
	for n := len(propios) - 1; n >= 0; n-- {
		if i >= propios[n].desde {
			return propios[n].fuente
		}
	}
	return FuenteComun
}

// entrada es lo que la web declara de cada icono: su clave y su único `d`.
type entrada struct{ clave, d string }

// catalogo va indexado por el propio Icono, así que no hay mapa ni búsqueda, y añadir
// uno en medio no descoloca a los demás. `TestElCatalogoEstaCompleto` comprueba que no
// queda ningún hueco.
var catalogo = [...]entrada{
	Ninguno:             {"", ""},
	Buscar:              {"search", "M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14ZM20 20l-3.8-3.8"},
	Correo:              {"mail", "M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2zM22 6l-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 6"},
	Campana:             {"bell", "M18 8.5a6 6 0 1 0-12 0c0 5.5-2.2 7-2.2 7h16.4s-2.2-1.5-2.2-7ZM10.3 20a2 2 0 0 0 3.4 0"},
	Cerrar:              {"close", "M18.5 5.5 5.5 18.5M5.5 5.5l13 13"},
	Tic:                 {"check", "M20 6.5 9.5 17.5 4 12"},
	ChevronAbajo:        {"chevronDown", "M5 8.5l7 7 7-7"},
	ChevronArriba:       {"chevronUp", "M5 15.5l7-7 7 7"},
	ChevronDerecha:      {"chevronRight", "M8.5 5l7 7-7 7"},
	ChevronIzquierda:    {"chevronLeft", "M15.5 5l-7 7 7 7"},
	Menu:                {"menu", "M3.5 6.5h17M3.5 12h17M3.5 17.5h17"},
	Rejilla:             {"grid", "M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z"},
	Usuario:             {"user", "M12 3.5a4 4 0 1 0 0 8 4 4 0 0 0 0-8ZM4 20.5a8 8 0 0 1 16 0"},
	Ajustes:             {"settings", "M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6Z M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z"},
	Salir:               {"logout", "M9 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h3M16 16.5l4.5-4.5L16 7.5M20.5 12H9"},
	Informacion:         {"info", "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18ZM12 11v5M12 7.5h.01"},
	Aviso:               {"warning", "M10.3 4.2 2.2 18a2 2 0 0 0 1.7 3h16.2a2 2 0 0 0 1.7-3L13.7 4.2a2 2 0 0 0-3.4 0ZM12 9.5v4.5M12 17.5h.01"},
	Peligro:             {"danger", "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18ZM12 8v4.5M12 16h.01"},
	Mas:                 {"plus", "M12 4.5v15M4.5 12h15"},
	Edificio:            {"building", "M3.5 21V6a2 2 0 0 1 2-2h7a2 2 0 0 1 2 2v15M14.5 21V10.5h3a2 2 0 0 1 2 2V21M2.5 21h19M7 8h3.5M7 11.5h3.5M7 15h3.5"},
	Usuarios:            {"users", "M16 21v-1.5a4.5 4.5 0 0 0-4.5-4.5h-4A4.5 4.5 0 0 0 3 19.5V21M9.5 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8ZM21 21v-1.5a4.5 4.5 0 0 0-3.4-4.36M16 3.13a4 4 0 0 1 0 7.74"},
	BaseDeDatos:         {"database", "M12 3c-4.4 0-8 1.3-8 3s3.6 3 8 3 8-1.3 8-3-3.6-3-8-3ZM4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"},
	Cubo:                {"bucket", "M12 3.5c-4.4 0-8 1.1-8 2.5s3.6 2.5 8 2.5 8-1.1 8-2.5-3.6-2.5-8-2.5ZM4.3 6.2l1.6 14a1 1 0 0 0 1 .8h10.2a1 1 0 0 0 1-.8l1.6-14"},
	Cohete:              {"rocket", "M12 3.8c2.6 2.2 4 5.2 4 8.2V16H8v-4c0-3 1.4-6 4-8.2ZM12 9.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3ZM8 14l-3 2.7V20l3-1.4M16 14l3 2.7V20l-3-1.4M12 16v4"},
	Nube:                {"cloud", "M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"},
	Auditoria:           {"audit", "M15 4.5h2A1.5 1.5 0 0 1 18.5 6v13.5A1.5 1.5 0 0 1 17 21H7a1.5 1.5 0 0 1-1.5-1.5V6A1.5 1.5 0 0 1 7 4.5h2M9 3.5h6a1 1 0 0 1 1 1V6a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1V4.5a1 1 0 0 1 1-1ZM9 13.5l2 2 3.5-4"},
	Candado:             {"lock", "M8 10V7a4 4 0 0 1 8 0v3M5 11a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-8ZM12 14.5v2.5"},
	Globo:               {"globe", "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18ZM3.5 12h17M12 3a14 14 0 0 1 0 18 14 14 0 0 1 0-18Z"},
	Tabla:               {"table", "M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1ZM3 9.5h18M9.5 5.5V18.5M14.5 5.5V18.5"},
	IA:                  {"ai", "M12 3.5 13.7 8.3 18.5 10 13.7 11.7 12 16.5 10.3 11.7 5.5 10 10.3 8.3ZM18 15.5l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8Z"},
	Puntos:              {"ellipsis", "M6 12h.01M12 12h.01M18 12h.01"},
	Codigo:              {"code", "M9 8l-4.5 4 4.5 4M15 8l4.5 4-4.5 4M13.5 5.5l-3 13"},
	Terminal:            {"terminal", "M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1ZM7 10l2.5 2L7 14M12.5 15H17"},
	Grafico:             {"chart", "M4 4v16h16M8 20v-4M12 20v-8M16 20v-11"},
	Reloj:               {"clock", "M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18ZM12 7.5V12l3 2"},
	Recibo:              {"receipt", "M6 4.5a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1V20.5l-2-1.2-2 1.2-2-1.2-2 1.2-2-1.2-2 1.2ZM9 8h6M9 11.5h6"},
	Llave:               {"key", "M12 3.5a3.75 3.75 0 1 0 0 7.5 3.75 3.75 0 0 0 0-7.5ZM12 11v9.5M12 15h3.5M12 18h2.5"},
	Escudo:              {"shield", "M12 3.5 4.5 6.6v5.4c0 4.3 3 7.5 7.5 9 4.5-1.5 7.5-4.7 7.5-9V6.6L12 3.5Z"},
	Dispositivo:         {"device", "M5.5 5.5h13a1 1 0 0 1 1 1v8.5h-15V6.5a1 1 0 0 1 1-1ZM2.5 18.5h19"},
	UsuarioVisto:        {"userCheck", "M9.5 3.5a4 4 0 1 0 0 8 4 4 0 0 0 0-8ZM2 20.5a7.5 7.5 0 0 1 15 0M16.5 13l1.8 1.8L22 11"},
	Etiqueta:            {"tag", "M11.7 3.5H19a1.5 1.5 0 0 1 1.5 1.5v7.3a1.5 1.5 0 0 1-.44 1.06l-6.7 6.7a1.5 1.5 0 0 1-2.12 0l-7.3-7.3a1.5 1.5 0 0 1 0-2.12l6.7-6.7A1.5 1.5 0 0 1 11.7 3.5ZM16.3 8.2h.01"},
	Mano:                {"hand", "M15 12V7a1.5 1.5 0 0 1 3 0v8a6 6 0 0 1-6 6h-1a5 5 0 0 1-3.5-1.5l-3.2-3.2a1.5 1.5 0 0 1 2.1-2.1L9 15V6a1.5 1.5 0 0 1 3 0v6M12 12V4.5a1.5 1.5 0 0 1 3 0V12"},
	Rotulador:           {"highlighter", "M13.5 2.5 20.5 9.5 10.5 19.5H4v-6.5ZM4 22h16"},
	Subrayado:           {"underline", "M7 4v6a5 5 0 0 0 10 0V4M5.5 20.5h13"},
	Tachado:             {"strikethrough", "M4.5 12h15M8 7.5A3.5 3.5 0 0 1 11.5 4.5h1a3.5 3.5 0 0 1 3.4 2.6M16 16.5a3.5 3.5 0 0 1-3.5 3h-1a3.5 3.5 0 0 1-3.4-2.6"},
	Comentario:          {"comment", "M4.5 4.5h15a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H11l-5 4v-4H4.5a1 1 0 0 1-1-1v-10a1 1 0 0 1 1-1Z"},
	Lapiz:               {"pen", "M4 20.5h4l11-11a2.1 2.1 0 0 0-3-3L5 17.5v3ZM14.5 6.5l3 3"},
	Cuadrado:            {"square", "M4.5 5.5h15a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1v-11a1 1 0 0 1 1-1Z"},
	Circulo:             {"circle", "M12 5.5c4.7 0 8.5 2.9 8.5 6.5s-3.8 6.5-8.5 6.5S3.5 15.6 3.5 12 7.3 5.5 12 5.5Z"},
	Linea:               {"line", "M4.5 19.5 19.5 4.5"},
	FlechaArribaDerecha: {"arrowUpRight", "M5 19 19 5M9.5 5H19v9.5"},
	Regla:               {"ruler", "M2.9 16.2 16.2 2.9a1.5 1.5 0 0 1 2.1 0l2.8 2.8a1.5 1.5 0 0 1 0 2.1L7.8 21.1a1.5 1.5 0 0 1-2.1 0l-2.8-2.8a1.5 1.5 0 0 1 0-2.1ZM7 12l2 2M10 9l2 2M13 6l2 2"},
	Texto:               {"text", "M5 4.5h14M12 4.5v15M9 19.5h6"},
	Negrita:             {"bold", "M7 4.5h6.5a3.75 3.75 0 0 1 0 7.5H7zM7 12h7.5a3.75 3.75 0 0 1 0 7.5H7z"},
	Cursiva:             {"italic", "M10 4.5h8M6 19.5h8M14.5 4.5l-5 15"},
	Enlace:              {"link", "M10 13.5a4.5 4.5 0 0 0 6.36.36l2.5-2.5a4.5 4.5 0 0 0-6.36-6.36l-1.25 1.25M14 10.5a4.5 4.5 0 0 0-6.36-.36l-2.5 2.5a4.5 4.5 0 0 0 6.36 6.36l1.25-1.25"},
	Cita:                {"quote", "M17.5 6.5h-14M20.5 12h-13M20.5 17.5h-13M3.5 12v5.5"},
	Lista:               {"list", "M4 6.5h.01M4 12h.01M4 17.5h.01M8.5 6.5h11.5M8.5 12h11.5M8.5 17.5h11.5"},
	Sello:               {"stamp", "M4.5 20.5h15M6.5 17.5h11v-1.8a2 2 0 0 0-1.5-1.9l-2.2-.6a1.3 1.3 0 0 1-.9-1.5l.5-2.9A3.4 3.4 0 0 0 12 3.5a3.4 3.4 0 0 0-3.4 4.3l.5 2.9a1.3 1.3 0 0 1-.9 1.5l-2.2.6a2 2 0 0 0-1.5 1.9Z"},
	CampoDeFormulario:   {"formField", "M3.5 7.5h17v9h-17ZM8 10.5h4M10 10.5v3"},
	Censura:             {"redact", "M4.5 5.5h15M4.5 18.5h9M3.5 10h17v4h-17Z"},
	Menos:               {"minus", "M4.5 12h15"},
	AjustarAncho:        {"fitWidth", "M3.5 12h17M7 8.5 3.5 12 7 15.5M17 8.5 20.5 12 17 15.5"},
	AjustarPagina:       {"fitPage", "M14.5 3.5H20v5.5M20 3.5l-6.5 6.5M9.5 20.5H4V15M4 20.5 10.5 14"},
	DosPaginas:          {"dosPaginas", "M4 4.5h6.5v15H4zM13.5 4.5H20v15h-6.5z"},
	Camara:              {"camera", "M4 8.5A1.5 1.5 0 0 1 5.5 7H8l1.4-2h5.2L16 7h2.5A1.5 1.5 0 0 1 20 8.5V17a1.5 1.5 0 0 1-1.5 1.5h-13A1.5 1.5 0 0 1 4 17ZM12 16a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z"},
	Microfono:           {"microfono", "M12 4a3 3 0 0 1 3 3v5a3 3 0 0 1-6 0V7a3 3 0 0 1 3-3ZM6 11.5a6 6 0 0 0 12 0M12 17.5V20"},
	MicrofonoApagado:    {"microfonoOff", "M9 7a3 3 0 0 1 6 0v3M15 13.2a3 3 0 0 1-6-1.2M6 11.5a6 6 0 0 0 9.3 5M18 11.5a6 6 0 0 1-.4 2.2M12 17.5V20M4 4l16 16"},
	Pantalla:            {"pantalla", "M3.5 5.5h17v10h-17zM8.5 19.5h7M12 15.5v4M12 12V7.5M9.5 10 12 7.5 14.5 10"},
	Corazon:             {"corazon", "M20.8 5.7a5.5 5.5 0 0 0-7.8 0l-1 1-1-1a5.5 5.5 0 0 0-7.8 7.8l8.8 8.7 8.8-8.7a5.5 5.5 0 0 0 0-7.8Z"},
	Imagen:              {"image", "M4 6a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2ZM8.5 10.5a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3ZM20 15l-4.5-4.5L7 19"},
	Album:               {"album", "M8 8V6a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-2M4 10a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2ZM16 20l-4-4-4 4"},
	Rotar:               {"rotate", "M20 12a8 8 0 1 1-2.3-5.7M20 4v4.5h-4.5"},
	VoltearH:            {"flipH", "M12 3v18M4 8l4 4-4 4V8ZM20 8l-4 4 4 4V8Z"},
	VoltearV:            {"flipV", "M3 12h18M8 4l4 4 4-4H8ZM8 20l4-4 4 4H8Z"},
	Deshacer:            {"undo", "M9 14 4 9l5-5M4 9h10a6 6 0 0 1 0 12h-3"},
	Guardar:             {"save", "M5 3h11l3 3v15H5zM8 3v6h8V3M8 21v-7h8v7"},
	Descargar:           {"download", "M12 4v12M7 11l5 5 5-5M4 20h16"},
	Subir:               {"upload", "M12 16V4M7 9l5-5 5 5M4 20h16"},
	Intercambiar:        {"swap", "M8 20V5M4 9l4-4 4 4M16 4v15M12 15l4 4 4-4"},
	Calendario:          {"calendar", "M4 6h16v14H4zM4 10h16M8 3v4M16 3v4"},
	Copias:              {"copies", "M8 8h12v12H8zM4 16V4h12"},
	Papelera:            {"trash", "M4 7h16M10 11v6M14 11v6M6 7l1 12a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-12M9 7V4h6v3"},
	Recorte:             {"recorte", "M7 2.5v14.5h14.5M2.5 7H17v14.5"},
	Voz:                 {"voz", "M4 9.5h3.5L12 5.5v13L7.5 14.5H4zM16 9a4 4 0 0 1 0 6M18.5 6.5a7.5 7.5 0 0 1 0 11"},
	Emitir:              {"emitir", "M12 10.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3ZM8.5 8.5a5 5 0 0 0 0 7M15.5 8.5a5 5 0 0 1 0 7M5.6 5.6a9 9 0 0 0 0 12.8M18.4 5.6a9 9 0 0 1 0 12.8"},
	Explorar:            {"explorar", "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18ZM15.5 8.5l-2 5-5 2 2-5z"},
	Inicio:              {"home", "M4 10.5 12 4l8 6.5V20h-5.5v-6h-5v6H4Z"},
	Maximizar:           {"maximizar", "M4 9V4h5M15 4h5v5M20 15v5h-5M9 20H4v-5"},
	Minimizar:           {"minimizar", "M9 4v5H4M20 9h-5V4M15 20v-5h5M4 15h5v5"},
	Ojo:                 {"ojo", "M2.5 12s3.5-6.5 9.5-6.5 9.5 6.5 9.5 6.5-3.5 6.5-9.5 6.5S2.5 12 2.5 12ZM12 9.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5Z"},
	Pausa:               {"pause", "M7 5h3v14H7zM14 5h3v14h-3z"},
	ImagenEnImagen:      {"pip", "M3.5 5.5h17v13h-17zM12.5 12h6v4.5h-6z"},
	Reproducir:          {"play", "M7 4.8v14.4a.8.8 0 0 0 1.2.7l11.4-7.2a.8.8 0 0 0 0-1.4L8.2 4.1A.8.8 0 0 0 7 4.8Z"},
	Video:               {"video", "M3.5 6.5h12v11h-12zM15.5 10.5l5-3v9l-5-3"},
	VozApagada:          {"vozOff", "M4 9.5h3.5L12 5.5v13L7.5 14.5H4zM16 9.5l5 5M21 9.5l-5 5"},
	OjoTachado:          {"ojoOff", "M10 5.7A9.5 9.5 0 0 1 12 5.5c6 0 9.5 6.5 9.5 6.5a17 17 0 0 1-2.4 3.2M6.3 6.8C3.9 8.4 2.5 12 2.5 12s3.5 6.5 9.5 6.5a9 9 0 0 0 4.7-1.3M10.2 10.2a2.5 2.5 0 0 0 3.6 3.6M4 4l16 16"},

	// Los glifos propios de Sytue Drive, verbatim de su `glyphs.tsx`.
	DriveCarpeta:      {"folder", "M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"},
	DriveFichero:      {"file", "M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6"},
	DriveDocumento:    {"document", "M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8zM14 3v5h5M9 13h6M9 17h4"},
	DriveHoja:         {"spreadsheet", "M4 5h16v14H4zM4 10h16M4 15h16M10 5v14"},
	DrivePresentacion: {"presentation", "M4 4h16v11H4zM12 15v4M9 21h6"},
	DriveEstrella:     {"star", "m12 3 2.9 5.9 6.5.9-4.7 4.6 1.1 6.4L12 17.8l-5.8 3 1.1-6.4-4.7-4.6 6.5-.9Z"},
}

// Clave es el nombre del icono en la web. Solo para mensajes de error y para el
// guardián que compara con Icon.tsx; una pantalla pide el icono por su constante.
func (i Icono) Clave() string { return catalogo[i].clave }

// Los tamaños de icono de la web. Son píxeles literales del `size={…}` de Icon.tsx y
// NO salen de la escala de espaciado, así que no se calculan: se copian. Contados
// sobre sus usos en `apps/frontend`: 16 aparece 363 veces, 14 166, 18 89, 12 42, 28 41,
// 20 41 y 24 37. Se declaran aquí para que ninguna pantalla escriba el número.
const (
	TallaXXS float32 = 12
	TallaXS  float32 = 14
	TallaSM  float32 = 16
	TallaMD  float32 = 18
	TallaLG  float32 = 20 // el de por defecto de Icon.tsx
	TallaXL  float32 = 24
	TallaXXL float32 = 28
)
