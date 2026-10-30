// Renderer core, parsing chain and JSX reused from Cindy 779b5237917a173fd32d2a6a381c5fe61b21695b, Apache-2.0.
// File resolution, clipboard and app primitives are WinOne host adapters.
import {createElement,memo,useCallback,useEffect,useRef,useState,useMemo,isValidElement,type HTMLAttributes,type ReactNode} from 'react';
import ReactMarkdown,{defaultUrlTransform,type Components,type UrlTransform} from 'react-markdown';
import type {PluggableList} from 'unified';
import remarkGfm from 'remark-gfm';
import remarkCjkFriendly from 'remark-cjk-friendly';
import remarkMath from 'remark-math';
import rehypeKatex from 'rehype-katex';
import rehypeSlug from 'rehype-slug';
import 'katex/dist/katex.min.css';
import 'highlight.js/styles/github.css';
import {Bot,Check,Copy} from 'lucide-react';
import {cn} from '../adapter';
import {Tip} from '../panel-ui/tooltip';
import {useTranslation} from './translation';
import {copyText} from '../../clipboard';
import {ChatFileImage,ChatFileLink,ChatInlineCode} from '../../chat-file-navigation';
import {CHAT_CODE_CLASS,CHAT_CODE_SURFACE_CLASS,CHAT_ICON_BUTTON_CLASS} from '../chatChrome';
import {useReducedMotion} from '../useReducedMotion';
import {normalizeMathDelimiters} from './mathMarkdown';
import {rehypeHighlightShared} from './rehypeHighlightShared';
import remarkTruncateCjkUrls from './remarkTruncateCjkUrls';
import remarkStrictInlineMath from './remarkStrictInlineMath';
import remarkLocalPathLinks,{BARE_PATH_ATTR} from './remarkLocalPathLinks';
import remarkHtmlImages from './remarkHtmlImages';
import remarkPreserveRawLocalDestinations,{RAW_LOCAL_IMAGE_SRC_PROP,RAW_LOCAL_LINK_HREF_PROP} from './remarkPreserveRawLocalDestinations';
import remarkSessionLinks from './remarkSessionLinks';
import {rehypeMathBlockMarker} from './rehypeMathBlockMarker';
import {FENCED_CODE_PROP,rehypeFencedCodeMarker} from './rehypeFencedCodeMarker';
import {getOrCreateWordFadeState,releaseWordFadeState} from './rehypeStreamWordFade';
import {repairStreamingMarkdown} from './repairStreamingMarkdown';
import {StreamingMarkdownChunk} from './StreamingMarkdownChunk';
import {getStreamingMarkdownThrottleInterval,splitStreamingMarkdownChunks,STREAMING_MARKDOWN_THROTTLE_BASE_MS} from './streamingMarkdownChunks';
import {CopyAsImageBlock,mathBlockToLatex,tableToTsv} from './CopyAsImageBlock';
import {FitMathBlock} from './FitMathBlock';
import {MarkdownDiffBlock} from './MarkdownDiffBlock';
import {MarkdownMermaidBlock} from './MarkdownMermaidBlock';
import {InlineReferenceChip} from '../rich/InlineReferenceChip';
import {parseBotReferenceHref} from '../../../../../../../modules/chat-pi/cindy/tasks/core/agentInputProjection';
function nodeToText(node: ReactNode): string {
  if (node == null || typeof node === 'boolean') return '';
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(nodeToText).join('');
  if (isValidElement(node)) {
    const props = node.props as { children?: ReactNode };
    return nodeToText(props?.children);
  }
  return '';
}

function omitMarkdownInternalProps(props: Record<string, unknown>): Record<string, unknown> {
  const safeProps = { ...props };
  delete safeProps.className;
  delete safeProps.node;
  return safeProps;
}

/**
 * Detect whether a `pre` child element is a `<code className="language-diff">`.
 * Matches both the canonical `language-diff` token and a bare `diff` class
 * for defensive parity.
 */
function isDiffCodeChild(child: ReactNode): boolean {
  if (!isValidElement(child)) return false;
  const className = (child.props as { className?: string })?.className ?? '';
  return /\b(language-)?diff\b/.test(className);
}

