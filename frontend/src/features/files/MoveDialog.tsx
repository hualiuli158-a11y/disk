import { useEffect, useMemo, useRef, useState } from 'react';
import type { KeyboardEvent, ReactNode } from 'react';
import type { Folder } from '../../services/api';
import Icon from '../../components/Icon';
import { canMoveItemsTo } from './fileOperations';
import type { FileOperationItem } from './fileOperations';

interface Props {
  items: FileOperationItem[];
  rootId: number;
  folders: Record<number, Folder>;
  children: Record<number, number[]>;
  onLoadChildren: (folderId: number) => Promise<void>;
  onConfirm: (folderId: number) => Promise<boolean>;
  onClose: () => void;
}

export default function MoveDialog({ items, rootId, folders, children, onLoadChildren, onConfirm, onClose }: Props) {
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set([rootId]));
  const [targetId, setTargetId] = useState<number | null>(null);
  const [pending, setPending] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);
  const itemIdentity = useMemo(() => items.map(item => `${item.type}:${item.id}`).sort().join(','), [items]);

  useEffect(() => {
    setExpanded(new Set([rootId]));
    setTargetId(null);
    setPending(false);
  }, [itemIdentity, rootId]);

  useEffect(() => {
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogRef.current?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus();
    return () => { if (previousFocus?.isConnected) previousFocus.focus(); };
  }, []);

  const handleDialogKey = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape' && !pending) {
      event.preventDefault();
      event.stopPropagation();
      onClose();
    }
    if (event.key !== 'Tab') return;
    const buttons = Array.from(dialogRef.current?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') || []);
    if (!buttons.length) { event.preventDefault(); return; }
    if (event.shiftKey && document.activeElement === buttons[0]) {
      event.preventDefault();
      buttons.at(-1)?.focus();
    } else if (!event.shiftKey && document.activeElement === buttons.at(-1)) {
      event.preventDefault();
      buttons[0].focus();
    }
  };

  const toggle = async (folderId: number) => {
    if (expanded.has(folderId)) {
      setExpanded(current => {
        const next = new Set(current);
        next.delete(folderId);
        return next;
      });
      return;
    }
    if (!Object.prototype.hasOwnProperty.call(children, folderId)) await onLoadChildren(folderId);
    setExpanded(current => new Set([...current, folderId]));
  };

  const renderFolder = (folderId: number, depth: number): ReactNode => {
    const folder = folders[folderId];
    if (!folder) return null;
    const open = expanded.has(folderId);
    const allowed = canMoveItemsTo(items, folderId, folders);
    return <div key={folderId}>
      <div className={`move-folder-row ${targetId === folderId ? 'selected' : ''} ${allowed ? '' : 'disabled'}`} style={{ paddingLeft: 10 + depth * 17 }}>
        <button type="button" className="move-folder-toggle" aria-label={open ? '收起' : '展开'} disabled={pending} onClick={() => void toggle(folderId)}><Icon name={open ? 'down' : 'chevron'} size={14} /></button>
        <button type="button" className="move-folder-target" disabled={!allowed || pending} onClick={() => setTargetId(folderId)}><Icon name="folder" size={17} /><span className="truncate">{folderId === rootId ? '我的空间' : folder.name}</span></button>
      </div>
      {open && (children[folderId] || []).map(childId => renderFolder(childId, depth + 1))}
    </div>;
  };

  const submit = async () => {
    if (targetId == null || pending) return;
    setPending(true);
    const moved = await onConfirm(targetId);
    setPending(false);
    if (moved) onClose();
  };

  return <div className="modal-backdrop" onMouseDown={() => { if (!pending) onClose(); }}>
    <div ref={dialogRef} className="modal move-dialog" role="dialog" aria-modal="true" aria-busy={pending} aria-labelledby="move-dialog-title" onKeyDown={handleDialogKey} onMouseDown={event => event.stopPropagation()}>
      <div className="modal-icon"><Icon name="move" size={22} /></div>
      <h2 id="move-dialog-title">移动 {items.length} 项</h2>
      <div className="move-folder-tree">{renderFolder(rootId, 0)}</div>
      <div className="modal-actions"><button type="button" onClick={onClose} disabled={pending}>取消</button><button type="button" className="primary-button" disabled={targetId == null || pending} onClick={() => void submit()}>{pending ? '移动中…' : '移动'}</button></div>
    </div>
  </div>;
}
