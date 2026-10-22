// Compatibilidad para desarrolladores que ejecutan `bun serve.mjs` desde esta
// carpeta. La implementación canónica también es la que copia el Dockerfile.
import { startServer } from '../../static-serve.mjs';

startServer();