/**
 * Same shape as isDiffCodeChild but for ```mermaid fenced blocks. Routed
 * through MarkdownMermaidBlock for SVG render with light/dark theme awareness.
 */
function isMermaidCodeChild(child: ReactNode): boolean {
  if (!isValidElement(child)) return false;
  const className = (child.props as { className?: string })?.className ?? '';
  return /\b(language-)?mermaid\b/.test(className);
}

export const REMARK_PLUGINS: PluggableList = [
  [remarkGfm, { singleTilde: false }],
  remarkCjkFriendly,
  remarkMath,
  remarkStrictInlineMath,
  remarkTruncateCjkUrls,
  remarkHtmlImages,
  remarkLocalPathLinks,
  remarkPreserveRawLocalDestinations,
];
export const REMARK_PLUGINS_PRIVILEGED: PluggableList = [
  [remarkGfm, { singleTilde: false }],
  remarkCjkFriendly,
  remarkMath,
  remarkStrictInlineMath,
  remarkTruncateCjkUrls,
  remarkHtmlImages,
  remarkSessionLinks,
  remarkLocalPathLinks,
  remarkPreserveRawLocalDestinations,
];
// rehypeSlug: assigns a slug-style `id` to every heading. Without it
// in-document anchor links (`[Section](#section-name)`) hit dead targets.
// Click-to-scroll behaviour is wired up at the document level below.
// rehypeKatex 必须排在 rehypeHighlight 之前:remark-math 产出的 hast 是
// `<code class="language-math ...">`,先让 katex 消费掉,否则 highlight 会往
// 里面塞 hljs span 破坏纯文本结构。strict:'ignore' 静默非致命 LaTeX 告警;
// 解析失败的公式回落为正文色原文,避免模型格式错误把普通聊天染成错误红。
// rehypeMathBlockMarker 紧随 rehypeKatex:把裸 `<span class="katex-display">`
// 包进 `<div data-math-block>`,让下方 div 渲染器能挂「复制为图片」工具栏
// (components 映射只认 tagName,认不了 class)。
// rehypeFencedCodeMarker 必须排在最后:katex 已消费掉 `$$…$$` 的 `<pre><code>`、
// highlight 已注入 hljs span,此时剩下的 `pre > code` 就是真正的代码块,给它们
// 打 data-fenced-code 供下方 code 渲染器按结构(而非语言标注)分派。
const REHYPE_PLUGINS: PluggableList = [
  rehypeSlug,
  [rehypeKatex, { strict: 'ignore', errorColor: 'inherit' }],
  rehypeMathBlockMarker,
  rehypeHighlightShared,
  rehypeFencedCodeMarker,
];

/** MarkdownRenderer 与所有“实际是否渲染”判定共用的输入归一化。 */
export function normalizeMarkdownRendererContent(
  content: string,
  preserveLineCount = false,
): string {
  return normalizeMathDelimiters(content, { preserveLineCount });
}
/**
 * 聊天正文里一切「可点」的行内元素共用这一套外观:**正文色 + 常显下划线**。
 *
 * 两条拍板规则(2026-07-30,权威正文见 docs/design-rules/DESIGN.md「聊天正文的
 * 可点性信号」):
 *   ① **下划线常显 = 可点**,且是唯一的交互信号。markdown 里粗体占了字重、斜体占了
 *      倾斜、行内 code 占了等宽+底色,下划线是唯一没被排版语义占用的通道 —— 所以把
 *      它专门留给交互语义。反过来:**没有下划线的一律不可点**,行内 code 的等宽+底色
 *      从此只表示「这是代码/路径」这一层排版含义,不再兼职表达可点。
 *      (改前:可点的 FileTargetChip 用实色底 1.26:1,不可点的行内 code 用半透明底
 *      dark 1.26~1.28:1 —— 静止状态下数值几乎相同,只能靠 hover 与指针形状区分,
 *      等于「必须拿鼠标扫一遍才知道哪个能点」。)
 *   ② **外链与本地文件不做外观区分**,只表达「可点」;去哪由文本自身可读性承担
 *      (斜杠路径 vs `https://` 前缀)。
 *
 * 因此这里**刻意不用** `--msg-link`:它是主题契约(10 个内置主题各自定义,solarized
 * 绿 / monokai 黄 / eclipse 青,用户导入 VS Code 主题时 linkColor 也映射到它),而手机端
 * 根本没有链接色概念。要「两端统一 + 外链本地同形」,聊天正文就得退出这个 token。
 * token 本身保留(其它界面仍在用),只是不再进 markdown 正文。
 */
