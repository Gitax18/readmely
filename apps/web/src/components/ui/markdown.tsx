import { Children, isValidElement, type ReactNode } from "react";
import ReactMarkdown, { type ExtraProps } from "react-markdown";
import remarkGfm from "remark-gfm";
import { cn } from "@/lib/utils";

const BADGE = /^https:\/\/img\.shields\.io\/badge\//;

/** GitHub's anchor for a heading (services/ai/app/readme.py does the same), prefixed to keep page ids apart. */
export function headingAnchor(text: string): string {
  const slug = text
    .trim()
    .toLowerCase()
    .replace(/[^\p{L}\p{N}_\- ]/gu, "")
    .replaceAll(" ", "-");
  return `readme-${slug}`;
}

function textOf(node: ReactNode): string {
  return Children.toArray(node)
    .map((child) => {
      if (typeof child === "string" || typeof child === "number") return String(child);
      if (isValidElement<{ children?: ReactNode }>(child)) return textOf(child.props.children);
      return "";
    })
    .join("");
}

function isMermaid(node: ExtraProps["node"]): boolean {
  const code = node?.children[0];
  if (code?.type !== "element" || code.tagName !== "code") return false;
  const names = code.properties.className;
  return Array.isArray(names) && names.includes("language-mermaid");
}

/**
 * Rendered Markdown for README previews. Raw HTML is not rendered and images are never fetched (an
 * image in a README must not make the browser load an address of its choosing): a shields.io badge
 * shows as a chip with its text, any other image as its alt text. Mermaid diagrams are shown as their
 * code with a note, since GitHub draws them. Links to a heading scroll the preview; other links open in
 * a new tab without passing the page on.
 */
export function MarkdownView({ source, className }: { source: string; className?: string }) {
  return (
    <div
      className={cn(
        "text-sm leading-relaxed",
        "[&_a]:text-accent [&_a]:underline [&_a]:underline-offset-2",
        "[&_blockquote]:border-border-strong [&_blockquote]:text-muted-foreground [&_blockquote]:mb-3 [&_blockquote]:border-l-2 [&_blockquote]:pl-3",
        "[&_code]:bg-surface-2 [&_code]:rounded [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.85em]",
        "[&_h1]:mb-3 [&_h1]:text-2xl [&_h1]:font-semibold [&_h1]:tracking-tight",
        "[&_h2]:border-border [&_h2]:mt-6 [&_h2]:mb-2 [&_h2]:scroll-mt-4 [&_h2]:border-b [&_h2]:pb-1 [&_h2]:text-lg [&_h2]:font-semibold [&_h2]:tracking-tight",
        "[&_h3]:mt-4 [&_h3]:mb-1 [&_h3]:font-semibold",
        "[&_hr]:border-border [&_hr]:my-4",
        "[&_li]:ml-5 [&_li]:list-disc [&_ol_li]:list-decimal",
        "[&_p]:mb-3",
        "[&_pre]:bg-code [&_pre]:text-code-foreground [&_pre]:mb-3 [&_pre]:overflow-auto [&_pre]:rounded-lg [&_pre]:p-3 [&_pre]:text-xs",
        "[&_pre_code]:bg-transparent [&_pre_code]:p-0",
        "[&_table]:mb-3 [&_table]:block [&_table]:w-max [&_table]:max-w-full [&_table]:overflow-auto [&_td]:border [&_td]:border-border [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:border-border [&_th]:bg-surface-2 [&_th]:px-2 [&_th]:py-1 [&_th]:text-left",
        className,
      )}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          a: ({ href, children }) =>
            href?.startsWith("#") ? (
              <a href={`#readme-${href.slice(1)}`}>{children}</a>
            ) : (
              <a href={href} target="_blank" rel="noreferrer noopener nofollow">
                {children}
              </a>
            ),
          h2: ({ children }) => <h2 id={headingAnchor(textOf(children))}>{children}</h2>,
          img: ({ alt, src }) =>
            typeof src === "string" && BADGE.test(src) ? (
              <span className="border-border bg-surface-2 mr-1 mb-1 inline-flex items-center rounded border px-1.5 py-0.5 font-mono text-[11px] font-medium tracking-wide uppercase">
                {alt || "badge"}
              </span>
            ) : (
              <span>[{alt || "image"}]</span>
            ),
          pre: ({ node, children }) =>
            isMermaid(node) ? (
              <figure>
                <figcaption className="text-muted-foreground mb-1 text-xs">
                  Mermaid diagram. GitHub draws it from this code.
                </figcaption>
                <pre>{children}</pre>
              </figure>
            ) : (
              <pre>{children}</pre>
            ),
        }}
      >
        {source}
      </ReactMarkdown>
    </div>
  );
}
