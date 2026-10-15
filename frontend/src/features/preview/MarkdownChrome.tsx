import { useEffect, useRef, useState } from 'react';
import type { CSSProperties } from 'react';
import Icon from '../../components/Icon';
import type { MarkdownFormat } from './CodeEditor';
import type { Heading } from './MarkdownPreview';

export function MarkdownFindBox({ open, query, current, count, onQueryChange, onPrevious, onNext, onClose }: {
  open: boolean;
  query: string;
  current: number;
  count: number;
  onQueryChange: (value: string) => void;
  onPrevious: () => void;
  onNext: () => void;
  onClose: () => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  useEffect(() => { if (open) inputRef.current?.focus(); }, [open]);
  if (!open) return null;
  return <div className="preview-find-box" role="search">
    <Icon name="search" size={15} />
    <input ref={inputRef} value={query} placeholder="在文档中查找" aria-label="在文档中查找"
      onChange={event => onQueryChange(event.target.value)}
      onKeyDown={event => {
        if (event.key === 'Escape') onClose();
        if (event.key === 'Enter') event.shiftKey ? onPrevious() : onNext();
      }} />
    <span>{query ? `${count ? current + 1 : 0}/${count}` : '0/0'}</span>
    <button type="button" disabled={!count} aria-label="上一个匹配" title="上一个匹配" onClick={onPrevious}><Icon name="down" className="up-icon" size={15} /></button>
    <button type="button" disabled={!count} aria-label="下一个匹配" title="下一个匹配" onClick={onNext}><Icon name="down" size={15} /></button>
    <button type="button" aria-label="关闭查找" title="关闭查找" onClick={onClose}><Icon name="close" size={15} /></button>
  </div>;
}

export function ChapterRail({ headings, activeId, visible, onJump }: {
  headings: Heading[];
  activeId: string | null;
  visible: boolean;
  onJump: (heading: Heading) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const closeTimer = useRef<number | null>(null);
  const listRef = useRef<HTMLDivElement>(null);
  useEffect(() => () => { if (closeTimer.current != null) window.clearTimeout(closeTimer.current); }, []);
  useEffect(() => {
    const list = listRef.current;
    const index = headings.findIndex(heading => heading.id === activeId);
    if (list && index >= 0) list.scrollTop = Math.max(0, index * 28 - list.clientHeight / 2);
  }, [activeId, headings]);
  if (!headings.length) return null;
  const open = () => { if (closeTimer.current != null) window.clearTimeout(closeTimer.current); closeTimer.current = null; setExpanded(true); };
  const closeSoon = () => { closeTimer.current = window.setTimeout(() => setExpanded(false), 120); };
  return <nav className={`chapter-rail ${visible || expanded ? 'visible' : ''} ${expanded ? 'expanded' : ''}`} aria-label="文档章节"
    onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) setExpanded(false); }}>
    <div className="chapter-card" onPointerEnter={open} onPointerLeave={closeSoon}>
      <div ref={listRef} className="chapter-list" style={{ '--chapter-rows': Math.min(headings.length, 10) } as CSSProperties}>
        {headings.map(heading => {
          const normalized = Math.min(1, Math.log1p(Math.max(0, Array.from(heading.text).length - 2)) / Math.log1p(80));
          return <button key={heading.id} type="button" className={`chapter-marker ${activeId === heading.id ? 'active' : ''}`}
            style={{ '--chapter-indent': `${Math.min(3, heading.level - 1) * .875}rem`, '--chapter-width': `${(.5 + normalized * .5) * 1.5}em` } as CSSProperties}
            title={heading.text} aria-label={`跳转到 ${heading.text}`} onFocus={open} onMouseEnter={open} onClick={() => onJump(heading)}>
            <span className="chapter-label">{heading.text}</span><span className="chapter-line" aria-hidden="true" />
          </button>;
        })}
      </div>
    </div>
  </nav>;
}

const FORMAT_ACTIONS: Array<{ format: MarkdownFormat; label: string; text: string }> = [
  { format: 'heading1', label: '一级标题', text: 'H1' },
  { format: 'heading', label: '二级标题', text: 'H2' },
  { format: 'heading3', label: '三级标题', text: 'H3' },
  { format: 'bold', label: '粗体', text: 'B' },
  { format: 'italic', label: '斜体', text: 'I' },
  { format: 'quote', label: '引用', text: '“' },
  { format: 'code', label: '行内代码', text: '</>' },
  { format: 'strikethrough', label: '删除线', text: 'S̶' },
  { format: 'codeblock', label: '代码块', text: '{ }' },
  { format: 'rule', label: '分隔线', text: '—' },
  { format: 'link', label: '链接', text: '↗' },
  { format: 'bullet', label: '无序列表', text: '•' },
  { format: 'number', label: '有序列表', text: '1.' },
];

export function MarkdownFormatBar({ position, onFormat, onClose }: {
  position: { x: number; y: number };
  onFormat: (format: MarkdownFormat) => void;
  onClose: () => void;
}) {
  const menuRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const menu = menuRef.current;
    if (!menu) return;
    const rect = menu.getBoundingClientRect();
    menu.style.left = `${Math.max(4, Math.min(position.x, window.innerWidth - rect.width - 4))}px`;
    menu.style.top = `${Math.max(4, Math.min(position.y, window.innerHeight - rect.height - 4))}px`;
    const handlePointer = (event: PointerEvent) => { if (!menu.contains(event.target as Node)) onClose(); };
    const handleKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); };
    const handleScroll = () => onClose();
    const timer = window.setTimeout(() => {
      document.addEventListener('pointerdown', handlePointer, true);
      document.addEventListener('keydown', handleKey);
      window.addEventListener('scroll', handleScroll, true);
    });
    return () => {
      window.clearTimeout(timer);
      document.removeEventListener('pointerdown', handlePointer, true);
      document.removeEventListener('keydown', handleKey);
      window.removeEventListener('scroll', handleScroll, true);
    };
  }, [position, onClose]);
  return <div ref={menuRef} className="markdown-format-bar" role="toolbar" aria-label="Markdown 格式" style={{ left: position.x, top: position.y }} onContextMenu={event => event.preventDefault()}>
    {FORMAT_ACTIONS.map(action => <button type="button" key={action.format} title={action.label} aria-label={action.label}
      className={`format-${action.format}`} onMouseDown={event => event.preventDefault()} onClick={() => onFormat(action.format)}>{action.text}</button>)}
  </div>;
}