const MARKDOWN_LINK_CLASS = 'underline underline-offset-2 cursor-pointer [overflow-wrap:anywhere]';
/**
 * markdown 行内 code —— 几何与底色对齐 GitHub(6px 圆角 + 左右内距 + 半透明淡底,
 * 见 --msg-md-inline-code-bg 的说明)。
 *
 * 底色刻意用 --msg-md-inline-code-bg 而不是 --msg-code-inline-bg:后者是实色的
 * chip / hover 底,同时被可点的 FileTargetChip 与十余处 hover:bg- 复用;行内 code
 * 若用同一个值,就和「有底色 = 可点路径 chip」这个信号撞车了。
 *
 * 移动端**刻意不同形态**(零底色 + 文字压暗):那边聊天流是 RN 嵌套 Text,不认
 * borderRadius,淡底只能是直角方块。这里是 CSS,按 GitHub 原样实现。
 */
const INLINE_CODE_CLASS = 'font-mono text-14 rounded-[6px] px-[0.4em] py-[0.2em] bg-[var(--msg-md-inline-code-bg)]';

function useStreamingThrottle(value: string, enabled: boolean): string {
  const intervalMs = enabled
    ? getStreamingMarkdownThrottleInterval(value)
    : STREAMING_MARKDOWN_THROTTLE_BASE_MS;
  const [throttled, setThrottled] = useState(value);
  const throttledRef = useRef(value);
  const latestRef = useRef(value);
  const lastEmitRef = useRef(0);
  const timerRef = useRef<number | null>(null);

  const emit = useCallback((nextValue: string, now: number) => {
    lastEmitRef.current = now;
    if (throttledRef.current === nextValue) return;
    throttledRef.current = nextValue;
    setThrottled(nextValue);
  }, []);

  useEffect(() => {
    latestRef.current = value;

    if (!enabled) {
      if (timerRef.current != null) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      // Stream just ended — flush whatever the most recent value is so
      // the final frame is never the last throttled snapshot.
      if (throttledRef.current !== value) {
        throttledRef.current = value;
        setThrottled(value);
      }
      // A new stream should get a leading frame even if it starts shortly
      // after the previous one ended.
      lastEmitRef.current = 0;
      return;
    }

    const now = performance.now();
    const elapsed = now - lastEmitRef.current;
    if (elapsed >= intervalMs) {
      if (timerRef.current != null) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      emit(value, now);
      return;
    }

    // Re-arm on every update so an interval bucket change cannot leave a
    // timer using the previous (shorter) delay. The latest ref keeps the
    // trailing edge lossless even when several batches arrive in between.
    if (timerRef.current != null) window.clearTimeout(timerRef.current);
    timerRef.current = window.setTimeout(
      () => {
        timerRef.current = null;
        emit(latestRef.current, performance.now());
      },
      Math.max(0, intervalMs - elapsed),
    );
  }, [emit, value, enabled, intervalMs]);

  useEffect(() => {
    return () => {
      if (timerRef.current != null) window.clearTimeout(timerRef.current);
    };
  }, []);

  // useEffect 在 paint 后才执行。流式结束时本次 render 直接返回最新原文，
  // effect 只负责同步内部 state，供未来可能的新流式周期使用。
  return enabled ? throttled : value;
}

