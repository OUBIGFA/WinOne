import {fileURLToPath,URL} from "node:url";
import {defineConfig,type UserConfig} from "vite";
import base from "./vite.config";

// The MyGo window embeds only the Chat tools page (chat-embed.html) and what
// it loads, into its Go package (apps/desktop-mygo/webui/dist). The input
// replaces the settings window's entries rather than merging with them.
const config=base as UserConfig;
export default defineConfig({
 ...config,
 build:{
  ...config.build,
  outDir:fileURLToPath(new URL("../../desktop-mygo/webui/dist",import.meta.url)),
  emptyOutDir:true,
  rollupOptions:{...config.build?.rollupOptions,input:{chatEmbed:fileURLToPath(new URL("./chat-embed.html",import.meta.url))}},
 },
});
