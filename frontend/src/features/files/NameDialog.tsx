import type { FormEvent } from 'react';
import type { ContentItem } from '../../services/api';
import Icon from '../../components/Icon';

export type Dialog = { kind: 'folder' | 'note' | 'rename'; item?: ContentItem } | null;

export default function NameDialog({ dialog, value, onValueChange, onClose, onSubmit }: { dialog: NonNullable<Dialog>; value: string; onValueChange: (value: string) => void; onClose: () => void; onSubmit: (event: FormEvent) => void }) {
  return <div className="modal-backdrop" onMouseDown={onClose}><form className="modal" onMouseDown={event => event.stopPropagation()} onSubmit={onSubmit}><div className="modal-icon"><Icon name={dialog.kind === 'folder' ? 'folderPlus' : dialog.kind === 'note' ? 'file' : 'edit'} size={22} /></div><h2>{dialog.kind === 'folder' ? '新建文件夹' : dialog.kind === 'note' ? '新建笔记' : '重命名'}</h2><label className="sr-only" htmlFor="dialog-name">名称</label><input id="dialog-name" autoFocus value={value} onChange={event => onValueChange(event.target.value)} placeholder={dialog.kind === 'note' ? '笔记名称' : '输入名称'} /><div className="modal-actions"><button type="button" onClick={onClose}>取消</button><button type="submit" className="primary-button">{dialog.kind === 'rename' ? '保存' : '创建'}</button></div></form></div>;
}
