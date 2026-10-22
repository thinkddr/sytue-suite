# Mi cuenta

Perfil, seguridad, dispositivos y accesos del propio usuario. Astro 7 + Preact,
SSR para el BFF, sobre la API de Sytue ID.

## Por qué es una app y no una página de Sytue ID

Esta pantalla se servía como HTML desde Go, sin bundler, así que no podía montar
la Sytue Shell: se llegaba a ella y no había selector de aplicaciones ni salida
hacia el resto de la suite.

Replicar el selector en Go significaba duplicar el catálogo de productos
—nombres, iconos, colores de catorce— en un segundo lenguaje. Ese duplicado
diverge a la primera aplicación nueva, así que se movió la página aquí.

Los flujos de Sytue ID que **no** son la cuenta —login, OIDC, SAML— siguen en Go.
Son pantallas sin navegación por diseño y no ganan nada con esto.

## La API ya existía

Sytue ID publica `/api/v1/me*` desde hace tiempo, con sus tests de integración, y
hasta ahora **no tenía consumidor en `apps/`**: el navegador usaba las rutas HTML
y el JSON quedaba para clientes nativos. Su propio código lo dice.

Así que esta app no inventa contrato: consume el que ya está.

| Sección        | API                                                                         |
| -------------- | --------------------------------------------------------------------------- |
| Perfil         | `GET /me`, `PATCH /me` (los siete campos, parcheo parcial)                  |
| Contraseña     | `POST /me/password` (cierra TODAS las sesiones)                             |
| Segundo factor | `POST /me/mfa/totp/{enroll,confirm,recovery-codes}`, `DELETE /me/mfa/totp`  |
| Sesiones       | `GET /me/sessions`, `DELETE /me/sessions/{id}`, `DELETE /me/sessions`       |
| Dispositivos   | `GET /me/devices`, `POST /me/devices/{id}/trust`, `DELETE /me/devices/{id}` |

| Passkeys | `GET /webauthn/credentials`, `DELETE /webauthn/credentials/{id}` |
| Contraseñas de aplicación | `GET`, `POST /me/app-passwords`, `DELETE /me/app-passwords/{id}` |

El **alta** de passkeys no es una llamada más: `navigator.credentials.create()`
abre un diálogo del navegador que habla con el autenticador (Touch ID, una llave
USB, el móvil), así que esa parte vive en `src/lib/passkeys.ts` y el BFF solo
pone el reto antes y verifica la attestation después.

El perfil de configuración de Apple (`.mobileconfig`) es un **enlace** a Sytue ID
y no un proxy: lo sirve exigiendo su COOKIE de sesión, no un bearer, y quien está
en esta pantalla tiene sesión SSO allí por definición. En una navegación
top-level la cookie `SameSite=Lax` viaja, así que el enlace descarga. Proxyarlo
habría obligado a duplicar el generador del plist.

## Direcciones

Canónica la de la organización, `https://<slug>.<dominio>/account`; su subdominio
propio sirve el mismo prefijo. Client OIDC `sytue-account` (migración 0060).

**`id.<dominio>/account` redirige aquí**, al host de organización, que es la
dirección canónica de la suite: mismas cookies, mismo idioma y mismo selector de
aplicaciones. Solo cae al subdominio `account.<dominio>` cuando la sesión todavía
no tiene organización.

La enciende `global.accountAppPath` del chart, y **vacía la apaga**: una
instalación que no despliegue esta superficie y recibiera el 302 dejaría a su
gente sin pantalla de cuenta —y sin forma de cambiar la contraseña—. Por eso es
un valor y no una constante, y por eso el HTML de Go sigue en pie: es lo que
responde donde la app no está, y el único camino de quien tenga esa pestaña
abierta. Retirarlo es otro cambio, y con la app ya sirviéndose.

## Estado

- [x] Armazón con la Shell y el selector de aplicaciones
- [x] Perfil completo: nombre, nombre de pila, apellidos, puesto, departamento,
      ubicación e idioma (el correo en solo lectura)
- [x] Sesiones (cerrar una o todas menos la actual)
- [x] Dispositivos (olvidar cualquiera, marcar de confianza el actual)
- [x] Registrada en el chart y en el despliegue piloto
- [x] Contraseña y segundo factor (TOTP, códigos de recuperación)
- [x] Passkeys: listar y retirar (el alta, con el diálogo WebAuthn, va aparte)
- [x] Contraseñas de aplicación (crear, listar y revocar)
- [x] Perfil de configuración de Apple (enlace al IdP)
- [x] Alta de passkeys (diálogo WebAuthn en el navegador)
- [x] Redirección desde `id.<dominio>/account` (`global.accountAppPath`)
- [ ] Retirada del HTML de Go, cuando la app lleve un tiempo sirviéndose
