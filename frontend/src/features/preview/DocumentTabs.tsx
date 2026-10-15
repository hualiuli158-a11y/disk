import type { WheelEvent } from 'react';
import type { ContentItem } from '../../services/api';
import Button from '../../components/Button';
import FileGlyph from '../files/FileGlyph';
import type { MarkdownDraft } from './useMarkdownDrafts';
import { draftHasUnsavedChanges } from './useMarkdownDrafts';

function horizontalWheel(event: WheelEvent<HTMLDivElement>) {
  const list = event.currentTarget;
  if (list.scrollWidth <= list.clientWidth) return;
  const delta = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY;
  const next = Math.max(0, Math.min(list.scrollWidth - list.clientWidth, list.scrollLeft + delta));
  if (next === list.scrollLeft) return;
  event.preventDefault();
  list.scrollLeft = next;
}

export default function DocumentTabs({ tabs, activeFileId, drafts, onOpen, onClose }: {
  tabs: ContentItem[];
  activeFileId: number | null;
  drafts: Record<number, MarkdownDraft>;
  onOpen: (file: ContentItem) => void;
  onClose: (id: number) => void;
}) {
  if (!tabs.length) return null;
  return <div className="tabbar"><div className="tabs" onWheel={horizontalWheel}>
    {tabs.map(file => {
      const dirty = draftHasUnsavedChanges(drafts[file.id]);
      return <div key={file.id} className={`tab ${activeFileId === file.id ? 'active' : ''}`} onDoubleClick={() => onClose(file.id)}>
        <button type="button" onClick={() => onOpen(file)} title={file.name}><FileGlyph item={file} size={14} /><span className="truncate">{file.name}</span>{dirty && <span className="tab-dirty" title="有未保存的修改" />}</button>
        <Button icon="close" label={`关闭 ${file.name}`} onClick={() => onClose(file.id)} />
      </div>;
    })}
  </div></div>;
}
