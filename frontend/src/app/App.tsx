import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ChangeEvent, DragEvent as ReactDragEvent, FormEvent, KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from 'react';
import { api, ApiError, getToken, isOfflinePreview, setToken } from '../services/api';
import type { ContentItem, ContentSortField, Folder, Page, Profile, Quota, SearchKind, SortOrder, TrashItem } from '../services/api';
import Button from '../components/Button';
import Empty from '../components/Empty';
import Icon from '../components/Icon';
import UploadStatus from '../components/UploadStatus';
import useAutoDismiss from '../components/useAutoDismiss';
import type { UploadProgress } from '../components/UploadStatus';
import AuthPage from '../features/auth/AuthPage';
import { registrationError } from '../features/auth/validation';
import Explorer from '../features/explorer/Explorer';
import FileList from '../features/files/FileList';
import type { InlineFileEdit, Section } from '../features/files/FileList';
import MoveDialog from '../features/files/MoveDialog';
import {
  canMoveItemsTo,
  clearFileDragPayload,
  compactOperationItems,
  dataTransferHasFiles,
  fileItemKey,
  folderIsWithin,
  readFileDragPayload,
} from '../features/files/fileOperations';
import type { FileOperationItem } from '../features/files/fileOperations';
import { describeFile, formatSize } from '../features/files/fileUtils';
import Inspector from '../features/inspector/Inspector';
import DocumentPreview from '../features/preview/DocumentPreview';
import DocumentTabs from '../features/preview/DocumentTabs';
import { getOutline } from '../features/preview/markdownParser';
import useFilePreview from '../features/preview/useFilePreview';
import useMarkdownDrafts, { draftHasUnsavedChanges } from '../features/preview/useMarkdownDrafts';
import { useWorkspaceStore } from '../stores/workspaceStore';
import { calculatePanelLayout, clampPanel, PANEL_DEFAULTS, readPanelPreference, resetPanelPreferences, savePanelPreference } from '../stores/previewSlice';
import type { PanelSide } from '../stores/previewSlice';
import { userErrorMessage } from '../services/userMessages';

function uniqueDraftName(items: ContentItem[], baseName: string): string {
  const names = new Set(items.map(item => item.name));
  if (!names.has(baseName)) return baseName;
  const dot = baseName.lastIndexOf('.');
  const stem = dot > 0 ? baseName.slice(0, dot) : baseName;
  const extension = dot > 0 ? baseName.slice(dot) : '';
  let index = 2;
  while (names.has(`${stem} ${index}${extension}`)) index += 1;
  return `${stem} ${index}${extension}`;
}

function validItemName(value: string): boolean {
  return !!value && value !== '.' && value !== '..' && !/[\\/]/.test(value);
}

function LayoutSettingsDialog({ left, right, leftVisible, rightVisible, returnFocus, onLeftChange, onRightChange, onReset, onClose }: {
  left: number;
  right: number;
  leftVisible: boolean;
  rightVisible: boolean;
  returnFocus: HTMLElement | null;
  onLeftChange: (width: number) => void;
  onRightChange: (width: number) => void;
  onReset: () => void;
  onClose: () => void;
}) {
  const cardRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    cardRef.current?.querySelector<HTMLButtonElement>('[data-settings-close]')?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closeRef.current();
      }
      if (event.key !== 'Tab') return;
      const controls = Array.from(cardRef.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled)') || []);
      if (!controls.length) { event.preventDefault(); return; }
      if (event.shiftKey && document.activeElement === controls[0]) {
        event.preventDefault();
        controls.at(-1)?.focus();
      } else if (!event.shiftKey && document.activeElement === controls.at(-1)) {
        event.preventDefault();
        controls[0].focus();
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
      if (returnFocus?.isConnected) returnFocus.focus();
      else if (previousFocus?.isConnected && previousFocus !== document.body) previousFocus.focus();
    };
  }, []);

  return <div className="layout-settings-overlay" onMouseDown={event => { if (event.target === event.currentTarget) onClose(); }}>
    <div ref={cardRef} className="layout-settings-dialog" role="dialog" aria-modal="true" aria-labelledby="layout-settings-title" onMouseDown={event => event.stopPropagation()}>
      <header className="layout-settings-header"><button type="button" data-settings-close aria-label="返回文件空间" onClick={onClose}><Icon name="arrowLeft" size={20} /></button><h2 id="layout-settings-title">设置</h2></header>
      <div className="layout-settings-body">
        <nav aria-label="设置分类"><button type="button" className="active" aria-current="page"><Icon name="columns" size={17} />界面</button></nav>
        <div className="layout-settings-main">
          <h3>界面</h3><p className="layout-settings-intro">拖动画面分隔线，或在这里滑动调节面板宽度。设置只保存在当前浏览器中。</p>
          <section className="layout-settings-group" aria-labelledby="layout-size-title"><h4 id="layout-size-title">工作区比例</h4>
            <div className="layout-settings-preview" aria-hidden="true"><span style={{ flex: `${left} 1 0` }}>目录</span><span style={{ flex: '400 1 0' }}>文件</span><span style={{ flex: `${right} 1 0` }}>详情</span></div>
            <div className="layout-settings-control"><label htmlFor="explorer-width">目录栏宽度 <output htmlFor="explorer-width">{left} px</output></label><input id="explorer-width" type="range" min={180} max={400} step={1} value={left} disabled={!leftVisible} onChange={event => onLeftChange(Number(event.currentTarget.value))} />{!leftVisible && <small>窄屏时目录栏会暂时隐藏。</small>}</div>
            <div className="layout-settings-control"><label htmlFor="inspector-width">详情栏宽度 <output htmlFor="inspector-width">{right} px</output></label><input id="inspector-width" type="range" min={220} max={600} step={1} value={right} disabled={!rightVisible} onChange={event => onRightChange(Number(event.currentTarget.value))} />{!rightVisible && <small>窄屏时详情栏浮在文件区上方，不占并排宽度。</small>}</div>
            <p className="layout-settings-note">窗口缩小时，会暂时收窄面板，优先为文件区留下空间；保存的宽度不会被覆盖。</p>
            <button type="button" className="layout-settings-reset" onClick={onReset}>恢复默认宽度</button>
          </section>
        </div>
      </div>
    </div>
  </div>;
}

