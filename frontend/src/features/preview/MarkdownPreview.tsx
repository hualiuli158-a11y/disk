import { useEffect, useMemo, useRef } from 'react';
import DOMPurify from 'dompurify';
import Token from 'markdown-it/lib/token.mjs';
import { parser, getOutline } from './markdownParser';
export { getOutline } from './markdownParser';
export type { Heading } from './markdownParser';
import katex from '@traptitech/markdown-it-katex';
import 'katex/dist/katex.min.css';
import { escapeCode, highlightCode } from './highlightCode';

parser.use(katex, { throwOnError: false, trust: false });
parser.renderer.rules.fence = (tokens, index) => {
  const token = tokens[index];
  const language = token.info.trim().split(/\s+/)[0] || '';
  return `<div class="code-block-wrap"><div class="code-block-heading"><span>${escapeCode(language || 'text')}</span><button type="button" class="code-copy-button" aria-label="复制代码">复制</button></div><pre><code>${highlightCode(token.content, language)}</code></pre></div>`;
};
parser.renderer.rules.table_open = () => '<div class="markdown-table-scroll"><table>';
parser.renderer.rules.table_close = () => '</table></div>';
const defaultLinkOpen = parser.renderer.rules.link_open;
parser.renderer.rules.link_open = (tokens, index, options, env, self) => {
  const href = tokens[index].attrGet('href') || '';
  if (/^(https?:)?\/\//i.test(href)) {
    tokens[index].attrSet('target', '_blank');
    tokens[index].attrSet('rel', 'noopener noreferrer');
  }
  return defaultLinkOpen ? defaultLinkOpen(tokens, index, options, env, self) : self.renderToken(tokens, index, options);
};
const defaultImage = parser.renderer.rules.image;
parser.renderer.rules.image = (tokens, index, options, env, self) => {
  tokens[index].attrSet('loading', 'lazy');
  tokens[index].attrSet('decoding', 'async');
  tokens[index].attrSet('referrerpolicy', 'no-referrer');
  return defaultImage ? defaultImage(tokens, index, options, env, self) : self.renderToken(tokens, index, options);
};
const defaultHeadingOpen = parser.renderer.rules.heading_open;
parser.renderer.rules.heading_open = (tokens, index, options, env, self) => {
  const headingIndex = Number(env.headingIndex || 0);
  tokens[index].attrSet('id', `heading-${String(env.fileId)}-${headingIndex}`);
  env.headingIndex = headingIndex + 1;
  return defaultHeadingOpen ? defaultHeadingOpen(tokens, index, options, env, self) : self.renderToken(tokens, index, options);
};
parser.core.ruler.after('inline', 'task-list-checkboxes', state => {
  for (let index = 2; index < state.tokens.length; index += 1) {
    const inline = state.tokens[index];
    if (inline.type !== 'inline' || state.tokens[index - 1]?.type !== 'paragraph_open' || state.tokens[index - 2]?.type !== 'list_item_open') continue;
    const match = /^\[([ xX])\]\s+/.exec(inline.content);
    if (!match || !inline.children?.length) continue;
    inline.content = inline.content.slice(match[0].length);
    const first = inline.children[0];
    if (first.type === 'text') first.content = first.content.slice(match[0].length);
    const checkbox = new Token('html_inline', '', 0);
    checkbox.content = `<input class="task-checkbox" type="checkbox" disabled${match[1].toLowerCase() === 'x' ? ' checked' : ''}>`;
    inline.children.unshift(checkbox);
    state.tokens[index - 2].attrJoin('class', 'task-list-item');
  }
});

function clearFindMarks(root: HTMLElement): void {
  root.querySelectorAll('mark.preview-find-mark').forEach(mark => {
    mark.replaceWith(document.createTextNode(mark.textContent || ''));
  });
  root.normalize();
}

