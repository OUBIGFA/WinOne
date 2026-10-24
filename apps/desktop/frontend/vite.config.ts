import {fileURLToPath,URL} from "node:url";
import {defineConfig} from "vite";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
 cacheDir:process.env.WINONE_VITE_CACHE_DIRECTORY,
 plugins:[tailwindcss()],
 resolve:{alias:{"@":fileURLToPath(new URL("./src",import.meta.url))}},
 // Share only the pinned provider artwork with the native result UI.
 server:{fs:{allow:[fileURLToPath(new URL(".",import.meta.url)),fileURLToPath(new URL("../../result-ui/assets/providers",import.meta.url)),fileURLToPath(new URL("../../result-ui/assets/provider-bitmaps",import.meta.url)),fileURLToPath(new URL("../../../internal/branding",import.meta.url))]}},
 // Small font subsets must remain same-origin files under default-src 'self'.
 build:{assetsInlineLimit:path=>/\.(?:woff2?|ttf|otf)$/i.test(path)?false:undefined,rollupOptions:{input:{settings:fileURLToPath(new URL("./index.html",import.meta.url)),capture:fileURLToPath(new URL("./capture.html",import.meta.url)),interconnectChat:fileURLToPath(new URL("./interconnect-chat.html",import.meta.url))}}},
});
