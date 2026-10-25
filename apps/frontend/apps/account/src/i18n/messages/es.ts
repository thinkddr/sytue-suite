/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const es: AppCatalog<typeof en> = {
  'app.title': 'Mi cuenta',

  'profile.title': 'Perfil',
  'profile.name': 'Nombre visible',
  'profile.name.hint': 'Cómo apareces en toda la suite.',
  'profile.username': 'Nombre de usuario',
  'profile.username.hint':
    'Con él te encuentran personas de otras organizaciones de Sytue One para escribirte o llamarte.',
  'profile.email': 'Dirección de correo',
  'profile.email.hint': 'Verificada. Cambiarla es otro proceso.',
  'profile.save': 'Guardar cambios',
  'profile.saving': 'Guardando…',
  'profile.saved': 'Cambios guardados.',
  'profile.failed': 'No se han podido guardar los cambios. Inténtalo de nuevo.',
  'profile.avatarChange': 'Cambiar avatar',
  'profile.avatarAdd': 'Añadir avatar',
  'profile.avatarPresets': 'Colores predefinidos',
  'profile.avatarUpload': 'Subir una foto…',
  'profile.avatarRemove': 'Quitar avatar',
  'photo.errorFormat': 'Formato no admitido. Usa JPEG, PNG, WebP o GIF.',
  'photo.errorProcess': 'No se pudo procesar la imagen.',
  'photo.errorTooLarge': 'La imagen es demasiado grande incluso tras optimizarla.',
  'photo.errorRead': 'No se pudo leer la imagen.',

  // --- Sesiones ---
  'sessions.title': 'Sesiones abiertas',
  'sessions.current': 'Este dispositivo',
  'sessions.close_one': 'Cerrar esta sesión',
  'sessions.close_others': 'Cerrar todas las sesiones',
  'sessions.close_others_note': 'Esta incluida: tendrás que volver a entrar.',
  'sessions.empty': 'No hay otras sesiones abiertas.',
  'sessions.failed': 'No se han podido cargar las sesiones.',
  'common.loading': 'Cargando…',

  // --- Dispositivos ---
  'devices.title': 'Dispositivos',
  'devices.current': 'Este dispositivo',
  'devices.trusted': 'De confianza',
  'devices.trust': 'Marcar de confianza',
  'devices.forget': 'Olvidar este dispositivo',
  'devices.empty': 'Todavía no hay dispositivos reconocidos.',
  'devices.failed': 'No se han podido cargar los dispositivos.',

  // --- Seguridad ---
  'security.title': 'Seguridad',
  'security.failed': 'Algo ha fallado. Inténtalo de nuevo.',
  'security.password.title': 'Contraseña',
  'security.password.warning': 'Cambiarla cierra la sesión en todas partes, también aquí.',
  'security.password.current': 'Contraseña actual',
  'security.password.new': 'Contraseña nueva',
  'security.password.submit': 'Cambiar la contraseña',
  'security.password.done':
    'Contraseña cambiada. Se han cerrado todas las sesiones, esta incluida.',
  'security.password.signin_again': 'Volver a entrar',
  'security.mfa.title': 'Verificación en dos pasos',
  'security.mfa.off': 'Sin configurar. Tu contraseña sola abre la cuenta.',
  'security.mfa.required':
    'Esta operación requiere la verificación en dos pasos. Configúrala aquí abajo y vuelve a entrar.',
  'security.mfa.on': 'Activada. Se te pide un código al entrar.',
  'security.mfa.enable': 'Configurar',
  'security.mfa.scan': 'Escanea esto con tu aplicación de autenticación.',
  'security.mfa.qr_alt': 'Código QR de configuración de la verificación en dos pasos',
  'security.mfa.manual': 'O introduce esta clave a mano:',
  'security.mfa.code': 'Código de la aplicación',
  'security.mfa.confirm': 'Activar',
  'security.mfa.disable': 'Desactivar',
  'security.mfa.disable_hint': 'Para desactivarla, escribe un código vigente o tu contraseña.',
  'security.mfa.code_or_password': 'Código de la aplicación',
  'security.mfa.new_codes': 'Códigos de recuperación nuevos',
  'security.codes.title': 'Códigos de recuperación',
  'security.codes.once':
    'Se enseñan una vez. Guárdalos en sitio seguro: cada uno sirve una sola vez.',
  'security.codes.saved': 'Ya los he guardado',
  'common.cancel': 'Cancelar',

  // --- Perfil: el resto de la ficha ---
  'profile.given_name': 'Nombre',
  'profile.family_name': 'Apellidos',
  'profile.job_title': 'Puesto',
  'profile.department': 'Departamento',
  'profile.location': 'Ubicación',
  'profile.locale': 'Idioma',
  'profile.locale.hint': 'Se usa en toda la suite y en los mensajes que te enviamos.',

  // --- Passkeys ---
  'security.passkeys.title': 'Claves de acceso',
  'security.passkeys.remove': 'Quitar',
  'security.passkeys.unnamed': 'Clave sin nombre',

  // --- Contraseñas de aplicación ---
  'app_passwords.title': 'Contraseñas de aplicación',
  'app_passwords.hint':
    'Para las aplicaciones de correo que piden una contraseña. Cada una vale por sí sola y se revoca sin tocar las demás.',
  'app_passwords.label': 'Para qué es',
  'app_passwords.label.hint': 'Por ejemplo: correo del portátil, móvil.',
  'app_passwords.create': 'Crear contraseña',
  'app_passwords.revoke': 'Revocar',
  'app_passwords.empty': 'Todavía no hay contraseñas de aplicación.',
  'app_passwords.failed': 'No se han podido cargar las contraseñas de aplicación.',
  'app_passwords.unnamed': 'Sin nombre',
  'app_passwords.once': 'Se enseña una vez. Cópiala ya en la aplicación: no se puede volver a ver.',
  'app_passwords.saved': 'Ya la he copiado',
  'app_passwords.never_used': 'Sin usar',
  'app_passwords.last_used': 'Usada por última vez el {date}',

  // --- Perfil de configuración de Apple ---
  'profile.apple': 'Configurar Correo y Calendario en dispositivos Apple',
  'profile.apple.hint': 'Descarga un perfil de configuración para iPhone, iPad y Mac.',

  // --- Alta de passkeys ---
  'security.passkeys.hint':
    'Entra con tu huella, tu cara o una llave de seguridad, sin escribir contraseña.',
  'security.passkeys.add': 'Añadir clave de acceso',
  'security.passkeys.adding': 'Esperando a tu dispositivo…',
  'security.passkeys.name': 'Ponle nombre',
  'security.passkeys.name.hint': 'Por ejemplo: este portátil, llave USB.',
  'security.passkeys.duplicada': 'Esa llave ya está registrada en esta cuenta.',
  'security.passkeys.no_soportado': 'Este navegador no admite claves de acceso.',
  'security.passkeys.error': 'No se ha podido añadir la clave. Inténtalo de nuevo.',

  // --- Plan y almacenamiento (cuentas personales) ---
  'plan.title': 'Plan y almacenamiento',
  'plan.loading': 'Cargando tu plan…',
  'plan.free': 'Plan gratuito',
  'plan.pro': 'Plan Pro · 200 GB',
  'plan.enterprise': 'Plan Enterprise · 1 TB · Soporte Premium',
  'plan.storage': 'Espacio usado',
  'plan.externalNote': 'El almacenamiento conectado amplía tu capacidad en Sytue.',
  'plan.externalCapacity': 'El almacenamiento conectado añade {capacity}.',
  'plan.externalUnknown': 'El proveedor no informa de su capacidad.',
  'plan.baseUsage': 'Sytue: {used} de {total}',
  'plan.paidUntil': 'Pagado hasta el',
  'plan.upgrade': 'Ampliar espacio',
  'plan.upgrade.hint':
    '200 GB por 5 €/Usuario/Mes (Pro) o 1 TB con Soporte Premium por 25 €/Usuario/Mes (Enterprise), para tu correo, archivos, fotos y documentos. Lo cancelas cuando quieras.',
  'plan.upgrade.soon': 'Los planes de pago llegarán pronto; de momento no se pueden contratar.',
  'plan.unavailable': 'Los pagos aún no están disponibles en esta instalación.',
  'plan.error': 'No se pudo cargar tu plan. Prueba de nuevo en un momento.',

  // Acceso y recuperación (ADR 0041).
  'recovery.title': 'Acceso y recuperación',
  'recovery.hint':
    'Puedes entrar con tu @usuario, tu dirección de Sytue, tu móvil o tu correo de recuperación, siempre con la misma contraseña. El correo de recuperación recibe también el enlace para restablecerla.',
  'recovery.phone': 'Móvil',
  'recovery.phone.hint': 'Verificado por SMS al crear la cuenta.',
  'recovery.email': 'Correo de recuperación',
  'recovery.new': 'Nuevo correo de recuperación',
  'recovery.send': 'Enviar enlace de confirmación',
  'recovery.sent': 'Enlace enviado. Ábrelo desde esa dirección para confirmarla.',
  'recovery.remove': 'Quitar',
  'recovery.removed': 'Correo de recuperación quitado.',
  'recovery.invalid': 'Esa dirección no sirve: tiene que ser una dirección válida fuera de Sytue.',
  'recovery.wrong_password': 'La contraseña no es correcta.',
  'recovery.failed': 'No se ha podido guardar. Inténtalo de nuevo en un momento.',
  'delete.title': 'Borrar mi cuenta',
  'delete.hint':
    'Se borran tus credenciales y lo que es solo tuyo; lo que es de tu organización se queda en ella. Durante 30 días se puede recuperar.',
  'delete.link': 'Ver qué se borra y continuar',
};
