import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react';
import { defaultKeymap, history, historyKeymap, redo } from '@codemirror/commands';
import { markdown, markdownLanguage } from '@codemirror/lang-markdown';
import { bracketMatching, syntaxHighlighting } from '@codemirror/language';
import { Compartment, EditorSelection, EditorState, Transaction } from '@codemirror/state';
import { drawSelection, EditorView, keymap, lineNumbers } from '@codemirror/view';
import { markdownTheme, textTheme } from './editor/theme';
import { markdownHighlight } from './editor/highlight';
import { markdownBlockDecoField, markdownDecoPlugin } from './editor/md-decorations';
import { tableDecoField } from './editor/table-field';
import { setHeading, toggleBold, toggleItalic, toggleInlineCode, toggleBlockquote, toggleList, toggleOrderedList, toggleStrikethrough, insertCodeBlock, insertHorizontalRule } from './editor/markdown-commands';
import 'katex/dist/katex.min.css';
import './editor/editor.css';

export type MarkdownFormat = 'heading' | 'bold' | 'italic' | 'quote' | 'code' | 'link' | 'bullet' | 'number' | 'heading1' | 'heading3' | 'strikethrough' | 'codeblock' | 'rule';

export interface CodeEditorHandle {
  focus: () => void;
  scrollToLine: (line: number) => void;
  selectMatch: (query: string, index: number) => number;
  applyFormat: (format: MarkdownFormat) => void;
}

interface Props {
  value: string;
  fileId: string;
  markdownMode: boolean;
  readOnly?: boolean;
  onChange: (value: string) => void;
  onSave?: () => void;
  onContextMenu?: (x: number, y: number) => void;
  onActiveLineChange?: (line: number) => void;
}

function allMatches(content: string, query: string): Array<{ from: number; to: number }> {
  if (!query) return [];
  const result: Array<{ from: number; to: number }> = [];
  const haystack = content.toLocaleLowerCase();
  const needle = query.toLocaleLowerCase();
  let from = 0;
  while (from <= haystack.length - needle.length) {
    const index = haystack.indexOf(needle, from);
    if (index < 0) break;
    result.push({ from: index, to: index + query.length });
    from = index + Math.max(1, query.length);
  }
  return result;
}

function wrapSelection(view: EditorView, before: string, after: string, placeholder: string): void {
  const range = view.state.selection.main;
  const selected = view.state.sliceDoc(range.from, range.to);
  const body = selected || placeholder;
  const insert = `${before}${body}${after}`;
  view.dispatch({
    changes: { from: range.from, to: range.to, insert },
    selection: EditorSelection.range(range.from + before.length, range.from + before.length + body.length),
  });
  view.focus();
}

// Adapted from OpenHanako PreviewEditor (Apache-2.0): external document
// updates must not reset the caret, scroll position or trigger another save.
function replaceDocumentPreservingSelection(view: EditorView, content: string): void {
  const current = view.state.doc.toString();
  if (current === content) return;
  const { anchor, head } = view.state.selection.main;
  const { scrollTop, scrollLeft } = view.scrollDOM;
  const clamp = (position: number) => Math.max(0, Math.min(position, content.length));
  view.dispatch({
    changes: { from: 0, to: current.length, insert: content },
    selection: EditorSelection.single(clamp(anchor), clamp(head)),
    annotations: Transaction.remote.of(true),
  });
  const restoreScroll = () => { view.scrollDOM.scrollTop = scrollTop; view.scrollDOM.scrollLeft = scrollLeft; };
  restoreScroll();
  queueMicrotask(restoreScroll);
  window.requestAnimationFrame?.(restoreScroll);
}

