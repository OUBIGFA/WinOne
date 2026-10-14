import { defineConfig } from "vitest/config";
import {fileURLToPath,URL} from "node:url";

// Bound concurrent DOM environments so full builds do not time out otherwise
// passing interactions when every CPU core starts a separate browser fixture.
// The AI settings page mounts every service tab. Full builds also run native
// compilers beside it, so the default interaction budget needs headroom.
export default defineConfig({resolve:{alias:{"@":fileURLToPath(new URL("./src",import.meta.url))}}, server:{fs:{allow:[fileURLToPath(new URL(".",import.meta.url)),fileURLToPath(new URL("../../result-ui/assets/providers",import.meta.url)),fileURLToPath(new URL("../../result-ui/assets/provider-bitmaps",import.meta.url))]}}, test: { maxWorkers: 4, testTimeout: 20000, setupFiles: ["./tests/setup.ts"] } });
