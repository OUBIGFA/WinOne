import {defineConfig} from "vite";
import {fileURLToPath} from "node:url";
import base from "../vite.config.ts";

// Dedicated prototype build; no product HTML or output directory is changed.
export default defineConfig({...base,root:fileURLToPath(new URL("../",import.meta.url)),build:{outDir:fileURLToPath(new URL("../../../../_temp/native-capture-host-probe-1002/assets",import.meta.url)),emptyOutDir:false,rollupOptions:{input:fileURLToPath(new URL("./native-toolbar.html",import.meta.url))}}});
