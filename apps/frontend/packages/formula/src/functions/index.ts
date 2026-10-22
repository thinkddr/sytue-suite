/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Fusiona las 10 tablas de funciones (una por familia) en un único Registry.
 * Los agentes rellenan SOLO su fichero de familia; este índice no se toca.
 */
import { Registry } from '../registry.js';
import { funcs as math } from './math.js';
import { funcs as mathtrig } from './mathtrig.js';
import { funcs as stats } from './stats.js';
import { funcs as statsif } from './statsif.js';
import { funcs as logical } from './logical.js';
import { funcs as text } from './text.js';
import { funcs as lookup } from './lookup.js';
import { funcs as datetime } from './datetime.js';
import { funcs as financial } from './financial.js';
import { funcs as information } from './information.js';
import { funcs as array } from './array.js';
import { funcs as engineering } from './engineering.js';
import { funcs as database } from './database.js';
import { funcs as aggregate } from './aggregate.js';
import { funcs as ets } from './ets.js';

/** Construye un registro nuevo con todas las funciones de la librería. */
export function buildRegistry(): Registry {
  return new Registry()
    .add(math)
    .add(mathtrig)
    .add(stats)
    .add(statsif)
    .add(logical)
    .add(text)
    .add(lookup)
    .add(datetime)
    .add(financial)
    .add(information)
    .add(array)
    .add(engineering)
    .add(database)
    .add(aggregate)
    .add(ets);
}