function CodeBlockPre({ children, ...props }: HTMLAttributes<HTMLPreElement>) {
  const { t } = useTranslation();
  const preRef = useRef<HTMLPreElement>(null);
  const [copied, setCopied] = useState(false);
  const [copyError,setCopyError]=useState(false);
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (timerRef.current != null) window.clearTimeout(timerRef.current);
    };
  }, []);

  async function handleCopy() {
    const text = preRef.current?.textContent ?? '';
    if (!text) return;
    try {
      await copyText(text);
      setCopyError(false);
      setCopied(true);
      if (timerRef.current != null) window.clearTimeout(timerRef.current);
      timerRef.current = window.setTimeout(() => {
        setCopied(false);
        timerRef.current = null;
      }, 1500);
    } catch {
      setCopyError(true);
    }
  }

  return (
    <div className="group relative my-3">
      <pre
        ref={preRef}
        className={cn(
          CHAT_CODE_SURFACE_CLASS,
          CHAT_CODE_CLASS,
          'p-4',
          // 取消横向滚动:长行/长 token 自动折行,避免出现横滚条
          'whitespace-pre-wrap break-all',
        )}
        {...props}
      >
        {children}
      </pre>
      <Tip text={copyError ? t('chat.media.copyFailed') : copied ? t('chat.markdownRenderer.codeCopied') : t('chat.markdownRenderer.copyCode')}>
        <button
          type="button"
          onClick={handleCopy}
          aria-label={copyError ? t('chat.media.copyFailed') : copied ? t('chat.markdownRenderer.codeCopied') : t('chat.markdownRenderer.copyCode')}
          className={cn(
            CHAT_ICON_BUTTON_CLASS,
            'absolute right-2 top-2 h-7 w-7 border border-[var(--msg-code-block-border)]',
            'bg-[var(--msg-code-block-bg)] text-[var(--msg-tool-text)]',
            'opacity-0 transition-[color,background-color,opacity] duration-[var(--motion-fast)]',
            'group-hover:opacity-100 focus-visible:opacity-100',
            'enabled:hover:bg-[var(--cmd-palette-item-hover)] enabled:hover:text-[var(--msg-assistant-text)]',
          )}
        >
          {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
        </button>
      </Tip>
    </div>
  );
}

