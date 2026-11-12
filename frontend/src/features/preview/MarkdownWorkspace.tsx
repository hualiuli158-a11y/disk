import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import type { PointerEvent as ReactPointerEvent } from 'react';
import type { CodeEditorHandle, MarkdownFormat } from './CodeEditor';
import { getOutline } from './markdownParser';
import { ChapterRail, MarkdownFindBox, MarkdownFormatBar } from './MarkdownChrome';
import type { MarkdownDraft } from './useMarkdownDrafts';

const CodeEditor = lazy(() => import('./CodeEditor'));
const MarkdownPreview = lazy(() => import('./MarkdownPreview'));

function countMatches(content: string, query: string): number {
  if (!query) return 0;
  const source = content.toLocaleLowerCase();
  const needle = query.toLocaleLowerCase();
  let count = 0;
  let from = 0;
  while (from <= source.length - needle.length) {
    const match = source.indexOf(needle, from);
    if (match < 0) break;
    count += 1;
    from = match + Math.max(1, needle.length);
  }
  return count;
}

export default function MarkdownWorkspace({ fileId, draft, mode, plainText = false, findRequest = 0, onDraftChange, onSave }: {
  fileId: number;
  draft: MarkdownDraft;
  mode?: 'preview' | 'edit';
  plainText?: boolean;
  findRequest?: number;
  onDraftChange: (content: string) => void;
  onSave: (createVersion?: boolean) => void;
}) {
  const activeMode = mode ?? 'preview';
  const [editorMounted, setEditorMounted] = useState(activeMode === 'edit');
  const [formatMenu, setFormatMenu] = useState<{ x: number; y: number } | null>(null);
  const [findOpen, setFindOpen] = useState(false);
  const [findQuery, setFindQuery] = useState('');
  const [findIndex, setFindIndex] = useState(0);
  const [findCount, setFindCount] = useState(0);
  const [activeHeadingId, setActiveHeadingId] = useState<string | null>(null);
  const [chapterRailVisible, setChapterRailVisible] = useState(false);
  const previewScrollRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<CodeEditorHandle>(null);
  const saveRef = useRef(onSave);
  saveRef.current = onSave;
  const headings = useMemo(() => plainText ? [] : getOutline(draft.content, String(fileId)), [draft.content, fileId, plainText]);

  useEffect(() => {
    setFindIndex(0);
    setActiveHeadingId(null);
  }, [fileId]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { if (activeMode !== 'edit') setFormatMenu(null); }, [activeMode]);
  useEffect(() => { if (activeMode === 'edit') setEditorMounted(true); }, [activeMode]);
  useEffect(() => { if (findRequest > 0) setFindOpen(true); }, [findRequest]);

  useEffect(() => {
    if (draft.state !== 'dirty') return;
    const timer = window.setTimeout(() => saveRef.current(false), 900);
    return () => window.clearTimeout(timer);
  }, [draft.content, draft.state]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (!(event.ctrlKey || event.metaKey)) return;
      if (event.key.toLowerCase() === 's') {
        if (!event.defaultPrevented) { event.preventDefault(); event.stopPropagation(); if (!event.repeat) saveRef.current(true); }
        return;
      }
      if (event.key.toLowerCase() === 'f') { event.preventDefault(); setFindOpen(true); }
    };
    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, []);

  useEffect(() => {
    setFindIndex(0);
  }, [findQuery, fileId, activeMode]);

  useEffect(() => {
    if (activeMode !== 'edit' && !plainText) return;
    const count = countMatches(draft.content, findQuery);
    setFindCount(count);
    if (findOpen && findQuery && count) editorRef.current?.selectMatch(findQuery, findIndex);
  }, [draft.content, findIndex, findOpen, findQuery, activeMode, plainText]);

  useEffect(() => {
    if (activeMode !== 'preview') return;
    const container = previewScrollRef.current;
    if (!container) return;
    let stored = 0;
    try { stored = Number(sessionStorage.getItem(`qipan-reading-${fileId}`)); } catch { /* Reading works without browser storage. */ }
    if (Number.isFinite(stored) && stored > 0) requestAnimationFrame(() => { container.scrollTop = stored; });
  }, [fileId, activeMode]);

  const trackReadingPosition = useCallback(() => {
    const container = previewScrollRef.current;
    if (!container) return;
    try { sessionStorage.setItem(`qipan-reading-${fileId}`, String(container.scrollTop)); } catch { /* Position persistence is optional. */ }
    const top = container.getBoundingClientRect().top + 70;
    let active = headings[0]?.id || null;
    for (const heading of headings) {
      const element = document.getElementById(heading.id);
      if (element && element.getBoundingClientRect().top <= top) active = heading.id;
      else if (element) break;
    }
    setActiveHeadingId(active);
  }, [fileId, headings]);

  const updateActiveHeading = useCallback((line: number) => {
    let active = headings[0]?.id || null;
    for (const heading of headings) {
      if (heading.line <= line) active = heading.id;
      else break;
    }
    setActiveHeadingId(active);
  }, [headings]);

  const jumpToHeading = useCallback((heading: (typeof headings)[number]) => {
    setActiveHeadingId(heading.id);
    if (activeMode === 'edit') editorRef.current?.scrollToLine(heading.line);
    else document.getElementById(heading.id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }, [activeMode]);

  const goFind = (direction: 1 | -1) => {
    if (!findCount) return;
    setFindIndex(current => (current + direction + findCount) % findCount);
  };

  const applyFormat = (format: MarkdownFormat) => { editorRef.current?.applyFormat(format); setFormatMenu(null); };
  const handlePointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    setChapterRailVisible(rect.right - event.clientX <= 72 && event.clientY - rect.top > 58);
  };

  return <div className={`markdown-workspace mode-${activeMode} ${plainText ? 'plain-text' : ''}`} onPointerMove={handlePointerMove} onPointerLeave={() => setChapterRailVisible(false)}>
    <MarkdownFindBox open={findOpen} query={findQuery} current={Math.min(findIndex, Math.max(0, findCount - 1))} count={findCount}
      onQueryChange={setFindQuery} onPrevious={() => goFind(-1)} onNext={() => goFind(1)} onClose={() => setFindOpen(false)} />
    <ChapterRail headings={headings} activeId={activeHeadingId} visible={chapterRailVisible} onJump={jumpToHeading} />
    {formatMenu && createPortal(<MarkdownFormatBar position={formatMenu} onFormat={applyFormat} onClose={() => setFormatMenu(null)} />, document.body)}
    {activeMode === 'preview' && !plainText
      ? <div ref={previewScrollRef} className="markdown-scroll" onScroll={trackReadingPosition}><div className="preview-pane"><div className="paper">
          <Suspense fallback={<div className="editor-loading" role="status">正在排版…</div>}><MarkdownPreview content={draft.content} fileId={String(fileId)} findQuery={findOpen ? findQuery : ''} activeMatch={findIndex} onMatchCount={setFindCount} /></Suspense>
        </div></div></div> : null}
    {(plainText || editorMounted || activeMode === 'edit') && <div className="editor-pane" hidden={!plainText && activeMode !== 'edit'}><Suspense fallback={<div className="editor-loading"><span className="loading-dot" role="status" aria-label="正在加载编辑器" /></div>}>
          <CodeEditor ref={editorRef} value={draft.content} fileId={String(fileId)} markdownMode={!plainText} readOnly={plainText && activeMode !== 'edit'} onChange={onDraftChange} onSave={() => onSave(true)} onActiveLineChange={updateActiveHeading} onContextMenu={plainText ? undefined : (x, y) => setFormatMenu({ x, y })} />
        </Suspense></div>}
  </div>;
}
