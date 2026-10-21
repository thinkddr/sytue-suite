// Checks de la secuencia RGA del cliente. Ejecuta: `bun test`.
//
// Lo que de verdad hay que probar aquí no es «insertar inserta», sino la
// CONVERGENCIA: dos réplicas que reciben las mismas operaciones en distinto
// orden tienen que acabar con el mismo texto. Si eso falla, el fallo no se ve
// como un error sino como dos personas mirando documentos distintos, que es la
// forma más cara de romperse.
//
// El orden de hermanos (descendente por punto) y el desempate del punto
// (contador y luego actor) son un CONTRATO con `services/docs/internal/domain/crdt.go`.
// Los casos de `TestOrdenReplicaAlServidor` están escritos con los mismos
// valores que el test de Go, para que una divergencia rompa un lado u otro.
import { expect, test } from 'bun:test';
import { SecuenciaRGA, puntoMenor, type OpRGA } from './rga.js';

/** Baraja determinista: mismos elementos, orden distinto y reproducible. */
function barajar<T>(xs: T[], semilla: number): T[] {
  const out = [...xs];
  let s = semilla;
  for (let i = out.length - 1; i > 0; i--) {
    s = (s * 1103515245 + 12345) & 0x7fffffff;
    const j = s % (i + 1);
    [out[i], out[j]] = [out[j]!, out[i]!];
  }
  return out;
}

test('escribir letra a letra da el texto escrito', () => {
  const s = new SecuenciaRGA('a');
  s.insertarLocal(0, 'hola', 1);
  expect(s.texto()).toBe('hola');
  expect(s.longitud).toBe(4);
});

test('insertar en medio, al principio y al final', () => {
  const s = new SecuenciaRGA('a');
  s.insertarLocal(0, 'hola', 1);
  s.insertarLocal(4, ' mundo', 10);
  expect(s.texto()).toBe('hola mundo');
  s.insertarLocal(0, '¡', 20);
  expect(s.texto()).toBe('¡hola mundo');
  s.insertarLocal(5, ',', 30);
  expect(s.texto()).toBe('¡hola, mundo');
});

test('borrar deja tumbas pero el texto visible es el correcto', () => {
  const s = new SecuenciaRGA('a');
  s.insertarLocal(0, 'hola mundo', 1);
  s.borrarLocal(4, 6, 20);
  expect(s.texto()).toBe('hola');
  // Y se puede seguir escribiendo sobre lo que queda.
  s.insertarLocal(4, '!', 30);
  expect(s.texto()).toBe('hola!');
});

test('los emoji no se parten por la mitad', () => {
  // Recorrer por unidades UTF-16 crearía dos medios elementos que por separado
  // no son ningún carácter, y al intercalar otra inserción saldría basura.
  const s = new SecuenciaRGA('a');
  const ops = s.insertarLocal(0, 'a👍b', 1);
  expect(s.texto()).toBe('a👍b');
  expect(ops.length).toBe(3); // tres code points, no cuatro unidades
});

// --- Convergencia, que es el motivo de todo esto -----------------------------

test('dos actores escribiendo en el mismo punto convergen al mismo texto', () => {
  // El caso que el LWW de documento entero rompía: Ana y Beto escriben a la vez
  // al final del documento. Con `set` el último guardado borraba al otro.
  const base = new SecuenciaRGA('base');
  const opsBase = base.insertarLocal(0, 'hola ', 1);

  const ana = new SecuenciaRGA('ana');
  const beto = new SecuenciaRGA('beto');
  for (const op of opsBase) {
    ana.aplicarRemota(op);
    beto.aplicarRemota(op);
  }

  const opsAna = ana.insertarLocal(5, 'Ana', 10);
  const opsBeto = beto.insertarLocal(5, 'Beto', 10);

  // Cada uno recibe lo del otro.
  for (const op of opsBeto) ana.aplicarRemota(op);
  for (const op of opsAna) beto.aplicarRemota(op);

  expect(ana.texto()).toBe(beto.texto());
  // Y NINGUNA de las dos aportaciones se ha perdido, que es lo que fallaba.
  expect(ana.texto()).toContain('Ana');
  expect(ana.texto()).toContain('Beto');
});

