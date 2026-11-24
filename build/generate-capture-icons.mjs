// Rasterizes the locked Solar Linear icons for the native capture toolbar
// (modules/capture/NativeUI/Assets/Icons) and the tray menus
// (third_party/systray/assets, internal/branding/menu-icons), as mapped in
// third_party/solar-icons.json (surfaces.capture and surfaces.tray).
//
//   node build/generate-capture-icons.mjs              # toolbar and tray
//   node build/generate-capture-icons.mjs --tray-only  # tray menus only
import {createRequire} from "node:module";
import {mkdir, writeFile, rm} from "node:fs/promises";
import {existsSync} from "node:fs";
import {resolve} from "node:path";
import {root, catalog, solarSVG, solarBody, licenseText} from "./solar-icons.mjs";

const require = createRequire(import.meta.url);
const {chromium} = require(process.env.WINONE_PLAYWRIGHT_MODULE || "playwright");
const names = catalog.surfaces.capture.icons;
const tray = catalog.surfaces.tray.icons;
const trayOnly = process.argv.includes("--tray-only");
const license = licenseText();

function toolbarSVG(id, size, color) {
  // Solar has no GIF mark; the toolbar shows the format's name.
  if (id === "gif") return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24"><text x="12" y="15.5" text-anchor="middle" font-family="Segoe UI, sans-serif" font-weight="600" font-size="10" fill="${color}">GIF</text></svg>`;
  const name = names[id];
  if (!name) throw Error("Capture icon has no Solar mapping: " + id);
  let body = solarBody(name);
  // Solar has no numbered badge: its circle holds a "1" in the same stroke.
  if (id === "step") body += '<path d="M10.5 8.5H12v7M10 15.5h4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>';
  // Pinned state turns the same pin by 45 degrees, as the result windows do.
  if (id === "pin-active") body = `<g transform="rotate(45 12 12)">${body}</g>`;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" color="${color}">${body}</svg>`;
}

const context = await chromium.launchPersistentContext(resolve(root, "_temp/native-icon-generator-" + Date.now()), {headless: true, executablePath: process.env.WINONE_CHROMIUM_PATH});
try {
  const page = await context.newPage();
  const rasterize = (svg, size) => page.evaluate(async ({svg, size}) => {
    const image = new Image();
    image.src = "data:image/svg+xml," + encodeURIComponent(svg);
    await image.decode();
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = size;
    canvas.getContext("2d").drawImage(image, 0, 0, size, size);
    return canvas.toDataURL("image/png").split(",")[1];
  }, {svg, size}).then(data => Buffer.from(data, "base64"));

  if (!trayOnly) {
    const output = resolve(root, "modules/capture/NativeUI/Assets/Icons");
    await mkdir(output, {recursive: true});
    for (const id of [...Object.keys(names), "gif"]) for (const size of [18, 24, 36, 48, 72]) for (const [suffix, color] of [["", "#171717"], ["-dark", "#fafafa"]]) {
      await writeFile(resolve(output, `${id}${suffix}-${size}.png`), await rasterize(toolbarSVG(id, size, color), size));
    }
    await writeFile(resolve(output, "LICENSE.txt"), license);
    await writeFile(resolve(output, "README.md"), `# 截图工具栏图标\n\n由锁定的 Solar Linear（${catalog.source.repository} @ ${catalog.source.commit.slice(0, 7)}）按 \`third_party/solar-icons.json\` 的 capture 映射生成，许可证见 LICENSE.txt。置顶状态沿用 Pin 的 45 度旋转；GIF 使用文字标识；序号图标为 Solar 圆形内加同线宽的数字 1。在仓库运行 node build/generate-capture-icons.mjs 可重新生成。运行软件时无需浏览器或联网。\n`);
  }

  // Rasterize the official path, never a text-font approximation. A large
  // local source supports fractional DPI; the native menu filters it to 16 DIP.
  const trayOutput = resolve(root, "third_party/systray/assets");
  await mkdir(trayOutput, {recursive: true});
  for (const [theme, color] of [["light", "#262626"], ["dark", "#e9e9e9"]]) {
    await writeFile(resolve(trayOutput, `chevron-right-${theme}.png`), await rasterize(solarSVG(tray["chevron-right"], {size: 96, color}), 96));
  }
  await writeFile(resolve(trayOutput, "Solar-LICENSE.txt"), license);
  const retired = resolve(trayOutput, "Lucide-LICENSE.txt");
  if (existsSync(retired)) await rm(retired);

  // Alpha templates: the popup's theme/DPI snapshot tints them, and the
  // neutral source stays readable in the native contrast fallback.
  const hostOutput = resolve(root, "internal/branding/menu-icons");
  await mkdir(hostOutput, {recursive: true});
  for (const id of ["open", "exit"]) {
    await writeFile(resolve(hostOutput, `${id}.png`), await rasterize(solarSVG(tray[id], {size: 96, color: "#737373"}), 96));
  }
  await writeFile(resolve(hostOutput, "LICENSE.txt"), license);
  console.log("Generated local Solar PNG icons.");
} finally {
  await context.close();
}
