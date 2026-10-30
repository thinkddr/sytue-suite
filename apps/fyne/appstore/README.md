# App Store: las fichas de las quince apps

Todo lo que App Store Connect pide de cada app —iOS y macOS— y se puede tener en el
repositorio. Lo sube `apps/fyne/scripts/appstore-publicar.py` desde el workflow
`appstore-fichas` de Codemagic, que encarga `appstore-publicar.yml`; el porqué y el
funcionamiento, en «App Store: ficha y envío por la API» de `docs/operations.md`. Es el
gemelo de [`apps/fyne/play/`](../play/README.md) y sale del mismo generador.

```
ficha.json                                 lo común y, por app, Apple ID, categorías y notas
<app>/<locale>/name.txt                    ≤ 30 caracteres (el Name de FyneApp.toml)
<app>/<locale>/subtitle.txt                ≤ 30; opcional (no está donde nada cabe)
<app>/<locale>/promotional_text.txt        ≤ 170
<app>/<locale>/description.txt             ≤ 4000 (la misma de Play)
<app>/<locale>/keywords.txt                ≤ 100, con sus comas
<app>/capturas/iphone-N.png                1179 × 2556, RGB sin alfa, de 1 a 10
<app>/capturas/ipad-N.png                  2048 × 2732
<app>/capturas/mac-N.png                   2880 × 1800
<app>/privacidad.txt                       las etiquetas de «App Privacy», para marcar en la web
```

`<app>` es el directorio de `apps/fyne/cmd/`; el bundle id sale de su `FyneApp.toml`
(`eu.sytue.<app>`, y `eu.sytue.one` para el Hub) y el guion comprueba que el Apple ID de
`ficha.json` es de ese bundle antes de escribir nada.

## Cómo se regenera

- **Textos y `privacidad.txt`**: `bun apps/fyne/play/generar.ts` desde la raíz, el mismo
  que escribe la ficha de Play. La descripción es la de Play; el texto promocional, su
  descripción corta; el subtítulo, lo que el Hub pone bajo el nombre de la app
  (`app.<clave>.description` del catálogo común) si cabe en 30, y si no la corta de Play, y
  si tampoco, **no hay subtítulo en ese idioma** (51 de 285; antes que recortar una frase).
  Las palabras clave son las siglas y formatos que el catálogo nombra en ese producto más
  las palabras de su descripción breve en ese idioma. **DECISIÓN**: no las ha escrito nadie
  pensando en la búsqueda de la tienda; son correctas y pobres.
