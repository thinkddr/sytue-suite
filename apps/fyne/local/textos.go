// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los textos PROPIOS de la edición (Ajustes, nube, IA). Van aquí y no en
// `apps/fyne/catalogos`, que exige las claves de un `en.ts` de la web y esta pantalla no
// tiene web (PLAN §5.14). Fuente: español e inglés, escritos a mano; los otros
// veinticuatro de `i18n.Locales` están en textos_idiomas.go, y `TestLosTextosEstanEnTodosLosIdiomas`
// exige las mismas claves en todos.
var textos = map[string][2]string{ // clave: {es, en}
	"ajustes":                  {"Ajustes", "Settings"},
	"nube":                     {"Nube", "Cloud"},
	"cerrar":                   {"Cerrar", "Close"},
	"guardar":                  {"Guardar", "Save"},
	"cancelar":                 {"Cancelar", "Cancel"},
	"perfil":                   {"Perfil", "Profile"},
	"perfil.nombre":            {"Nombre", "Name"},
	"perfil.correo":            {"Correo", "Email"},
	"perfil.ayuda":             {"Firma tus comentarios, cambios y anotaciones. No sale de este equipo.", "Signs your comments, changes and annotations. It never leaves this device."},
	"apariencia":               {"Apariencia", "Appearance"},
	"idioma":                   {"Idioma", "Language"},
	"tema":                     {"Tema", "Theme"},
	"tema.sistema":             {"Como el sistema", "Same as system"},
	"tema.claro":               {"Claro", "Light"},
	"tema.oscuro":              {"Oscuro", "Dark"},
	"reiniciar":                {"Algunos cambios se ven al volver a abrir la app.", "Some changes apply when you reopen the app."},
	"ia":                       {"Inteligencia artificial", "Artificial intelligence"},
	"ia.proveedor":             {"Proveedor", "Provider"},
	"ia.modelo":                {"Modelo", "Model"},
	"ia.url":                   {"Dirección", "Address"},
	"ia.clave":                 {"Clave de API", "API key"},
	"ia.probar":                {"Probar", "Test"},
	"ia.probando":              {"Probando…", "Testing…"},
	"ia.ok":                    {"Responde correctamente.", "It answers correctly."},
	"ia.ayuda":                 {"Opcional. Apagada, ningún texto sale del equipo. Encendida, el texto sobre el que pidas ayuda se envía solo al proveedor elegido.", "Optional. When off, no text leaves your device. When on, only the text you ask about is sent, and only to the chosen provider."},
	"ia.destino":               {"Se enviará a: ", "Will be sent to: "},
	"ia.sistema.mac":           {"Crea en Atajos un atajo llamado «Sytue Office IA» que reciba texto, lo pase a «Usar modelo» y devuelva el resultado.", "In Shortcuts, create a shortcut named “Sytue Office IA” that takes text, passes it to “Use Model” and returns the result."},
	"ia.sistema.win":           {"Instala Foundry Local (winget install Microsoft.FoundryLocal) y arranca un modelo; en los PC Copilot+ usa la NPU.", "Install Foundry Local (winget install Microsoft.FoundryLocal) and start a model; on Copilot+ PCs it runs on the NPU."},
	"ia.ninguna":               {"Sin IA", "No AI"},
	"ia.local":                 {"Modelo en este equipo (Ollama, LM Studio)", "Model on this device (Ollama, LM Studio)"},
	"ia.otro":                  {"Otro compatible con OpenAI", "Other OpenAI-compatible service"},
	"ia.nombre.mac":            {"Apple Intelligence (por Atajos)", "Apple Intelligence (via Shortcuts)"},
	"ia.nombre.win":            {"IA de Windows en el dispositivo (Foundry Local)", "Windows on-device AI (Foundry Local)"},
	"ia.nombre.sistema":        {"IA del sistema (no disponible en este sistema)", "System AI (not available on this system)"},
	"ia.enEsteEquipo":          {"en este equipo", "on this device"},
	"nube.ayuda":               {"Opcional. Sin cuentas, todo se guarda solo en este equipo.", "Optional. Without accounts, everything stays on this device."},
	"nube.cuentas":             {"Cuentas conectadas", "Connected accounts"},
	"nube.ninguna":             {"Ninguna cuenta conectada.", "No accounts connected."},
	"nube.quitar":              {"Quitar", "Remove"},
	"nube.sytue":               {"Conectar Sytue Drive", "Connect Sytue Drive"},
	"nube.sytue.ayuda":         {"Con una contraseña de aplicación de tu cuenta de Sytue (Sytue ID → Seguridad). La app no inicia sesión con Sytue ID.", "With an app password from your Sytue account (Sytue ID → Security). The app never signs in with Sytue ID."},
	"nube.webdav":              {"Conectar WebDAV (Nextcloud…)", "Connect WebDAV (Nextcloud…)"},
	"nube.google":              {"Conectar Google Drive", "Connect Google Drive"},
	"nube.onedrive":            {"Conectar OneDrive / Microsoft 365", "Connect OneDrive / Microsoft 365"},
	"nube.cliente":             {"Identificador de cliente OAuth", "OAuth client ID"},
	"nube.cliente.ayuda":       {"Esta compilación no trae uno. Registra una app de escritorio en el proveedor y pega aquí su identificador.", "This build has none. Register a desktop app with the provider and paste its client ID here."},
	"nube.navegador":           {"Se abrirá el navegador para que des permiso…", "Your browser will open so you can grant access…"},
	"nube.usuario":             {"Usuario", "User"},
	"nube.contrasena":          {"Contraseña de aplicación", "App password"},
	"nube.sincronizadas":       {"Carpetas que ya sincroniza el sistema", "Folders your system already syncs"},
	"nube.sincronizadas.no":    {"No se ha encontrado ninguna (OneDrive, Google Drive, iCloud Drive, Sytue Drive).", "None found (OneDrive, Google Drive, iCloud Drive, Sytue Drive)."},
	"nube.usar":                {"Guardar mis datos aquí", "Keep my data here"},
	"nube.abrir":               {"Abrir de la nube…", "Open from cloud…"},
	"nube.subir":               {"Subir un archivo del equipo…", "Upload a file from this device…"},
	"nube.guardar":             {"Guardar en la nube…", "Save to cloud…"},
	"nube.guardarAqui":         {"Guardar aquí", "Save here"},
	"nube.atras":               {"Atrás", "Back"},
	"nube.vacia":               {"Carpeta vacía", "Empty folder"},
	"nube.descargando":         {"Descargando…", "Downloading…"},
	"nube.subido":              {"Guardado en la nube.", "Saved to the cloud."},
	"nube.elige":               {"Elige una cuenta o una carpeta", "Choose an account or a folder"},
	"datos":                    {"Datos", "Data"},
	"datos.carpeta":            {"Carpeta de datos", "Data folder"},
	"datos.cambiar":            {"Cambiar…", "Change…"},
	"datos.abrir":              {"Abrir la carpeta", "Open folder"},
	"datos.ayuda":              {"Todo lo tuyo está aquí, en formatos abiertos y sin cifrar. Cópiala para hacer una copia de seguridad.", "Everything of yours is here, in open, unencrypted formats. Copy it to back up."},
	"datos.secretos.si":        {"Las claves y contraseñas están en el llavero del sistema.", "Keys and passwords are in the system keychain."},
	"datos.secretos.no":        {"Este sistema no tiene llavero: las claves están en ficheros solo legibles por ti en la carpeta de ajustes.", "This system has no keychain: keys are stored in files only you can read in the settings folder."},
	"acerca":                   {"Acerca de", "About"},
	"acerca.texto":             {"Sytue Office funciona entera en este equipo: sin cuenta, sin cifrado propio y sin servidores de Sytue. Solo se conecta a la nube y a la IA que tú configures.", "Sytue Office runs entirely on this device: no account, no proprietary encryption and no Sytue servers. It only connects to the cloud and AI you set up."},
	"error":                    {"No se ha podido completar", "Could not complete"},
	"cuentas":                  {"Cuentas", "Accounts"},
	"cuentas.ninguna":          {"Ninguna cuenta externa. Sin cuentas, nada sale de este equipo.", "No external accounts. Without accounts, nothing leaves this device."},
	"cuentas.ok":               {"Conectada", "Connected"},
	"cuentas.reconectar":       {"Hay que volver a entrar", "Sign in again"},
	"cuentas.desde.correo":     {"en Mail", "in Mail"},
	"cuentas.desde.chat":       {"en Rooms", "in Rooms"},
	"cuentas.desde.nube":       {"en Nube", "in Cloud"},
	"servicio.correo":          {"Correo", "Mail"},
	"servicio.calendario":      {"Calendario", "Calendar"},
	"servicio.contactos":       {"Contactos", "Contacts"},
	"servicio.chat":            {"Mensajería", "Messaging"},
	"servicio.reuniones":       {"Reuniones", "Meetings"},
	"servicio.nube":            {"Archivos", "Files"},
	"red":                      {"Red", "Network"},
	"red.ayuda":                {"Sytue Office solo se conecta a lo que tú has configurado. Esta es la lista completa de destinos permitidos ahora mismo, y por qué.", "Sytue Office only connects to what you have set up. This is the complete list of destinations allowed right now, and why."},
	"red.ninguno":              {"Ninguno: ahora mismo esta app no puede conectarse a nada.", "None: right now this app cannot connect to anything."},
	"red.destinos":             {"Destinos permitidos", "Allowed destinations"},
	"red.sinConexion":          {"Sin conexión", "Offline"},
	"red.sinConexion.ayuda":    {"Corta todas las conexiones de todas las apps de Sytue Office. Los modelos de IA de este equipo siguen funcionando.", "Cuts every connection of every Sytue Office app. AI models on this device keep working."},
	"red.sinTLS":               {"sin cifrar (lo marcaste tú)", "unencrypted (you chose this)"},
	"red.temporal":             {"solo mientras dura lo que se ha pedido", "only while what was asked for is in progress"},
	"red.motivo.nube":          {"Cuenta de nube", "Cloud account"},
	"red.motivo.alta":          {"Conectar una cuenta", "Connecting an account"},
	"red.para.correo":          {"Correo", "Mail"},
	"red.para.calendario":      {"Calendario", "Calendar"},
	"red.para.contactos":       {"Contactos", "Contacts"},
	"red.para.chat":            {"Mensajería", "Messaging"},
	"red.para.ia":              {"IA", "AI"},
	"red.para.nube":            {"Nube", "Cloud"},
	"red.para.oauth":           {"Inicio de sesión de la cuenta", "Account sign-in"},
	"red.para.sello-de-tiempo": {"Sello de tiempo", "Timestamp"},
	"red.para.suscripcion-ics": {"Calendario suscrito", "Subscribed calendar"},
	"red.para.imagen-remota":   {"Imágenes remotas", "Remote images"},
	"red.para.baja-de-lista":   {"Darse de baja", "Unsubscribe"},
	"red.para.aeat":            {"Agencia Tributaria", "Spanish Tax Agency"},
	"red.para.descubrimiento":  {"Buscar los servidores de la cuenta", "Finding the account's servers"},
	"red.para.vies":            {"Comprobar un NIF-IVA en VIES", "Checking a VAT number in VIES"},
	"copia":                    {"Copia de seguridad", "Backup"},
	"copia.ayuda":              {"Un ZIP con tu carpeta de datos y tus ajustes, en formatos abiertos. Las claves y contraseñas no van dentro: siguen en el llavero.", "A ZIP with your data folder and settings, in open formats. Keys and passwords are not included: they stay in the keychain."},
	"copia.crear":              {"Crear copia ahora", "Back up now"},
	"copia.creando":            {"Creando la copia…", "Creating the backup…"},
	"copia.hecha":              {"Copia creada.", "Backup created."},
	"copia.lista":              {"Copias en este equipo", "Backups on this device"},
	"copia.ninguna":            {"Aún no hay ninguna copia.", "No backups yet."},
	"copia.restaurar":          {"Restaurar", "Restore"},
	"copia.restaurarOtra":      {"Restaurar desde un fichero…", "Restore from a file…"},
	"copia.restaurar.pregunta": {"Tus datos y ajustes se sustituirán por los de la copia. Antes se guarda una copia de lo que hay ahora. La app se cerrará al terminar.", "Your data and settings will be replaced by those in the backup. A backup of what you have now is made first. The app will close when it finishes."},
	"copia.restaurando":        {"Restaurando…", "Restoring…"},
	"copia.restaurada":         {"Copia restaurada. Vuelve a abrir las apps de Sytue Office.", "Backup restored. Reopen your Sytue Office apps."},
	"copia.previa":             {"Lo que había antes está en:", "What you had before is in:"},
	"copia.salir":              {"Cerrar la app", "Close the app"},
}

