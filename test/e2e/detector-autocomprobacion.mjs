// Autocomprobación del detector de desbordamiento.
//
// El detector decide qué es un defecto de maquetación y qué no, y equivocarse
// sale caro en las dos direcciones: un falso negativo deja pasar una pantalla
// rota, y un falso positivo manda a arreglar algo que está bien. Ha pasado: el
// título del editor de Docs salió como desbordamiento dos veces —el 2026-09-08
// y el 2026-09-11— y la segunda se midió que su cabecera ocupaba 393px de 393.
// Era el texto desplazándose DENTRO del campo, que es lo que hace un <input>.
//
//   node detector-autocomprobacion.mjs     (sale 1 si alguna falla)
import { chromium } from "playwright";
import { DETECTOR } from "./detector-desbordamiento.mjs";

const CASOS = [
  {
    nombre: "un campo con más texto del que cabe no es un desbordamiento",
    html: `<div style="width:300px"><input id="t" style="width:150px"
             value="un titulo bastante mas largo que su campo"></div>`,
    espera: 0,
  },
  {
    nombre: "ni un textarea con una parrafada",
    html: `<div style="width:300px"><textarea id="t" style="width:150px"
             rows="1">${"palabra ".repeat(40)}</textarea></div>`,
    espera: 0,
  },
  {
    nombre: "pero un bloque más ancho que la ventana sí lo es",
    html: `<div style="width:2000px;height:40px;background:#ccc">ancho fijo</div>`,
    espera: 1,
  },
];

const navegador = await chromium.launch();
const pagina = await navegador.newPage({
  viewport: { width: 390, height: 700 },
});
let fallos = 0;

for (const caso of CASOS) {
  await pagina.setContent(
    `<!doctype html><style>body{margin:0}</style>${caso.html}`,
  );
  const { hallazgos } = await pagina.evaluate(DETECTOR);
  const bien =
    caso.espera === 0 ? hallazgos.length === 0 : hallazgos.length > 0;
  if (!bien) {
    fallos += 1;
    console.error(`✗ ${caso.nombre}`);
    console.error(
      `   esperaba ${caso.espera === 0 ? "ningún" : "algún"} hallazgo y hubo ${hallazgos.length}`,
    );
    for (const h of hallazgos)
      console.error(`   · ${h.sel} +${h.exceso}px (${h.modo})`);
  } else {
    console.log(`✓ ${caso.nombre}`);
  }
}

await navegador.close();
if (fallos) {
  console.error(`\n${fallos} de ${CASOS.length} fallan.`);
  process.exit(1);
}
console.log(`\n${CASOS.length} de ${CASOS.length} bien.`);
