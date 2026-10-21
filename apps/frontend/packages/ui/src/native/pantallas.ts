/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PANTALLAS DECLARATIVAS DE LA SUITE SYTUE
 *
 * Cada pantalla define la estructura de UI canónica compartida entre la web y la
 * suite en Gio, sin webview, que la volcaba a JSON. Gio se retiró el 2026-09-23.
 */

import { ui, type NodoUI } from './nodos';

export interface DeclaracionPantalla {
  app: string;
  titulo: string;
  arbol: NodoUI;
}

export const pantallaMail: DeclaracionPantalla = {
  app: 'mail',
  titulo: 'Sytue Mail',
  arbol: ui.shell(
    // Rail: carpetas
    [
      ui.fila('normal', ui.icono('mail', 24), ui.texto('Sytue Mail', 'seccion', 'negrita')),
      ui.espacio('medio'),
      ui.t('compose.newTitle', ui.boton('Nuevo mensaje', 'redactar', 'principal', 'pencil')),
      ui.espacio('medio'),
      ui.lista('carpetas', [
        ui.fila(
          'apretada',
          ui.texto('{nombre}', 'cuerpo', 'medio'),
          ui.espacio('flexible'),
          ui.contador('{no_leidos}', 'exito'),
        ),
      ]),
      ui.espacio('flexible'),
      ui.divisor(),
      ui.ranura('sesion_usuario'),
    ],
    // Contenido: Lector / Redactor
    [ui.ranura('lector_correo')],
    // Panel secundario: lista de mensajes
    [
      ui.t('list.searchPlaceholder', ui.entrada('Buscar en el correo...', 'buscar')),
      ui.espacio('pequeno'),
      ui.lista('mensajes', [
        ui.columna(
          'apretada',
          ui.fila(
            'apretada',
            ui.texto('{de}', 'detalle', 'negrita'),
            ui.espacio('flexible'),
            ui.texto('{fecha}', 'micro', 'normal', 'mudo'),
          ),
          ui.texto('{asunto}', 'cuerpo', 'medio'),
          ui.texto('{resumen}', 'detalle', 'normal', 'mudo'),
        ),
      ]),
    ],
  ),
};

export const pantallaDrive: DeclaracionPantalla = {
  app: 'drive',
  titulo: 'Sytue Drive',
  arbol: ui.shell(
    // Rail: navegación de carpetas
    [
      ui.fila('normal', ui.icono('drive', 24), ui.texto('Sytue Drive', 'seccion', 'negrita')),
      ui.espacio('medio'),
      ui.t('files.empty.cta', ui.boton('Subir archivo', 'subir_archivo', 'principal', 'upload')),
      ui.t(
        'team.newFolder',
        ui.boton('Nueva carpeta', 'nueva_carpeta', 'secundario', 'folder-plus'),
      ),
      ui.espacio('medio'),
      ui.lista('arbol_carpetas', [
        ui.fila('apretada', ui.icono('folder', 16), ui.texto('{nombre}', 'cuerpo', 'normal')),
      ]),
      ui.espacio('flexible'),
      ui.divisor(),
      ui.ranura('almacenamiento_cuota'),
    ],
    // Contenido: detalles de archivo / visor
    [ui.ranura('detalle_archivo')],
    // Panel central: lista/cuadrícula de archivos
    [
      ui.fila(
        'normal',
        ui.t('files.search', ui.entrada('Buscar en Drive...', 'buscar')),
        ui.boton('Sincronizar', 'sincronizar', 'secundario', 'refresh'),
      ),
      ui.espacio('pequeno'),
      ui.lista('archivos', [
        ui.fila(
          'normal',
          ui.icono('{tipo_icono}', 20),
          ui.columna(
            'apretada',
            ui.texto('{nombre}', 'cuerpo', 'medio'),
            ui.texto('{tamano} • {modificado}', 'micro', 'normal', 'mudo'),
          ),
          ui.espacio('flexible'),
          ui.t('files.selection.download', ui.boton('Descargar', 'descargar', 'secundario')),
        ),
      ]),
    ],
  ),
};

export const pantallaRooms: DeclaracionPantalla = {
  app: 'rooms',
  titulo: 'Sytue Rooms',
  arbol: ui.shell(
    // Rail: Canales y Mensajes Directos
    [
      ui.fila('normal', ui.icono('rooms', 24), ui.texto('Sytue Rooms', 'seccion', 'negrita')),
      ui.espacio('medio'),
      ui.t('sidebar.channels', ui.texto('CANALES', 'micro', 'negrita', 'mudo')),
      ui.lista('canales', [
        ui.fila(
          'apretada',
          ui.texto('# {nombre}', 'cuerpo', 'medio'),
          ui.espacio('flexible'),
          ui.contador('{no_leidos}', 'principal'),
        ),
      ]),
      ui.espacio('medio'),
      ui.t('sidebar.directMessages', ui.texto('MENSAJES DIRECTOS', 'micro', 'negrita', 'mudo')),
      ui.lista('dms', [
        ui.fila('apretada', ui.avatar('{nombre}'), ui.texto('{nombre}', 'cuerpo', 'normal')),
      ]),
      ui.espacio('flexible'),
      ui.ranura('sesion_usuario'),
    ],
    // Contenido: Conversación en vivo
    [
      ui.columna(
        'normal',
        ui.barra('{canal_activo}'),
        ui.divisor(),
        ui.lista('mensajes_chat', [
          ui.fila(
            'normal',
            ui.avatar('{remitente}'),
            ui.columna(
              'apretada',
              ui.fila(
                'apretada',
                ui.texto('{remitente}', 'detalle', 'negrita'),
                ui.espacio('pequeno'),
                ui.texto('{hora}', 'micro', 'normal', 'mudo'),
              ),
              ui.texto('{texto}', 'cuerpo', 'normal'),
            ),
          ),
        ]),
        ui.espacio('flexible'),
        ui.fila(
          'apretada',
          ui.t(
            'composer.writeIn',
            ui.entrada('Escribe un mensaje en #{canal_activo}...', 'mensaje'),
          ),
          ui.t('composer.send', ui.boton('Enviar', 'enviar_mensaje', 'principal')),
        ),
      ),
    ],
  ),
};