const CodeEditor = forwardRef<CodeEditorHandle, Props>(function CodeEditor({ value, fileId, markdownMode, readOnly = false, onChange, onSave, onContextMenu, onActiveLineChange }, ref) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const editability = useRef(new Compartment());
  const onChangeRef = useRef(onChange);
  const onSaveRef = useRef(onSave);
  const onActiveLineRef = useRef(onActiveLineChange);
  onActiveLineRef.current = onActiveLineChange;
  onChangeRef.current = onChange;
  onSaveRef.current = onSave;

  useImperativeHandle(ref, () => ({
    focus: () => view.current?.focus(),
    scrollToLine: (lineNumber) => {
      const editor = view.current;
      if (!editor) return;
      const safeLine = Math.max(1, Math.min(lineNumber, editor.state.doc.lines));
      const line = editor.state.doc.line(safeLine);
      editor.dispatch({ selection: EditorSelection.cursor(line.from), effects: EditorView.scrollIntoView(line.from, { y: 'start' }) });
      editor.focus();
    },
    selectMatch: (query, index) => {
      const editor = view.current;
      if (!editor) return 0;
      const matches = allMatches(editor.state.doc.toString(), query);
      if (!matches.length) return 0;
      const match = matches[((index % matches.length) + matches.length) % matches.length];
      editor.dispatch({ selection: EditorSelection.range(match.from, match.to), effects: EditorView.scrollIntoView(match.from, { y: 'center' }) });
      return matches.length;
    },
    applyFormat: (format) => {
      const editor = view.current;
      if (!editor) return;
      if (format === 'heading') setHeading(editor, 2);
      else if (format === 'heading1') setHeading(editor, 1);
      else if (format === 'heading3') setHeading(editor, 3);
      else if (format === 'strikethrough') toggleStrikethrough(editor);
      else if (format === 'codeblock') insertCodeBlock(editor);
      else if (format === 'rule') insertHorizontalRule(editor);
      else if (format === 'quote') toggleBlockquote(editor);
      else if (format === 'bullet') toggleList(editor);
      else if (format === 'number') toggleOrderedList(editor);
      else if (format === 'bold') toggleBold(editor);
      else if (format === 'italic') toggleItalic(editor);
      else if (format === 'code') toggleInlineCode(editor);
      else if (format === 'link') wrapSelection(editor, '[', '](https://)', '链接文字');
    },
  }), []);

  useEffect(() => {
    if (!host.current) return;
    const editor = new EditorView({
      parent: host.current,
      state: EditorState.create({
        doc: value,
        extensions: [
          history(),
          // Native selection avoids duplicate selection layers and supports IME.
          bracketMatching(),
          ...(markdownMode ? [syntaxHighlighting(markdownHighlight)] : [lineNumbers(), drawSelection()]),
          EditorView.lineWrapping,
          markdownMode ? markdownTheme : textTheme,
          editability.current.of([EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)]),
          keymap.of([
            { key: 'Mod-Shift-z', run: redo, preventDefault: true },
            ...(markdownMode ? [
              { key: 'Mod-b', run: (editor: EditorView) => { toggleBold(editor); return true; } },
              { key: 'Mod-i', run: (editor: EditorView) => { toggleItalic(editor); return true; } },
            ] : []),
            ...defaultKeymap,
            ...historyKeymap,
            { key: 'Mod-s', preventDefault: true, run: () => { onSaveRef.current?.(); return true; } },
          ]),
          ...(markdownMode ? [markdown({ base: markdownLanguage }), markdownDecoPlugin, markdownBlockDecoField, tableDecoField] : []),
          EditorView.updateListener.of((update) => {
            if (update.docChanged && !update.transactions.some(transaction => transaction.annotation(Transaction.remote))) onChangeRef.current(update.state.doc.toString());
            if (update.selectionSet || update.viewportChanged) {
              const pos = update.selectionSet ? update.state.selection.main.head : update.view.viewport.from;
              onActiveLineRef.current?.(update.state.doc.lineAt(pos).number);
            }
          }),
        ],
      }),
    });
    view.current = editor;
    onActiveLineRef.current?.(1);
    return () => { editor.destroy(); view.current = null; };
    // Recreate only when switching files or editor language.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fileId, markdownMode]);

  useEffect(() => {
    const editor = view.current;
    if (editor) replaceDocumentPreservingSelection(editor, value);
  }, [value]);

  useEffect(() => {
    view.current?.dispatch({ effects: editability.current.reconfigure([EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)]) });
  }, [readOnly]);

  return <div className="code-editor" ref={host} aria-label={markdownMode ? 'Markdown 编辑器' : '文本编辑器'} onContextMenu={event => {
    if (!markdownMode || !onContextMenu) return;
    event.preventDefault();
    event.stopPropagation();
    onContextMenu?.(event.clientX, event.clientY);
  }} />;
});

export default CodeEditor;
