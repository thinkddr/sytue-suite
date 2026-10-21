# Cadena de suministro UE

Este directorio materializa el criterio de `docs/CONVENTIONS.md`: para ser
`eu-approved` no basta con alojar datos en una región europea. La entidad matriz
o controladora, la distribución del artefacto y cualquier tratamiento de datos
deben quedar bajo entidades y operación de la Unión Europea.

## Dos riesgos distintos que este inventario mezcla en una sola columna

Añadido el 2026-08-11 tras revisarlo con el propietario, que lo formuló así: **lo
que importa es que los contenidos de nuestro proyecto no pasen por proveedores
online ni conexiones hacia fuera de la UE**. Eso separa dos cosas que hoy
comparten el mismo `status`, y confundirlas hace leer mal el contador:

**A · SALIDA — algo nuestro viaja fuera.** Código, datos, secretos, nombres de
repositorios o de servicios, listas de dependencias, digests de artefactos,
telemetría, identificadores de usuario. Es el riesgo que el criterio persigue, y
no se arregla con una licencia permisiva ni alojando nada en Francia: o se corta,
o sale. Tres casos reales encontrados y cerrados ese mismo día:

- `osv-scanner` enviaba a `api.osv.dev` el inventario completo de dependencias
  del proyecto en **cada** ejecución. Cortado con `--offline-vulnerabilities`.
- `cosign` publicaba en el registro de transparencia de Sigstore (EE. UU.) la
  referencia completa de cada imagen, su digest y la fecha: el inventario de
  servicios y la cadencia de releases. Apagado en los dos extremos.
- `GOPRIVATE` vacío habría enviado al proxy de módulos y al registro de sumas de
  Google las rutas de nuestros módulos privados `git.sytue.eu/...` en cuanto un
  `go.mod` requiriera a otro nuestro. Cerrado antes de que ocurriera.

**B · ENTRADA — traemos un artefacto público.** Un binario, una imagen base, un
módulo, un chart. Aquí **no sale nada nuestro**: el riesgo es de integridad de la
cadena de suministro —que nos den algo distinto de lo que pedimos— y se mitiga
con digest fijado, sha256 comprobado y espejo propio, no cortando la conexión.

**Por qué importa la distinción al leer el contador.** De los 49 `blocked` de
agosto de 2026, **34 no eran proveedores**. Este apartado ya lo decía de los
espacios de nombres de módulos Go —`gonum.org`, `dario.cat`, `pgregory.net`,
`go.uber.org`, `gopkg.in`…—: con `GOPROXY=https://proxy.golang.org,direct` esos
dominios **no se contactan nunca**, porque quien sirve el módulo es
`proxy.golang.org`, inventariado aparte.

**El 2026-08-11 se implementó el estado `identifier` que este texto ya daba por
existente** —estaba documentado y no en el código— y se aplicó a las otras dos
familias que cumplen lo mismo:

- **Grupos de API y prefijos de etiqueta de Kubernetes.** `monitoring.coreos.com`
  es un campo `apiVersion`; `app.kubernetes.io/name` y `topology.kubernetes.io/zone`
  son etiquetas. **232 líneas** de `blocked-baseline.tsv` existían solo para
  silenciarlos.
- **Sitios de proyecto citados en metadatos**, como el `home:` de un `Chart.yaml`
  (`min.io`, `prometheus.io`, `www.pgbouncer.org`). El canal real de esos productos
  —`docker.io`, `docker.dragonflydb.io`— sigue bloqueado y contado aparte.

`identifier` **no relaja nada**: la fila sigue exigiendo evidencia, responsable y
revisión que caduca a los 183 días. Lo único que cambia es que deja de contarse
como dependencia, porque no lo es. El contador quedó en **15 canales de descarga
efectiva**, que son el trabajo real.

**Y de esos quince, cinco se cerraron el mismo día con `pinned-artifact`**: los
registros de imagen (`docker.io`, `gcr.io`, `ghcr.io`, `quay.io`,
`docker.dragonflydb.io`). Es la aplicación literal de lo que dice el apartado B de
arriba —el riesgo de la ENTRADA es de integridad y se mitiga con digest, no
cortando la conexión—, y aquí la mitigación **existe y se comprueba**:

