# Paquetes y Manifiestos de Tiendas y Marketplaces Oficiales

Este directorio contiene los paquetes y manifiestos de aplicación listos para publicar en las tiendas oficiales de terceros para Sytue Rooms y Sytue One:

- **Slack App Directory**: `slack/manifest.json`
- **Microsoft Teams / AppSource**: `teams/manifest.json`
- **Google Workspace Marketplace**: `google-workspace/manifest.json`
- **Zoom App Marketplace**: `zoom/manifest.json`

## Flujo de Trabajo para Organizaciones y Usuarios

1. **Configuración Global en Sytue Admin**:
   - El administrador de la organización o el usuario individual accede a **Sytue Admin** (`https://console.sytue.eu/admin/apps`).
   - Hace clic en **Conectar** sobre la aplicación deseada (Slack, Microsoft Teams, Google Workspace / Meet / Chat, Zoom).
   - Autoriza con un solo clic mediante OAuth2. Las credenciales quedan registradas de forma segura y centralizada.

2. **Uso en Sytue Rooms**:
   - Cualquier miembro del equipo abre la pestaña de **Integraciones** o pulsa **Conectar con otras Apps**.
   - Selecciona la integración ya configurada en Admin.
   - El canal o sala de videollamada queda enlazado de forma automática y bidireccional, sin necesidad de copiar ni pegar webhooks manuales.

## Guía de Publicación Oficial por Plataforma

### 1. Slack (`slack/manifest.json`)
1. Iniciar sesión en [api.slack.com/apps](https://api.slack.com/apps).
2. Pulsar en **Create New App** → **From an app manifest**.
3. Seleccionar el espacio de trabajo de desarrollo y pegar el contenido de `slack/manifest.json`.
4. En **OAuth & Permissions**, verificar que la URL de redirección apunte a `https://integrations.sytue.eu/api/v1/oauth/callback`.
5. Solicitar aprobación para el Slack App Directory desde la sección **Manage Distribution**.

### 2. Microsoft Teams (`teams/manifest.json`)
1. Abrir el **Microsoft Teams Developer Portal** ([dev.teams.microsoft.com](https://dev.teams.microsoft.com/)).
2. Seleccionar **Apps** → **Import app** y subir el paquete zip con `manifest.json`, `color.png` (192x192) y `outline.png` (32x32).
3. Configurar el ID de aplicación de Azure AD en **Single Sign-On / App Registration**.
4. Publicar la app en el catálogo de inquilino de la organización o enviarla a certificación para Microsoft AppSource.

### 3. Google Workspace (`google-workspace/manifest.json`)
1. Acceder a [Google Cloud Console](https://console.cloud.google.com/) en el proyecto de Sytue One.
2. Habilitar **Google Chat API** y **Google Workspace Marketplace SDK**.
3. En la configuración del add-on o script, importar el manifiesto `google-workspace/manifest.json`.
4. En el Marketplace SDK, rellenar los datos de la ficha pública (gráficos, política de privacidad) y enviar a revisión.

### 4. Zoom (`zoom/manifest.json`)
1. Acceder a [marketplace.zoom.us](https://marketplace.zoom.us/develop/create).
2. Crear una nueva app de tipo **OAuth App** (Account-level o User-level app).
3. Introducir las URLs de redirección (`https://integrations.sytue.eu/api/v1/oauth/callback`) y añadir los scopes declarados en el manifiesto.
4. Enviar a revisión técnica y de seguridad de Zoom.