test('el mismo lote de ops en cualquier orden da el mismo texto', () => {
  const emisor = new SecuenciaRGA('emisor');
  const ops: OpRGA[] = [
    ...emisor.insertarLocal(0, 'documento', 1),
    ...emisor.insertarLocal(0, 'Un ', 20),
    ...emisor.insertarLocal(12, ' compartido', 40),
  ];
  const esperado = emisor.texto();
  expect(esperado).toBe('Un documento compartido');

  for (const semilla of [1, 7, 42, 1234, 99999]) {
    const r = new SecuenciaRGA('receptor');
    for (const op of barajar(ops, semilla)) r.aplicarRemota(op);
    expect(r.texto()).toBe(esperado);
  }
});

test('un borrado que llega antes que su inserción no resucita el carácter', () => {
  // Pasa de verdad con reconexiones y replays desordenados. Sin tumba
  // anticipada, el carácter aparecería de nuevo y el documento divergiría.
  const emisor = new SecuenciaRGA('emisor');
  const ins = emisor.insertarLocal(0, 'XY', 1);
  const del = emisor.borrarLocal(0, 1, 10); // borra la X

  const receptor = new SecuenciaRGA('receptor');
  receptor.aplicarRemota(del[0]!); // el borrado llega PRIMERO
  for (const op of ins) receptor.aplicarRemota(op);

  expect(receptor.texto()).toBe('Y');
  expect(receptor.texto()).toBe(emisor.texto());
});

test('aplicar la misma op dos veces no duplica (idempotencia)', () => {
  // El servidor reenvía el eco de las ops propias; sin idempotencia el texto se
  // duplicaría en cada confirmación.
  const emisor = new SecuenciaRGA('emisor');
  const ops = emisor.insertarLocal(0, 'eco', 1);

  const r = new SecuenciaRGA('receptor');
  for (const op of ops) r.aplicarRemota(op);
  for (const op of ops) r.aplicarRemota(op); // otra vez
  expect(r.texto()).toBe('eco');
});

// --- El puente con el editor actual ------------------------------------------

test('opsDesdeTexto: escribir al final', () => {
  const s = new SecuenciaRGA('a');
  s.insertarLocal(0, 'hola', 1);
  const ops = s.opsDesdeTexto('hola mundo', 10);
  expect(s.texto()).toBe('hola mundo');
  expect(ops.every((o) => o.kind === 'insert')).toBe(true);
  expect(ops.length).toBe(6); // solo « mundo», no el documento entero
});

test('opsDesdeTexto: borrar del medio', () => {
  const s = new SecuenciaRGA('a');
  s.insertarLocal(0, 'hola cruel mundo', 1);
  const ops = s.opsDesdeTexto('hola mundo', 30);
  expect(s.texto()).toBe('hola mundo');
  expect(ops.every((o) => o.kind === 'delete')).toBe(true);
});

test('opsDesdeTexto: sustituir una selección', () => {
  const s = new SecuenciaRGA('a');
  s.insertarLocal(0, 'hola mundo', 1);
  const ops = s.opsDesdeTexto('hola planeta', 20);
  expect(s.texto()).toBe('hola planeta');
  expect(ops.some((o) => o.kind === 'delete')).toBe(true);
  expect(ops.some((o) => o.kind === 'insert')).toBe(true);
});

test('opsDesdeTexto: sin cambios no emite nada', () => {
  const s = new SecuenciaRGA('a');
  s.insertarLocal(0, 'igual', 1);
  expect(s.opsDesdeTexto('igual', 10)).toEqual([]);
});