export default function App() {
  const [token, setAuthToken] = useState<string | null>(getToken);
  const [authMode, setAuthMode] = useState<'login' | 'register'>('login');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [authBusy, setAuthBusy] = useState(false);
  const [profile, setProfile] = useState<Profile | null>(null);
  const [quota, setQuota] = useState<Quota | null>(null);
  const [rootId, setRootId] = useState<number | null>(null);
  const [folders, setFolders] = useState<Record<number, Folder>>({});
  const [children, setChildren] = useState<Record<number, number[]>>({});
  const [treeFiles, setTreeFiles] = useState<Record<number, ContentItem[]>>({});
  const [managingFiles, setManagingFiles] = useState(false);
  const [immersive, setImmersive] = useState(false);
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [activeFolderId, setActiveFolderId] = useState<number | null>(null);
  const [listing, setListing] = useState<Page<ContentItem> | null>(null);
  const [searchPage, setSearchPage] = useState<Page<ContentItem> | null>(null);
  const [searchPaths, setSearchPaths] = useState<Record<number, string | null>>({});
  const [mobileExplorerOpen, setMobileExplorerOpen] = useState(false);
  const [trashPage, setTrashPage] = useState<Page<TrashItem> | null>(null);
  const [inlineEdit, setInlineEdit] = useState<InlineFileEdit>(null);
  const [selectedItems, setSelectedItems] = useState<ContentItem[]>([]);
  const [moveDialogItems, setMoveDialogItems] = useState<FileOperationItem[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<UploadProgress | null>(null);
  const [loadingSection, setLoadingSection] = useState<Section | null>(null);
  const [listError, setListError] = useState<{ section: Section; message: string } | null>(null);
  const [bootRetry, setBootRetry] = useState(0);
  const [listRetry, setListRetry] = useState(0);
  const section = useWorkspaceStore(state => state.section);
  const setSection = useWorkspaceStore(state => state.setSection);
  const page = useWorkspaceStore(state => state.page);
  const setPage = useWorkspaceStore(state => state.setPage);
  const query = useWorkspaceStore(state => state.query);
  const setQuery = useWorkspaceStore(state => state.setQuery);
  const sort = useWorkspaceStore(state => state.sort);
  const setSort = useWorkspaceStore(state => state.setSort);
  const searchKind = useWorkspaceStore(state => state.searchKind);
  const setSearchKind = useWorkspaceStore(state => state.setSearchKind);
  const tabs = useWorkspaceStore(state => state.tabs);
  const setTabs = useWorkspaceStore(state => state.setTabs);
  const activeFileId = useWorkspaceStore(state => state.activeFileId);
  const setActiveFileId = useWorkspaceStore(state => state.setActiveFileId);
  const rightOpen = useWorkspaceStore(state => state.rightOpen);
  const setRightOpen = useWorkspaceStore(state => state.setRightOpen);
  const rightTab = useWorkspaceStore(state => state.rightTab);
  const setRightTab = useWorkspaceStore(state => state.setRightTab);
  const error = useWorkspaceStore(state => state.error);
  const errorRevision = useWorkspaceStore(state => state.errorRevision);
  const setError = useWorkspaceStore(state => state.setError);
  const errorDismissEvents = useAutoDismiss(!!error, errorRevision, () => setError(''));
  const notice = useWorkspaceStore(state => state.notice);
  const setNotice = useWorkspaceStore(state => state.setNotice);
  const [fileDragActive, setFileDragActive] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [explorerWidth, setExplorerWidth] = useState(() => readPanelPreference('left'));
  const [inspectorWidth, setInspectorWidth] = useState(() => readPanelPreference('right'));
  const [viewportWidth, setViewportWidth] = useState(() => window.innerWidth);
  const resizeCleanupRef = useRef<(() => void) | null>(null);
  const settingsTriggerRef = useRef<HTMLButtonElement>(null);
  const mobileFilesButtonRef = useRef<HTMLButtonElement>(null);
  const uploadRef = useRef<HTMLInputElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const noticeTimer = useRef<number | undefined>(undefined);
  const folderRequestRef = useRef(0);
  const treeRequestsRef = useRef(new Map<number, number>());
  const sessionGenerationRef = useRef(0);
  const fileDragDepthRef = useRef(0);
  const uploadTargetFolderRef = useRef<number | null>(null);
  const uploadInProgressRef = useRef(false);
  const uploadAbortRef = useRef<AbortController | null>(null);
  const uploadGenerationRef = useRef(0);
  const foldersRef = useRef(folders);
  foldersRef.current = folders;
  const locallySavedVersionsRef = useRef<Map<number, string>>(new Map());
  const saveInFlightRef = useRef<Map<number, symbol>>(new Map());
  const saveWaitersRef = useRef(new Map<number, Promise<string | null>>());
  const checkpointRetriesRef = useRef(new Set<number>());
  const sortRef = useRef(sort);
  const {
    drafts: markdownDrafts,
    ensureDraft,
    updateDraft,
    markSaving,
    markSaved,
    markConflict,
    markError,
    discardDraft,
    discardDrafts,
    hasUnsavedDrafts,
  } = useMarkdownDrafts();
  const markdownDraftsRef = useRef(markdownDrafts);
  markdownDraftsRef.current = markdownDrafts;

  useEffect(() => {
    const onResize = () => setViewportWidth(window.innerWidth);
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  useEffect(() => () => resizeCleanupRef.current?.(), []);

  useEffect(() => {
    if (viewportWidth > 720) setMobileExplorerOpen(false);
  }, [viewportWidth]);

  useEffect(() => {
    if (!mobileExplorerOpen) return;
    const onEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || document.querySelector('[aria-modal="true"]')) return;
      event.preventDefault();
      setMobileExplorerOpen(false);
      mobileFilesButtonRef.current?.focus();
    };
    window.addEventListener('keydown', onEscape);
    return () => window.removeEventListener('keydown', onEscape);
  }, [mobileExplorerOpen]);

  useEffect(() => {
    const narrow = window.matchMedia('(max-width: 980px)');
    const closeOnNarrow = () => { if (narrow.matches) setRightOpen(false); };
    closeOnNarrow();
    narrow.addEventListener('change', closeOnNarrow);
    return () => narrow.removeEventListener('change', closeOnNarrow);
  }, [setRightOpen]);

  useEffect(() => {
    if (!rightOpen) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && window.matchMedia('(max-width: 980px)').matches && !document.querySelector('[aria-modal="true"]')) setRightOpen(false);
    };
    window.addEventListener('keydown', closeOnEscape);
    return () => window.removeEventListener('keydown', closeOnEscape);
  }, [rightOpen, setRightOpen]);

  const toast = useCallback((message: string) => {
    setNotice(message);
    window.clearTimeout(noticeTimer.current);
    noticeTimer.current = window.setTimeout(() => setNotice(''), 3500);
  }, []);

  const clearSession = useCallback(() => {
    resizeCleanupRef.current?.();
    setSettingsOpen(false);
    setMobileExplorerOpen(false);
    setSearchPaths({});
    sessionGenerationRef.current += 1;
    folderRequestRef.current += 1;
    uploadGenerationRef.current += 1;
    saveInFlightRef.current.clear();
    saveWaitersRef.current.clear();
    checkpointRetriesRef.current.clear();
    locallySavedVersionsRef.current.clear();
    uploadAbortRef.current?.abort();
    uploadAbortRef.current = null;
    uploadInProgressRef.current = false;
    setUploadProgress(null);
    setBusy(false);
    setLoadingSection(null);
    setListError(null);
    discardDrafts(Object.keys(markdownDraftsRef.current).map(Number));
    setToken(null);
    setAuthToken(null);
    setProfile(null);
    setQuota(null);
    setRootId(null);
    setFolders({});
    setChildren({});
    setTreeFiles({});
    setManagingFiles(false);
    setImmersive(false);
    setExpanded(new Set());
    setActiveFolderId(null);
    setListing(null);
    setSearchPage(null);
    setTrashPage(null);
    setSelectedItems([]);
    setInlineEdit(null);
    setMoveDialogItems(null);
    useWorkspaceStore.getState().resetNavigation();
    useWorkspaceStore.getState().resetPreview();
    window.clearTimeout(noticeTimer.current);
    setNotice('');
  }, [discardDrafts]);

  const handleError = useCallback((cause: unknown) => {
    if (cause instanceof ApiError && cause.status === 401) {
      clearSession();
      setError('登录已过期，请重新登录。');
      return;
    }
    setError(userErrorMessage(cause));
  }, [clearSession]);

  const loadChildren = useCallback(async (folderId: number, contentsRequest?: Promise<Page<ContentItem>>, contentsSort: { sort: ContentSortField; order: SortOrder } = { sort: 'name', order: 'asc' }) => {
    const generation = sessionGenerationRef.current;
    const requestId = (treeRequestsRef.current.get(folderId) || 0) + 1;
    treeRequestsRef.current.set(folderId, requestId);
    const isCurrent = () => generation === sessionGenerationRef.current && treeRequestsRef.current.get(folderId) === requestId;
    const [result, first] = await Promise.all([api.children(folderId), contentsRequest || api.contents(folderId, 1, contentsSort)]);
    if (!isCurrent()) return;
    setFolders(previous => {
      const next = { ...previous, [result.folder.id]: result.folder };
      for (const child of result.children) next[child.id] = child;
      return next;
    });
    setChildren(previous => ({ ...previous, [folderId]: result.children.map(child => child.id) }));
    const items = [...first.items];
    for (let page = 2; page <= first.total_pages; page += 1) {
      if (!isCurrent()) return;
      items.push(...(await api.contents(folderId, page, contentsSort)).items);
    }
    if (isCurrent()) {
      setTreeFiles(previous => ({ ...previous, [folderId]: items.filter(item => item.type === 'file').sort((a, b) => a.name.localeCompare(b.name, 'zh-CN', { numeric: true })) }));
    }
  }, []);

  const loadFolder = useCallback(async (
    folderId: number,
    nextPage = 1,
    sortField = sortRef.current.field,
    sortOrder = sortRef.current.order,
  ) => {
    const generation = sessionGenerationRef.current;
    const requestId = ++folderRequestRef.current;
    setListing(null);
    setListError(previous => previous?.section === 'files' ? null : previous);
    if (useWorkspaceStore.getState().section === 'files') setLoadingSection('files');
    setError('');
    try {
      const contentsRequest = api.contents(folderId, nextPage, { sort: sortField, order: sortOrder });
      const [contents] = await Promise.all([
        contentsRequest,
        loadChildren(folderId, nextPage === 1 ? contentsRequest : undefined, { sort: sortField, order: sortOrder }),
      ]);
      if (generation === sessionGenerationRef.current && requestId === folderRequestRef.current) setListing(contents);
    } catch (cause) {
      if (generation !== sessionGenerationRef.current || requestId !== folderRequestRef.current) return;
      if (cause instanceof ApiError && cause.status === 401) handleError(cause);
      else setListError({ section: 'files', message: userErrorMessage(cause) });
    } finally {
      if (generation === sessionGenerationRef.current && requestId === folderRequestRef.current) setLoadingSection(current => current === 'files' ? null : current);
    }
  }, [handleError, loadChildren]);

  useEffect(() => {
    setSearchPaths({});
    if (!searchPage || rootId == null) return;
    let cancelled = false;
    const generation = sessionGenerationRef.current;
    const known = new Map<number, Folder>(Object.entries(foldersRef.current).map(([id, folder]) => [Number(id), folder]));
    const pending = new Map<number, Promise<Folder>>();
    const fetchFolder = (id: number): Promise<Folder> => {
      const cached = known.get(id);
      if (cached) return Promise.resolve(cached);
      const existing = pending.get(id);
      if (existing) return existing;
      const request = api.children(id).then(result => {
        known.set(id, result.folder);
        for (const child of result.children) known.set(child.id, child);
        if (!cancelled && generation === sessionGenerationRef.current) {
          setFolders(previous => ({ ...previous, [result.folder.id]: result.folder, ...Object.fromEntries(result.children.map(child => [child.id, child])) }));
          setChildren(previous => ({ ...previous, [id]: result.children.map(child => child.id) }));
        }
        return result.folder;
      });
      pending.set(id, request);
      return request;
    };
    const resolvePath = async (folderId: number): Promise<string | null> => {
      const names: string[] = [];
      const visited = new Set<number>();
      let id: number | null = folderId;
      while (id != null && !visited.has(id)) {
        visited.add(id);
        const folder: Folder = await fetchFolder(id);
        if (id === rootId) return ['我的空间', ...names].join(' / ');
        names.unshift(folder.name);
        id = folder.parent_id;
      }
      return null;
    };
    const ids = new Set(searchPage.items.map(item => item.parent_id).filter((id): id is number => id != null));
    for (const id of ids) {
      void resolvePath(id).then(path => {
        if (!cancelled && generation === sessionGenerationRef.current) setSearchPaths(previous => ({ ...previous, [id]: path }));
      }).catch(cause => {
        if (cancelled || generation !== sessionGenerationRef.current) return;
        if (cause instanceof ApiError && cause.status === 401) handleError(cause);
        else setSearchPaths(previous => ({ ...previous, [id]: null }));
      });
    }
    return () => { cancelled = true; };
  }, [searchPage, rootId, handleError]);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    let handedOff = false;
    setLoadingSection('files');
    setListError(null);
    Promise.all([api.profile(), api.quota(), api.root()]).then(([p, q, root]) => {
      if (cancelled) return;
      setProfile(p);
      setQuota(q);
      setRootId(root.root.id);
      setActiveFolderId(root.root.id);
      setFolders({ [root.root.id]: root.root, ...Object.fromEntries(root.children.map(child => [child.id, child])) });
      setChildren({ [root.root.id]: root.children.map(child => child.id) });
      setExpanded(new Set([root.root.id]));
      handedOff = true;
      void loadFolder(root.root.id);
    }).catch(cause => {
      if (cancelled) return;
      if (cause instanceof ApiError && cause.status === 401) handleError(cause);
      else setListError({ section: 'files', message: userErrorMessage(cause) });
    }).finally(() => {
      if (!cancelled && !handedOff) setLoadingSection(current => current === 'files' ? null : current);
    });
    return () => { cancelled = true; };
  }, [token, handleError, loadFolder, bootRetry]);

  useEffect(() => {
    if (!token || section === 'files') return;
    setListError(previous => previous?.section === section ? null : previous);
    if (section === 'search') setSearchPage(null);
    else setTrashPage(null);
    if (section === 'search' && !query.trim()) {
      setLoadingSection(current => current === 'search' ? null : current);
      return;
    }
    let cancelled = false;
    setLoadingSection(section);
    const timer = window.setTimeout(() => {
      const job = section === 'search'
        ? api.search(query.trim(), page, { kind: searchKind, sort: sort.field, order: sort.order })
        : api.trash(page);
      job.then(result => {
        if (cancelled) return;
        if (section === 'search') setSearchPage(result as Page<ContentItem>);
        else setTrashPage(result as Page<TrashItem>);
      }).catch(cause => {
        if (cancelled) return;
        if (cause instanceof ApiError && cause.status === 401) handleError(cause);
        else setListError({ section, message: userErrorMessage(cause) });
      }).finally(() => {
        if (!cancelled) setLoadingSection(current => current === section ? null : current);
      });
    }, section === 'search' ? 250 : 0);
    return () => { cancelled = true; window.clearTimeout(timer); };
  }, [token, section, query, page, searchKind, sort.field, sort.order, handleError, listRetry]);

  useEffect(() => {
    if (!fileDragActive) return;
    const clearDragState = () => {
      fileDragDepthRef.current = 0;
      setFileDragActive(false);
      clearFileDragPayload();
    };
    window.addEventListener('drop', clearDragState);
    window.addEventListener('dragend', clearDragState);
    window.addEventListener('blur', clearDragState);
    return () => {
      window.removeEventListener('drop', clearDragState);
      window.removeEventListener('dragend', clearDragState);
      window.removeEventListener('blur', clearDragState);
    };
  }, [fileDragActive]);

  const currentFile = tabs.find(file => file.id === activeFileId) || null;
  const activeFolder = activeFolderId == null ? null : folders[activeFolderId];
  const selectedKeys = useMemo(() => new Set(selectedItems.map(fileItemKey)), [selectedItems]);
  const { preview, reload: reloadPreview } = useFilePreview(currentFile);
  const currentDraft = currentFile ? markdownDrafts[currentFile.id] : undefined;
  const outlineSource = currentDraft?.content ?? (preview?.status === 'ready' ? preview.text : '') ?? '';
  const outline = useMemo(() => currentFile && preview?.fileId === currentFile.id && preview.status === 'ready' && preview.descriptor.kind === 'markdown' ? getOutline(outlineSource, String(currentFile.id)) : [], [currentFile, outlineSource, preview]);

  useEffect(() => {
    if (!currentFile || preview?.fileId !== currentFile.id || preview.status !== 'ready' || !(preview.descriptor.preview === 'markdown' || preview.descriptor.kind === 'text')) return;
    const shaMismatch = !!currentFile.sha256 && !!preview.sha256 && currentFile.sha256 !== preview.sha256;
    if (shaMismatch && locallySavedVersionsRef.current.get(currentFile.id) === currentFile.sha256) return;
    if (preview.sha256 && currentFile.sha256 !== preview.sha256) {
      const applyPreviewMetadata = (item: ContentItem): ContentItem => item.type === 'file' && item.id === currentFile.id
        ? {
            ...item,
            sha256: preview.sha256,
            size: preview.size ?? item.size,
            updated_at: preview.updatedAt || item.updated_at,
          }
        : item;
      setTabs(previous => previous.map(applyPreviewMetadata));
      setTreeFiles(previous => Object.fromEntries(Object.entries(previous).map(([id, files]) => [id, files.map(applyPreviewMetadata)])));
      setListing(previous => previous ? { ...previous, items: previous.items.map(applyPreviewMetadata) } : previous);
      setSearchPage(previous => previous ? { ...previous, items: previous.items.map(applyPreviewMetadata) } : previous);
    }
    ensureDraft(currentFile.id, preview.text || '', preview.sha256 || currentFile.sha256 || null);
  }, [currentFile, ensureDraft, preview]);

  useEffect(() => {
    if (!hasUnsavedDrafts) return;
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', warnBeforeUnload);
    return () => window.removeEventListener('beforeunload', warnBeforeUnload);
  }, [hasUnsavedDrafts]);

  const refresh = useCallback(async () => {
    if (activeFolderId != null) await loadFolder(activeFolderId, section === 'files' ? page : 1, sort.field, sort.order);
    if (section === 'trash') {
      try {
        setTrashPage(await api.trash(page));
        setListError(previous => previous?.section === 'trash' ? null : previous);
      } catch (cause) {
        setTrashPage(null);
        setListError({ section: 'trash', message: userErrorMessage(cause) });
        throw cause;
      }
    }
    if (section === 'search' && query.trim()) {
      try {
        setSearchPage(await api.search(query.trim(), page, { kind: searchKind, sort: sort.field, order: sort.order }));
        setListError(previous => previous?.section === 'search' ? null : previous);
      } catch (cause) {
        setSearchPage(null);
        setListError({ section: 'search', message: userErrorMessage(cause) });
        throw cause;
      }
    }
    setQuota(await api.quota());
  }, [activeFolderId, loadFolder, section, page, query, searchKind, sort.field, sort.order]);
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;

  async function runAction(action: () => Promise<unknown>, success: string): Promise<boolean> {
    setBusy(true);
    setError('');
    try { await action(); await refresh(); toast(success); return true; }
    catch (cause) { handleError(cause); return false; }
    finally { setBusy(false); }
  }

  async function authenticate(event: FormEvent) {
    event.preventDefault();
    if (authMode === 'register') {
      const validationError = registrationError(username, password);
      if (validationError) { setError(validationError); return; }
    }
    setAuthBusy(true);
    setError('');
    try {
      const response = authMode === 'login' ? await api.login(username.trim(), password) : await api.register(username.trim(), password);
      setToken(response.access_token);
      setAuthToken(response.access_token);
      setPassword('');
    } catch (cause) { setError(userErrorMessage(cause, authMode)); }
    finally { setAuthBusy(false); }
  }

  function logout() {
    if (Object.values(markdownDrafts).some(draft => draft.state === 'saving')) {
      setError('文件仍在保存中，请稍候再退出。');
      return;
    }
    if (hasUnsavedDrafts && !window.confirm('仍有未保存的文本草稿，确定退出登录吗？')) return;
    clearSession();
    setError('');
  }

  function selectFolder(id: number) {
    setManagingFiles(id === rootId);
    if (id === rootId) setMobileExplorerOpen(false);
    setSection('files');
    setPage(1);
    setActiveFolderId(id);
    setActiveFileId(null);
    setSelectedItems([]);
    setInlineEdit(null);
    setExpanded(previous => {
      const next = new Set([...previous, id]);
      let ancestor = foldersRef.current[id]?.parent_id;
      const visited = new Set<number>([id]);
      while (ancestor != null && !visited.has(ancestor)) {
        visited.add(ancestor);
        next.add(ancestor);
        ancestor = foldersRef.current[ancestor]?.parent_id;
      }
      return next;
    });
    void loadFolder(id, 1, sort.field, sort.order);
  }

  async function toggleFolder(id: number) {
    if (expanded.has(id)) {
      setExpanded(previous => { const next = new Set(previous); next.delete(id); return next; });
      return;
    }
    setExpanded(previous => new Set([...previous, id]));
    try { await loadChildren(id); } catch (cause) { handleError(cause); }
  }

  function selectSection(next: Section) {
    setManagingFiles(false);
    setImmersive(false);
    setMobileExplorerOpen(false);
    setSection(next);
    setPage(1);
    setActiveFileId(null);
    setSelectedItems([]);
    setInlineEdit(null);
    if (next === 'search') window.setTimeout(() => searchRef.current?.focus(), 0);
    if (next === 'files' && activeFolderId != null) void loadFolder(activeFolderId, 1, sort.field, sort.order);
  }

  function openFile(item: ContentItem) {
    if (item.type === 'folder') { selectFolder(item.id); return; }
    setMobileExplorerOpen(false);
    setManagingFiles(false);
    setTabs(previous => previous.some(file => file.id === item.id) ? previous.map(file => file.id === item.id ? item : file) : [...previous, item]);
    setActiveFileId(item.id);
    setSection('files');
    if (item.parent_id != null && item.parent_id !== activeFolderId) {
      setActiveFolderId(item.parent_id);
      setPage(1);
      void loadFolder(item.parent_id, 1, sort.field, sort.order);
    }
  }

  function closeTab(id: number) {
    const draft = markdownDrafts[id];
    if (draft?.state === 'saving') {
      setError('文件仍在保存中，请稍候再关闭。');
      return;
    }
    if (draftHasUnsavedChanges(draft) && !window.confirm('该文件仍有未保存的修改，确定关闭吗？')) return;
    discardDraft(id);
    const next = tabs.filter(file => file.id !== id);
    setTabs(next);
    if (activeFileId === id) setActiveFileId(next.at(-1)?.id || null);
  }

  async function download(item: ContentItem) {
    setError('');
    try {
      const blob = await api.download(item.id);
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = item.name;
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 30_000);
    } catch (cause) { handleError(cause); }
  }

  async function saveCurrentMarkdown(createVersion?: boolean): Promise<void> {
    if (!currentFile) return;
    return saveMarkdown(currentFile.id, createVersion ?? checkpointRetriesRef.current.has(currentFile.id));
  }

  async function saveMarkdown(fileId: number, createVersion: boolean, expectedOverride?: string): Promise<void> {
    if (createVersion) checkpointRetriesRef.current.add(fileId);
    const generation = sessionGenerationRef.current;
    const pending = saveWaitersRef.current.get(fileId);
    if (pending) {
      if (!createVersion) return;
      const sha = await pending;
      if (sha && generation === sessionGenerationRef.current) await saveMarkdown(fileId, true, sha);
      return;
    }
    const draft = markdownDraftsRef.current[fileId];
    if (!draft || (!createVersion && !draftHasUnsavedChanges(draft)) || draft.state === 'conflict') return;
    const requestToken = Symbol();
    saveInFlightRef.current.set(fileId, requestToken);
    let finish!: (sha: string | null) => void;
    let savedSHA: string | null = null;
    saveWaitersRef.current.set(fileId, new Promise(resolve => { finish = resolve; }));
    markSaving(fileId);
    try {
      const saved = await api.updateFileContent(fileId, draft.content, expectedOverride || draft.expectedSHA256, createVersion);
      if (generation !== sessionGenerationRef.current) return;
      savedSHA = saved.sha256;
      if (createVersion) checkpointRetriesRef.current.delete(fileId);
      markSaved(fileId, draft.content, saved.sha256);
      // The next autosave may start while metadata is being refreshed.
      locallySavedVersionsRef.current.set(saved.id, saved.sha256);
      window.setTimeout(() => {
        if (locallySavedVersionsRef.current.get(saved.id) === saved.sha256) locallySavedVersionsRef.current.delete(saved.id);
      }, 0);
      const applySavedMetadata = (item: ContentItem): ContentItem => item.type === 'file' && item.id === saved.id
        ? { ...item, name: saved.name, size: saved.size, sha256: saved.sha256, updated_at: saved.updated_at }
        : item;
      setTabs(previous => previous.map(applySavedMetadata));
      setTreeFiles(previous => Object.fromEntries(Object.entries(previous).map(([id, files]) => [id, files.map(applySavedMetadata)])));
      setListing(previous => previous ? { ...previous, items: previous.items.map(applySavedMetadata) } : previous);
      setSearchPage(previous => previous ? { ...previous, items: previous.items.map(applySavedMetadata) } : previous);
      if (createVersion) toast('已保存版本快照');
      // Keep editing uninterrupted: metadata is already patched locally, and
      // a full folder refresh would churn the view on every autosave.
      void api.quota().then(next => {
        if (generation === sessionGenerationRef.current) setQuota(next);
      }).catch(() => { /* The document is saved even if quota refresh fails. */ });
    } catch (cause) {
      if (generation !== sessionGenerationRef.current) return;
      if (cause instanceof ApiError && cause.status === 412) {
        markConflict(fileId, '服务器上的文件版本比当前草稿更新，本地草稿仍被保留。');
        return;
      }
      const message = userErrorMessage(cause);
      markError(fileId, message);
      if (cause instanceof ApiError && cause.status === 401) handleError(cause);
    } finally {
      if (saveInFlightRef.current.get(fileId) === requestToken) {
        saveInFlightRef.current.delete(fileId);
        saveWaitersRef.current.delete(fileId);
      }
      finish(savedSHA);
    }
  }

  function discardCurrentDraftAndReload() {
    if (!currentFile) return;
    const draft = markdownDrafts[currentFile.id];
    if (draftHasUnsavedChanges(draft) && !window.confirm('放弃当前未保存的草稿并重新加载远端内容？')) return;
    discardDraft(currentFile.id);
    reloadPreview();
  }

  async function restoreCurrentVersion(versionId: number): Promise<boolean> {
    if (!currentFile) return false;
    const draft = markdownDrafts[currentFile.id];
    if (draft?.state === 'saving') {
      setError('文件仍在保存中，请稍候再恢复历史版本。');
      return false;
    }
    if (draftHasUnsavedChanges(draft) && !window.confirm('恢复历史版本会放弃当前未保存的草稿，是否继续？')) return false;
    const expectedSHA256 = draft?.expectedSHA256 || currentFile.sha256;
    if (!expectedSHA256) {
      setError('当前文件版本信息缺失，请重新加载后再试。');
      return false;
    }
    setBusy(true);
    setError('');
    try {
      const restored = await api.restoreFileVersion(currentFile.id, versionId, expectedSHA256);
      const applyRestoredMetadata = (item: ContentItem): ContentItem => item.type === 'file' && item.id === restored.id
        ? { ...item, name: restored.name, size: restored.size, sha256: restored.sha256, updated_at: restored.updated_at }
        : item;
      discardDraft(currentFile.id);
      locallySavedVersionsRef.current.set(restored.id, restored.sha256);
      window.setTimeout(() => {
        if (locallySavedVersionsRef.current.get(restored.id) === restored.sha256) locallySavedVersionsRef.current.delete(restored.id);
      }, 0);
      setTabs(previous => previous.map(applyRestoredMetadata));
      setListing(previous => previous ? { ...previous, items: previous.items.map(applyRestoredMetadata) } : previous);
      setSearchPage(previous => previous ? { ...previous, items: previous.items.map(applyRestoredMetadata) } : previous);
      reloadPreview();
      toast('历史版本已恢复，恢复前的内容仍保留在版本记录中');
      try { await refresh(); } catch { /* Restore succeeded; metadata refresh can wait. */ }
      return true;
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 412 && draft) {
        markConflict(currentFile.id, '恢复前检测到服务器文件已经变化，本地草稿仍被保留。');
      } else {
        handleError(cause);
      }
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function uploadFiles(files: File[], folderId = activeFolderId, retainedFailures: UploadProgress['failures'] = []): Promise<void> {
    if (!files.length || folderId == null) return;
    if (uploadInProgressRef.current) { toast('已有文件正在上传，请等待完成后再添加。'); return; }
    const controller = new AbortController();
    const generation = ++uploadGenerationRef.current;
    uploadAbortRef.current = controller;
    uploadInProgressRef.current = true;
    let succeeded = 0;
    let failed = retainedFailures.length;
    let transferredBytes = 0;
    const totalBytes = files.reduce((total, file) => total + file.size, 0);
    const failures: UploadProgress['failures'] = [...retainedFailures];
    const progress = (status: UploadProgress['status'], phase: UploadProgress['phase'], currentName: string | null, currentPercent = 0, currentSize = 0) => {
      const bytes = Math.min(totalBytes, transferredBytes + Math.round(currentSize * currentPercent / 100));
      const percent = totalBytes > 0
        ? Math.round(bytes / totalBytes * 100)
        : Math.round((succeeded + failed) / (files.length + retainedFailures.length) * 100);
      setUploadProgress({ folderId, total: files.length + retainedFailures.length, completed: succeeded + failed, succeeded, failed, currentName, status, phase, failures: [...failures], percent, currentPercent, transferredBytes: bytes, totalBytes });
    };
    progress('uploading', 'transferring', files[0].name);
    try {
      for (const file of files) {
        if (controller.signal.aborted) break;
        let currentTransferred = 0;
        progress('uploading', 'transferring', file.name);
        try {
          await api.uploadWithProgress(folderId, file, controller.signal, percent => {
            if (generation !== uploadGenerationRef.current) return;
            currentTransferred = Math.round(file.size * percent / 100);
            progress('uploading', percent >= 100 ? 'processing' : 'transferring', file.name, percent, file.size);
          });
          transferredBytes += file.size;
          succeeded += 1;
        } catch (cause) {
          transferredBytes += currentTransferred;
          if (controller.signal.aborted) break;
          if (cause instanceof ApiError && cause.status === 401) { handleError(cause); return; }
          failed += 1;
          failures.push({ file, message: userErrorMessage(cause) });
        }
        if (generation !== uploadGenerationRef.current) return;
      }
      if (generation !== uploadGenerationRef.current) return;
      if (succeeded || controller.signal.aborted) {
        progress('uploading', 'refreshing', null);
        try { await refreshRef.current(); }
        catch (cause) { handleError(cause); }
      }
      if (generation !== uploadGenerationRef.current) return;
      progress(controller.signal.aborted ? 'cancelled' : 'complete', 'refreshing', null);
      if (succeeded) toast(`成功上传 ${succeeded} 个文件`);
    } finally {
      if (generation === uploadGenerationRef.current) {
        uploadInProgressRef.current = false;
        uploadAbortRef.current = null;
      }
    }
  }

  function chooseUploadTarget(folderId: number) {
    if (uploadInProgressRef.current) { toast('已有文件正在上传，请等待完成后再添加。'); return; }
    uploadTargetFolderRef.current = folderId;
    uploadRef.current?.click();
  }

  async function importFiles(event: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.target.files || []);
    event.target.value = '';
    const folderId = uploadTargetFolderRef.current ?? activeFolderId;
    uploadTargetFolderRef.current = null;
    await uploadFiles(files, folderId);
  }

  function canAcceptWorkspaceDrop(event: ReactDragEvent<HTMLElement>): boolean {
    if (busy || section !== 'files' || activeFolderId == null) return false;
    return !!readFileDragPayload(event.dataTransfer) || dataTransferHasFiles(event.dataTransfer);
  }

  function handleWorkspaceDragEnter(event: ReactDragEvent<HTMLElement>) {
    if (!canAcceptWorkspaceDrop(event)) return;
    event.preventDefault();
    fileDragDepthRef.current += 1;
    setFileDragActive(true);
  }

  function handleWorkspaceDragOver(event: ReactDragEvent<HTMLElement>) {
    if (!canAcceptWorkspaceDrop(event)) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = readFileDragPayload(event.dataTransfer) ? 'move' : 'copy';
  }

  function handleWorkspaceDragLeave(event: ReactDragEvent<HTMLElement>) {
    if (!fileDragActive) return;
    event.preventDefault();
    fileDragDepthRef.current = Math.max(0, fileDragDepthRef.current - 1);
    if (fileDragDepthRef.current === 0) setFileDragActive(false);
  }

  function handleWorkspaceDrop(event: ReactDragEvent<HTMLElement>) {
    if (!canAcceptWorkspaceDrop(event) || activeFolderId == null) return;
    const internal = readFileDragPayload(event.dataTransfer);
    const files = Array.from(event.dataTransfer.files || []);
    event.preventDefault();
    event.stopPropagation();
    fileDragDepthRef.current = 0;
    setFileDragActive(false);
    clearFileDragPayload();
    if (internal) void moveItems(internal, activeFolderId);
    else void uploadFiles(files, activeFolderId);
  }

  function updateSelection(items: ContentItem[]) {
    const unique = new Map(items.map(item => [fileItemKey(item), item]));
    setSelectedItems([...unique.values()]);
  }

  function changeSort(field: ContentSortField, order: SortOrder) {
    if (busy) return;
    const next = { field, order };
    sortRef.current = next;
    setSort(next);
    setPage(1);
    setSelectedItems([]);
    setInlineEdit(null);
    if (section === 'search') setSearchPage(null);
    if (section === 'files' && activeFolderId != null) void loadFolder(activeFolderId, 1, field, order);
  }

  function changeSearchKind(kind: SearchKind) {
    setSearchKind(kind);
    setPage(1);
    setSearchPage(null);
    setSelectedItems([]);
    setInlineEdit(null);
  }

  function startCreate(kind: 'folder' | 'note') {
    setManagingFiles(true);
    if (activeFolderId == null) return;
    const baseName = kind === 'folder' ? '新建文件夹' : '未命名笔记.md';
    if (section !== 'files') {
      setSection('files');
      setPage(1);
      void loadFolder(activeFolderId, 1, sort.field, sort.order);
    }
    setActiveFileId(null);
    setSelectedItems([]);
    setInlineEdit({ mode: 'create', kind, initialValue: uniqueDraftName(listing?.items || [], baseName) });
  }

  function startCreateInside(folderId: number, kind: 'folder' | 'note') {
    setManagingFiles(true);
    const baseName = kind === 'folder' ? '新建文件夹' : '未命名笔记.md';
    setSection('files');
    setPage(1);
    setActiveFolderId(folderId);
    setActiveFileId(null);
    setSelectedItems([]);
    setExpanded(previous => new Set([...previous, folderId]));
    setInlineEdit({ mode: 'create', kind, initialValue: baseName });
    void loadFolder(folderId, 1, sort.field, sort.order);
  }

  function beginRename(item: ContentItem) {
    setManagingFiles(true);
    setSelectedItems([item]);
    if (activeFileId === item.id) setActiveFileId(null);
    if (section === 'files' && item.parent_id != null && item.parent_id !== activeFolderId) {
      setActiveFolderId(item.parent_id);
      setPage(1);
      void loadFolder(item.parent_id, 1, sort.field, sort.order);
    }
    setInlineEdit({ mode: 'rename', item });
  }

  async function commitInlineEdit(rawName: string): Promise<boolean> {
    if (!inlineEdit || activeFolderId == null) return false;
    let name = rawName.trim();
    if (!validItemName(name)) {
      setError('请输入有效名称，不能包含斜杠。');
      return false;
    }
    if (inlineEdit.mode === 'create' && inlineEdit.kind === 'note' && !/\.md$/i.test(name)) name += '.md';
    if (inlineEdit.mode === 'rename' && inlineEdit.item.type === 'file') {
      const draft = markdownDrafts[inlineEdit.item.id];
      if (draft?.state === 'saving') {
        setError('文件仍在保存中，请稍候再重命名。');
        return false;
      }
      const renamedDescriptor = describeFile(name);
      if (draftHasUnsavedChanges(draft) && !(renamedDescriptor.preview === 'markdown' || renamedDescriptor.kind === 'text')) {
        setError('该文件仍有未保存的文本草稿，请先保存或放弃草稿后再更改为其他文件类型。');
        return false;
      }
    }
    if (inlineEdit.mode === 'rename' && name === inlineEdit.item.name) {
      setInlineEdit(null);
      return true;
    }

    setBusy(true);
    setError('');
    try {
      if (inlineEdit.mode === 'create') {
        if (inlineEdit.kind === 'folder') await api.createFolder(activeFolderId, name);
        else await api.upload(activeFolderId, new File([`# ${name.replace(/\.md$/i, '')}\n\n`], name, { type: 'text/markdown' }));
        toast('已创建');
      } else {
        const target = inlineEdit.item;
        if (target.type === 'folder') {
          await api.renameFolder(target.id, name);
          setFolders(previous => previous[target.id] ? { ...previous, [target.id]: { ...previous[target.id], name } } : previous);
        } else {
          await api.renameFile(target.id, name);
          setTabs(previous => previous.map(file => file.id === target.id ? { ...file, name } : file));
        }
        toast('已重命名');
      }
      setInlineEdit(null);
      setSelectedItems([]);
      await refresh();
      return true;
    } catch (cause) {
      handleError(cause);
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function renameFolderInline(folderId: number, rawName: string): Promise<boolean> {
    const name = rawName.trim();
    if (!validItemName(name)) {
      setError('请输入有效名称，不能包含斜杠。');
      return false;
    }
    const folder = folders[folderId];
    if (!folder || folder.name === name) return true;
    setBusy(true);
    setError('');
    try {
      await api.renameFolder(folderId, name);
      setFolders(previous => ({ ...previous, [folderId]: { ...previous[folderId], name } }));
      await refresh();
      toast('已重命名');
      return true;
    } catch (cause) {
      handleError(cause);
      return false;
    } finally {
      setBusy(false);
    }
  }

  function requestMove(items: readonly FileOperationItem[]) {
    const compacted = compactOperationItems(items, folders);
    setInlineEdit(null);
    if (compacted.length) setMoveDialogItems(compacted);
  }

  async function moveItems(items: readonly FileOperationItem[], destinationId: number): Promise<boolean> {
    if (busy) return false;
    const compacted = compactOperationItems(items, folders);
    if (!canMoveItemsTo(compacted, destinationId, folders)) {
      setError('无法移动到该目录：目标可能是原目录、项目自身或它的子目录。');
      return false;
    }

    setBusy(true);
    setError('');
    const moved: FileOperationItem[] = [];
    const failures: Array<{ item: FileOperationItem; message: string }> = [];
    for (const item of compacted) {
      if (item.parent_id === destinationId) continue;
      try {
        if (item.type === 'folder') await api.moveFolder(item.id, destinationId);
        else await api.moveFile(item.id, destinationId);
        moved.push(item);
      } catch (cause) {
        failures.push({ item, message: userErrorMessage(cause) });
        if (cause instanceof ApiError && cause.status === 401) {
          handleError(cause);
          break;
        }
      }
    }

    if (moved.length) {
      const movedFolderIds = new Set(moved.filter(item => item.type === 'folder').map(item => item.id));
      setFolders(previous => {
        const next = { ...previous };
        for (const folderId of movedFolderIds) {
          if (next[folderId]) next[folderId] = { ...next[folderId], parent_id: destinationId };
        }
        return next;
      });
      const movedFileIds = new Set(moved.filter(item => item.type === 'file').map(item => item.id));
      setTabs(previous => previous.map(file => movedFileIds.has(file.id) ? { ...file, parent_id: destinationId } : file));
    }

    try {
      const affectedFolders = new Set<number>([destinationId]);
      for (const item of compacted) if (item.parent_id != null) affectedFolders.add(item.parent_id);
      await Promise.all([...affectedFolders].map(folderId => loadChildren(folderId).catch(() => undefined)));
      await refresh();
    } catch (cause) { handleError(cause); }
    setSelectedItems([]);
    setBusy(false);
    if (moved.length) toast(`已移动 ${moved.length} 项`);
    if (failures.length) {
      setError(`移动失败：${failures.slice(0, 4).map(failure => `${failure.item.name}（${failure.message}）`).join('；')}${failures.length > 4 ? '；还有更多项目失败' : ''}`);
    }
    return moved.length > 0;
  }

  async function removeItems(items: readonly FileOperationItem[]): Promise<void> {
    if (busy) return;
    const compacted = compactOperationItems(items, folders);
    if (compacted.some(item => item.type === 'file' && markdownDrafts[item.id]?.state === 'saving')) {
      setError('选中的文件仍在保存中，请稍候再删除。');
      return;
    }
    if (!compacted.length) return;
    const label = compacted.length === 1 ? `「${compacted[0].name}」` : `${compacted.length} 个项目`;
    const includesUnsavedDraft = compacted.some(item => item.type === 'file' && draftHasUnsavedChanges(markdownDrafts[item.id]));
    const warning = includesUnsavedDraft ? '其中包含未保存的文本草稿，删除后草稿也会丢失。\n\n' : '';
    if (!window.confirm(`${warning}将${label}移到回收站？`)) return;

    setBusy(true);
    setError('');
    const removed: FileOperationItem[] = [];
    const failures: Array<{ item: FileOperationItem; message: string }> = [];
    for (const item of compacted) {
      try {
        if (item.type === 'folder') await api.deleteFolder(item.id);
        else await api.deleteFile(item.id);
        removed.push(item);
      } catch (cause) {
        failures.push({ item, message: userErrorMessage(cause) });
        if (cause instanceof ApiError && cause.status === 401) {
          handleError(cause);
          break;
        }
      }
    }

    const removedFileIds = new Set(removed.filter(item => item.type === 'file').map(item => item.id));
    if (removedFileIds.size) {
      discardDrafts(removedFileIds);
      setTabs(previous => previous.filter(file => !removedFileIds.has(file.id)));
      if (activeFileId != null && removedFileIds.has(activeFileId)) setActiveFileId(null);
    }
    const activeFolderRemoved = activeFolderId != null && removed.some(item => item.type === 'folder' && folderIsWithin(activeFolderId, item.id, folders));
    try {
      const affectedParents = new Set<number>();
      for (const item of compacted) if (item.parent_id != null) affectedParents.add(item.parent_id);
      await Promise.all([...affectedParents].map(folderId => loadChildren(folderId).catch(() => undefined)));
      if (activeFolderRemoved && rootId != null) {
        setActiveFolderId(rootId);
        setSection('files');
        setPage(1);
        await loadFolder(rootId, 1, sort.field, sort.order);
      } else {
        await refresh();
      }
    } catch (cause) { handleError(cause); }
    setSelectedItems([]);
    setBusy(false);
    if (removed.length) toast(`已将 ${removed.length} 项移到回收站`);
    if (failures.length) {
      setError(`删除失败：${failures.slice(0, 4).map(failure => `${failure.item.name}（${failure.message}）`).join('；')}${failures.length > 4 ? '；还有更多项目失败' : ''}`);
    }
  }

  useEffect(() => {
    if (section !== 'files' || activeFolderId == null) return;
    const ancestors: number[] = [];
    const visited = new Set<number>();
    let id: number | null = activeFolderId;
    while (id != null && folders[id] && !visited.has(id)) {
      visited.add(id);
      ancestors.push(id);
      id = folders[id].parent_id;
    }
    setExpanded(previous => ancestors.every(folderId => previous.has(folderId)) ? previous : new Set([...previous, ...ancestors]));
  }, [activeFolderId, folders, section]);

  const path = useMemo(() => {
    const result: Folder[] = [];
    let id = activeFolderId;
    const visited = new Set<number>();
    while (id != null && folders[id] && !visited.has(id)) {
      visited.add(id);
      result.unshift(folders[id]);
      id = folders[id].parent_id;
    }
    return result;
  }, [activeFolderId, folders]);

  function retryList() {
    setListError(null);
    if (section === 'files') {
      if (activeFolderId != null) void loadFolder(activeFolderId, page, sort.field, sort.order);
      else setBootRetry(sequence => sequence + 1);
    } else setListRetry(sequence => sequence + 1);
  }

  const currentPage = section === 'files' ? listing : section === 'search' ? searchPage : trashPage;
  const visibleItems = section === 'files' ? listing?.items || [] : section === 'search' ? searchPage?.items || [] : trashPage?.items || [];
  const mediaItems = (listing?.items || []).filter(item => item.type === 'file' && ['image', 'video'].includes(describeFile(item.name).preview));
  const mediaIndex = currentFile ? mediaItems.findIndex(item => item.id === currentFile.id) : -1;
  const heading = section === 'files' ? activeFolderId === rootId ? '我的空间' : activeFolder?.name || '我的空间' : section === 'search' ? '搜索文件' : '回收站';
  const layout = calculatePanelLayout(viewportWidth, explorerWidth, inspectorWidth, rightOpen);

  function changePanelWidth(side: PanelSide, width: number) {
    const bounded = Math.round(clampPanel(width, side === 'left' ? 180 : 220, side === 'left' ? 400 : 600));
    if (side === 'left') setExplorerWidth(bounded);
    else setInspectorWidth(bounded);
    savePanelPreference(side, bounded);
  }

  function startPanelResize(event: ReactPointerEvent<HTMLDivElement>, side: PanelSide) {
    if (event.button !== 0) return;
    event.preventDefault();
    resizeCleanupRef.current?.();
    const startX = event.clientX;
    const startWidth = side === 'left' ? layout.left : layout.right;
    const minWidth = side === 'left' ? 180 : 220;
    const maxWidth = side === 'left' ? layout.leftMax : layout.rightMax;
    const update = side === 'left' ? setExplorerWidth : setInspectorWidth;
    let latest = startWidth;
    let active = true;
    const shield = document.createElement('div');
    shield.className = 'layout-resize-shield';
    document.body.appendChild(shield);
    document.body.classList.add('layout-resizing');

    const onMove = (move: PointerEvent) => {
      latest = Math.round(clampPanel(startWidth + (side === 'left' ? move.clientX - startX : startX - move.clientX), minWidth, maxWidth));
      update(latest);
    };
    const finish = (save: boolean) => {
      if (!active) return;
      active = false;
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
      window.removeEventListener('pointercancel', onCancel);
      window.removeEventListener('blur', onUp);
      shield.remove();
      document.body.classList.remove('layout-resizing');
      if (save) savePanelPreference(side, latest);
      else update(startWidth);
      resizeCleanupRef.current = null;
    };
    const onUp = () => finish(true);
    const onCancel = () => finish(false);
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    window.addEventListener('pointercancel', onCancel);
    window.addEventListener('blur', onUp);
    resizeCleanupRef.current = onUp;
  }

  function handleResizeKey(event: ReactKeyboardEvent<HTMLDivElement>, side: PanelSide) {
    const current = side === 'left' ? layout.left : layout.right;
    const min = side === 'left' ? 180 : 220;
    const max = side === 'left' ? layout.leftMax : layout.rightMax;
    const next = event.key === 'ArrowRight' ? current + (side === 'left' ? 16 : -16)
      : event.key === 'ArrowLeft' ? current + (side === 'left' ? -16 : 16)
      : event.key === 'Home' ? min : event.key === 'End' ? max : null;
    if (next === null) return;
    event.preventDefault();
    changePanelWidth(side, clampPanel(next, min, max));
  }

  function resetPanelWidths() {
    resetPanelPreferences();
    setExplorerWidth(PANEL_DEFAULTS.left);
    setInspectorWidth(PANEL_DEFAULTS.right);
  }

  if (!token) return <AuthPage mode={authMode} username={username} password={password} busy={authBusy} error={error}
    onUsernameChange={value => { setUsername(value); setError(''); }} onPasswordChange={value => { setPassword(value); setError(''); }} onSubmit={event => void authenticate(event)}
    onSwitchMode={() => { setAuthMode(authMode === 'login' ? 'register' : 'login'); setError(''); }} />;

  return <div className={`app-shell ${immersive ? 'immersive' : ''}`}>
    <header className="topbar">
      <div className="brand"><div className="brand-mark"><Icon name="folder" size={19} /></div><span>栖盘</span></div>
      <div className="topbar-right">
        {quota && <div className="quota-meter" title={`已使用 ${formatSize(quota.used)}，共 ${formatSize(quota.limit)}`}><span style={{ width: `${quota.limit > 0 ? Math.min(100, (quota.used / quota.limit) * 100) : 0}%` }} /></div>}
        <span className="user-avatar" title={profile?.username || '当前用户'}>{profile?.username?.trim().charAt(0).toUpperCase() || '栖'}</span>
        <Button icon={immersive ? 'columns' : 'fit'} label={immersive ? '退出沉浸阅读' : '沉浸阅读'} onClick={() => setImmersive(value => !value)} />
        <Button icon="panel" label={rightOpen && !immersive ? '收起详情' : '显示详情'} onClick={() => { setImmersive(false); setRightOpen(immersive || !rightOpen); }} />
        {isOfflinePreview ? <span className="offline-preview-badge" title="仅用于本地开发；刷新页面会重置演示数据">离线预览</span> : <Button icon="logout" label="退出登录" onClick={logout} />}
      </div>
    </header>
    <div className="app-body">
      <nav className="activity-rail" aria-label="主导航"><div className="rail-main">{([['files', 'folder', '文件'], ['search', 'search', '搜索'], ['trash', 'trash', '回收站']] as const).map(([value, icon, label]) => <button key={value} ref={value === 'files' ? mobileFilesButtonRef : undefined} type="button" className={`rail-button ${section === value && !settingsOpen ? 'active' : ''}`} title={label} aria-label={label} aria-controls={value === 'files' && !layout.showLeft ? 'folder-explorer' : undefined} aria-expanded={value === 'files' && !layout.showLeft ? mobileExplorerOpen : undefined} onClick={() => {
        if (value === 'files' && !layout.showLeft) {
          if (section !== 'files') selectSection('files');
          setMobileExplorerOpen(open => section === 'files' ? !open : true);
        } else selectSection(value);
      }}><Icon name={icon} size={20} /></button>)}</div><button type="button" ref={settingsTriggerRef} className={`rail-button rail-settings ${settingsOpen ? 'active' : ''}`} title="设置" aria-label="设置" aria-pressed={settingsOpen} onClick={() => { setMobileExplorerOpen(false); setSettingsOpen(true); }}><Icon name="settings" size={20} /></button></nav>
      {mobileExplorerOpen && !layout.showLeft && <button type="button" className="mobile-explorer-backdrop" aria-label="关闭目录" onClick={() => setMobileExplorerOpen(false)} />}
      <Explorer width={layout.showLeft ? layout.left : undefined} mobileOpen={mobileExplorerOpen && !layout.showLeft} onCloseMobile={() => { setMobileExplorerOpen(false); mobileFilesButtonRef.current?.focus(); }} rootId={rootId} activeFolderId={activeFolderId} activeFileId={activeFileId} folders={folders}
        files={treeFiles} onOpenFile={openFile} onDownload={item => void download(item)} onRenameFile={beginRename}
        loading={loadingSection === 'files'} loadError={listError?.section === 'files' ? listError.message : null} onRetry={retryList}
        onManage={() => { setManagingFiles(true); setSection('files'); setActiveFileId(null); setMobileExplorerOpen(false); }}
        children={children} expanded={expanded} busy={busy} uploading={uploadProgress?.status === 'uploading'} onSelectFolder={selectFolder}
        onToggleFolder={id => void toggleFolder(id)} onStartCreateInside={startCreateInside}
        onRenameFolder={renameFolderInline} onChooseUploadTarget={chooseUploadTarget}
        onUploadFilesToFolder={(files, folderId) => void uploadFiles(files, folderId)}
        onRequestMove={requestMove} onMoveItems={(items, destinationId) => void moveItems(items, destinationId)}
        onRemoveItems={items => void removeItems(items)} onCreateFolder={() => startCreate('folder')}
        onUpload={() => { if (activeFolderId != null) chooseUploadTarget(activeFolderId); }} />
      {layout.showLeft && <div className="panel-resize-handle" role="separator" tabIndex={0} aria-label="调整目录栏宽度" aria-orientation="vertical" aria-valuemin={180} aria-valuemax={layout.leftMax} aria-valuenow={layout.left} onPointerDown={event => startPanelResize(event, 'left')} onKeyDown={event => handleResizeKey(event, 'left')} onDoubleClick={() => changePanelWidth('left', PANEL_DEFAULTS.left)} />}
      <main className={`workspace ${fileDragActive ? 'file-drag-active' : ''}`} onDragEnter={handleWorkspaceDragEnter} onDragOver={handleWorkspaceDragOver} onDragLeave={handleWorkspaceDragLeave} onDrop={handleWorkspaceDrop}>
        {managingFiles && section === 'files' && <button className="preview-return" type="button" onClick={() => { setManagingFiles(false); setInlineEdit(null); }}><Icon name="arrowLeft" size={14} />返回阅读</button>}
        {fileDragActive && <div className="file-drop-overlay" aria-hidden="true"><span><Icon name="upload" size={28} /></span><strong>松开以上传</strong></div>}
        {section === 'files' && !managingFiles && <DocumentTabs tabs={tabs} activeFileId={activeFileId} drafts={markdownDrafts} onOpen={openFile} onClose={closeTab} />}
        {path.length > 0 && <div className="breadcrumb-bar"><div className="breadcrumbs">{path.map((folder, index) => <span className="crumb" key={folder.id}>{index > 0 && <Icon name="chevron" size={14} />}<button onClick={() => selectFolder(folder.id)}>{folder.id === rootId ? '我的空间' : folder.name}</button></span>)}</div></div>}
        {error && <div className="inline-error" role="alert" {...errorDismissEvents}>{error}<button onClick={() => setError('')}>关闭</button></div>}
        {section === 'files' && !managingFiles && !currentFile ? <div className="preview-empty"><Empty icon="file" title="暂无预览内容" text="从左侧选择一个文件，开始阅读" /><span className="preview-empty-formats">Markdown · 代码 · PDF · 图片</span></div>
          : currentFile && section === 'files' && !managingFiles ? <DocumentPreview file={currentFile} preview={preview} draft={currentDraft}
            mediaNavigation={{
              hasPrevious: mediaIndex > 0,
              hasNext: mediaIndex >= 0 && mediaIndex < mediaItems.length - 1,
              onPrevious: () => { if (mediaIndex > 0) openFile(mediaItems[mediaIndex - 1]); },
              onNext: () => { if (mediaIndex >= 0 && mediaIndex < mediaItems.length - 1) openFile(mediaItems[mediaIndex + 1]); },
            }}
            onDraftChange={content => updateDraft(currentFile.id, content)} onSave={createVersion => void saveCurrentMarkdown(createVersion)}
            onDiscardDraftAndReload={discardCurrentDraftAndReload} onDownload={item => void download(item)} onReload={reloadPreview}
            onRestoreVersion={restoreCurrentVersion} />
          : <FileList section={section} heading={heading} loading={loadingSection === section} loadError={listError?.section === section ? listError.message : null} busy={busy} uploading={uploadProgress?.status === 'uploading'} query={query} searchRef={searchRef} onRetry={retryList}
              page={page} currentPage={currentPage} items={visibleItems}
              sortField={sort.field} sortOrder={sort.order} searchKind={searchKind}
              selectedKeys={selectedKeys} inlineEdit={inlineEdit}
              onQueryChange={value => { setQuery(value); setSearchPage(null); setPage(1); setSelectedItems([]); setInlineEdit(null); }}
              onRefresh={() => { if (activeFolderId != null) void loadFolder(activeFolderId, page, sort.field, sort.order); }}
              onSortChange={changeSort} onSearchKindChange={changeSearchKind}
              onSelectionChange={updateSelection} onStartCreate={startCreate} onStartCreateInside={startCreateInside}
              onBeginRename={beginRename} onCommitInlineEdit={commitInlineEdit} onCancelInlineEdit={() => setInlineEdit(null)}
              onUpload={() => { if (activeFolderId != null) chooseUploadTarget(activeFolderId); }}
              onChooseUploadTarget={chooseUploadTarget} onUploadFilesToFolder={(files, folderId) => void uploadFiles(files, folderId)}
              onOpen={openFile} searchPaths={searchPaths} onLocateFolder={selectFolder} onDownload={item => void download(item)} onRequestMove={requestMove}
              onMoveItems={(items, destinationId) => void moveItems(items, destinationId)} onRemoveItems={items => void removeItems(items)}
              onRestore={item => void runAction(() => api.restore(item.id), '已恢复')}
              onPermanentlyDelete={item => { if (window.confirm(`永久删除「${item.name}」？此操作无法撤销。`)) void runAction(() => api.permanentlyDelete(item.id), '已永久删除'); }}
              onPageChange={next => { setPage(next); setSelectedItems([]); setInlineEdit(null); if (section === 'files' && activeFolderId != null) void loadFolder(activeFolderId, next, sort.field, sort.order); }} />}
      </main>
      {rightOpen && <><button type="button" className="inspector-backdrop" aria-label="关闭详情面板" onClick={() => setRightOpen(false)} />{layout.showRight && <div className="panel-resize-handle" role="separator" tabIndex={0} aria-label="调整详情栏宽度" aria-orientation="vertical" aria-valuemin={220} aria-valuemax={layout.rightMax} aria-valuenow={layout.right} onPointerDown={event => startPanelResize(event, 'right')} onKeyDown={event => handleResizeKey(event, 'right')} onDoubleClick={() => changePanelWidth('right', PANEL_DEFAULTS.right)} />}<Inspector width={layout.showRight ? layout.right : undefined} tab={rightTab} outline={outline} currentFile={currentFile} activeFolder={activeFolder}
        onTabChange={setRightTab} onClose={() => setRightOpen(false)} onDownload={item => void download(item)} onRename={beginRename}
        onRemove={item => void removeItems([item])} /></>}
    </div>
    <input ref={uploadRef} className="sr-only" type="file" multiple onChange={event => void importFiles(event)} />
    {uploadProgress && <UploadStatus progress={uploadProgress} onCancel={() => uploadAbortRef.current?.abort()} onClose={() => setUploadProgress(current => current?.status === 'uploading' ? current : null)} onLocate={folderId => { selectFolder(folderId); setUploadProgress(null); }} onRetry={(files, folderId) => void uploadFiles(files, folderId, uploadProgress.failures.filter(failure => !files.includes(failure.file)))} />}
    {notice && <div className="toast" role="status"><Icon name="check" size={16} />{notice}</div>}
    {moveDialogItems && rootId != null && <MoveDialog items={moveDialogItems} rootId={rootId} folders={folders} children={children}
      onLoadChildren={async folderId => { try { await loadChildren(folderId); } catch (cause) { handleError(cause); } }} onConfirm={destinationId => moveItems(moveDialogItems, destinationId)}
      onClose={() => setMoveDialogItems(null)} />}
    {settingsOpen && <LayoutSettingsDialog left={explorerWidth} right={inspectorWidth} leftVisible={layout.showLeft} rightVisible={viewportWidth > 980} returnFocus={settingsTriggerRef.current}
      onLeftChange={width => changePanelWidth('left', width)} onRightChange={width => changePanelWidth('right', width)} onReset={resetPanelWidths} onClose={() => setSettingsOpen(false)} />}
  </div>;
}
