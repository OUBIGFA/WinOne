import {fileURLToPath,URL} from "node:url";
import {defineConfig} from "vite";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
 plugins:[tailwindcss()],
 resolve:{alias:{"@":fileURLToPath(new URL("./src",import.meta.url))}},
 build:{rollupOptions:{input:{settings:fileURLToPath(new URL("./index.html",import.meta.url)),capture:fileURLToPath(new URL("./capture.html",import.meta.url))}}},
});
