#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Instala veraPDF (CLI) como control EXTERNO de PDF/UA-1 para los tests de
# platform/pkg/pdfdoc/uacheck. Es una herramienta de desarrollo y de CI (GPL-3/MPL-2): no se
# distribuye ni entra en producción. Necesita Java 11+. Uso:
#   scripts/verapdf-instalar.sh [directorio]      # por defecto /srv/agentes/verapdf
#   SYTUE_VERAPDF=<directorio>/inst/verapdf go test ./platform/pkg/pdfdoc/uacheck/
set -eu
dir="${1:-/srv/agentes/verapdf}"
mkdir -p "$dir"
cd "$dir"
[ -f vp.zip ] || curl -fsSL -o vp.zip "https://software.verapdf.org/releases/verapdf-installer.zip"
unzip -q -o vp.zip
# La versión la pone el zip, que siempre trae la última: fijarla aquí rompía
# el guion en cuanto salía otra (pasó con la 1.30.3).
cd verapdf-greenfield-*/
cat > auto.xml <<XML
<AutomatedInstallation langpack="eng">
<com.izforge.izpack.panels.htmlhello.HTMLHelloPanel id="welcome"/>
<com.izforge.izpack.panels.target.TargetPanel id="install_dir"><installpath>$dir/inst</installpath></com.izforge.izpack.panels.target.TargetPanel>
<com.izforge.izpack.panels.packs.PacksPanel id="sdk_pack_select"><pack index="0" name="veraPDF Mac and *nix Scripts" selected="true"/><pack index="1" name="veraPDF Validation model" selected="false"/><pack index="2" name="veraPDF Documentation" selected="false"/><pack index="3" name="veraPDF Sample Plugins" selected="false"/></com.izforge.izpack.panels.packs.PacksPanel>
<com.izforge.izpack.panels.install.InstallPanel id="install"/>
<com.izforge.izpack.panels.finish.FinishPanel id="finish"/>
</AutomatedInstallation>
XML
java -jar verapdf-izpack-installer-*.jar auto.xml
echo "veraPDF instalado en $dir/inst; export SYTUE_VERAPDF=$dir/inst/verapdf"