// tablas son los textos propios de la cáscara, cada uno con sus traducciones: los de
// Ajustes, la nube y la IA (arriba y textos_idiomas.go), los de las pestañas por app
// (textos_ajustes_apps.go), los de las cuentas de calendario y contactos
// (textos_agenda.go), los de la IA del sistema del móvil (textos_ia.go), los del cifrado
// del disco y mover la carpeta de datos (textos_datos.go) y los de borrar los datos de
// antes de la migración (textos_antes.go). Una clave está en una sola
// (TestCadaClaveEstaEnUnaTabla).
var tablas = []Propios{
	{Base: textos, Traducciones: traducciones},
	{Base: textosAjustesApps, Traducciones: traduccionesAjustesApps},
	{Base: textosAgenda, Traducciones: traduccionesAgenda},
	{Base: textosIA, Traducciones: traduccionesIA},
	{Base: textosDatos, Traducciones: traduccionesDatos},
	{Base: textosAntes, Traducciones: traduccionesAntes},
}

// T es el texto propio de clave en el idioma de la app: español e inglés de su tabla, y
// los otros veinticuatro de sus traducciones; lo que falte cae al inglés.
func (l *App) T(clave string) string { return Texto(l.Idioma(), clave) }

// Texto es T para un idioma cualquiera; una clave que no está en ninguna tabla sale tal cual.
func Texto(idioma i18n.Locale, clave string) string {
	for _, p := range tablas {
		if _, ok := p.Base[clave]; ok {
			return p.En(idioma, clave)
		}
	}
	return clave
}
