// Las páginas offline y los cascarones de Capacitor no cargan bundles: reciben
// los mismos tokens y script inicial, incrustados para funcionar sin red.
import spacing from "../branding/tokens/spacing.json";
import { themeInitScript } from "../apps/frontend/packages/ui/src/theme/init.js";

const root = new URL("../", import.meta.url);
const colors = await Bun.file(
  new URL("branding/tokens/colors.css", root),
).text();
const appearance = await Bun.file(
  new URL("branding/tokens/appearance.css", root),
).text();
const begin = "<!-- sytue-theme:start -->";
const end = "<!-- sytue-theme:end -->";
const generated = `${begin}
<style>
${colors}
${appearance}
:root { --radius-control: ${spacing.radii.sm}; }
</style>
<script>${themeInitScript}</script>
${end}`;

for (const path of [
  "apps/frontend/apps/portal/public/offline.html",
  "apps/mobile/rooms/www/index.html",
  "apps/mobile/suite/www/index.html",
]) {
  const file = new URL(path, root);
  const source = await Bun.file(file).text();
  const start = source.indexOf(begin);
  const finish = source.indexOf(end, start);
  if (start < 0 || finish < 0)
    throw new Error(`Faltan las marcas de tema en ${path}`);
  await Bun.write(
    file,
    source.slice(0, start) + generated + source.slice(finish + end.length),
  );
}

// El IdP sirve HTML desde Go y usa el mismo inicializador, autorizado por CSP.
await Bun.write(
  new URL("services/id/internal/http/theme_init.js", root),
  themeInitScript,
);
