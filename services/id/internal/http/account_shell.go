// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Shell visual de «Mi cuenta». Existe aparte de loginPage porque las dos
// pantallas tienen formas opuestas: el login es una tarjeta estrecha con una
// sola decisión, y la cuenta es un panel con varias áreas independientes
// —perfil, seguridad, dispositivos, accesos—. Servir la segunda dentro del
// contenedor de 384 px de la primera era lo que la convertía en una columna
// interminable donde todo pesaba igual y nada se encontraba.
//
// Sigue sin dependencias externas ni framework: HTML servido y CSS propio, con
// el mismo vocabulario visual que el resto de la suite (marfil, grafito,
// acento latón, tipografía del sistema).
package http

import (
	"html"
	"strings"
)

// accountShellCSS es el estilo del panel de cuenta. Contenido puro (sin
// <style>): su sha256 entra en la CSP (ver cspStyleHashes en middleware).
//
// Nota sobre el ancho: `columns: 24rem 2` deja que el número de columnas lo
// decida el espacio disponible y no un breakpoint fijo. En un portátil son dos;
// en un móvil, una; y no hay ningún tamaño intermedio en el que las tarjetas
// queden ni apretadas ni desparramadas.
const accountShellCSS = `body.acc{display:block;place-items:normal;padding:0;background-color:var(--bg)}` +
	`.acc-top{border-bottom:1px solid var(--border);background:var(--surface)}` +
	`.acc-top-in{max-width:960px;margin:0 auto;padding:14px 24px;display:flex;align-items:center;gap:12px}` +
	`.acc-top .brand{margin-right:auto}` +
	`.acc-wrap{max-width:960px;margin:0 auto;padding:32px 24px 56px}` +
	`.acc-id{display:flex;align-items:center;gap:16px;margin:0 0 28px}` +
	`.acc-av{flex:none;width:56px;height:56px;border-radius:50%;display:grid;place-items:center;` +
	`background:var(--brand);color:#fff;font-size:22px;font-weight:600;letter-spacing:-.01em;text-transform:uppercase}` +
	`.acc-id h1{margin:0;font-size:22px;line-height:1.25;letter-spacing:-.02em}` +
	`.acc-id .acc-mail{margin:2px 0 0;color:var(--muted);font-size:14px;overflow-wrap:anywhere}` +
	// Multi-columna en vez de rejilla: las tarjetas tienen alturas muy distintas
	// —«Perfil» es un formulario largo y «Tu cuenta» son cuatro filas— y una
	// rejilla alinea por filas, así que la tarjeta baja de cada fila dejaba un
	// hueco del tamaño de la más alta. Con columnas, cada una fluye hasta donde
	// llega y el empaquetado no deja aire muerto.
	//
	// El precio es que el orden de lectura pasa a ser por columnas (toda la
	// izquierda y luego la derecha) en vez de por filas. Se asume porque las
	// tarjetas son independientes entre sí: no hay una secuencia que seguir.
	`.acc-grid{columns:24rem 2;column-gap:16px}` +
	`.card2{background:var(--surface);border:1px solid var(--border);border-radius:var(--r);` +
	`padding:20px 22px;min-width:0;margin:0 0 16px;break-inside:avoid}` +
	`.card2 h2{margin:0;font-size:15px;font-weight:600;letter-spacing:-.01em;display:flex;align-items:center;gap:8px}` +
	`.card2 h2 svg{width:16px;height:16px;flex:none;color:var(--muted)}` +
	`.card2 .hint{margin:4px 0 0;color:var(--muted);font-size:13px;line-height:1.5}` +
	`.acc-foot{display:flex;flex-wrap:wrap;gap:8px;margin-top:24px;padding-top:20px;border-top:1px solid var(--border)}` +
	`@media(max-width:560px){.acc-wrap{padding:24px 16px 40px}.acc-top-in{padding:12px 16px}` +
	`.acc-id{gap:12px}.acc-av{width:44px;height:44px;font-size:17px}.acc-id h1{font-size:19px}}`