const baseComponents: Components = {
  // NOTE: the `code` renderer is NOT defined here at module level anymore.
  // Inline-code path detection (text-lightbox-trigger-extension v2) needs
  // workingDir + setLightboxSrc + setTextLightboxFile to make detected
  // paths clickable, so it lives inside the instance-level `useMemo` below
  // alongside `a` and `img`. Fenced code blocks still pass through to the
  // simpler renderer there. See line ~310 for the implementation.

  pre({ children, node: _node, ...props }) {
    // Intercept ```diff fenced blocks before they fall through to highlight.js
    // — we render them with our own row/gutter/symbol layout (MarkdownDiffBlock)
    // so the visual matches the Edit-tool DiffView card and the GitHub red/green
    // tokens (`--diff-add-fg` / `--diff-del-fg`) get applied predictably.
    // Other languages (js/ts/python/...) still go through the default pre+code
    // path with rehype-highlight.
    const firstChild = Array.isArray(children) ? children[0] : children;
    if (isDiffCodeChild(firstChild)) {
      const codeChildren = (firstChild as { props: { children?: ReactNode } })
        .props.children;
      const raw = nodeToText(codeChildren);
      return <MarkdownDiffBlock raw={raw} />;
    }
    if (isMermaidCodeChild(firstChild)) {
      const codeChildren = (firstChild as { props: { children?: ReactNode } })
        .props.children;
      const raw = nodeToText(codeChildren);
      return <MarkdownMermaidBlock raw={raw} />;
    }

    return <CodeBlockPre {...props}>{children}</CodeBlockPre>;
  },

  // Tables (GFM)。外层 CopyAsImageBlock 承载「复制为图片 / 标注」hover 工具栏,
  // overflow 容器下沉为内层(工具栏挂 overflow 容器内会被裁剪并随横滚漂移)。
  table({ children, node: _node, ...props }) {
    return (
      <CopyAsImageBlock
        className="my-3"
        contentClassName="min-w-0 max-w-full"
        extractPlainText={tableToTsv}
      >
        <table
          className={cn(
            'w-full border-collapse',
            'text-14',
          )}
          {...props}
        >
          {children}
        </table>
      </CopyAsImageBlock>
    );
  },

  // 块级 KaTeX 公式(rehypeMathBlockMarker 包装出的 div[data-math-block])。
  // 其余 div 原样透传——markdown 输出里 div 极少见,不影响其它内容。
  div({ children, node, ...props }) {
    if (node?.properties && 'dataMathBlock' in node.properties) {
      return (
        <CopyAsImageBlock extractPlainText={mathBlockToLatex}><FitMathBlock>{children}</FitMathBlock></CopyAsImageBlock>
      );
    }
    return <div {...props}>{children}</div>;
  },

  th({ children, node: _node, ...props }) {
    return (
      <th
        className={cn(
          'border border-[var(--msg-table-border)] px-3 py-2 text-left',
          'bg-[var(--msg-table-header-bg)] font-medium',
          // 长 token (文件路径、逗号分隔列表等) 没有词边界,
          // 不强制断行会撑爆 td 宽度 → 表格超过消息流容器 → 触发
          // overflow-x-auto 横滚 + .is-scrolling 自动隐藏滚动条,
          // 视觉上像被右边截断。anywhere 让浏览器在任意位置可断行。
          'break-words [overflow-wrap:anywhere]',
        )}
        {...props}
      >
        {children}
      </th>
    );
  },

  td({ children, node: _node, ...props }) {
    return (
      <td
        className={cn(
          'border border-[var(--msg-table-border)] px-3 py-2',
          'break-words [overflow-wrap:anywhere]',
        )}
        {...props}
      >
        {children}
      </td>
    );
  },

  // Lists
  ul({ children, node: _node, ...props }) {
    return (
      <ul className="my-2 ml-6 list-disc space-y-1" {...props}>
        {children}
      </ul>
    );
  },

  ol({ children, node: _node, ...props }) {
    return (
      <ol className="my-2 ml-6 list-decimal space-y-1" {...props}>
        {children}
      </ol>
    );
  },

  // Paragraphs
  p({ children, node: _node, ...props }) {
    return (
      <p className="my-2 first:mt-0 last:mb-0" {...props}>
        {children}
      </p>
    );
  },

  // NOTE: the `a` renderer is NOT defined here at module level anymore.
  // It needs to capture `workingDir` + setLightboxSrc + setTextLightboxFile
  // (text-lightbox-trigger-extension F1), so it lives inside the instance-
  // level `useMemo` below. The other 13 renderers stay module-level for the
  // existing performance protection (no inline arrays / no per-render
  // components object).

  // Headings
  //
  // 颜色走 --md-hN-fg token(默认值 `inherit`,见 themes/colors.ts):默认主题下与
  // 引入 token 前逐像素一致,外部导入的主题(Obsidian --hN-color / VSCode
  // markup.heading)才会把它染成分级色。字号字重不受 token 影响。
  h1({ children, node: _node, ...props }) {
    return <h1 className="my-3 text-20 leading-[1.4] font-medium text-[var(--md-h1-fg)]" {...props}>{children}</h1>;
  },
  h2({ children, node: _node, ...props }) {
    return <h2 className="my-3 text-18 leading-[1.556] font-medium text-[var(--md-h2-fg)]" {...props}>{children}</h2>;
  },
  h3({ children, node: _node, ...props }) {
    return <h3 className="my-2 text-16 leading-[1.5] font-medium text-[var(--md-h3-fg)]" {...props}>{children}</h3>;
  },
  // h4-h6 此前没有 renderer(走 react-markdown 默认的裸 tag,字号字重由 Tailwind
  // preflight 归一成继承)。这里只补颜色 class,刻意不加字号/字重/间距 —— 保持
  // 原有观感不变。
  h4({ children, node: _node, ...props }) {
    return <h4 className="text-[var(--md-h4-fg)]" {...props}>{children}</h4>;
  },
  h5({ children, node: _node, ...props }) {
    return <h5 className="text-[var(--md-h5-fg)]" {...props}>{children}</h5>;
  },
  h6({ children, node: _node, ...props }) {
    return <h6 className="text-[var(--md-h6-fg)]" {...props}>{children}</h6>;
  },

  // Content strong is absolute 700: nested emphasis must not accumulate to 900.
  strong({ children, node: _node, ...props }) {
    return <strong className="font-bold text-[var(--md-strong-fg)]" {...props}>{children}</strong>;
  },

  // Blockquote
  blockquote({ children, node: _node, ...props }) {
    return (
      <blockquote
        className={cn(
          // border-l-2 + --msg-blockquote-border(= --agent-actions-rail):与
          // WorkGroupBlock / ThinkingCard / AgentActionsBlock 的 left rail 完全
          // 统一,宽度和颜色都不在引用块这里另搞一套。
          // 不用斜体:中文没有真斜体,机械倾斜反而降低可读性;引用语义由 rail +
          // 内缩表达已经足够。
          'my-2 border-l-2 border-[var(--msg-blockquote-border)] pl-4',
          'text-[var(--msg-blockquote-text)]',
        )}
        {...props}
      >
        {children}
      </blockquote>
    );
  },

  // Horizontal rule
  hr({node: _node,...props}) {
    return <hr className="my-4 border-[var(--msg-hr-border)]" {...props} />;
  },
};

