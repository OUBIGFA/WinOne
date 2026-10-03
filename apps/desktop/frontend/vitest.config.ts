import { defineConfig } from "vitest/config";
import {fileURLToPath,URL} from "node:url";

// Bound concurrent DOM environments so full builds do not time out otherwise
// passing interactions when every CPU core starts a separate browser fixture.
export default defineConfig({resolve:{alias:{"@":fileURLToPath(new URL("./src",import.meta.url))}}, test: { maxWorkers: 4, setupFiles: ["./tests/setup.ts"] } });
