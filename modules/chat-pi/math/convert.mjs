// TeX to vector shapes with MathJax. Cindy draws math with KaTeX, whose output
// needs a browser; MathJax lays out without one and covers the same TeX. Every
// glyph is an inline path, so the native view paints the result without a font.
import {mathjax} from '@mathjax/src/js/mathjax.js';
import {TeX} from '@mathjax/src/js/input/tex.js';
import {SVG} from '@mathjax/src/js/output/svg.js';
import {liteAdaptor} from '@mathjax/src/js/adaptors/liteAdaptor.js';
import {RegisterHTMLHandler} from '@mathjax/src/js/handlers/html.js';
import '@mathjax/src/js/input/tex/base/BaseConfiguration.js';
import '@mathjax/src/js/input/tex/ams/AmsConfiguration.js';
import '@mathjax/src/js/input/tex/newcommand/NewcommandConfiguration.js';
import '@mathjax/src/js/input/tex/boldsymbol/BoldsymbolConfiguration.js';
import '@mathjax/src/js/input/tex/braket/BraketConfiguration.js';
import '@mathjax/src/js/input/tex/cancel/CancelConfiguration.js';
import '@mathjax/src/js/input/tex/cases/CasesConfiguration.js';
import '@mathjax/src/js/input/tex/color/ColorConfiguration.js';
import '@mathjax/src/js/input/tex/enclose/EncloseConfiguration.js';
import '@mathjax/src/js/input/tex/extpfeil/ExtpfeilConfiguration.js';
import '@mathjax/src/js/input/tex/mathtools/MathtoolsConfiguration.js';
import '@mathjax/src/js/input/tex/mhchem/MhchemConfiguration.js';
import '@mathjax/src/js/input/tex/physics/PhysicsConfiguration.js';
import '@mathjax/src/js/input/tex/unicode/UnicodeConfiguration.js';
import '@mathjax/src/js/input/tex/upgreek/UpgreekConfiguration.js';
import {MathJaxNewcmFont} from '@mathjax/mathjax-newcm-font/js/svg.js';
// Loaded here, as a bundled worker has nothing to fetch ranges from. The ranges
// of other writing systems are left out: their text is a run of the UI font.
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/accents.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/accents-b-i.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/arrows.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/calligraphic.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/double-struck.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/fraktur.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/greek.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/greek-ss.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/latin.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/latin-b.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/latin-bi.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/latin-i.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/marrows.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/math.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/monospace.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/monospace-ex.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/monospace-l.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/mshapes.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/sans-serif.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/sans-serif-b.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/sans-serif-bi.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/sans-serif-ex.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/sans-serif-i.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/sans-serif-r.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/script.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/shapes.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/symbols.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/symbols-b-i.js';
import '@mathjax/mathjax-newcm-font/js/svg/dynamic/variants.js';

const MAX_TEX = 16 << 10;
// The ranges are loaded above, so MathJax's load step has nothing to do and it
// applies a range synchronously the first time a formula needs it.
mathjax.asyncLoad = () => undefined;
mathjax.asyncIsSynchronous = true;
const adaptor = liteAdaptor();
RegisterHTMLHandler(adaptor);
const input = new TeX({
  packages: ['base', 'ams', 'newcommand', 'boldsymbol', 'braket', 'cancel', 'cases', 'color', 'enclose', 'extpfeil', 'mathtools', 'mhchem', 'physics', 'unicode', 'upgreek'],
  formatError(_jax, error) { throw error; }, // The view shows the source instead.
});
// Native inline math is one baseline object. Browser-style inline line breaks
// split MathJax output into sibling SVGs, and taking the first would drop terms.
const output = new SVG({fontCache: 'none', fontData: MathJaxNewcmFont, linebreaks: {inline: false}});
const document = mathjax.document('', {InputJax: input, OutputJax: output});

const multiply = (a, b) => [
  a[0] * b[0] + a[2] * b[1], a[1] * b[0] + a[3] * b[1],
  a[0] * b[2] + a[2] * b[3], a[1] * b[2] + a[3] * b[3],
  a[0] * b[4] + a[2] * b[5] + a[4], a[1] * b[4] + a[3] * b[5] + a[5],
];

function matrixOf(node) {
  let matrix = [1, 0, 0, 1, 0, 0];
  const x = Number(adaptor.getAttribute(node, 'x')), y = Number(adaptor.getAttribute(node, 'y'));
  if (adaptor.kind(node) === 'svg' && (x || y)) matrix = [1, 0, 0, 1, x || 0, y || 0];
  for (const [, name, values] of (adaptor.getAttribute(node, 'transform') ?? '').matchAll(/(\w+)\(([^)]*)\)/g)) {
    const v = values.split(/[\s,]+/).filter(Boolean).map(Number);
    if (name === 'translate') matrix = multiply(matrix, [1, 0, 0, 1, v[0] ?? 0, v[1] ?? 0]);
    else if (name === 'scale') matrix = multiply(matrix, [v[0], 0, 0, v[1] ?? v[0], 0, 0]);
    else if (name === 'matrix') matrix = multiply(matrix, v);
    else throw Error('Unsupported formula transform');
  }
  return matrix;
}

// Text a font has no glyph for, as Chinese in \text{}, leaves the shapes and
// becomes a run the native view draws in the UI font at the laid out place.
function takeText(node, matrix, runs) {
  for (const child of [...adaptor.childNodes(node)]) {
    if (adaptor.kind(child) === '#text') continue;
    const at = multiply(matrix, matrixOf(child));
    if (adaptor.kind(child) === 'text') {
      const text = adaptor.textContent(child);
      const size = Number(adaptor.getAttribute(child, 'font-size')?.replace('px', '')) || 1000;
      runs.push({text, x: at[4], y: at[5], size: size * Math.abs(at[3]),
        italic: (adaptor.getAttribute(child, 'font-style') ?? '') === 'italic',
        bold: (adaptor.getAttribute(child, 'font-weight') ?? '') === 'bold'});
      adaptor.remove(child);
    } else takeText(child, at, runs);
  }
}

// Sizes are in em, from the viewBox's 1000 units an em: the width, what stands
// above the baseline and what hangs below it. Runs are in the viewBox's units.
export function convert(tex, display = false) {
  if (typeof tex !== 'string' || !tex.trim() || tex.length > MAX_TEX) throw Error('Invalid formula');
  const container = document.convert(tex, {display});
  const svg = adaptor.firstChild(container);
  const box = (adaptor.getAttribute(svg, 'viewBox') ?? '').split(/\s+/).map(Number);
  if (adaptor.kind(svg) !== 'svg' || box.length !== 4 || box.some(value => !Number.isFinite(value))) throw Error('Invalid formula output');
  const texts = [];
  takeText(svg, [1, 0, 0, 1, 0, 0], texts);
  adaptor.removeAttribute(svg, 'style');
  adaptor.removeAttribute(svg, 'width');
  adaptor.removeAttribute(svg, 'height');
  return {svg: adaptor.serializeXML(svg), box, width: box[2] / 1000, ascent: -box[1] / 1000, descent: (box[1] + box[3]) / 1000, texts};
}
