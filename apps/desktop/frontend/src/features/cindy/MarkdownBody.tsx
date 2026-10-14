// MarkdownRenderer base renderers adapted from Cindy a152497a, Apache-2.0.
// WinOne supplies copy actions. Horizontal wrapping follows the upstream code
// block rule and is also applied to tables, as required by the host UI.
import type {Components} from "react-markdown";
export const markdownBody:Components={
 p:({children,node:_,...props})=><p className="my-2 first:mt-0 last:mb-0" {...props}>{children}</p>,
 ul:({children,node:_,...props})=><ul className="my-2 ml-6 list-disc space-y-1" {...props}>{children}</ul>,
 ol:({children,node:_,...props})=><ol className="my-2 ml-6 list-decimal space-y-1" {...props}>{children}</ol>,
 h1:({children,node:_,...props})=><h1 className="my-3 text-[20px] leading-[1.4] font-medium text-[var(--md-h1-fg)]" {...props}>{children}</h1>,
 h2:({children,node:_,...props})=><h2 className="my-3 text-[18px] leading-[1.556] font-medium text-[var(--md-h2-fg)]" {...props}>{children}</h2>,
 h3:({children,node:_,...props})=><h3 className="my-2 text-[16px] leading-[1.5] font-medium text-[var(--md-h3-fg)]" {...props}>{children}</h3>,
 h4:({children,node:_,...props})=><h4 className="text-[var(--md-h4-fg)]" {...props}>{children}</h4>,
 h5:({children,node:_,...props})=><h5 className="text-[var(--md-h5-fg)]" {...props}>{children}</h5>,
 h6:({children,node:_,...props})=><h6 className="text-[var(--md-h6-fg)]" {...props}>{children}</h6>,
 strong:({children,node:_,...props})=><strong className="font-bold text-[var(--md-strong-fg)]" {...props}>{children}</strong>,
 blockquote:({children,node:_,...props})=><blockquote className="my-2 border-l-2 border-[var(--msg-blockquote-border)] pl-4 text-[var(--msg-blockquote-text)]" {...props}>{children}</blockquote>,
 hr:({node:_,...props})=><hr className="my-4 border-[var(--msg-hr-border)]" {...props}/>,
 table:({children,node:_,...props})=><table className="my-3 w-full table-fixed border-collapse text-sm" {...props}>{children}</table>,
 th:({children,node:_,...props})=><th className="border border-[var(--msg-table-border)] px-3 py-2 text-left bg-[var(--msg-table-header-bg)] font-medium break-words [overflow-wrap:anywhere]" {...props}>{children}</th>,
 td:({children,node:_,...props})=><td className="border border-[var(--msg-table-border)] px-3 py-2 break-words [overflow-wrap:anywhere]" {...props}>{children}</td>,
 code:({children,node:_,className,...props})=><code className={className} {...props}>{children}</code>,
 a:({children,href,node:_,...props})=><a className="underline underline-offset-2 cursor-pointer [overflow-wrap:anywhere]" href={href} target="_blank" rel="noopener noreferrer" {...props}>{children}</a>,
};
