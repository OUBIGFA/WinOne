// Adapted from Cindy, Copyright 2026 XD Inc., Apache-2.0.
// Source and WinOne adapter changes: third_party/cindy/ui-source.json.



/**
 * Composer 纯文本 Markdown 列表的兼容接续。结构化 bullet / ordered list
 * 由编辑器 schema 处理；本模块继续覆盖粘贴或旧草稿中的文本前缀，以及尚未
 * 结构化的待办和引用前缀。Shift/Alt+Enter 接续或退出当前项，普通 Enter
 * 始终保持发送语义。
 */

export type ListContinuation =
  | { action: 'continue'; insert: string }
  | { action: 'exit' };

export interface ListPrefixMatch {
  /** 前缀(含缩进与其后空白)在行内占据的长度。 */
  prefixLength: number;
  /** 换行后应插入的下一个前缀(缩进 / 分隔符 / 空白与当前行保持一致)。 */
  nextPrefix: string;
}

// 顺序敏感:checkbox 必须先于 bullet 匹配(`- [ ] ` 也满足 bullet 的模式)。
// checkbox 允许行在 `]` 处截止(`(\s+|$)`),这样 "- [ ]" 也能识别成空项退出。
const CHECKBOX_RE = /^(\s*)([-+*])(\s+)\[[ xX]\](\s+|$)/;
const BULLET_RE = /^(\s*)([-+*•])(\s+)/;
const ORDERED_RE = /^(\s*)(\d{1,9})([.)])(\s+)/;
// 中文顿号序号(`1、`)不要求后随空格,符合中文输入习惯。
const ORDERED_CJK_RE = /^(\s*)(\d{1,9})(、)(\s*)/;
const QUOTE_RE = /^(\s*)(>)(\s+)/;

/**
 * 匹配行首的列表 / 待办 / 引用前缀。除列表接续外,也被 composer 的列表行
 * 缩进 decoration(ComposerListIndentDecoration)复用来判断"这行是列表项"。
 */
export function matchListPrefix(line: string): ListPrefixMatch | null {
  let m = line.match(CHECKBOX_RE);
  if (m) {
    // 新项永远是未勾选态;`]` 后无空白(行尾截止)时补一个空格。
    return {
      prefixLength: m[0].length,
      nextPrefix: `${m[1]}${m[2]}${m[3]}[ ]${m[4] || ' '}`,
    };
  }
  m = line.match(BULLET_RE);
  if (m) return { prefixLength: m[0].length, nextPrefix: m[0] };
  m = line.match(ORDERED_RE);
  if (m) {
    return {
      prefixLength: m[0].length,
      nextPrefix: `${m[1]}${Number(m[2]) + 1}${m[3]}${m[4]}`,
    };
  }
  m = line.match(ORDERED_CJK_RE);
  if (m) {
    return {
      prefixLength: m[0].length,
      nextPrefix: `${m[1]}${Number(m[2]) + 1}${m[3]}${m[4]}`,
    };
  }
  m = line.match(QUOTE_RE);
  if (m) return { prefixLength: m[0].length, nextPrefix: m[0] };
  return null;
}

/**
 * 根据"光标前的当前行文本"决定换行行为:
 * - `null`      → 不是列表行,走默认换行。
 * - `continue`  → 插入换行 + `insert` 前缀。
 * - `exit`      → 当前项为空,调用方应删除光标前的整段行文本(即前缀)退出列表。
 *
 * 行文本中的原子节点(mention chip 等)由调用方以 U+FFFC 占位表示——行首是
 * chip 时天然匹配不上任何前缀,无需特判。
 */
export function computeListContinuation(
  lineBeforeCaret: string,
  lineAfterCaret = '',
): ListContinuation | null {
  const match = matchListPrefix(lineBeforeCaret);
  if (!match) return null;
  const rest = lineBeforeCaret.slice(match.prefixLength);
  // 空项判定看**整行**(光标前 rest + 光标后正文):否则 `1. |todo`(光标在标记后、
  // 正文前)会被当成空项退出、删掉标记留下裸 "todo"。光标后仍有正文时
  // 走 continue —— 在光标处插换行 + 下一前缀,把 "todo" 顺成下一项(拆分列表)。
  if (rest.trim().length === 0 && lineAfterCaret.trim().length === 0) return { action: 'exit' };
  return { action: 'continue', insert: match.nextPrefix };
}

/**
 * 提取光标所在"行"里、光标之前的文本与行起点(段落内以 hardBreak 划分行)。
 * 原子节点(mention chip 等)以 U+FFFC 占位——行首是 chip 时天然匹配不上
 * 任何前缀,无需特判。lineStartOffset 是相对段落内容起点的 offset。
 */
