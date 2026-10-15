import { useEffect, useState } from 'react';
import type { DragEvent, KeyboardEvent, MouseEvent, ReactNode } from 'react';
import type { ContentItem, Folder } from '../../services/api';
import FileGlyph from '../files/FileGlyph';
import Button from '../../components/Button';
import ContextMenu from '../../components/ContextMenu';
import type { ContextMenuEntry } from '../../components/ContextMenu';
import Icon from '../../components/Icon';
import InlineNameInput from '../../components/InlineNameInput';
import {
  clearFileDragPayload,
  dataTransferHasFiles,
  readFileDragPayload,
  writeFileDragPayload,
} from '../files/fileOperations';
import type { FileOperationItem } from '../files/fileOperations';

interface Props {
  width?: number;
  mobileOpen?: boolean;
  onCloseMobile?: () => void;
  rootId: number | null;
  activeFolderId: number | null;
  activeFileId: number | null;
  folders: Record<number, Folder>;
  files: Record<number, ContentItem[]>;
  onOpenFile: (file: ContentItem) => void;
  onDownload: (file: ContentItem) => void;
  onRenameFile: (file: ContentItem) => void;
  onManage: () => void;
  loading: boolean;
  loadError: string | null;
  onRetry: () => void;
  children: Record<number, number[]>;
  expanded: Set<number>;
  busy: boolean;
  uploading: boolean;
  onSelectFolder: (id: number) => void;
  onToggleFolder: (id: number) => void;
  onStartCreateInside: (folderId: number, kind: 'folder' | 'note') => void;
  onRenameFolder: (id: number, name: string) => Promise<boolean>;
  onChooseUploadTarget: (folderId: number) => void;
  onUploadFilesToFolder: (files: File[], folderId: number) => void;
  onRequestMove: (items: FileOperationItem[]) => void;
  onMoveItems: (items: FileOperationItem[], destinationId: number) => void;
  onRemoveItems: (items: FileOperationItem[]) => void;
  onCreateFolder: () => void;
  onUpload: () => void;
}

interface MenuState { position: { x: number; y: number }; items: ContextMenuEntry[] }

