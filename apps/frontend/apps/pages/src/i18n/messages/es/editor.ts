/**
 * Mensajes en español para el área del editor visual (editor).
 * Prefijo: editor.*
 */
export const editor = {
  'editor.title': 'Editor visual',
  'editor.preview': 'Vista previa',
  'editor.preview_creating': 'Creando vista previa…',
  'editor.preview_wait_save': 'Espera a que se guarden los cambios antes de crear la vista previa.',
  'editor.conflict.title': 'Conflicto de edición en el borrador',
  'editor.conflict.body':
    'El borrador de este sitio ha cambiado en otra pestaña o desde otra cuenta. Si sigues guardando, puedes sobrescribir esos cambios.',
  'editor.conflict.cancel': 'Cancelar',
  'editor.conflict.overwrite': 'Sobrescribir',
  'editor.conflict.reload': 'Recargar borrador',
  'editor.bases.insert': 'Tabla desde Sytue Bases',
  'editor.bases.desc': 'Los registros de una vista, tal como están ahora',
  'editor.bases.title': 'Tabla desde Sytue Bases',
  'editor.bases.prompt':
    'Pega el token de una conexión de solo lectura concedida para Sytue Webs. Los datos de esa vista pasarán a ser contenido del sitio y serán públicos al publicar.',
  'editor.bases.confirm': 'Traer la vista',
  'editor.bases.invalid':
    'Esa conexión no vale para Sytue Webs: puede estar revocada, caducada o concedida a otro producto u organización.',
  'editor.bases.large':
    'La vista no cabe en una tabla del sitio (50 filas y 8 columnas como mucho). Acótala en Sytue Bases.',
  'editor.bases.forbidden': 'Solo administración puede traer datos de Sytue Bases a un sitio.',
  'editor.bases.generic': 'No se ha podido leer de Sytue Bases. Inténtalo de nuevo.',
  'editor.pages.title': 'Páginas del sitio ({count})',
  'editor.pages.add': 'Añadir',
  'editor.pages.new_title': 'Título de la nueva página',
  'editor.pages.new_placeholder': 'P. ej. Sobre nosotros',
  'editor.pages.cancel': 'Cancelar',
  'editor.pages.create': 'Crear página',
  'editor.pages.home': 'Inicio',
  'editor.pages.move_up': 'Subir {titulo}',
  'editor.pages.move_down': 'Bajar {titulo}',
  'editor.pages.duplicate': 'Duplicar {titulo}',
  'editor.pages.delete': 'Eliminar {titulo}',
  'editor.pages.settings': 'Ajustes: {titulo}',
  'editor.pages.field_title': 'Título ({idioma})',
  'editor.pages.field_slug': 'Slug de la URL ({idioma})',
  'editor.pages.field_description': 'Meta descripción (SEO)',
  'editor.pages.description_placeholder': 'Lo que un buscador enseñará bajo el título',
  'editor.pages.counter': '{n}/{max}',
  'editor.pages.show_header': 'Mostrar cabecera común',
  'editor.pages.show_footer': 'Mostrar pie de página común',
  'editor.pages.noindex': 'Ocultar de buscadores (noindex)',
  'editor.pages.delete_title': 'Eliminar página',
  'editor.pages.delete_confirm_button': 'Eliminar',
  'editor.pages.delete_confirm':
    '¿Eliminar la página «{titulo}»? Puedes deshacerlo desde el historial del editor mientras no salgas.',
};
