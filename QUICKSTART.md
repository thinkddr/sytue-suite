<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr) -->

# Instalar Sytue Suite

Tres comandos y un asistente que hace el resto: comprueba los requisitos, genera todas
las contraseñas y claves, construye la suite, la arranca, crea tu cuenta de
administrador y verifica que todo responde.

---

## 1. Antes de empezar

| Necesitas | Cómo comprobarlo | Si no lo tienes |
| --- | --- | --- |
| **Docker** con **4 GB de memoria** (la suite completa arrancada usa ~1,4 GB; la primera construcción compila en paralelo y agradece 8 GB) | `docker info` | [Docker Desktop](https://docs.docker.com/get-docker/) (macOS/Windows) o Docker Engine (Linux). En Docker Desktop: *Settings → Resources → Memory*. |
| **mkcert** (HTTPS local de confianza) | `mkcert -version` | macOS: `brew install mkcert nss` · Debian/Ubuntu: `sudo apt install mkcert libnss3-tools` · Windows: `choco install mkcert` |
| **git**, **bash** y **openssl** | `git --version` | Vienen con macOS y Linux. En Windows, usa WSL 2. |

Unos **30 GB de disco** libres (la primera construcción llega a ~25 GB entre imágenes y caché) y los puertos **80** y **443** de tu equipo sin usar.

---

## 2. Instalar

```bash
git clone https://github.com/thinkddr/sytue-suite.git
cd sytue-suite
./sytue-deploy
```

Pulsa **Enter** en cada pregunta para quedarte con la opción recomendada. El asistente
recorre cinco pasos:

1. **Dónde instalar** → *En este equipo con Docker* (para un servidor con tu dominio, mira la [sección 6](#6-instalar-en-un-servidor-con-tu-dominio)).
2. **Requisitos** → comprueba Docker, su memoria, mkcert y los puertos. Si falta algo, te
   dice cómo instalarlo y se detiene sin tocar nada.
3. **Servicios** → *Suite completa*, *Ofimática* (menos memoria), *Mínimo* o *A medida*.
4. **Configuración y secretos** → elige la contraseña del administrador (Enter = una
   aleatoria) y, si quieres, un proveedor para Sytue AI. Genera `deploy/compose/.env` con
   claves únicas para tu equipo (permisos 0600, Git lo ignora).
5. **Construcción y arranque** → crea el certificado HTTPS local, construye las imágenes
   (**la primera vez tarda entre 15 y 40 minutos**; después es casi inmediato), arranca los
   contenedores, crea tu administrador en Sytue ID, espera a que todos los servicios estén
   sanos y lanza una verificación de extremo a extremo.

¿Sin preguntas? `./sytue-deploy --yes` instala la suite completa con los valores
recomendados (`--profile office` para la versión ligera).

---

## 3. Entrar

Al terminar, el asistente muestra tus credenciales. Abre **https://one.sytue.localhost**:

| Campo | Valor |
| --- | --- |
| Correo | `admin@sytue.localhost` |
| Contraseña | la que elegiste o la que generó el asistente (está en `deploy/compose/.env`, variable `SYTUE_ID_DEV_SEED_PASSWORD`) |
| Código de verificación | la cuenta de administración pide un segundo factor: añade a tu app de autenticación (Google Authenticator, 1Password, Aegis…) la clave **`JBSWY3DPEHPK3PXP`** |

En la pantalla de acceso, marca la casilla de **verificación humana** antes de pulsar
*Continuar* (se resuelve sola en un segundo). `member@sytue.localhost`, con la misma
contraseña y sin segundo factor, es una cuenta de usuario normal (sin acceso a Sytue Admin).

Cada aplicación vive en su subdominio:

| Aplicación | Dirección |
| --- | --- |
| Sytue One (portal) | https://one.sytue.localhost |
| Sytue Admin | https://admin.sytue.localhost |
| Mail · Drive · Calendar · Contacts | https://mail.sytue.localhost · https://drive.sytue.localhost · https://calendar.sytue.localhost · https://contacts.sytue.localhost |
| SyDocus · SyCalcus · SyDiapos · SyCertiPDF | https://docus.sytue.localhost · https://calcus.sytue.localhost · https://diapos.sytue.localhost · https://certipdf.sytue.localhost |
| Rooms · Notepads | https://rooms.sytue.localhost · https://notes.sytue.localhost |
| SyERP · Cast · Web · Bases | https://erp.sytue.localhost · https://cast.sytue.localhost · https://web.sytue.localhost · https://bases.sytue.localhost |
| Sytue Cloud | https://cloud.sytue.localhost |
| Sitio público | https://www.sytue.localhost |

`*.sytue.localhost` apunta a tu propio equipo sin configurar nada: los navegadores
resuelven `.localhost` a `127.0.0.1`.

> **Limitación conocida del modo local.** Varias apps guardan tus datos cifrados en el
> dispositivo con claves custodiadas por el servidor de mensajería cifrada (Matrix), que
> producción despliega y el entorno local **todavía no incluye**. Hasta entonces, en local
> funcionan Sytue Admin, Sytue Web, Sytue Cloud y Sytue Cast; Mail, Drive, Docus, Calcus,
> Diapos, CertiPDF, Calendar, Contacts, Notepads, Rooms, SyERP, Bases y la portada del
> portal se quedan en «Cargando…». Lo mismo ocurre en el entorno local de sytue-one.

---

## 4. Día a día

| Quiero… | Comando |
| --- | --- |
| Parar la suite (conserva los datos) | `make dev-down` |
| Volver a arrancarla | `make dev-up` |
| Ver el estado de cada servicio | `make dev-ps` |
| Ver los logs de un servicio | `make dev-logs SVC=id` |
| Actualizar a la última versión | `git pull && ./sytue-deploy` (reutiliza tu configuración y tus datos) |
| Cargar datos de ejemplo | `make dev-demo` |
| Empezar de cero (**borra todos los datos**) | `./sytue-deploy --reset` |

---

## 5. Si algo falla

- **«Docker está instalado pero no responde»** → abre Docker Desktop (o `colima start`,
  o `sudo systemctl start docker`) y vuelve a ejecutar `./sytue-deploy`.
- **El navegador avisa de un certificado no válido** → ejecuta `mkcert -install` y
  reinicia el navegador.
- **Un servicio se queda reiniciándose** → `make dev-ps` para ver cuál y
  `make dev-logs SVC=<servicio>` para ver por qué. Si es por memoria, sube la de Docker o
  instala la versión ligera: `./sytue-deploy --profile office`.
- **«password authentication failed» tras borrar `deploy/compose/.env`** → las
  contraseñas de las bases de datos se fijan la primera vez; sin aquel `.env`, empieza de
  cero con `./sytue-deploy --reset`.
- **Comprobar que todo responde** → `scripts/e2e/smoke.sh` (el asistente lo ejecuta al
  final y guarda el resultado en `deploy/compose/.verificacion.log`).

---

## 6. Instalar en un servidor con tu dominio

Toda la suite en **una sola máquina Linux con Docker**, accesible como
`https://<app>.tu-dominio` y con certificados de **Let's Encrypt** que se renuevan solos.

**Antes:** un registro DNS comodín `*.suite.tuempresa.eu` (y `suite.tuempresa.eu`)
apuntando a la IP del servidor, y abiertos los puertos `80` y `443/TCP` (web y
certificados), `7881/TCP` y `50000-50100/UDP` (videollamadas). No hace falta mkcert.

```bash
./sytue-deploy --mode servidor --domain suite.tuempresa.eu --acme-email tu@tuempresa.eu
```

El asistente te pregunta además:

- **Quién la usa** (`--usuarios`):
  - `uno`: solo tu cuenta, sin registro público. Ideal para uso personal.
  - `varios` (por defecto): un equipo. Invitas a cada persona desde
    `https://admin.suite.tuempresa.eu`; con `--registro-abierto`, además, cualquiera puede
    crearse una cuenta en `https://id.suite.tuempresa.eu/signup` (`--registro-cerrado` lo
    vuelve a cerrar).
- **Propietario** (`--admin-email`) y **organización** (`--org-slug`). La contraseña del
  propietario se genera y queda en `deploy/compose/.env` (`SYTUE_PROPIETARIO_PASSWORD`).
  Al entrar por primera vez, activa la verificación en dos pasos en
  `https://id.suite.tuempresa.eu/account`.

Para una **red local o pruebas** sin Internet, usa `--tls interno` (certificados de
mkcert: cada equipo que entre debe confiar en su CA, `mkcert -CAROOT`).

Volver a ejecutar el mismo comando es seguro: reutiliza la configuración, construye solo lo
que cambió y vuelve a levantar lo que estuviera parado. Así se cambia también el modo de
usuarios (`--usuarios uno|varios`) sin tocar los datos. `--reset` empieza de cero
**borrando los datos**.

> SMTP e IMAP para clientes de correo externos no se publican en este modo: el correo web
> funciona completo. Para publicar un servidor de correo, usa el clúster (sección 8).

---

## 7. Todo en un solo contenedor

El mismo modo servidor, empaquetado en **un contenedor y un volumen**: dentro corre su
propio Docker (Docker in Docker, por eso pide `--privileged`) con toda la suite.

```bash
docker build -f deploy/todo-en-uno/Dockerfile -t sytue-suite .
docker run -d --name sytue --privileged --restart unless-stopped \
  -p 80:80 -p 443:443 -p 7881:7881 -p 50000-50100:50000-50100/udp \
  -v sytue-datos:/datos \
  -e SYTUE_DOMINIO=suite.tuempresa.eu -e SYTUE_ACME_EMAIL=tu@tuempresa.eu \
  -e SYTUE_USUARIOS=varios \
  sytue-suite
docker logs -f sytue          # la primera vez construye la suite dentro (20-60 min)
```

Sin `SYTUE_DOMINIO`, se queda en `https://one.suite.localhost` para probarla en tu propio
equipo (certificados internos: `docker cp sytue:/datos/ca/rootCA.pem .` e impórtalo
como autoridad de confianza). Todas las variables están en la cabecera de
[`deploy/todo-en-uno/Dockerfile`](deploy/todo-en-uno/Dockerfile). Todo lo que hay que
conservar vive en el volumen `sytue-datos`: borrar el contenedor no borra la suite.

---

## 8. Instalar en un clúster Kubernetes (avanzado)

Para producción con tu propio dominio. Necesitas `kubectl`, `helm`, un registro de
imágenes, PostgreSQL 16+ y almacenamiento S3.

```bash
./sytue-deploy --mode helm
```

El asistente pide dominio, registro, PostgreSQL, S3 y el propietario inicial, y genera en
`deploy/helm/` (fuera de Git) todo lo necesario. Al final imprime los siete comandos a
ejecutar en orden: publicar las imágenes en tu registro, crear las bases de datos,
aplicar los Secrets, instalar el chart y crear el propietario. Detalle y referencia de
variables: [`docs/operations.md`](docs/operations.md#13-prod-en-scaleway-terraform--helm).
