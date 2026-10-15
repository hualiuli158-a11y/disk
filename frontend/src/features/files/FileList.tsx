import { useEffect, useMemo, useRef, useState } from 'react';
import type { DragEvent, MouseEvent, RefObject } from 'react';
import type { ContentItem, ContentSortField, Page, SearchKind, SortOrder, TrashItem } from '../../services/api';
import Button from '../../components/Button';
import ContextMenu from '../../components/ContextMenu';
import type { ContextMenuEntry } from '../../components/ContextMenu';
import Empty from '../../components/Empty';
import Icon from '../../components/Icon';
import InlineNameInput from '../../components/InlineNameInput';
import FileGlyph from './FileGlyph';
import {
  clearFileDragPayload,
  dataTransferHasFiles,
  fileItemKey,
  readFileDragPayload,
  writeFileDragPayload,
} from './fileOperations';
import type { FileOperationItem } from './fileOperations';
import { describeFile, formatDate, formatSize, formatTrashExpiry } from './fileUtils';

export type Section = 'files' | 'search' | 'trash';
export type InlineFileEdit =
  | { mode: 'create'; kind: 'folder' | 'note'; initialValue: string }
  | { mode: 'rename'; item: ContentItem }
  | null;

interface Props {
  section: Section;
  heading: string;
  loading: boolean;
  loadError: string | null;
  busy: boolean;
  uploading: boolean;
  query: string;
  searchRef: RefObject<HTMLInputElement | null>;
  page: number;
  currentPage: Page<ContentItem> | Page<TrashItem> | null;
  items: ContentItem[] | TrashItem[];
  sortField: ContentSortField;
  sortOrder: SortOrder;
  searchKind: SearchKind;
  selectedKeys: ReadonlySet<string>;
  inlineEdit: InlineFileEdit;
  onQueryChange: (value: string) => void;
  onRefresh: () => void;
  onRetry: () => void;
  onSortChange: (field: ContentSortField, order: SortOrder) => void;
  onSearchKindChange: (kind: SearchKind) => void;
  onSelectionChange: (items: ContentItem[]) => void;
  onStartCreate: (kind: 'folder' | 'note') => void;
  onStartCreateInside: (folderId: number, kind: 'folder' | 'note') => void;
  onBeginRename: (item: ContentItem) => void;
  onCommitInlineEdit: (value: string) => Promise<boolean>;
  onCancelInlineEdit: () => void;
  onUpload: () => void;
  onChooseUploadTarget: (folderId: number) => void;
  onUploadFilesToFolder: (files: File[], folderId: number) => void;
  onOpen: (item: ContentItem) => void;
  searchPaths: Record<number, string | null>;
  onLocateFolder: (folderId: number) => void;
  onDownload: (item: ContentItem) => void;
  onRequestMove: (items: FileOperationItem[]) => void;
  onMoveItems: (items: FileOperationItem[], destinationId: number) => void;
  onRemoveItems: (items: FileOperationItem[]) => void;
  onRestore: (item: TrashItem) => void;
  onPermanentlyDelete: (item: TrashItem) => void;
  onPageChange: (page: number) => void;
}

interface MenuState { position: { x: number; y: number }; items: ContextMenuEntry[] }

const SORT_OPTIONS: Array<{ label: string; field: ContentSortField; order: SortOrder }> = [
  { label: '名称 A–Z', field: 'name', order: 'asc' },
  { label: '名称 Z–A', field: 'name', order: 'desc' },
  { label: '最近修改', field: 'updated_at', order: 'desc' },
  { label: '最早修改', field: 'updated_at', order: 'asc' },
  { label: '最近创建', field: 'created_at', order: 'desc' },
  { label: '文件最大优先', field: 'size', order: 'desc' },
  { label: '文件最小优先', field: 'size', order: 'asc' },
];

