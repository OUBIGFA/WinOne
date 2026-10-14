import {fileURLToPath,URL} from "node:url";
import {defineConfig} from "vite";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
 plugins:[tailwindcss()],
 resolve:{alias:{"@":fileURLToPath(new URL("./src",import.meta.url))}},
 // Share only the pinned provider artwork with the native result UI.
 server:{fs:{allow:[fileURLToPath(new URL(".",import.meta.url)),fileURLToPath(new URL("../../result-ui/assets/providers",import.meta.url)),fileURLToPath(new URL("../../result-ui/assets/provider-bitmaps",import.meta.url)),fileURLToPath(new URL("../../../internal/branding",import.meta.url))]}},
 build:{rollupOptions:{input:{settings:fileURLToPath(new URL("./index.html",import.meta.url)),capture:fileURLToPath(new URL("./capture.html",import.meta.url))}}},
});