// accountPage envuelve el panel de cuenta en el shell ancho, con la barra de
// marca arriba y la identidad del usuario como encabezado.
//
// `name` y `email` llegan SIN escapar y se escapan aquí, para que ningún
// llamante pueda olvidarlo con datos que el propio usuario controla.
func accountPage(loc, title, name, email, body string) string {
	esc := html.EscapeString
	// La inicial del avatar sale del nombre y, si está vacío, del correo. Se toma
	// por runa y no por byte: en un nombre que empieza por «Á» o «Ø», ua[:1]
	// partiría el carácter UTF-8 por la mitad y pintaría basura.
	inicial := "?"
	for _, src := range []string{name, email} {
		if t := strings.TrimSpace(src); t != "" {
			inicial = string([]rune(t)[:1])
			break
		}
	}
	return `<!doctype html><html data-theme="light" lang="` + esc(loc) + `"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<link rel="icon" href="/favicon.svg" type="image/svg+xml">` +
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png">` +
		`<meta name="color-scheme" content="light">` +
		`<meta name="robots" content="noindex"><title>` + esc(title) + ` · Sytue ID</title>` +
		// Cada hoja va en su propio <style> porque la CSP autoriza el hash de cada
		// bloque por separado; concatenarlas daría un bloque cuyo hash no está en
		// la lista y el navegador se quedaría con la página sin estilos.
		`<style>` + loginCSS + `</style><script>` + themeInitJS + `</script>` +
		`<style>` + accountShellCSS + `</style>` +
		`<style>` + accountCSS + `</style>` +
		`<style>` + securityCSS + `</style>` +
		`<style>` + sessionsCSS + `</style>` +
		`<style>` + profileCSS + `</style>` +
		`<style>` + passkeysCSS + `</style>` +
		`</head><body class="acc"><header class="acc-top"><div class="acc-top-in">` +
		brandHTML() +
		`</div></header><main class="acc-wrap">` +
		`<div class="acc-id"><div class="acc-av" aria-hidden="true">` + esc(inicial) + `</div>` +
		`<div><h1>` + esc(nonEmpty(name, email)) + `</h1>` +
		`<p class="acc-mail">` + esc(email) + `</p></div></div>` +
		body +
		`</main><script>` + passkeysScript + `</script></body></html>`
}

// brandHTML es la marca Sytue ID: la misma que el login, extraída para que las
// dos pantallas no se separen visualmente por un cambio en una sola de ellas.
func brandHTML() string {
	return `<div class="brand"><span class="logo"><svg viewBox="0 0 40 40" aria-hidden="true">` +
		`<defs><clipPath id="m"><rect width="40" height="40" rx="9"/></clipPath></defs>` +
		`<g clip-path="url(#m)"><rect width="40" height="40" fill="#0A0A0A"/>` +
		`<polygon points="40,14 40,40 14,40" fill="#b98f4f"/></g></svg></span>Sytue<span class="tag">ID</span></div>`
}

// nonEmpty devuelve el primero no vacío (nombre visible, si no el correo).
func nonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// cardHTML es una tarjeta del panel: título con icono y contenido.
func cardHTML(id, icon, title, body string) string {
	return `<section class="card2" id="` + id + `"><h2>` + icon +
		html.EscapeString(title) + `</h2>` + body + `</section>`
}

// Iconos de las tarjetas: trazo de 1.5 sobre rejilla de 16, como el resto de la
// suite. aria-hidden porque el título de al lado ya nombra la sección.
const (
	iconUser = `<svg viewBox="0 0 16 16" fill="none" aria-hidden="true">` +
		`<circle cx="8" cy="5.25" r="2.75" stroke="currentColor" stroke-width="1.5"/>` +
		`<path d="M2.75 13.75c.6-2.5 2.7-3.75 5.25-3.75s4.65 1.25 5.25 3.75" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>`
	iconShield = `<svg viewBox="0 0 16 16" fill="none" aria-hidden="true">` +
		`<path d="M8 1.75l5 1.9v4.2c0 3-2.05 5.35-5 6.4-2.95-1.05-5-3.4-5-6.4v-4.2l5-1.9z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/></svg>`
	iconKey = `<svg viewBox="0 0 16 16" fill="none" aria-hidden="true">` +
		`<circle cx="5.5" cy="8" r="2.75" stroke="currentColor" stroke-width="1.5"/>` +
		`<path d="M8.25 8H14m-2 0v2.25M10.25 8v1.75" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>`
	iconDevice = `<svg viewBox="0 0 16 16" fill="none" aria-hidden="true">` +
		`<rect x="1.75" y="2.75" width="12.5" height="8" rx="1.25" stroke="currentColor" stroke-width="1.5"/>` +
		`<path d="M5.75 13.25h4.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>`
	iconClock = `<svg viewBox="0 0 16 16" fill="none" aria-hidden="true">` +
		`<circle cx="8" cy="8" r="6.25" stroke="currentColor" stroke-width="1.5"/>` +
		`<path d="M8 4.5V8l2.25 1.75" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>`
	iconGrid = `<svg viewBox="0 0 16 16" fill="none" aria-hidden="true">` +
		`<rect x="2.25" y="2.25" width="4.75" height="4.75" rx="1" stroke="currentColor" stroke-width="1.5"/>` +
		`<rect x="9" y="2.25" width="4.75" height="4.75" rx="1" stroke="currentColor" stroke-width="1.5"/>` +
		`<rect x="2.25" y="9" width="4.75" height="4.75" rx="1" stroke="currentColor" stroke-width="1.5"/>` +
		`<rect x="9" y="9" width="4.75" height="4.75" rx="1" stroke="currentColor" stroke-width="1.5"/></svg>`
)