export default function FileList(props: Props) {
  const {
    section, heading, loading, loadError, busy, uploading, query, searchRef, page, currentPage, items,
    sortField, sortOrder, searchKind, selectedKeys, inlineEdit,
    onQueryChange, onRefresh, onRetry, onSortChange, onSearchKindChange, onSelectionChange,
    onStartCreate, onStartCreateInside, onBeginRename, onCommitInlineEdit, onCancelInlineEdit,
    onUpload, onChooseUploadTarget, onUploadFilesToFolder, onOpen, searchPaths, onLocateFolder, onDownload,
    onRequestMove, onMoveItems, onRemoveItems, onRestore, onPermanentlyDelete, onPageChange,
  } = props;
  const [menu, setMenu] = useState<MenuState | null>(null);
  const [dropTargetKey, setDropTargetKey] = useState<string | null>(null);
  const selectionAnchorRef = useRef<number | null>(null);
  const contentItems = section === 'trash' ? [] : items as ContentItem[];
  const contentIdentity = contentItems.map(fileItemKey).join('|');
  const selectedItems = useMemo(
    () => contentItems.filter(item => selectedKeys.has(fileItemKey(item))),
    [contentItems, selectedKeys],
  );

  useEffect(() => { selectionAnchorRef.current = null; }, [contentIdentity]);

  const toggleItem = (item: ContentItem, index: number) => {
    selectionAnchorRef.current = index;
    const key = fileItemKey(item);
    onSelectionChange(selectedKeys.has(key) ? selectedItems.filter(selected => fileItemKey(selected) !== key) : [...selectedItems, item]);
  };

  const selectFromEvent = (item: ContentItem, index: number, event: MouseEvent) => {
    const multi = event.ctrlKey || event.metaKey;
    if (event.shiftKey && selectionAnchorRef.current != null) {
      const start = Math.min(selectionAnchorRef.current, index);
      const end = Math.max(selectionAnchorRef.current, index);
      const range = contentItems.slice(start, end + 1);
      onSelectionChange(multi ? [...selectedItems, ...range] : range);
      return;
    }
    selectionAnchorRef.current = index;
    if (multi) { toggleItem(item, index); return; }
    onSelectionChange([item]);
  };

  const operationItemsFor = (item: ContentItem): ContentItem[] => {
    return selectedKeys.has(fileItemKey(item)) && selectedItems.length > 0 ? selectedItems : [item];
  };

  const showMenu = (position: { x: number; y: number }, menuItems: ContextMenuEntry[]) => setMenu({ position, items: menuItems });

  const itemMenu = (item: ContentItem): ContextMenuEntry[] => {
    const operationItems = operationItemsFor(item);
    const single = operationItems.length === 1;
    const entries: ContextMenuEntry[] = [];
    if (single) {
      entries.push({ label: item.type === 'folder' ? '打开文件夹' : '打开预览', icon: item.type === 'folder' ? 'folder' : 'eye', action: () => onOpen(item) });
      if (item.type === 'file') entries.push({ label: '下载', icon: 'download', action: () => onDownload(item) });
      if (item.type === 'folder') {
        entries.push({ kind: 'separator' });
        entries.push({ label: '新建子文件夹', icon: 'folderPlus', action: () => onStartCreateInside(item.id, 'folder') });
        entries.push({ label: '上传到此处', icon: 'upload', action: () => onChooseUploadTarget(item.id) });
      }
      entries.push({ kind: 'separator' });
      if (section === 'search' && item.parent_id != null) entries.push({ label: '定位到文件夹', icon: 'folder', action: () => onLocateFolder(item.parent_id!) });
      entries.push({ label: '重命名', icon: 'edit', action: () => onBeginRename(item) });
    }
    entries.push({ label: `移动${single ? '' : ` ${operationItems.length} 项`}`, icon: 'move', action: () => onRequestMove(operationItems) });
    entries.push({ kind: 'separator' });
    entries.push({ label: `移到回收站${single ? '' : `（${operationItems.length} 项）`}`, icon: 'trash', danger: true, action: () => onRemoveItems(operationItems) });
    return entries;
  };

  const openItemMenu = (event: MouseEvent, item: ContentItem) => {
    event.preventDefault();
    event.stopPropagation();
    if (!selectedKeys.has(fileItemKey(item))) onSelectionChange([item]);
    showMenu({ x: event.clientX, y: event.clientY }, itemMenu(item));
  };

  const openItemMenuFromButton = (event: MouseEvent<HTMLButtonElement>, item: ContentItem) => {
    event.preventDefault();
    event.stopPropagation();
    if (!selectedKeys.has(fileItemKey(item))) onSelectionChange([item]);
    const rect = event.currentTarget.getBoundingClientRect();
    showMenu({ x: rect.right, y: rect.bottom + 4 }, itemMenu(item));
  };

  const handleFolderDragOver = (event: DragEvent, item: ContentItem) => {
    if (busy || item.type !== 'folder') return;
    const internal = readFileDragPayload(event.dataTransfer);
    if (!internal && !dataTransferHasFiles(event.dataTransfer)) return;
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = internal ? 'move' : 'copy';
    setDropTargetKey(fileItemKey(item));
  };

  const handleFolderDrop = (event: DragEvent, item: ContentItem) => {
    if (busy || item.type !== 'folder') return;
    const internal = readFileDragPayload(event.dataTransfer);
    const externalFiles = Array.from(event.dataTransfer.files || []);
    if (!internal && externalFiles.length === 0) return;
    event.preventDefault();
    event.stopPropagation();
    setDropTargetKey(null);
    clearFileDragPayload();
    if (internal) onMoveItems(internal, item.id);
    else onUploadFilesToFolder(externalFiles, item.id);
  };

  function renderRow(item: ContentItem, index: number) {
    const key = fileItemKey(item);
    const meta = item.type === 'folder'
      ? `文件夹 · ${formatDate(item.updated_at || item.created_at)}`
      : `${formatSize(item.size)} · ${formatDate(item.updated_at || item.created_at)}`;
    const renaming = inlineEdit?.mode === 'rename' && fileItemKey(inlineEdit.item) === key;
    const selected = selectedKeys.has(key);
    const location = item.parent_id == null ? null : searchPaths[item.parent_id];
    return <div
      className={`file-list-row ${section === 'search' ? 'search-row' : ''} ${selected ? 'selected' : ''} ${dropTargetKey === key ? 'drop-target' : ''}`}
      key={key}
      title={section === 'search' && location ? `${meta} · ${location}` : meta}
      onContextMenu={event => openItemMenu(event, item)}
      draggable={!renaming && !busy}
      onDragStart={event => {
        const dragItems = selected ? selectedItems : [item];
        if (!selected) onSelectionChange([item]);
        writeFileDragPayload(event.dataTransfer, dragItems);
      }}
      onDragEnd={() => { clearFileDragPayload(); setDropTargetKey(null); }}
      onDragOver={item.type === 'folder' ? event => handleFolderDragOver(event, item) : undefined}
      onDragLeave={item.type === 'folder' ? event => { if (!event.currentTarget.contains(event.relatedTarget as Node)) setDropTargetKey(null); } : undefined}
      onDrop={item.type === 'folder' ? event => handleFolderDrop(event, item) : undefined}
    >
      <input type="checkbox" className="row-select" aria-label={`选择「${item.name}」`} checked={selected} disabled={renaming || busy} onChange={() => toggleItem(item, index)} />
      {renaming ? <div className="row-primary"><FileGlyph item={item} size={20} /><InlineNameInput className="inline-name-input" initialValue={item.name} selectStem={item.type === 'file'} onCommit={onCommitInlineEdit} onCancel={onCancelInlineEdit} /></div>
        : <button type="button" className="row-primary" title={item.name} onClick={event => {
            if (event.ctrlKey || event.metaKey || event.shiftKey) { selectFromEvent(item, index, event); return; }
            onSelectionChange([]);
            onOpen(item);
          }}><FileGlyph item={item} size={20} /><span className="row-label"><span className="row-name truncate">{item.name}</span>{section === 'search' && <span className="row-location truncate" title={location || undefined}>{location === undefined ? '路径加载中…' : location || '路径暂不可用'}</span>}</span></button>}
      <span className="row-type">{item.type === 'folder' ? '文件夹' : describeFile(item.name).label}</span>
      <span className="row-size">{item.type === 'folder' ? '—' : formatSize(item.size)}</span>
      <span className="row-date">{formatDate(item.updated_at || item.created_at)}</span>
      <div className="row-actions">{section === 'search' && item.parent_id != null && <button type="button" className="locate-folder" title="定位到文件夹" aria-label="定位到文件夹" onClick={() => onLocateFolder(item.parent_id!)}><Icon name="folder" size={15} /></button>}<button type="button" className="icon-button" title="更多操作" aria-label={`更多操作：${item.name}`} onClick={event => openItemMenuFromButton(event, item)}><Icon name="more" size={17} /></button></div>
    </div>;
  }

  function renderTrashRow(item: TrashItem) {
    return <div className="file-list-row trash-row" key={item.id} title={`删除于 ${formatDate(item.trashed_at)}；计划清理于 ${formatDate(item.expires_at)}`}>
      <div className="row-primary"><span className="folder-glyph"><Icon name={item.type === 'folder' ? 'folder' : 'file'} size={20} /></span><span className="row-name truncate">{item.name}</span></div>
      <span className="trash-expiry">{formatTrashExpiry(item.expires_at)}</span>
      <div className="row-actions"><Button icon="arrowLeft" label="恢复" onClick={() => onRestore(item)} /><Button icon="trash" label="永久删除" onClick={() => onPermanentlyDelete(item)} /></div>
    </div>;
  }

  const openSortMenu = (event: MouseEvent<HTMLButtonElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    showMenu({ x: rect.left, y: rect.bottom + 5 }, SORT_OPTIONS.map(option => ({
      label: option.label,
      checked: option.field === sortField && option.order === sortOrder,
      action: () => onSortChange(option.field, option.order),
    })));
  };

  const openFilterMenu = (event: MouseEvent<HTMLButtonElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const options: Array<{ kind: SearchKind; label: string }> = [{ kind: 'all', label: '全部' }, { kind: 'file', label: '只看文件' }, { kind: 'folder', label: '只看文件夹' }];
    showMenu({ x: rect.left, y: rect.bottom + 5 }, options.map(option => ({ label: option.label, checked: searchKind === option.kind, action: () => onSearchKindChange(option.kind) })));
  };

  const hasInlineCreate = section === 'files' && inlineEdit?.mode === 'create';
  const showContentList = section !== 'trash' && (contentItems.length > 0 || hasInlineCreate);

  return <div className="listing-page" onClick={event => { if (event.target === event.currentTarget) onSelectionChange([]); }}>
    <div className="listing-heading"><div className="listing-title"><Icon name={section === 'trash' ? 'trash' : section === 'search' ? 'search' : 'folder'} size={18} /><h1>{heading}</h1>{currentPage && <span className="listing-count">{currentPage.total} 项</span>}</div>
      {section === 'files' && <div className="quick-actions"><button onClick={() => onStartCreate('note')} disabled={busy}><Icon name="plus" size={15} />新建笔记</button><button onClick={() => onStartCreate('folder')} disabled={busy}><Icon name="folderPlus" size={15} />文件夹</button><button onClick={onUpload} disabled={busy || uploading}><Icon name="upload" size={15} />上传</button></div>}
      <div className="listing-tools">
      {loading && <span className="loading-dot" role="status" aria-label="正在加载" />}
      {section === 'search' && <button type="button" className={`icon-button list-tool-button ${searchKind !== 'all' ? 'active' : ''}`} title="筛选搜索结果" aria-label="筛选搜索结果" onClick={openFilterMenu}><Icon name="filter" size={17} /></button>}
      {section !== 'trash' && <button type="button" className="icon-button list-tool-button" title="排序" aria-label="排序" onClick={openSortMenu}><Icon name="sort" size={17} /></button>}
      {section === 'files' && <Button icon="clock" label="刷新" onClick={onRefresh} />}
    </div></div>
    {section === 'search' && <div className="main-search"><Icon name="search" size={20} /><input ref={searchRef} value={query} onChange={event => onQueryChange(event.target.value)} placeholder="搜索文件或文件夹" /></div>}
    {selectedItems.length > 0 && <div className="selection-bar"><strong>已选 {selectedItems.length} 项</strong><div>{selectedItems.length < contentItems.length && <button type="button" onClick={() => onSelectionChange(contentItems)}>全选本页</button>}<button type="button" onClick={() => onRequestMove(selectedItems)}><Icon name="move" size={16} />移动</button><button type="button" className="danger" onClick={() => onRemoveItems(selectedItems)}><Icon name="trash" size={16} />删除</button><button type="button" onClick={() => onSelectionChange([])}>取消</button></div></div>}
    {section === 'search' && !query.trim() ? <Empty icon="search" title="开始搜索" text="输入文件或文件夹名称，查找你的内容。" />
      : loading ? <div className="list-loading" role="status"><span className="loading-dot" />正在加载内容…</div>
      : loadError ? <Empty icon="folder" title="加载失败" text={loadError}
          action={<button type="button" className="empty-action" onClick={onRetry}>重试加载</button>} />
      : section === 'trash' && items.length ? <div className="file-list">{(items as TrashItem[]).map(renderTrashRow)}</div>
      : showContentList ? <div className="file-list">
          <div className="list-header"><input type="checkbox" className="row-select" aria-label="全选本页" checked={contentItems.length > 0 && selectedItems.length === contentItems.length} disabled={busy || !contentItems.length} onChange={event => onSelectionChange(event.target.checked ? contentItems : [])} /><span>名称</span><span className="row-type">类型</span><span className="row-size">大小</span><span className="row-date">修改时间</span><span /></div>
          {hasInlineCreate && <div className="file-list-row inline-create-row"><span className="row-select-placeholder" aria-hidden="true" /><div className="row-primary"><FileGlyph item={{ id: -1, type: inlineEdit.kind === 'folder' ? 'folder' : 'file', parent_id: null, name: inlineEdit.initialValue, created_at: '', updated_at: '' }} size={20} /><InlineNameInput className="inline-name-input" initialValue={inlineEdit.initialValue} selectStem={inlineEdit.kind === 'note'} onCommit={onCommitInlineEdit} onCancel={onCancelInlineEdit} /></div></div>}
          {contentItems.map(renderRow)}
        </div>
      : <Empty icon={section === 'trash' ? 'trash' : section === 'search' ? 'search' : 'folder'}
          title={section === 'trash' ? '回收站是空的' : section === 'search' ? '没有找到' : '这里还是空的'}
          text={section === 'trash' ? '移到回收站的文件会暂时保存在这里。' : section === 'search' ? '试试缩短关键词，或切换文件类型筛选。' : '上传文件，或在上方新建笔记和文件夹。'}
          action={section === 'files' ? <button type="button" className="empty-action" onClick={onUpload} disabled={busy || uploading}><Icon name="upload" size={17} />上传文件</button> : undefined} />}
    {!!currentPage && currentPage.total_pages > 1 && <div className="pagination"><button aria-label="上一页" disabled={page <= 1} onClick={() => onPageChange(page - 1)}><Icon name="arrowLeft" size={16} /></button><span>{page} / {currentPage.total_pages}</span><button aria-label="下一页" disabled={page >= currentPage.total_pages} onClick={() => onPageChange(page + 1)}><Icon name="chevron" size={16} /></button></div>}
    {menu && <ContextMenu position={menu.position} items={menu.items} onClose={() => setMenu(null)} />}
  </div>;
}