- En ejecución los charts apuntan solo a `rg.fr-par.scw.cloud`; las veinte imágenes
  de terceros están en `upstream-images.txt` con digest inmutable y firmadas, y
  `upstream-vigencia-test.sh` rechaza cualquiera que no esté catalogada.
- En build, los **treinta `FROM`** de los Dockerfile llevan `@sha256:`.

Eso último no se toma como palabra: `TestLosRegistrosFijadosLoEstanDeVerdad` recorre
los Dockerfile y falla si alguno pierde el digest. La etiqueta describe una
mitigación real o no vale nada.

Tres filas, además, contradecían su propio contenido: `dario.cat` declara sede en
**España**, `alpinelinux.org` anfitrión fiscal en la **Open Collective Europe
Foundation (UE)** y `postgresql.org` en **Canadá**. Estaban las tres como
bloqueadas por gobernanza extracomunitaria.

**Y el contador vuelve a subir el mismo día**, porque el perfil on-premise entró
en git y trajo consigo cuatro canales que antes no se veían: `get.k3s.io`,
`download.docker.com`, `raw.githubusercontent.com` y `charts.jetstack.io`.
Son descargas de verdad y sin mitigación —dos de ellas ejecutan lo descargado
(`curl … | sh`) y una apunta a la rama `main` de un tercero—, así que van como
`blocked` con su fila de baseline, no como `pinned-artifact`. Efectivos quedan
**catorce**, no diez: cerrarlos exige fijar versión y checksum en
`deploy/helm/bin/install-host.sh`, que hoy no lo hace.

Regla práctica al clasificar una fila: **preguntarse qué viaja y en qué
dirección**. Si la respuesta es «algo nuestro, hacia fuera», es un incumplimiento
que hay que cerrar. Si es «un artefacto público, hacia dentro», lo que toca es
fijar su digest y espejarlo, y el `status` describe su gobernanza, no una fuga.

## Archivos y gate

- `providers.tsv`: inventario versionado con controlador, sede, región,
  finalidad, evidencia primaria, responsable, fecha de revisión, caducidad y
  licencia o términos aplicables.
- `blocked-baseline.tsv`: deuda heredada exacta `host + archivo`. No es una
  allowlist: cada fila continúa siendo un incumplimiento que debe desaparecer.
- `tools/eusupply`: extrae referencias de ficheros tracked de CI, Docker,
  Compose, Helm, Terraform, scripts, módulos Go y locks Bun. Falla ante un host
  desconocido, evidencia con más de 183 días, una excepción caducada, una
  nueva referencia bloqueada o una fila obsoleta de la línea base.

Ejecutar localmente:

```sh
make eu-supply-chain-check
```

El mismo gate forma parte de `make gates`, GitHub `security` durante la
transición y el piloto de Sytue Git. La línea base solo puede disminuir; no debe
ampliarse para hacer pasar CI sin una decisión documentada.

## Estado de la migración (2026-08-02)

Ya aprobados o migrados:

- automatización de Sytue Git (Gitea) autohospedado sobre Scaleway Francia y workflows de
  piloto preparados; el alta real sigue pendiente de aplicar la infraestructura;
- runtime, registry, S3 y observabilidad gestionada sobre Scaleway;
- pagos con Mollie B.V. (Países Bajos);
- inferencia IA por defecto con Scaleway Generative APIs y Mistral; Anthropic ha
  sido retirado del servicio;
- Traefik (SAS francesa), con su chart vendorizado, y base GeoIP estática de
  DB-IP (Francia).

**Retirada el 2026-10-06**: la forja es GitHub, la pasarela `services/ghcompat`
salió del repositorio (auditoría de seguridad, CLOUD-02) y con ella la fila de
`claude.ai` de `blocked-baseline.tsv`. Lo que sigue es el registro de la
decisión que estuvo vigente hasta entonces.

Decidido el 2026-08-15, **Claude Code sobre Sytue Git**: la pasarela presentaba
la forja como un GitHub Enterprise Server para poder conectarla con Claude Code,
que con una forja autohospedada solo habla GHES.

