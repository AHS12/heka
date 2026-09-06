// components/Markdown.tsx — the one markdown renderer for the embedded
// changelog (What's New modal + About page). Component overrides keep every
// element inside the app's theme tokens; no dangerouslySetInnerHTML anywhere.
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type {Components} from 'react-markdown'

const components: Components = {
  h1: ({node, ...props}) => (
    <h2 className="mb-2 mt-5 text-base font-bold tracking-tight text-foreground first:mt-0" {...props} />
  ),
  h2: ({node, ...props}) => (
    <h3 className="mb-2 mt-5 text-sm font-bold tracking-tight text-foreground first:mt-0" {...props} />
  ),
  h3: ({node, ...props}) => (
    <h4 className="mb-1.5 mt-4 text-[11px] font-semibold uppercase tracking-wider text-foreground/50 first:mt-0" {...props} />
  ),
  p: ({node, ...props}) => (
    <p className="my-2 text-[13px] leading-relaxed text-foreground/80 first:mt-0 last:mb-0" {...props} />
  ),
  ul: ({node, ...props}) => (
    <ul className="my-2 list-disc space-y-1.5 pl-5 text-[13px] leading-relaxed text-foreground/80" {...props} />
  ),
  ol: ({node, ...props}) => (
    <ol className="my-2 list-decimal space-y-1.5 pl-5 text-[13px] leading-relaxed text-foreground/80" {...props} />
  ),
  li: ({node, ...props}) => <li className="pl-0.5 marker:text-foreground/40" {...props} />,
  strong: ({node, ...props}) => <strong className="font-semibold text-foreground" {...props} />,
  em: ({node, ...props}) => <em className="text-foreground/85" {...props} />,
  a: ({node, ...props}) => (
    <a
      className="font-medium text-accent underline decoration-accent/40 underline-offset-2 hover:decoration-accent"
      target="_blank"
      rel="noreferrer"
      {...props}
    />
  ),
  code: ({node, className, ...props}) => (
    <code
      className={`rounded bg-surface-secondary px-1 py-0.5 font-mono text-[11.5px] text-foreground/85 ${className ?? ''}`}
      {...props}
    />
  ),
  pre: ({node, ...props}) => (
    <pre
      className="my-2 overflow-x-auto rounded-xl border border-border/80 bg-surface-secondary/60 p-3 font-mono text-[11.5px] leading-relaxed text-foreground/85"
      {...props}
    />
  ),
  hr: ({node, ...props}) => <hr className="my-4 border-border/70" {...props} />,
  blockquote: ({node, ...props}) => (
    <blockquote
      className="my-2 border-l-2 border-border pl-3 text-[13px] italic leading-relaxed text-foreground/70"
      {...props}
    />
  ),
}

export function Markdown({text}: {text: string}) {
  return <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>{text}</ReactMarkdown>
}
