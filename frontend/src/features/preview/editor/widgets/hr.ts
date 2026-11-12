// Adapted from OpenHanako (Apache-2.0). See docs/third-party/openhanako.md for source and modifications.
import { WidgetType, Decoration } from '@codemirror/view';

export class HrWidget extends WidgetType {
  toDOM() {
    const el = document.createElement('span');
    el.className = 'cm-hr-widget';
    return el;
  }
  eq() { return true; }
}

export const hrDecoration = Decoration.replace({ widget: new HrWidget() });

