// Adapted from OpenHanako editor/theme.ts (Apache-2.0); tokens scoped to this workspace.
import { EditorView } from '@codemirror/view';

export const textTheme = EditorView.theme({
  '&': { height: '100%', backgroundColor: 'transparent', color: 'var(--ink)', fontSize: '14px' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': { overflow: 'auto', fontFamily: '"Cascadia Code", Consolas, "Microsoft YaHei", monospace', lineHeight: '1.8' },
  '.cm-content': { padding: '22px 18px 80px', minHeight: '100%' },
  '.cm-line': { padding: '0 2px' },
  '.cm-gutters': { border: 'none', borderRight: '1px solid var(--line)', backgroundColor: 'var(--surface)', color: 'var(--muted)' },
  '.cm-gutterElement': { padding: '0 12px 0 16px' },
  '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': { backgroundColor: '#d9e6dc' },
});

export const markdownTheme = EditorView.theme({
  '&': { height: '100%', backgroundColor: 'transparent', color: 'var(--ink)', fontSize: 'var(--editor-markdown-font-size)' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': { overflow: 'auto', fontFamily: 'var(--editor-markdown-font-family)', lineHeight: 'var(--editor-markdown-line-height)', padding: '54px 0 128px' },
  '.cm-content': { width: '100%', minHeight: '100%', padding: '0 var(--editor-markdown-content-padding-x)', caretColor: 'var(--ink)' },
  '.cm-line': { maxWidth: 'var(--editor-markdown-content-width)', margin: '0 auto', padding: '0 2px', boxSizing: 'border-box' },
  '.cm-line.cm-unconfirmed-heading-line *': { fontSize: 'var(--editor-markdown-font-size)', fontWeight: 'inherit' },
  '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'transparent' },
  '.cm-gutters': { display: 'none' },
  '.cm-cursor': { borderLeftColor: 'var(--ink)' },
  '.cm-md-mark': { backgroundColor: 'var(--cm-md-mark-bg, #fff88fb8)', borderRadius: '2px', padding: '0 1px' },
  '.cm-math-block-widget': { display: 'block', maxWidth: 'var(--editor-markdown-content-width)', margin: '0 auto', overflowX: 'auto', padding: '4px 0', cursor: 'text' },
  '@media(max-width:720px)': { '.cm-scroller': { paddingTop: '28px', paddingBottom: '90px' } },
});