export default function Explorer(props: Props) {
  const {
    width, mobileOpen, onCloseMobile, rootId, activeFolderId, activeFileId, folders, children, expanded, busy, uploading,
    onSelectFolder, onToggleFolder, onStartCreateInside, onRenameFolder,
    onChooseUploadTarget, onUploadFilesToFolder, onRequestMove, onMoveItems,
    onRemoveItems, onCreateFolder, onUpload,
  } = props;
  const [menu, setMenu] = useState<MenuState | null>(null);
  const [renamingId, setRenamingId] = useState<number | null>(null);
  const [dropTargetId, setDropTargetId] = useState<number | null>(null);
  const [filter, setFilter] = useState('');

  function renderFiles(folderId: number, depth: number): ReactNode {
    return (props.files[folderId] || []).filter(file => !filter || file.name.toLocaleLowerCase().includes(filter.toLocaleLowerCase())).map(file => <div key={`file-${file.id}`}
      role="treeitem" tabIndex={0} aria-label={file.name} aria-selected={activeFileId === file.id}
      className={`tree-row file-row ${activeFileId === file.id ? 'selected' : ''}`}
      style={{ paddingLeft: 37 + depth * 16 }} title={file.name}
      onClick={() => props.onOpenFile(file)} onKeyDown={event => {
        if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); props.onOpenFile(file); }
      }} draggable={!busy} onDragStart={event => writeFileDragPayload(event.dataTransfer, [file])} onDragEnd={clearFileDragPayload}
      onContextMenu={event => {
        event.preventDefault(); event.stopPropagation();
        setMenu({ position: { x: event.clientX, y: event.clientY }, items: [
          { label: '打开预览', icon: 'eye', action: () => props.onOpenFile(file) },
          { label: '下载', icon: 'download', action: () => props.onDownload(file) },
          { kind: 'separator' },
          { label: '重命名', icon: 'edit', action: () => props.onRenameFile(file) },
          { label: '移动', icon: 'move', action: () => onRequestMove([file]) },
          { label: '移到回收站', icon: 'trash', danger: true, action: () => onRemoveItems([file]) },
        ] });
      }}><FileGlyph item={file} size={15} /><span className="truncate">{file.name}</span></div>);
  }

  useEffect(() => {
    if (dropTargetId == null) return;
    const clear = () => setDropTargetId(null);
    window.addEventListener('drop', clear);
    window.addEventListener('dragend', clear);
    window.addEventListener('blur', clear);
    return () => {
      window.removeEventListener('drop', clear);
      window.removeEventListener('dragend', clear);
      window.removeEventListener('blur', clear);
    };
  }, [dropTargetId]);

  const asOperationItem = (folder: Folder): FileOperationItem => ({ id: folder.id, type: 'folder', parent_id: folder.parent_id, name: folder.name });

  const contextItems = (folder: Folder, isRoot: boolean): ContextMenuEntry[] => {
    const item = asOperationItem(folder);
    return [
      { label: '打开', icon: 'folder', action: () => onSelectFolder(folder.id) },
      { kind: 'separator' },
      { label: '新建子文件夹', icon: 'folderPlus', action: () => onStartCreateInside(folder.id, 'folder') },
      { label: '新建笔记', icon: 'markdown', action: () => onStartCreateInside(folder.id, 'note') },
      { label: '上传到此处', icon: 'upload', action: () => onChooseUploadTarget(folder.id) },
      ...(!isRoot ? [
        { kind: 'separator' as const },
        { label: '重命名', icon: 'edit', action: () => setRenamingId(folder.id) },
        { label: '移动', icon: 'move', action: () => onRequestMove([item]) },
        { kind: 'separator' as const },
        { label: '移到回收站', icon: 'trash', danger: true, action: () => onRemoveItems([item]) },
      ] : []),
    ];
  };

  const openContextMenu = (event: MouseEvent, folder: Folder, isRoot: boolean) => {
    event.preventDefault();
    event.stopPropagation();
    setMenu({ position: { x: event.clientX, y: event.clientY }, items: contextItems(folder, isRoot) });
  };

  const handleDragOver = (event: DragEvent, folderId: number) => {
    if (busy) return;
    const internal = readFileDragPayload(event.dataTransfer);
    if (!internal && !dataTransferHasFiles(event.dataTransfer)) return;
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = internal ? 'move' : 'copy';
    setDropTargetId(folderId);
  };

  const handleDrop = (event: DragEvent, folderId: number) => {
    if (busy) return;
    const internal = readFileDragPayload(event.dataTransfer);
    const files = Array.from(event.dataTransfer.files || []);
    if (!internal && files.length === 0) return;
    event.preventDefault();
    event.stopPropagation();
    setDropTargetId(null);
    clearFileDragPayload();
    if (internal) onMoveItems(internal, folderId);
    else onUploadFilesToFolder(files, folderId);
  };

  const handleFolderKey = (event: KeyboardEvent<HTMLDivElement>, folderId: number, open: boolean) => {
    if (event.target !== event.currentTarget) return;
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onSelectFolder(folderId);
    } else if ((event.key === 'ArrowRight' && !open) || (event.key === 'ArrowLeft' && open)) {
      event.preventDefault();
      onToggleFolder(folderId);
    }
  };

  function renderTree(id: number, depth: number): ReactNode {
    const folder = folders[id];
    if (!folder) return null;
    const open = expanded.has(id);
    const isRoot = id === rootId;
    if (isRoot) return open ? <>{(children[id] || []).map(childId => renderTree(childId, depth))}{renderFiles(id, depth)}</> : null;
    const operationItem = asOperationItem(folder);
    return <div key={id}>
      <div
        role="treeitem"
        tabIndex={0}
        aria-expanded={open}
        aria-selected={activeFolderId === id && !activeFileId}
        aria-label={folder.name}
        className={`tree-row folder-row ${activeFolderId === id && !activeFileId ? 'selected' : ''} ${dropTargetId === id ? 'drop-target' : ''}`}
        style={{ paddingLeft: 15 + depth * 16 }}
        onClick={() => { if (renamingId !== id) onSelectFolder(id); }}
        onKeyDown={event => handleFolderKey(event, id, open)}
        onContextMenu={event => openContextMenu(event, folder, false)}
        draggable={renamingId !== id && !busy}
        onDragStart={event => writeFileDragPayload(event.dataTransfer, [operationItem])}
        onDragEnd={() => { clearFileDragPayload(); setDropTargetId(null); }}
        onDragOver={event => handleDragOver(event, id)}
        onDragLeave={event => { if (!event.currentTarget.contains(event.relatedTarget as Node)) setDropTargetId(null); }}
        onDrop={event => handleDrop(event, id)}
      >
        <button type="button" className="tree-caret" aria-label={open ? '收起文件夹' : '展开文件夹'} onClick={event => { event.stopPropagation(); onToggleFolder(id); }}><Icon name={open ? 'down' : 'chevron'} size={14} /></button>
        <Icon name="folder" size={16} />
        {renamingId === id ? <InlineNameInput className="tree-inline-name" initialValue={folder.name} onCommit={async name => {
          const accepted = await onRenameFolder(id, name);
          if (accepted) setRenamingId(null);
          return accepted;
        }} onCancel={() => setRenamingId(null)} /> : <span className="truncate">{folder.name}</span>}
      </div>
      {open && <div role="group">{(children[id] || []).map(childId => renderTree(childId, depth + 1))}{renderFiles(id, depth + 1)}</div>}
    </div>;
  }

  const root = rootId == null ? null : folders[rootId];
  return <aside id="folder-explorer" className={`explorer ${mobileOpen ? 'mobile-open' : ''}`} aria-label="目录" style={width == null ? undefined : { width }}><div className="explorer-heading"><h2>文件</h2><div className="explorer-actions"><Button icon="folderPlus" label="新建文件夹" onClick={onCreateFolder} disabled={busy} /><Button icon="upload" label="上传文件" onClick={onUpload} disabled={busy || uploading} />{mobileOpen && <button type="button" className="explorer-close" aria-label="关闭目录" onClick={onCloseMobile}><Icon name="close" size={18} /></button>}</div></div>
    <label className="explorer-search"><Icon name="search" size={14} /><input aria-label="筛选已展开目录中的文件" placeholder="筛选文件…" value={filter} onChange={event => setFilter(event.target.value)} /></label>
    {rootId != null && root && <><div
      role="button"
      tabIndex={0}
      aria-current={activeFolderId === rootId && !activeFileId ? 'page' : undefined}
      className={`space-row ${activeFolderId === rootId && !activeFileId ? 'selected' : ''} ${dropTargetId === rootId ? 'drop-target' : ''}`}
      onClick={() => onSelectFolder(rootId)}
      onKeyDown={event => {
        if (event.target !== event.currentTarget) return;
        if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); onSelectFolder(rootId); }
      }}
      onContextMenu={event => openContextMenu(event, root, true)}
      onDragOver={event => handleDragOver(event, rootId)}
      onDragLeave={event => { if (!event.currentTarget.contains(event.relatedTarget as Node)) setDropTargetId(null); }}
      onDrop={event => handleDrop(event, rootId)}
    ><span className="space-icon"><Icon name="grid" size={18} /></span>我的空间</div><div className="tree-scroll" role="tree">{renderTree(rootId, 0)}</div></>}
    {props.loading && <div className="explorer-status" role="status">正在加载文件…</div>}
    {props.loadError && <div className="explorer-status" role="alert"><span>{props.loadError}</span><button type="button" onClick={props.onRetry}>重试</button></div>}
    <button type="button" className="explorer-manage" onClick={props.onManage}><Icon name="grid" size={16} />管理文件<Icon name="chevron" size={14} /></button>
    {menu && <ContextMenu position={menu.position} items={menu.items} onClose={() => setMenu(null)} />}
  </aside>;
}