test('opsDesdeTexto: vaciar el documento entero', () => {
  const s = new SecuenciaRGA('a');
  s.insertarLocal(0, 'todo fuera', 1);
  s.opsDesdeTexto('', 20);
  expect(s.texto()).toBe('');
});

test('opsDesdeTexto: las ops reproducen el cambio en otra réplica', () => {
  // La prueba que importa del puente: lo que se emite tiene que bastar para que
  // el otro lado llegue al mismo sitio, sin mandarle el documento entero.
  const local = new SecuenciaRGA('local');
  const iniciales = local.insertarLocal(0, 'primera versión', 1);

  const remoto = new SecuenciaRGA('remoto');
  for (const op of iniciales) remoto.aplicarRemota(op);

  const ops = local.opsDesdeTexto('primera versión corregida', 50);
  for (const op of ops) remoto.aplicarRemota(op);

  expect(remoto.texto()).toBe(local.texto());
  expect(remoto.texto()).toBe('primera versión corregida');
});

// --- Contrato de orden con el servidor ---------------------------------------

test('puntoMenor ordena por contador y desempata por actor', () => {
  expect(puntoMenor({ a: 'x', c: 1 }, { a: 'x', c: 2 })).toBe(true);
  expect(puntoMenor({ a: 'x', c: 2 }, { a: 'x', c: 1 })).toBe(false);
  expect(puntoMenor({ a: 'a', c: 5 }, { a: 'b', c: 5 })).toBe(true);
  expect(puntoMenor({ a: 'b', c: 5 }, { a: 'a', c: 5 })).toBe(false);
  expect(puntoMenor({ a: 'x', c: 3 }, { a: 'x', c: 3 })).toBe(false);
});

test('los hermanos van de mayor a menor punto, como en el servidor', () => {
  // Tres inserciones concurrentes en la MISMA posición (todas cuelgan de la
  // raíz). El servidor ordena los hermanos descendente, así que el contador más
  // alto queda primero. Estos mismos valores están en el test de Go.
  const s = new SecuenciaRGA('lector');
  const ops: OpRGA[] = [
    { kind: 'insert', dot: { a: 'ana', c: 1 }, value: 'A', lamport: 1 },
    { kind: 'insert', dot: { a: 'beto', c: 2 }, value: 'B', lamport: 2 },
    { kind: 'insert', dot: { a: 'ana', c: 3 }, value: 'C', lamport: 3 },
  ];
  for (const op of ops) s.aplicarRemota(op);
  expect(s.texto()).toBe('CBA');
});

test('empate de contador: gana el actor mayor, y va primero', () => {
  const s = new SecuenciaRGA('lector');
  for (const op of [
    { kind: 'insert', dot: { a: 'ana', c: 1 }, value: 'A', lamport: 1 } as OpRGA,
    { kind: 'insert', dot: { a: 'zoe', c: 1 }, value: 'Z', lamport: 1 } as OpRGA,
  ]) {
    s.aplicarRemota(op);
  }
  expect(s.texto()).toBe('ZA');
});

test('rehidratar desde el estado del servidor conserva orden y tumbas', () => {
  const s = SecuenciaRGA.desdeElementos('yo', [
    { dot: { a: 'ana', c: 1 }, value: 'h' },
    { dot: { a: 'ana', c: 2 }, after: { a: 'ana', c: 1 }, value: 'o' },
    { dot: { a: 'ana', c: 3 }, after: { a: 'ana', c: 2 }, value: 'X', deleted: true },
    { dot: { a: 'ana', c: 4 }, after: { a: 'ana', c: 3 }, value: 'la' },
  ]);
  expect(s.texto()).toBe('hola');
});