- **19 idiomas, no 26**: la App Store no tiene búlgaro, estonio, irlandés, islandés, letón,
  lituano ni maltés ([App Store localizations](https://developer.apple.com/help/app-store-connect/reference/app-store-localizations);
  el esloveno, `sl-SI`, entró el 2026-03-31). Los locales son los de la API: `cs da de-DE el
en-US es-ES fi fr-FR hr hu it nl-NL no pl pt-PT ro sk sl-SI sv`. El principal es `es-ES`:
  las capturas solo se suben a él y la tienda las enseña en los demás.
- **Capturas**: el recorrido de cada app (`TestRecorridoConSesion`) contra su servicio
  falso, con `SYTUE_CAPTURAS_PLAY=<dir>` y la medida en `SYTUE_CAPTURAS_MEDIDA`. La vía
  habitual es el CI; se reparte por clase para revisar cada colección por separado:

  ```sh
  gh workflow run ci-desktop.yml --ref <rama> -F empaquetado_solo=true \
    -F captura_fichas=true -f apps_fichas=todas -f clase_fichas=iphone
  ```

  `clase_fichas` admite `iphone`, `ipad` y `mac` (también `android`, para Play), y
  `apps_fichas` una lista de apps separadas por espacios o `todas`. El CI importa la
  selección mediante `capturas.sh --seleccion <clase> <app>`, guarda el índice completo
  del recorrido y conserva solo los PNG elegidos. Si GitHub rechaza el artefacto por la
  cuota de almacén, los mismos PNG se conservan en bloques base64 identificados en el
  log. Una captura ausente sigue haciendo fallar el trabajo. Para un recorrido hecho en
  otro entorno remoto, `capturas.sh <dir>` copia las elegidas y, si hay ImageMagick, las
  recomprime sin pérdida.

  | Clase    | Medida del recorrido | Píxeles     | Tipo de la API          | Por qué esa                                                                   |
  | -------- | -------------------- | ----------- | ----------------------- | ----------------------------------------------------------------------------- |
  | `iphone` | `393x852@3`          | 1179 × 2556 | `APP_IPHONE_61`         | «iPhone with Dynamic Island (medium display)»: la única de iPhone obligatoria |
  | `ipad`   | `1024x1366@2`        | 2048 × 2732 | `APP_IPAD_PRO_3GEN_129` | iPad de 13": obligatoria porque las apps declaran `UIDeviceFamily` 1 **y 2**  |
  | `mac`    | `1440x900@2`         | 2880 × 1800 | `APP_DESKTOP`           | Mac, 16:10: obligatoria para la ficha de macOS                                |

  Las medidas obligatorias son las de [Screenshot specifications](https://developer.apple.com/help/app-store-connect/reference/screenshot-specifications)
  leídas el 2026-10-06; las demás pantallas las escala App Store Connect. **Lo que estas
  capturas no son**: no salen de un iPhone, un iPad ni un Mac, sino del driver de pruebas de
  Fyne a esa medida. No llevan la barra de estado, la isla ni la barra de título de la
  ventana de macOS, y la tipografía es la que embebe la app, no la del sistema. Apple pide
  que las capturas enseñen la app en uso, y esto lo es; si App Review las objeta, hay que
  sacarlas de un dispositivo o de un simulador en un Mac.

  > **Revisión visual completada el 2026-10-14: 125 PNG de las quince apps**, 50 de
  > iPhone, 38 de iPad y 37 de Mac. Se regeneraron y se miraron las tres colecciones:
  > [iPhone](https://github.com/thinkddr/sytue-one/actions/runs/38607261142),
  > [iPad](https://github.com/thinkddr/sytue-one/actions/runs/38607260811) y
  > [Mac](https://github.com/thinkddr/sytue-one/actions/runs/38607262007). Rooms y Drive
  > se sustituyeron después por las de
  > [la regeneración dirigida](https://github.com/thinkddr/sytue-one/actions/runs/38613047745),
  > también revisadas: reacciones y selector con emojis de color, y acciones de selección
  > completas sobre la lista. Las de CertiPDF muestran páginas dibujadas con un PDF
  > válido y el render de prueba, sin el aviso de fallo de dibujo. Se excluyeron
  > estados de error o carga del servicio falso, texto introducido por el recorrido y
  > la cuarta captura de Cast en Mac, que correspondía a una vista con miniaturas aún
  > sin cargar. La selección final está en `capturas.sh`.
  >
  > Esta revisión comprueba los PNG del driver de software y los estados elegidos para
  > la ficha. Sigue pendiente la validación en dispositivos físicos; no acredita el
  > funcionamiento completo de las quince apps en iOS o macOS.

## Lo que hace la API y lo que no

| Paso                                                                       | ¿Por la API?                                                                | Quién                                               |
| -------------------------------------------------------------------------- | --------------------------------------------------------------------------- | --------------------------------------------------- |
| Crear el App ID y la ficha de la app                                       | No (el alta de apps no está en la API)                                      | Hecho por el dueño el 2026-10-06, las quince        |
| Nombre, subtítulo y URL de privacidad, por idioma                          | Sí, `appInfoLocalizations`                                                  | `--ficha`                                           |
| Categoría principal y secundaria                                           | Sí, `appInfos`                                                              | `--ficha`                                           |
| Clasificación por edades                                                   | Sí, `ageRatingDeclarations`                                                 | `--ficha`                                           |
| Derechos de contenido («no usa contenido de terceros»)                     | Sí, `apps.contentRightsDeclaration`                                         | `--ficha`                                           |
| Descripción, palabras clave, texto promocional, URL de soporte y marketing | Sí, `appStoreVersionLocalizations`                                          | `--ficha`                                           |
| Copyright, número de versión y publicación tras aprobar                    | Sí, `appStoreVersions`                                                      | `--ficha`                                           |
| Capturas de iPhone, iPad y Mac                                             | Sí, `appScreenshotSets` + `appScreenshots`                                  | `--ficha`                                           |
| Notas para la revisión                                                     | Sí, `appStoreReviewDetails`                                                 | `--ficha` o sólo `--notas`                          |
| Contacto y **cuenta de prueba** para el revisor                            | Sí, pero **no están en el repositorio**                                     | El dueño: variables en Codemagic, o en la web       |
| Precio gratis                                                              | Sí, `appPriceSchedules` (solo si no hay ninguno puesto)                     | `--ficha`                                           |
| Países (UE + EEE + CH, sin Francia: veintinueve)                           | Sí, `appAvailabilities` v2 (solo si no hay ninguno puesto)                  | `--ficha`                                           |
| **Etiquetas de privacidad («App Privacy»)**                                | **No**: la API no las tiene                                                 | El dueño, con `<app>/privacidad.txt`                |
| **Cifrado (cumplimiento de exportación)**                                  | Cuestionario por API autorizado; exención documental en el plist: ver abajo | Pipeline y revisión de los hechos                   |
| Estado de comerciante de la DSA                                            | No                                                                          | Hecho por el dueño                                  |
| Acuerdos, fiscalidad y banca                                               | No                                                                          | El dueño (las apps son gratuitas: basta el acuerdo) |
| Subir el build                                                             | Sí, pero no es de este guion                                                | Codemagic `ios-fyne` (y el `.pkg` de macOS)         |
| Atar el build y **enviar a revisión**                                      | Sí, `reviewSubmissions`                                                     | `--revision`, con la frase de confirmación          |
| Publicar tras la aprobación                                                | Automático (`releaseType: AFTER_APPROVAL`)                                  | —                                                   |

Las fichas de las quince apps y sus dos plataformas se actualizaron por la API el
2026-10-14. Esto no equivale a envío a revisión ni publicación: siguen siendo necesarias
la cuenta del revisor, las etiquetas de privacidad y la información de cifrado del build.
El contrato se verifica además contra una API de prueba
(`deploy/production/tests/appstore-publicar-test.sh`).

## `ficha.json`

- **URL**: soporte y marketing `https://sytue.eu`; privacidad `https://sytue.eu/privacidad`.
- **Copyright**: `2026 Sytue Projects, S.L.` (Apple antepone el ©).
- **Categoría** (DECISIÓN; el encargo pedía Productividad o Negocios):

  | App                                                                                    | Principal     | Secundaria    | Por qué                                                                          |
  | -------------------------------------------------------------------------------------- | ------------- | ------------- | -------------------------------------------------------------------------------- |
  | Hub, Mail, Calendar, Contacts, Drive, Docus, Calcus, Diapos, CertiPDF, Notepads, Bases | Productividad | Negocios      | Herramientas de trabajo de una persona; es donde están sus equivalentes          |
  | Photos                                                                                 | Productividad | Foto y vídeo  | Es la galería de la organización, no un editor de fotos de consumo               |
  | Rooms                                                                                  | Negocios      | Productividad | Mensajería y reuniones de equipo: en la tienda, las de trabajo están en Negocios |
  | Cast                                                                                   | Negocios      | Foto y vídeo  | Vídeo y directos de una organización, no una red de vídeo abierta                |
  | ERP                                                                                    | Negocios      | Finanzas      | Contabilidad y facturación de una empresa                                        |

  En macOS, `empaquetar.sh` escribe `LSApplicationCategoryType = productivity` en todas; la
  categoría de la tienda es la de App Store Connect, y si no coinciden la subida avisa. Para
  Rooms, Cast y ERP habría que escribir `public.app-category.business` (es del empaquetado,
  que aquí no se toca).

- **Publicación**: `AFTER_APPROVAL` —sale a la tienda en cuanto Apple la aprueba—, como en
  Play. Para publicarla a mano tras la aprobación, `"publicacion": "MANUAL"` en `ficha.json`.
- **Notas para la revisión**: en inglés, comunes, más un párrafo propio en Rooms (dónde
  están bloquear y denunciar) y en Cast.
- **Cuenta de prueba**: `revision.cuenta_de_prueba` es `true` y **el campo va vacío**. La
  pone el dueño, con una cuenta suya de una organización de demostración, **sin segundo
  factor ni passkey obligatoria**, como variables secretas del grupo `apple-firma` de
  Codemagic: `APPSTORE_DEMO_USUARIO`, `APPSTORE_DEMO_CLAVE`, `APPSTORE_CONTACTO_NOMBRE`,
  `APPSTORE_CONTACTO_APELLIDOS`, `APPSTORE_CONTACTO_TELEFONO` y `APPSTORE_CONTACTO_CORREO`.
  O las escribe en la web (_App Review Information_), treinta veces. Sin ellas `--ficha`
  no toca esos campos y `--revision` se niega. `--seco` es rojo si algo con pinta de
  credencial aparece en `ficha.json`.

Las notas de revisión se componen en un único sitio: `revision.notas`, las
`notas` de cada app y, al final, `revision.notas_por_plataforma[IOS|MAC_OS]`.
La justificación del listener OIDC de escritorio vive en `MAC_OS`, de modo que
`--ficha` la conserva al regenerar las notas y no la envía a iOS. El límite de
4000 bytes UTF-8 incluye el anexo y los separadores de cada plataforma.
Credenciales y contactos siguen en los campos privados de App Store Connect.
El vídeo de un dispositivo físico se incorpora sólo cuando exista: las notas
del repositorio no afirman que se haya grabado ni contienen enlaces de ejemplo.

Para reparar sólo esas notas, `--notas --version X.Y.Z` exige la versión exacta ya
existente y editable, con su detalle de revisión ya creado. Primero comprueba todos
los destinos seleccionados (por defecto las quince apps en iOS y macOS) y después
hace únicamente `PATCH` de `notes`, sin cambiar contactos, cuenta de prueba, builds,
versiones ni envíos. Recomprueba el destino antes de escribir y relee las notas para
confirmar su igualdad; el recibo muestra sólo el resultado y su SHA-256. Un rechazo
anterior pendiente no se cancela ni se reenvía. En el workflow manual se elige
`modo=notas`, `ref=vX.Y.Z` y, opcionalmente, apps y plataformas.

## Clasificación por edades, pregunta a pregunta

Va por la API (`edades` en `ficha.json`). Coherente con lo declarado a Google
(`../play/README.md`, «Clasificación de contenido»): sale **4+**, y **18+ en Cast** (abajo).

| Pregunta de App Store Connect                                                     | Respuesta                                                            | Clave de la API                                                                                                      |
| --------------------------------------------------------------------------------- | -------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Controles en la app: control parental, verificación de edad                       | Ninguno                                                              | `parentalControls`, `ageAssurance` = `false`                                                                         |
| Acceso web sin restricciones                                                      | No (el login abre el navegador del sistema; no hay navegador dentro) | `unrestrictedWebAccess` = `false`                                                                                    |
| **Contenido generado por usuarios**                                               | **Sí**, en las quince: todas comparten dentro de la organización     | `userGeneratedContent` = `true`                                                                                      |
| Redes sociales                                                                    | No                                                                   | `socialMedia`, `socialMediaAgeRestricted` = `false`                                                                  |
| **Mensajería y chat**                                                             | **Sí** en Mail, Rooms y Cast; no en las otras doce                   | `messagingAndChat`                                                                                                   |
| Publicidad                                                                        | No                                                                   | `advertising` = `false`                                                                                              |
| Blasfemias o humor grosero; terror o miedo; alcohol, tabaco o drogas              | Ninguno                                                              | `profanityOrCrudeHumor`, `horrorOrFearThemes`, `alcoholTobaccoOrDrugUseOrReferences`                                 |
| Información médica o de tratamientos; salud o bienestar                           | Ninguno; No                                                          | `medicalOrTreatmentInformation`, `healthOrWellnessTopics`                                                            |
| Temas adultos o sugerentes; contenido sexual o desnudos; contenido sexual gráfico | Ninguno                                                              | `matureOrSuggestiveThemes`, `sexualContentOrNudity`, `sexualContentGraphicAndNudity`                                 |
| Violencia de dibujos o fantasía; realista; realista prolongada o sádica; armas    | Ninguno                                                              | `violenceCartoonOrFantasy`, `violenceRealistic`, `violenceRealisticProlongedGraphicOrSadistic`, `gunsOrOtherWeapons` |
| Apuestas; apuestas simuladas; concursos; cajas de botín                           | No; ninguno; ninguno; No                                             | `gambling`, `gamblingSimulated`, `contests`, `lootBox`                                                               |
| ¿Hecha para niños?                                                                | No (no se manda `kidsAgeBand`)                                       | —                                                                                                                    |

- **DECISIÓN**: el público objetivo declarado a Google es 18+ y aquí la clasificación sale
  4+: no se contradicen (una es a quién se dirige, la otra qué contiene), pero si se quiere
  subir la edad en la App Store existe `ageRatingOverrideV2`; solo se usa en Cast (abajo).
- **Contenido generado por usuarios y la directriz 1.2** (filtrar, denunciar, bloquear y
  contacto publicado). **Rooms** ([ADR 0073](../../../docs/decisions.md#0073)) y **Cast**
  ([ADR 0074](../../../docs/decisions.md#0074), app nativa desde #566) lo tienen, y sus notas
  para la revisión dicen dónde está cada cosa. Condición para enviarlas, la misma que en
  Play: que el build que se envía **y** el servicio desplegado lleven los dos la función.
- **Cast, 18+** (DECISIÓN, `ageRatingOverrideV2: EIGHTEEN_PLUS` solo en Cast). A Google se
  le declaró que en Cast los usuarios **sí** pueden compartir públicamente desnudos o
  violencia (decisión del dueño del 2026-10-06, `../play/README.md`): nada lo impide y un
  vídeo llega a la portada sin revisión previa. El cuestionario de Apple pregunta por lo que
  la app trae, no por lo que pueden subir otros, así que sus respuestas siguen en «ninguno»
  y la coherencia con Google se da subiendo la edad, que es lo que el override existe para
  hacer. Si el dueño cumple lo que el README de Play pide para responder «No» a Google,
  se quita el override aquí en el mismo cambio. Las otras catorce, 4+.

## Etiquetas de privacidad («App Privacy»), pregunta a pregunta

**No van por la API**: se marcan en la web, en _App Privacy_ de cada app, con
`<app>/privacidad.txt`, que el generador saca de los **mismos datos** que el
`seguridad-datos.csv` de Play. Lo declarado a Apple y a Google no puede divergir.

- **¿Recoge datos esta app?** Sí, en las quince.
- **¿Se usan para seguimiento?** No, ninguno: no hay SDK de terceros, anuncios, analítica
  ni intermediarios de datos (inventario del 2026-09-14).
- Cada tipo de dato: finalidad **App Functionality** y nada más (Apple no tiene «gestión de
  la cuenta»: autenticar es funcionalidad); **vinculado a la identidad: sí** (todo cuelga
  de la cuenta); **seguimiento: no**.
- Diferencias de forma con Play, que no son de fondo: el historial de búsqueda, que en
  Play va como «efímero», aquí **no se declara** (para Apple «recoger» es guardar más de lo
  que dura atender la petición); calendario, ficheros y contenido creado van a «Other User
  Content»; fotos y vídeos son un solo tipo; correos y otros mensajes, también.
- **URL de la política de privacidad**: `https://sytue.eu/privacidad` (esta sí va por la API).
- Eliminación de la cuenta (directriz 5.1.1(v)): `https://id.sytue.eu/account/delete`, la
  misma que se dio a Google. Tiene que responder antes de enviar a revisión.

## Cifrado (cumplimiento de exportación)

**Declaración autorizada por el dueño el 2026-10-14.** Las respuestas del cuestionario
y las declaraciones de _Distribución → Información de la app
→ Documentación de cifrado de la app_ pueden registrarse por la API con esa autorización.
Esto sustituye la decisión anterior de dejar el cuestionario a una persona en cada build.

**Cifrado estándar sin distribución en Francia: exento de documentación.** El dueño
decidió no distribuir de momento las quince apps en Francia, sin tramitar la declaración
francesa de ANSSI. Con los algoritmos estándar de estas apps y sin algoritmos propietarios,
la [tabla de Apple](https://developer.apple.com/help/app-store-connect/reference/app-information/export-compliance-documentation-for-encryption)
solo exige la declaración francesa cuando se distribuye en Francia. Por eso los plists
de iOS y macOS llevan **`ITSAppUsesNonExemptEncryption=false`**, sin
`ITSEncryptionExportComplianceCode`: significa exento de documentación, **no que no haya
cifrado**, y evita repetir el cuestionario en futuras subidas según el
[procedimiento de Apple](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation).
Hay que revisar esta decisión antes de habilitar Francia o incorporar algoritmos
propietarios. El guardián comprueba la exclusión de `FRA` con la lista del emisor de fichas.

Como antecedente, el piloto de Calendar del 2026-10-14 con
`ITSAppUsesNonExemptEncryption=true` y sin código archivó para iOS y macOS, pero Apple
rechazó ambas subidas con **90592: Invalid Export Compliance Code**. Las quince
declaraciones creadas cuando Francia estaba habilitada permanecen como historial en
`CREATED`, con `codeValue=null`; no acreditan aprobación y no se necesitan para esta
exención documental.

Si Apple requiere documentación en el futuro, tras aprobarla emite el código de esa app.
Se configura **solo en el workflow de esa app** como
`SYTUE_APP_ENCRYPTION_EXPORT_CODE`; `ci_post_clone.sh` añade entonces
`ITSAppUsesNonExemptEncryption=true` junto a `ITSEncryptionExportComplianceCode` en
los dos plists compartidos de iOS y macOS. Sin valor, conserva la exención documental.
El post-clone rechaza metadatos ausentes, tipos incorrectos, `false` con código o
`true` sin código válido antes de instalar Go. No comprueba
la aprobación contra Apple: quien configure la variable debe usar el código de la
declaración **APPROVED de esa app**, nunca uno de otra app, un valor ficticio o uno pendiente.
Crear una `appEncryptionDeclaration` en estado `CREATED` no equivale a `APPROVED` ni
la deja vinculada automáticamente al build.

La ficha comercial sigue sin ser la fuente del cifrado: `--seco` es rojo si la clave aparece
en `ficha.json`, y `appstore-publicar.py --revision` se niega a enviar un build sin la
información de cifrado. Antes de darlo por disponible se comprueban el estado de
TestFlight y su pertenencia al grupo; `true` no acredita que Apple haya aprobado la
documentación para enviar la app a revisión.

Para builds anteriores cuyo indicador siga en `null`, el modo manual
`cifrado-exento` del workflow puede completar únicamente `null` → `false`, con
`confirmacion_cifrado=ESTANDAR_EXENTO_SIN_FRANCIA` y el tag explícito. Reutiliza el
[PATCH de builds de Apple](https://developer.apple.com/documentation/appstoreconnectapi/patch-v1-builds-_id_).
Antes del primer PATCH comprueba todas las apps solicitadas: bundle, build válido y
vigente, ausencia de revisión activa y declaración vinculada, Francia desactivada en la
disponibilidad real y territorios nuevos desactivados. Relee los cambios antes de escribir
y exige un GET posterior con `false`. Una respuesta ya `false` no se reescribe; `true`, un
campo ilegible o una declaración existente detienen la operación. No crea documentos,
cambia países ni manda a revisión. Un fallo posterior se comunica sin revertir los builds
ya confirmados; la API no ofrece una transacción entre apps.

Los hechos, para responderlo con verdad:

- Las **quince** hablan con el servidor por TLS con la implementación **de Go**
  (`crypto/tls`), no con la del sistema operativo de Apple.
- **Notepads** cifra cuadernos en el cliente con **AES-256-GCM**.
- **Rooms y Cast** llevan **DTLS-SRTP** propio para la voz y el vídeo.
- Ninguna usa algoritmos propios ni no estándar.

| Pregunta                                                                                                    | Respuesta veraz                                                                                          |
| ----------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| ¿La app usa cifrado (lo implementa, lo incorpora o accede a él)?                                            | **Sí**, las quince                                                                                       |
| ¿Usa algoritmos propietarios o no aceptados como estándar por organismos internacionales (IEEE, IETF, ITU)? | **No**                                                                                                   |
| ¿Usa algoritmos estándar **en lugar de, o además de**, el cifrado del sistema operativo de Apple?           | **Sí**, las quince: el TLS es el de Go; además AES-256-GCM en Notepads y DTLS-SRTP en Rooms y Cast       |
| ¿Se limita al cifrado del sistema operativo (HTTPS de `URLSession`, etc.)?                                  | **No**: marcar esto sería falso, aunque sea la respuesta que evita papeles                               |
| ¿Estará disponible en Francia?                                                                              | **No**, por decisión del dueño del 2026-10-14; Francia queda fuera de la distribución de las quince apps |

Lo que se deriva de esas respuestas —si hace falta documentación de exportación de EE. UU.,
el informe anual de autoclasificación o la declaración francesa— lo determina Apple en el
propio cuestionario y es una declaración legal del dueño. El texto exacto de las preguntas
es el de App Store Connect el día que se responda; arriba va el sentido. Los valores
predeterminados de `--ficha` son los otros veintinueve países, con altas automáticas de
nuevos territorios desactivadas. Si una app ya tiene disponibilidad configurada, el
guion la conserva: la retirada de Francia se aplica y verifica en App Store Connect.
Los países de Google Play y los textos en francés no cambian por esta decisión.

## Lo que le queda al dueño, en este orden

1. **Variables en Codemagic**, grupo `apple-firma`: las seis de la cuenta de prueba y el
   contacto (arriba).
2. `appstore-publicar.yml` con `modo=sondear`: dice, sin cambiar nada, si cada Apple ID es
   de su bundle y qué versiones hay en iOS y macOS.
3. **Revisar las capturas de iPad y Mac** (`<app>/capturas/ipad-*.png` y `mac-*.png`): se
   eligieron sin verlas. Cambiar una es cambiar su número en `capturas.sh`.
4. `modo=ficha` con el tag de la versión que se va a publicar.
5. En la web, por app: **App Privacy** con `<app>/privacidad.txt`, y revisar lo que subió
   la ficha.
6. Subir los builds: un tag `tienda-vX.Y.Z` sobre la release
   (`git push origin 'vX.Y.Z^{commit}:refs/tags/tienda-vX.Y.Z'`); Xcode Cloud compila y sube
   las quince en iOS y macOS. No con cada release: 30 subidas por tag agotan el cupo diario
   de la cuenta («Upload limit reached», 90383). Ver «Xcode Cloud» en `docs/operations.md`.
7. Comprobar el **cumplimiento de exportación** de cada app: cifrado estándar, Francia
   excluida y exención documental en el plist; revisar antes de cambiar cualquiera de
   esos hechos. Si Apple requiere documentos, obtener su aprobación y usar su código.
8. `modo=revision` con el mismo tag y la frase `ENVIAR_<n>_APPS_A_REVISION`.
