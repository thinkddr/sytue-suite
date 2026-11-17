// SPDX-License-Identifier: Apache-2.0

//go:build imagen

package chathttp

import "embed"

// LA COMPILACIÓN QUE VA A LA IMAGEN NO COMPILA SIN EL NÚCLEO DE CLIENTE.
//
// La v3.20.3 publicó Rooms con client_core_assets vacío —el RUN que lo generaba
// estaba en la etapa de compilación del Dockerfile y la release no la ejecutaba—,
// y el `//go:embed client_core_assets` de desarrollo lo aceptó sin rechistar: el
// fallo solo se vio en producción, como un 503 «client core unavailable» que
// dejaba la web sin cifrado de extremo a extremo (docs/operations.md#client-core-vacio).
//
// Con la etiqueta `imagen` —la que ponen services/rooms/Dockerfile y, leída de él,
// scripts/publicar-imagen-go.sh— cada fichero que el manifiesto necesita va por su
// nombre, y un patrón de //go:embed que no casa con nada es un error de
// compilación. Es el mismo FS que el de desarrollo: los patrones se solapan con el
// directorio y no duplican nada.
//
//go:embed client_core_assets
//go:embed client_core_assets/core.wasm client_core_assets/wasm_exec.js
//go:embed client_core_assets/office/core.wasm client_core_assets/office/wasm_exec.js
//go:embed client_core_assets/mail/core.wasm client_core_assets/mail/wasm_exec.js
var clientCoreAssets embed.FS