function markMatches(root: HTMLElement, query: string): HTMLElement[] {
  clearFindMarks(root);
  if (!query) return [];
  const nodes: Text[] = [];
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode: node => node.parentElement?.closest('button, script, style') ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT,
  });
  while (walker.nextNode()) nodes.push(walker.currentNode as Text);
  const needle = query.toLocaleLowerCase();
  const marks: HTMLElement[] = [];
  for (const node of nodes) {
    const text = node.data;
    const lower = text.toLocaleLowerCase();
    let cursor = 0;
    let match = lower.indexOf(needle);
    if (match < 0) continue;
    const fragment = document.createDocumentFragment();
    while (match >= 0) {
      fragment.append(text.slice(cursor, match));
      const mark = document.createElement('mark');
      mark.className = 'preview-find-mark';
      mark.textContent = text.slice(match, match + query.length);
      fragment.append(mark);
      marks.push(mark);
      cursor = match + query.length;
      match = lower.indexOf(needle, cursor);
    }
    fragment.append(text.slice(cursor));
    node.replaceWith(fragment);
  }
  return marks;
}

interface Props {
  content: string;
  fileId: string;
  findQuery?: string;
  activeMatch?: number;
  onMatchCount?: (count: number) => void;
}

export default function MarkdownPreview({ content, fileId, findQuery = '', activeMatch = 0, onMatchCount }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const marksRef = useRef<HTMLElement[]>([]);
  const html = useMemo(() => DOMPurify.sanitize(parser.render(content, { fileId, headingIndex: 0 })), [content, fileId]);

  useEffect(() => {
    const root = ref.current;
    if (!root) return;
    const timers = new Set<number>();
    const onClick = async (event: MouseEvent) => {
      const target = event.target instanceof Element ? event.target : null;
      const button = target?.closest<HTMLButtonElement>('.code-copy-button');
      if (button) {
        const code = button.closest('.code-block-wrap')?.querySelector('code') || button.parentElement?.querySelector('code');
        try { await navigator.clipboard.writeText(code?.textContent || ''); button.textContent = '已复制'; }
        catch { button.textContent = '复制失败'; }
        timers.add(window.setTimeout(() => { button.textContent = '复制'; }, 1400));
      }
      const link = target?.closest<HTMLAnchorElement>('a[href^="#"]');
      if (!link) return;
      let fragment: string;
      try { fragment = decodeURIComponent(link.hash.slice(1)); } catch { return; }
      const headings = getOutline(content, fileId);
      const heading = headings.find(item => item.text.toLocaleLowerCase().replace(/\s+/g, '-').replace(/[^\p{L}\p{N}_-]/gu, '') === fragment.toLocaleLowerCase());
      const destination = document.getElementById(heading?.id || fragment);
      if (destination && root.contains(destination)) { event.preventDefault(); destination.scrollIntoView({ behavior: 'smooth', block: 'start' }); }
    };
    root.addEventListener('click', onClick);
    return () => { root.removeEventListener('click', onClick); timers.forEach(window.clearTimeout); };
  }, [content, fileId]);

  useEffect(() => {
    const root = ref.current;
    if (!root) return;
    root.querySelectorAll('pre').forEach(pre => {
      if (pre.closest('.code-block-wrap') || pre.querySelector('.code-copy-button')) return;
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'code-copy-button';
      button.textContent = '复制';
      button.setAttribute('aria-label', '复制代码');
      pre.append(button);
    });
    marksRef.current = markMatches(root, findQuery);
    onMatchCount?.(marksRef.current.length);
  }, [findQuery, html, onMatchCount]);

  useEffect(() => {
    const marks = marksRef.current;
    marks.forEach(mark => mark.classList.remove('active'));
    if (!marks.length) return;
    const active = marks[((activeMatch % marks.length) + marks.length) % marks.length];
    active.classList.add('active');
    active.scrollIntoView({ block: 'center', inline: 'nearest' });
  }, [activeMatch, findQuery, html]);

  return <article ref={ref} className="markdown-body" dangerouslySetInnerHTML={{ __html: html }} />;
}