type BlockTag = 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6' | 'p' | 'pre' | 'table' | 'ul' | 'ol' | 'li' | 'blockquote' | 'hr';

interface MdastPositionedNode {
  position?: { start?: { line?: number } };
}

function lineAttrFor(node: unknown): { 'data-source-line'?: number } {
  const line = (node as MdastPositionedNode | undefined)?.position?.start?.line;
  return typeof line === 'number' ? { 'data-source-line': line } : {};
}

function wrapWithSourceLine<T extends BlockTag>(
  tag: T,
  base: Components[T] | undefined,
): Components[T] {
  // 类型上 react-markdown 的每个 tag 接收的 props 形态略有差异, 这里走 unknown
  // 通用桥接: 拿到 node 注入 lineAttr, 再透传给 base 或原生 tag。
  const wrapped = (props: Record<string, unknown>) => {
    const { node, children, ...rest } = props as { node?: unknown; children?: ReactNode } & Record<string, unknown>;
    const lineAttr = lineAttrFor(node);
    if (base) {
      // 把 lineAttr 加进 props 一起传给 base — base 内部 ...props spread 时
      // data-source-line 会落到根元素上, 不会污染 base 自己的 className/handler。
      return (base as unknown as (p: Record<string, unknown>) => ReactNode)({
        node,
        children,
        ...rest,
        ...lineAttr,
      });
    }
    // base 缺省时直接生成原生 tag。children 单独取出避免被 spread 进 attrs。
    return createElement(tag, { ...rest, ...lineAttr }, children);
  };
  return wrapped as unknown as Components[T];
}

function makeSourceLineWrappers(): Partial<Components> {
  return {
    h1: wrapWithSourceLine('h1', baseComponents.h1),
    h2: wrapWithSourceLine('h2', baseComponents.h2),
    h3: wrapWithSourceLine('h3', baseComponents.h3),
    h4: wrapWithSourceLine('h4', baseComponents.h4),
    h5: wrapWithSourceLine('h5', baseComponents.h5),
    h6: wrapWithSourceLine('h6', baseComponents.h6),
    p: wrapWithSourceLine('p', baseComponents.p),
    pre: wrapWithSourceLine('pre', baseComponents.pre),
    table: wrapWithSourceLine('table', baseComponents.table),
    ul: wrapWithSourceLine('ul', baseComponents.ul),
    ol: wrapWithSourceLine('ol', baseComponents.ol),
    li: wrapWithSourceLine('li', baseComponents.li),
    blockquote: wrapWithSourceLine('blockquote', baseComponents.blockquote),
    hr: wrapWithSourceLine('hr', baseComponents.hr),
  };
}


export interface MarkdownRendererProps {
 content:string;
 isStreaming?:boolean;
 streamFadeKey?:string;
 emitSourceLines?:boolean;
 allowPrivilegedLinks?:boolean;
}