La topología de partida fue la de **entorno autohospedado** —sesiones en un
runner nuestro en la UE, que clona desde dentro de la red— precisamente para no
abrir nada entrante. **Ese punto se revisó el mismo día, y hay que decirlo
claro**: la conexión de Claude Code no llega a establecerse sin que su
infraestructura alcance la instancia, así que el propietario decidió abrir el
puerto de la pasarela (8443) **restringido al rango saliente de Anthropic**,
`160.79.104.0/21`, en el grupo de seguridad de la forja.

Lo que eso significa, sin adornos:

- **La API traducida es alcanzable desde infraestructura de Estados Unidos**, y
  esa API sirve el contenido de los ficheros, no solo metadatos. Es un
  apartamiento del criterio EU-only estricto, tomado a sabiendas.
- **No es acceso general**: la puerta está cerrada a todo lo demás, y el alcance
  —qué repositorios se ven— no lo fija la regla de red sino el usuario bot
  `claude-bot`, que solo pertenece al equipo `claude` de la organización.
- **El clonado sigue siendo interno** mientras las sesiones corran en el runner
  autohospedado: quien clona es nuestra máquina.
- **Lo que cada sesión lea va al modelo**, que es inherente a usar Claude y no a
  esta pasarela.

Con el puerto abierto entran también el Code Review alojado y las métricas de
contribución, que antes quedaban fuera. La vía de vuelta, si se quiere estrechar,
es cerrar el 8443 y pasar al conector SCM del orquestador; estaba descrita en el
README del servicio, que ya no existe.

Excepciones temporales, ambas con salida obligatoria antes de 2026-10-01:

- GitHub exclusivamente como puente de migración hasta completar dos releases
  verificadas en Sytue Git;
- Bun mientras se retira el BFF/SSR JavaScript según ADR-0027.

Bloqueos que no se resuelven cambiando solo la región de hosting:

1. **Toolchains y gobernanza:** Go/Google, Kubernetes/Helm/CNCF/Linux
   Foundation, npm/Microsoft y Node/OpenJS no cumplen el criterio de sede UE.
   Requieren un ADR de arquitectura y migración de código, no un mero mirror.
2. **Artefactos upstream:** ~~Docker Hub, GCR, GHCR, Quay, imágenes base y el
   chart servido desde GitHub deben importarse una sola vez, verificar firma y
   checksum y promoverse al Registry/Object Storage de Scaleway.~~ **HECHO y
   verificado el 2026-08-11**: los cinco registros pasan a `pinned-artifact`
   (mirror con digest inmutable y firma en ejecución; `@sha256:` en los treinta
   `FROM` de build), y el chart de Traefik está vendorizado —`traefik.github.io`
   ya no se observa en ningún fichero—. Sigue siendo cierto lo segundo: **el
   mirror reduce la exposición operativa, no convierte el origen en UE**; por eso
   el estado se llama «artefacto fijado» y no «aprobado».
3. **PKI pública:** Let's Encrypt pertenece a una entidad estadounidense.
   HARICA (Grecia) ofrece ACME, pero exige alta y validación previa de la
   organización; no se cambia el issuer hasta completar ese proceso para no
   interrumpir TLS.
4. **Componentes autohospedados no UE o de control no verificable:** Dragonfly,
   PostgreSQL/PgBouncer, MinIO, LiveKit, PostgREST, Paketo y dependencias
   transitivas necesitan sustitución o una decisión expresa. Autohospedarlos
   evita transferencias de datos, pero no satisface la regla de origen.

## Orden de salida

1. finalizar Sytue Git y cortar GitHub tras dos releases verdes;
2. cerrar el ingress de artefactos con mirrors privados y builds reproducibles;
3. contratar/validar HARICA y cambiar los ClusterIssuers;
4. retirar Bun/Node del runtime y del build productivo;
5. aprobar el ADR de sustitución de Go/Kubernetes y ejecutar la migración por
   bounded context;
6. sustituir componentes restantes y llevar `blocked-baseline.tsv` a cero.

No se puede declarar la cadena "100 % UE" hasta que la línea base esté vacía
y el inventario no contenga excepciones temporales.
