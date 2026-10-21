# Power BI — alta en Sytue Platform
**Qué conecta**: consultas de lectura sobre un dataset publicado (sin OAuth de usuario: credencial del servicio).
1. En app.powerbi.com identifica el **dataset_id**: abre el semantic model → el GUID de la URL (`/datasets/XXXX-…`).
2. Necesitas una identidad con licencia Pro/Premium sobre ese dataset y su **api_base** (`https://api.powerbi.com`, o el de tu VNET/SSRS si es on-prem).
3. Crea la credencial de servicio (service principal en Entra + grupo de seguridad con acceso al dataset, o usuario técnico con licencia).
4. Sytue Platform → Integraciones → **Power BI**: dataset_id y api_base + credencial → «Conectar».
**Verificar**: una DAX de prueba (`EVALUATE ROW("x", 1)`) desde el panel.
**Errores**: `DatasetNotFound` = el GUID no es del dataset sino del informe; 401 = el principal no tiene acceso al espacio.
