// Adapted from OpenHanako (Apache-2.0). See docs/third-party/openhanako.md for source and modifications.
import { EditorView, WidgetType, Decoration } from '@codemirror/view';
import type { DecoRange } from '../md-decorations';

export class CheckboxWidget extends WidgetType {
  constructor(readonly checked: boolean) { super(); }

  eq(other: CheckboxWidget) { return this.checked === other.checked; }

  toDOM(view: EditorView) {
    const input = document.createElement('input');
    input.type = 'checkbox';
    input.checked = this.checked;
    input.className = 'cm-checkbox';
    input.setAttribute('aria-label', this.checked ? '标记为未完成' : '标记为已完成');
    input.addEventListener('mousedown', event => event.preventDefault());
    input.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      const from = view.posAtDOM(input);
      const to = from + 3; // [x] or [ ] is always 3 chars
      const replacement = this.checked ? '[ ]' : '[x]';
      if (!/^\[[ xX]\]$/.test(view.state.sliceDoc(from, to))) return;
      view.dispatch({ changes: { from, to, insert: replacement }, userEvent: 'input' });
    });
    return input;
  }

  ignoreEvent() { return true; }
}

export function handleCheckbox(ctx: {
  view: EditorView;
  node: { name: string; from: number; to: number };
  ranges: DecoRange[];
}) {
  const { view, node, ranges } = ctx;
  // TaskMarker node spans [x] or [ ] (including brackets, 3 chars)
  const text = view.state.doc.sliceString(node.from, node.to);
  const checked = text === '[x]' || text === '[X]';
  ranges.push({
    from: node.from,
    to: node.to,
    deco: Decoration.replace({ widget: new CheckboxWidget(checked) }),
  });
}

