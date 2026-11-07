import test from 'node:test';
import assert from 'node:assert/strict';
import {convert} from './convert.mjs';

test('formulas are self-contained vector shapes', () => {
  for (const tex of ['x^2+\\frac{a}{b}', '\\begin{cases}a&b\\\\c&d\\end{cases}', '\\begin{pmatrix}1&2\\\\3&4\\end{pmatrix}', '\\sum_{i=0}^n\\int_0^1\\sqrt{x}\\,dx', '\\mathcal{L}\\mathbb{R}\\mathfrak{g}\\hookrightarrow', '\\ce{H2O}', '\\boldsymbol{\\alpha}\\cancel{x}', '\\mathscr{F}\\mathsf{x}\\mathtt{y}']) {
    const value = convert(tex, true);
    assert.match(value.svg, /<path/, tex);
    assert.doesNotMatch(value.svg, /<use|<text|<foreignObject|merror/, tex);
    assert.ok(value.width > 0 && value.ascent > 0, tex);
  }
});

test('inline formulas keep every term rather than stopping at the first atom', () => {
  for (const [tex, minimumPaths] of [['E=mc^2',5], ['\\alpha+\\beta',3]]) {
    const value=convert(tex,false);
    assert.ok((value.svg.match(/<path/g)??[]).length>=minimumPaths, value.svg);
    assert.ok(value.width>2, 'A complete multi-term formula needs more than two em: '+tex);
  }
});

test('the baseline splits the height into what stands above and hangs below', () => {
  const plain = convert('x'), fraction = convert('\\frac{a}{b}', true);
  assert.ok(plain.ascent < 0.6 && plain.descent >= 0 && plain.descent < 0.2);
  assert.ok(fraction.ascent > plain.ascent && fraction.descent > plain.descent);
});

test('text outside the fonts becomes runs at their laid out place', () => {
  const value = convert('\\text{速度}=v');
  assert.doesNotMatch(value.svg, /<text/);
  assert.equal(value.texts.length, 1);
  assert.equal(value.texts[0].text, '速度');
  assert.ok(value.texts[0].size >= 800 && value.texts[0].size <= 1200);
  assert.ok(value.texts[0].x >= 0 && value.texts[0].x < value.box[2]);
});

test('text in a writing system without font data is still a run, not a failure', () => {
  assert.equal(convert('\\text{שלום}').texts.length, 1);
});

test('malformed or oversized TeX fails rather than drawing an error', () => {
  assert.throws(() => convert('\\undefinedcommand{x}'));
  assert.throws(() => convert('\\frac{a'));
  assert.throws(() => convert(' '));
  assert.throws(() => convert('x'.repeat(20000)));
});
