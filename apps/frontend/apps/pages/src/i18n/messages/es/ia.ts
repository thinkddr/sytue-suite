/**
 * Mensajes en español para el área de inteligencia artificial (ia).
 * Prefijo: ai.*
 */
export const ia = {
  // Ajustes
  'ai.settings.provider': 'Proveedor de IA',
  'ai.settings.model': 'Modelo',
  'ai.settings.key': 'Clave de API propia (BYOK)',
  'ai.settings.key_placeholder': 'Introduce tu clave secreta de API…',
  'ai.settings.key_help':
    'Tu clave se almacena cifrada con AES-256-GCM. Nunca se devuelve al navegador.',
  'ai.settings.check_key': 'Probar clave',
  'ai.settings.delete_key': 'Eliminar clave',
  'ai.settings.status_configured': 'Clave configurada',
  'ai.settings.tested_at': 'Comprobada el',
  'ai.settings.quota_title': 'Cupo mensual de IA',
  'ai.settings.quota_reset': 'Se reinicia el {date}',
  'ai.settings.notice_eu_title': 'Aviso de transferencia fuera del EEE',
  'ai.settings.notice_eu_desc':
    'El proveedor seleccionado ({provider}) procesa datos fuera del Espacio Económico Europeo. Conforme al art. 49.1.a del RGPD, debes consentir explícitamente esta transferencia para continuar.',
  'ai.settings.notice_eu_accept':
    'Acepto la transferencia internacional de datos fuera del EEE para peticiones de IA con este proveedor.',
  'ai.settings.success_save': 'Clave guardada y verificada correctamente.',
  'ai.settings.error_check': 'Error comprobando la clave con el proveedor.',
  'ai.settings.form_title': 'Configurar clave propia (BYOK)',
  'ai.settings.form_help':
    'Usa tus propias credenciales de Mistral AI, OpenAI, Anthropic o Zhipu AI para no depender del cupo mensual.',
  'ai.settings.provider_mistral': 'Mistral AI (UE, RGPD)',
  'ai.settings.provider_openai': 'OpenAI (EE. UU.)',
  'ai.settings.provider_anthropic': 'Anthropic (EE. UU.)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'UE, Francia',
  'ai.region.us': 'EE. UU.',
  'ai.region.intl': 'Internacional',
  'ai.settings.show_key': 'Mostrar clave',
  'ai.settings.hide_key': 'Ocultar clave',
  'ai.settings.key_required': 'Introduce una clave.',
  'ai.settings.notice_required': 'Debes aceptar el aviso de transferencia fuera del EEE.',
  'ai.settings.check_ok': 'Clave comprobada con éxito con el proveedor.',
  'ai.settings.error_save': 'No se pudo guardar la clave.',
  'ai.settings.error_delete': 'No se pudo eliminar la clave.',
  'ai.settings.checking': 'Comprobando…',
  'ai.settings.save_activate': 'Guardar y activar',
  'ai.settings.key_deleted': 'Clave eliminada. Se usará el cupo mensual por defecto.',
  'ai.settings.active_provider': 'Proveedor activo: {provider} ({model})',
  'ai.settings.no_key_default':
    'Sin clave propia configurada. Se aplica el límite mensual del plan.',
  'ai.settings.status_plan': 'Cupo del plan',
  'ai.settings.quota_unlimited': 'Ilimitado',
  'ai.settings.quota_unlimited_help': 'Peticiones gestionadas con tu propia clave de API.',
  'ai.settings.quota_jobs': 'Trabajos de diseño, traducción y exportación.',
  'ai.settings.loading': 'Cargando los ajustes de IA',
  'ai.settings.load_error': 'No se pudieron cargar los ajustes de IA.',
  'ai.settings.load_error_help':
    'No se ha podido consultar el servicio. Tu configuración no ha cambiado.',
  'ai.settings.key_invalid':
    'El proveedor ha rechazado la clave. Comprueba que sea correcta y esté activa.',
  'ai.settings.encryption_unavailable':
    'El servicio aún no está preparado para guardar claves de forma segura. Contacta con soporte.',
  'ai.settings.invalid_model': 'Elige un modelo válido para este proveedor.',
  'ai.settings.key_state': 'Estado de la clave',

  'ai.common.cancel': 'Cancelar',

  // Contexto común: cupo, aviso RGPD, errores y estado del trabajo
  'ai.context.quota_label': 'Cupo mensual de IA',
  'ai.context.quota_used': '{used} de {limit} trabajos este mes',
  'ai.context.quota_reset': 'Se renueva el {date}.',
  'ai.context.quota_exhausted_title': 'Has agotado el cupo mensual de IA',
  'ai.context.quota_exhausted_desc':
    'Podrás lanzar más trabajos el {date}. Quien administra la organización puede quitar el límite añadiendo una clave de API propia en los ajustes de IA.',
  'ai.context.byok': 'Tu organización usa su propia clave de API: este trabajo no gasta cupo.',
  'ai.context.loading': 'Consultando el cupo de IA',
  'ai.context.load_error': 'No se pudo consultar el cupo de IA. Puedes intentarlo igualmente.',
  'ai.context.retry': 'Reintentar',
  'ai.notice.eu': 'Los textos se procesan dentro de la UE, conforme al RGPD.',
  'ai.notice.non_eu_title': 'Este trabajo sale del Espacio Económico Europeo',
  'ai.notice.non_eu_desc':
    'La clave propia de tu organización es de {provider}, que procesa los datos fuera del EEE. Tu organización aceptó esa transferencia (art. 49.1.a del RGPD) al guardar la clave.',
  'ai.error.quota': 'Se ha agotado el cupo mensual de IA. Se renueva el {date}.',
  'ai.error.quota_nodate': 'Se ha agotado el cupo mensual de IA.',
  'ai.error.busy': 'Ya hay trabajos de IA en marcha. Espera a que terminen e inténtalo de nuevo.',
  'ai.error.admin': 'Solo quienes administran la organización pueden hacer esto.',
  'ai.error.prompt': 'Describe tu sitio antes de generar la propuesta.',
  'ai.error.languages': 'Elige al menos un idioma de destino.',
  'ai.error.session': 'Tu sesión ha caducado. Recarga la página e inicia sesión de nuevo.',
  'ai.error.generic': 'No se pudo iniciar el trabajo. Inténtalo de nuevo en unos minutos.',
  'ai.error.draft_not_saved':
    'No se pudo guardar el borrador. Resuelve el problema de guardado y vuelve a iniciar la IA.',
  'ai.job.state_queued': 'En cola',
  'ai.job.state_running': 'En curso',
  'ai.job.state_done': 'Hecho',
  'ai.job.state_failed': 'Con error',
  'ai.job.queued_text': 'Esperando a que el servidor acepte el trabajo…',
  'ai.job.running_text': 'La IA está trabajando. Puede tardar unos minutos.',
  'ai.job.done_text': 'El trabajo ha terminado.',
  'ai.job.failed_text': 'El trabajo no se ha podido completar.',
  'ai.job.lost_text': 'Se ha perdido la conexión con el servidor.',
  'ai.job.elapsed': 'Tiempo transcurrido: {time}',
  'ai.job.detail': 'Detalle técnico: {detail}',
  'ai.job.model': 'Modelo: {model}',
  'ai.job.badge_eu': 'Dentro de la UE',
  'ai.job.badge_non_eu': 'Fuera del EEE',
  'ai.job.badge_byok': 'Clave propia',
  'ai.job.back': 'Volver',
  'ai.job.retry': 'Reintentar',
  'ai.job.stop': 'Dejar de esperar',
  'ai.job.stop_help': 'El trabajo seguirá en el servidor y contará en tu cupo.',

  // Diseñar
  'ai.design.title': 'Diseñar con IA',
  'ai.design.desc':
    'Describe tu negocio y la IA propondrá las secciones de la página, con textos listos para editar.',
  'ai.design.prompt_label': 'Describe tu sitio o negocio',
  'ai.design.prompt_placeholder': 'Ej. Tienda de cosmética natural vegana con catálogo y reservas…',
  'ai.design.prompt_help':
    'Cuanto más concreto (qué ofreces, a quién y qué quieres que haga quien visite la web), mejor saldrá la propuesta.',
  'ai.design.ideas_label': 'Ideas para empezar',
  'ai.design.idea_1': 'Tienda gourmet de quesos y vinos artesanos con pedidos online',
  'ai.design.idea_2': 'Estudio de arquitectura y reformas integrales con galería de proyectos',
  'ai.design.idea_3': 'Clínica dental con cita previa, equipo médico y preguntas frecuentes',
  'ai.design.idea_4': 'Restaurante italiano con menú degustación y reserva de mesa',
  'ai.design.style_label': 'Estilo visual',
  'ai.design.style_modern': 'Moderno y limpio',
  'ai.design.style_elegant': 'Elegante y refinado',
  'ai.design.style_bold': 'Audaz y llamativo',
  'ai.design.style_minimal': 'Minimalista',
  'ai.design.lang_label': 'Idioma del contenido',
  'ai.design.button_generate': 'Generar propuesta',
  'ai.design.job_title': 'Diseñando la propuesta',
  'ai.design.result_title': 'Propuesta lista',
  'ai.design.result_sections': {
    one: '{count} sección lista para añadir al lienzo.',
    other: '{count} secciones listas para añadir al lienzo.',
  },
  'ai.design.result_help':
    'Se añadirán a la página que estás editando y podrás ajustarlas después.',
  'ai.design.result_empty': 'La IA no ha devuelto ninguna sección utilizable.',
  'ai.design.button_apply': 'Añadir al lienzo',
  'ai.design.button_again': 'Probar otra idea',

  // Traducir
  'ai.translate.title': 'Traducir con IA',
  'ai.translate.desc': 'Traduce los textos del sitio a otros idiomas sin tocar el diseño.',
  'ai.translate.from_label': 'Idioma de origen',
  'ai.translate.to_label': 'Idiomas de destino',
  'ai.translate.to_help':
    'Elige al menos uno. El idioma de origen no se puede elegir como destino. Al aplicar, los nuevos idiomas se añadirán al sitio.',
  'ai.translate.scope_label': 'Alcance de la traducción',
  'ai.translate.scope_site': 'Todo el sitio',
  'ai.translate.scope_page': 'Página actual',
  'ai.translate.scope_store': 'Solo el catálogo de la tienda',
  'ai.translate.store_unavailable':
    'La traducción del catálogo de la tienda aún no está disponible.',
  'ai.translate.scope_invalid': 'Elige una página existente o todo el sitio para traducir.',
  'ai.translate.overwrite_label': 'Sobrescribir las traducciones que ya existan',
  'ai.translate.button_translate': 'Traducir',
  'ai.translate.job_title': 'Traduciendo los textos',
  'ai.translate.result_title': 'Traducciones listas',
  'ai.translate.result_texts': {
    one: '{count} texto traducido.',
    other: '{count} textos traducidos.',
  },
  'ai.translate.result_help': 'Se añadirán al borrador; el diseño no cambia.',
  'ai.translate.result_empty': 'La IA no ha devuelto ninguna traducción.',
  'ai.translate.button_apply': 'Aplicar traducciones',
  'ai.translate.button_again': 'Traducir a otros idiomas',

  // Exportar
  'ai.export.title': 'Exportar con IA',
  'ai.export.desc':
    'Genera un paquete con tu sitio listo para llevarlo a otra plataforma, sin depender de Sytue.',
  'ai.export.destination_label': 'Plataforma de destino',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Tema completo con plantillas JSON y secciones Liquid, más el catálogo en CSV para importarlo desde el panel de Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Proyecto de una sola página conectado a la API pública de Sytue, con pago y formularios.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Aplicación con renderizado en servidor o estática, con la pasarela de pagos integrada.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Tema de bloques nativo de WordPress, con exportación de productos y atributos.',
  'ai.export.badge_available': 'Disponible',
  'ai.export.badge_soon': 'Próximamente',
  'ai.export.button_export': 'Generar paquete',
  'ai.export.job_title': 'Generando el paquete para {destination}',
  'ai.export.result_title': 'Paquete listo',
  'ai.export.result_help': 'El tema y el CSV del catálogo van juntos en un único ZIP.',
  'ai.export.steps_title': 'Cómo instalarlo en Shopify',
  'ai.export.step_1': 'Descarga el archivo ZIP.',
  'ai.export.step_2':
    'En el panel de Shopify, ve a Tienda online > Temas y elige «Subir archivo zip».',
  'ai.export.step_3':
    'Para los productos, ve a Productos > Importar y sube el archivo products_export_shopify.csv que va dentro del ZIP.',
  'ai.export.button_download': 'Descargar ZIP',
  'ai.export.button_again': 'Generar otra exportación',
};