export const pantallaCalendar: DeclaracionPantalla = {
  app: 'calendar',
  titulo: 'Sytue Calendar',
  arbol: ui.shell(
    [
      ui.fila('normal', ui.icono('calendar', 24), ui.texto('Sytue Calendar', 'seccion', 'negrita')),
      ui.espacio('medio'),
      ui.t('nav.createEvent', ui.boton('Crear evento', 'nuevo_evento', 'principal', 'plus')),
      ui.espacio('medio'),
      // El mini-calendario, el encabezado «MIS CALENDARIOS» con sus acciones y la lista
      // con sus casillas de visibilidad. Es una RANURA y no una `lista` porque nada de
      // eso es forma declarable: el mini-mes tiene su propio cursor y las filas llevan
      // casilla, color y acciones. Cuando era una `lista`, el menú de Calendar se
      // quedaba en siete viñetas donde la web tiene media pantalla de mandos.
      ui.ranura('menu_calendarios'),
    ],
    [ui.ranura('vista_calendario')],
  ),
};

export const pantallaContacts: DeclaracionPantalla = {
  app: 'contacts',
  titulo: 'Sytue Contacts',
  arbol: ui.shell(
    [
      ui.fila('normal', ui.icono('contacts', 24), ui.texto('Sytue Contacts', 'seccion', 'negrita')),
      ui.espacio('medio'),
      ui.t('nav.newContact', ui.boton('Nuevo contacto', 'nuevo_contacto', 'principal')),
      ui.espacio('pequeno'),
      ui.t('list.search', ui.entrada('Buscar contacto...', 'buscar')),
      ui.lista('lista_contactos', [
        ui.fila('normal', ui.avatar('{nombre}'), ui.texto('{nombre}', 'cuerpo', 'medio')),
      ]),
    ],
    [ui.ranura('detalle_contacto')],
  ),
};

export const pantallaNotes: DeclaracionPantalla = {
  app: 'notes',
  titulo: 'Sytue Notepads',
  arbol: ui.shell(
    [
      ui.fila('normal', ui.icono('notes', 24), ui.texto('Sytue Notepads', 'seccion', 'negrita')),
      ui.espacio('medio'),
      ui.t('note.new', ui.boton('Nueva nota', 'nueva_nota', 'principal')),
      ui.espacio('pequeno'),
      ui.lista('cuadernos', [
        ui.fila(
          'apretada',
          ui.texto('{nombre}', 'cuerpo', 'medio'),
          ui.espacio('flexible'),
          ui.contador('{cantidad}'),
        ),
      ]),
    ],
    [ui.ranura('editor_nota')],
    [
      ui.t('sidebar.search', ui.entrada('Buscar notas...', 'buscar')),
      ui.lista('lista_notas', [
        ui.columna(
          'apretada',
          ui.texto('{titulo}', 'cuerpo', 'negrita'),
          ui.texto('{resumen}', 'detalle', 'normal', 'mudo'),
          ui.texto('{fecha}', 'micro', 'normal', 'mudo'),
        ),
      ]),
    ],
  ),
};

export const pantallaHub: DeclaracionPantalla = {
  app: 'hub',
  titulo: 'Sytue One',
  arbol: ui.columna(
    'ancha',
    ui.barra('Sytue One — Lanzador de la Suite'),
    ui.tarjeta([
      ui.t('search.heading', ui.texto('Busca en toda la suite', 'seccion', 'negrita')),
      ui.t(
        'search.hint',
        ui.texto(
          'Escribe aquí arriba para buscar a la vez en tu correo, tus archivos, tus documentos, tus eventos, tus contactos y tus salas.',
          'detalle',
          'normal',
          'mudo',
        ),
      ),
      ui.espacio('medio'),
      ui.ranura('busqueda_federada'),
    ]),
    ui.tarjeta([
      ui.t('launcher.apps', ui.texto('Aplicaciones de la Suite', 'seccion', 'negrita')),
      ui.texto(
        'Accede a todas las herramientas soberanas de trabajo.',
        'detalle',
        'normal',
        'mudo',
      ),
      ui.espacio('medio'),
      ui.ranura('grid_aplicaciones'),
    ]),
    ui.tarjeta([
      ui.texto('Estado del Sistema y Sincronización', 'seccion', 'negrita'),
      ui.ranura('estado_conectividad'),
    ]),
  ),
};

export const todasLasPantallas: DeclaracionPantalla[] = [
  pantallaMail,
  pantallaDrive,
  pantallaRooms,
  pantallaCalendar,
  pantallaContacts,
  pantallaNotes,
  pantallaHub,
];