// A teammate reference (Cindy's bot deep link) stays intact for its chip below.
const trustedUrlTransform:UrlTransform=(url,key)=>/^(?:file:\/\/|[A-Za-z]:[\\/])/.test(url)||(key==='src'&&/^data:image\//i.test(url))||key==='href'&&parseBotReferenceHref(url)?url:defaultUrlTransform(url);
// Relative images still use the saved conversation's scoped preview resolver.
// Disabling privileged links must not remove document images from rich diffs.
const previewSafeUrlTransform:UrlTransform=(url,key)=>key==='src'&&/^data:image\//i.test(url)?url:defaultUrlTransform(url);

// WinOne supplies an already-scoped file/navigation context. Parsing and
// visual components remain the Cindy implementation on every reading surface.
export const MarkdownRenderer=memo(function MarkdownRenderer({content,isStreaming=false,streamFadeKey,emitSourceLines=false,allowPrivilegedLinks=true}:MarkdownRendererProps){
 const throttledContent=useStreamingThrottle(content,isStreaming);
 const reducedMotion=useReducedMotion();
 const streamFade=isStreaming&&!reducedMotion;
 const wordFadeState=useMemo(()=>streamFade?getOrCreateWordFadeState(streamFadeKey):null,[streamFade,streamFadeKey]);
 useEffect(()=>{if(!isStreaming)releaseWordFadeState(streamFadeKey);},[isStreaming,streamFadeKey]);
 const repaired=useMemo(()=>streamFade?repairStreamingMarkdown(throttledContent):throttledContent,[streamFade,throttledContent]);
 const renderedContent=useMemo(()=>normalizeMarkdownRendererContent(repaired,emitSourceLines),[repaired,emitSourceLines]);
 const chunks=useMemo(()=>isStreaming&&!emitSourceLines?splitStreamingMarkdownChunks(renderedContent):[{start:0,content:renderedContent}],[renderedContent,isStreaming,emitSourceLines]);
 const components=useMemo<Components>(()=>({
  ...baseComponents,...(emitSourceLines?makeSourceLineWrappers():{}),
  a:({children,href,node,...props})=>{
   const raw=props as Record<string,unknown>,safe=omitMarkdownInternalProps(raw);
   const original=raw[RAW_LOCAL_LINK_HREF_PROP]??node?.properties?.[RAW_LOCAL_LINK_HREF_PROP];
   delete safe[BARE_PATH_ATTR];delete safe[RAW_LOCAL_LINK_HREF_PROP];
   // Cindy SentAgentReferenceChip: a referenced teammate is a read-only chip with its name.
   const bot=allowPrivilegedLinks&&typeof href==='string'?parseBotReferenceHref(href):null;
   if(bot){
    const label=nodeToText(children).replace(/\s+/g,' ').trim()||bot.botId;
    return <InlineReferenceChip label={label} icon={<Bot aria-hidden/>} tooltip={label} ariaLabel={label} className="relative top-[-1px] -my-[1px] max-w-[min(240px,55vw)] align-middle"/>;
   }
   return <ChatFileLink {...safe} chip href={allowPrivilegedLinks&&typeof original==='string'?original:href}>{children}</ChatFileLink>;
  },
  img:({src,alt,node,...props})=>{
   const original=node?.properties?.[RAW_LOCAL_IMAGE_SRC_PROP];
   const safe={...props} as Record<string,unknown>;delete safe[RAW_LOCAL_IMAGE_SRC_PROP];
   return <ChatFileImage {...safe} src={allowPrivilegedLinks&&typeof original==='string'?original:src} alt={alt}/>;
  },
  code:({className,children,node,...props})=>{
   if(node?.properties?.[FENCED_CODE_PROP]!==undefined||className)return <code className={cn(className,'font-mono text-[length:var(--app-code-font-size)]')} {...props}>{children}</code>;
   return allowPrivilegedLinks?<ChatInlineCode inline className={INLINE_CODE_CLASS}>{children}</ChatInlineCode>:<code className={INLINE_CODE_CLASS}>{children}</code>;
  },
 }),[emitSourceLines,allowPrivilegedLinks]);
 return <div className="cindy-chat-ui msg-markdown select-text">
  {isStreaming?chunks.map(chunk=><StreamingMarkdownChunk key={chunk.start} sourceKey={String(chunk.start)} content={chunk.content} remarkPlugins={allowPrivilegedLinks?REMARK_PLUGINS_PRIVILEGED:REMARK_PLUGINS} rehypePlugins={REHYPE_PLUGINS} components={components} urlTransform={allowPrivilegedLinks?trustedUrlTransform:previewSafeUrlTransform} wordFadeState={wordFadeState} emitSourceLines={emitSourceLines} wholeDocument={chunks.length===1}/>):<ReactMarkdown remarkPlugins={allowPrivilegedLinks?REMARK_PLUGINS_PRIVILEGED:REMARK_PLUGINS} rehypePlugins={REHYPE_PLUGINS} components={components} urlTransform={allowPrivilegedLinks?trustedUrlTransform:previewSafeUrlTransform} skipHtml>{renderedContent}</ReactMarkdown>}
 </div>;
});