test('no se atasca con un ciclo de referencias', () => {
  // Un cliente malicioso o un estado corrupto pueden encadenar dos elementos
  // entre sí; el recorrido no debe girar para siempre.
  const s = new SecuenciaRGA('lector');
  s.aplicarRemota({
    kind: 'insert',
    dot: { a: 'x', c: 1 },
    after: { a: 'x', c: 2 },
    value: 'A',
    lamport: 1,
  });
  s.aplicarRemota({
    kind: 'insert',
    dot: { a: 'x', c: 2 },
    after: { a: 'x', c: 1 },
    value: 'B',
    lamport: 2,
  });
  expect(s.texto().length).toBeLessThanOrEqual(2); // termina, es lo que se prueba
});

test('escribir en medio de lo ajeno queda en su sitio', () => {
  // Quien abre un documento escrito por otro y teclea en medio tiene que ver su
  // letra ahí, y la autora también. Es `TestEscribirEnMedioDeLoAjenoQuedaEnSuSitio`
  // de `apps/nucleo/coedicion`, con los mismos valores. Se vio en ROJO fijando
  // solo el contador propio en `desdeElementos`: la «Z» salía al final.
  const ana = new SecuenciaRGA('ana');
  const ops = ana.insertarLocal(0, 'Hola mundo', 1);
  const beto = SecuenciaRGA.desdeElementos(
    'beto',
    ops.map((op) => ({
      dot: op.dot,
      ...(op.after ? { after: op.after } : {}),
      value: op.value as string,
    })),
  );
  const cambio = beto.opsDesdeTexto('HZola mundo', 100);
  expect(beto.texto()).toBe('HZola mundo');
  for (const op of cambio) ana.aplicarRemota(op);
  expect(ana.texto()).toBe('HZola mundo');
});

test('una op ajena también adelanta el contador', () => {
  // La misma regla por el otro camino: quien recibe la «Z» de antes y escribe
  // detrás de ella tiene que quedar detrás, no saltar al final.
  const beto = new SecuenciaRGA('beto');
  beto.aplicarRemota({ kind: 'insert', dot: { a: 'ana', c: 1 }, value: 'a', lamport: 1 });
  beto.aplicarRemota({
    kind: 'insert',
    dot: { a: 'ana', c: 2 },
    after: { a: 'ana', c: 1 },
    value: 'c',
    lamport: 2,
  });
  beto.opsDesdeTexto('abc', 10);
  expect(beto.texto()).toBe('abc');
});

test('puntoEn / indiceDe: el cursor anclado a un carácter sobrevive a las ediciones ajenas', () => {
  const a = new SecuenciaRGA('a');
  const b = new SecuenciaRGA('b');
  const ops = a.insertarLocal(0, 'hola', 0);
  for (const op of ops) b.aplicarRemota(op);
  // B pone el cursor tras «ho» (índice 2) y lo ancla a la «o».
  const ancla = b.puntoEn(2)!;
  expect(b.indiceDe(ancla)).toBe(2);
  // A escribe DELANTE y DETRÁS: el cursor de B se mueve con su carácter.
  for (const op of a.insertarLocal(0, 'XY', 10)) b.aplicarRemota(op);
  for (const op of a.insertarLocal(a.longitud, '!', 20)) b.aplicarRemota(op);
  expect(b.texto()).toBe('XYhola!');
  expect(b.indiceDe(ancla)).toBe(4); // tras «XYho»
  // Borrar el carácter anclado deja el cursor donde estaba
  for (const op of a.borrarLocal(3, 1, 30)) b.aplicarRemota(op); // la «o»
  expect(b.texto()).toBe('XYhla!');
  expect(b.indiceDe(ancla)).toBe(3);
  // El principio es el punto cero; un punto desconocido, null; fuera del texto, null.
  expect(b.puntoEn(0)).toEqual({ a: '', c: 0 });
  expect(b.indiceDe({ a: '', c: 0 })).toBe(0);
  expect(b.indiceDe({ a: 'zz', c: 9 })).toBeNull();
  expect(b.puntoEn(99)).toBeNull();
});
