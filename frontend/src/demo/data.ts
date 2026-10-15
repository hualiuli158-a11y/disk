import { ApiError } from '../services/api';
import type { BackendApi, ContentItem, ContentQuery, FileVersion, Page, SearchQuery, TrashItem, Folder as ServerFolder } from '../services/api';

export const STORAGE_KEY = 'disk-front-demo-v1';

export type FileType = 'markdown' | 'text' | 'image' | 'pdf' | 'binary';
export interface Folder { id: string; name: string; parentId: string | null; deleted: boolean }
export interface FileItem {
  id: string;
  folderId: string;
  name: string;
  type: FileType;
  mime: string;
  content?: string;
  dataUrl?: string;
  size?: number;
  starred: boolean;
  deleted: boolean;
  updatedAt: string;
}
export interface WorkspaceData { folders: Folder[]; files: FileItem[] }

const welcome = `# 欢迎来到栖盘

> 一个让文件有序安放、也能随时动笔的工作空间。

栖盘把 **文件管理** 和 **内容编辑** 放在同一个界面。你可以在左侧浏览目录，在这里写下想法，并随时切换到预览模式。

## 从这里开始

- 点击左侧文件树，打开一篇笔记或查看一个文件夹。
- 从左下角的 **管理文件** 新建笔记、整理文件，或在目录上使用右键菜单。
- 文件默认以阅读模式打开，点击 **编辑** 开始写作；右上角可进入 **沉浸阅读**。
- 搜索和回收站在最左侧的工具栏里。

## 编辑与预览

### Markdown 写作

支持标题、列表、引用、代码块、链接和图片等常见语法。编辑内容会自动保存在当前浏览器。

\`\`\`js
const workspace = {
  files: '清晰整理',
  notes: '随手记录',
  preview: '即写即看',
};
\`\`\`

> 当前是开发用离线预览，刷新页面会重置演示数据；连接后端时会使用真实账号和存储。

## 接下来

打开「项目计划.md」，看看文件工作区可以如何组织资料。你也可以自己新建一篇笔记。
`;

const plan = `# 项目计划

把存储服务变成一个真正好用的文件工作区。

## 第一阶段 · 前端体验

- [x] 文件树和工作区布局
- [x] Markdown 编辑与实时预览
- [x] 搜索、收藏和回收站的交互原型
- [x] 连接 Go 业务接口

## 第二阶段 · 在线保存

为现有文件增加内容更新接口。保存时检查版本，避免多个窗口相互覆盖。

## 第三阶段 · 文件体验

继续完善图片、PDF 和更多格式的预览体验。
`;

const readme = `# 设计说明

界面参考 Obsidian 的文件树与标签页，也参考 Hana 的工作区结构。

## 布局

- 左侧：文件导航与分类入口。
- 中间：文件编辑、预览与目录内容。
- 右侧：文档大纲与文件信息。

## 设计原则

1. 文件始终是主角。
2. 常用操作就在手边。
3. 辅助面板可以随时收起。
`;

export const initialData: WorkspaceData = {
  folders: [
    { id: 'root', name: '我的空间', parentId: null, deleted: false },
    { id: 'notes', name: '我的笔记', parentId: 'root', deleted: false },
    { id: 'projects', name: '项目资料', parentId: 'root', deleted: false },
    { id: 'design', name: '设计素材', parentId: 'root', deleted: false },
    { id: 'archive', name: '归档', parentId: 'root', deleted: false },
    { id: 'year', name: '2026', parentId: 'projects', deleted: false },
    { id: 'drafts', name: '方案草稿', parentId: 'year', deleted: false },
  ],
  files: [
    { id: 'welcome', folderId: 'notes', name: '欢迎使用栖盘.md', type: 'markdown', mime: 'text/markdown', content: welcome, starred: true, deleted: false, updatedAt: '2026-10-07T10:30:00+08:00' },
    { id: 'plan', folderId: 'projects', name: '项目计划.md', type: 'markdown', mime: 'text/markdown', content: plan, starred: false, deleted: false, updatedAt: '2026-10-06T18:20:00+08:00' },
    { id: 'readme', folderId: 'design', name: '设计说明.md', type: 'markdown', mime: 'text/markdown', content: readme, starred: false, deleted: false, updatedAt: '2026-10-05T16:00:00+08:00' },
    { id: 'memo', folderId: 'notes', name: '随手记.txt', type: 'text', mime: 'text/plain', content: '这是一个简单的文本文件。\n\n像 Markdown 一样，它也可以在这里直接编辑。', starred: false, deleted: false, updatedAt: '2026-10-04T11:15:00+08:00' },
    { id: 'nested-plan', folderId: 'drafts', name: '项目计划.md', type: 'markdown', mime: 'text/markdown', content: '# 方案草稿\n\n这里是一份位于深层文件夹的同名文档，用来预览搜索路径与定位操作。\n', starred: false, deleted: false, updatedAt: '2026-10-08T09:00:00+08:00' },
  ],
};

export function loadData(): WorkspaceData {
  try {
    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || 'null');
    if (stored && Array.isArray(stored.folders) && Array.isArray(stored.files)) return stored;
  } catch { /* Fall back to demo data. */ }
  return structuredClone(initialData);
}

export function formatDate(value?: string): string {
  if (!value) return '刚刚';
  return new Intl.DateTimeFormat('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value));
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function fileSize(file: FileItem): number {
  return file.type === 'image' || file.type === 'pdf' ? file.size || 0 : new Blob([file.content || '']).size;
}

// Adapt the visual fixtures above to the same numeric-ID contract as the Go API.
type DemoVersion = { id: number; blob: Blob; sha256: string; createdAt: string; source: FileVersion['source']; restoredFrom?: number };
type DemoFile = { item: ContentItem; blob: Blob; versions: DemoVersion[] };

export function createDemoApi(): BackendApi {
  const folders = new Map<number, ServerFolder>();
  const files = new Map<number, DemoFile>();
  const trashedFolders = new Set<number>();
  const trashedFiles = new Set<number>();
  const trash = new Map<number, TrashItem>();
  const ids = new Map(initialData.folders.map((folder, index) => [folder.id, index + 1]));
  let nextId = 100;
  let nextVersion = 1;
  let nextTrash = 1;
  let revision = 1;
  const rootId = ids.get('root')!;
  const now = () => new Date().toISOString();
  const sha = () => (revision++).toString(16).padStart(64, '0');
  const copyFolder = (folder: ServerFolder) => ({ ...folder });
  const copyItem = (item: ContentItem) => ({ ...item });
  const fail = (message: string, status = 404): never => { throw new ApiError(message, status); };
  const alive = (id: number): boolean => {
    const seen = new Set<number>();
    let current: number | null = id;
    while (current != null && !seen.has(current)) {
      seen.add(current);
      if (trashedFolders.has(current)) return false;
      const folder: ServerFolder | undefined = folders.get(current);
      if (!folder) return false;
      if (current === rootId) return true;
      current = folder.parent_id;
    }
    return false;
  };
  const folderAt = (id: number): ServerFolder => {
    const folder = folders.get(id);
    if (!folder || !alive(id)) return fail('文件夹不存在');
    return folder;
  };
  const fileAt = (id: number): DemoFile => {
    const file = files.get(id);
    if (!file || trashedFiles.has(id) || file.item.parent_id == null || !alive(file.item.parent_id)) return fail('文件不存在');
    return file;
  };
  const folderChildren = (id: number) => [...folders.values()].filter(folder => folder.parent_id === id && alive(folder.id)).map(copyFolder);
  const writeVersion = (file: DemoFile, blob: Blob, source: FileVersion['source'], restoredFrom?: number) => {
    const version: DemoVersion = { id: nextVersion++, blob, sha256: sha(), createdAt: now(), source, restoredFrom };
    file.versions.push(version);
    file.blob = blob;
    file.item.size = blob.size;
    file.item.sha256 = version.sha256;
    file.item.updated_at = version.createdAt;
    return version;
  };
  const addFile = (parentId: number, name: string, blob: Blob, createdAt = now()): DemoFile => {
    folderAt(parentId);
    const id = nextId++;
    const file: DemoFile = {
      item: { id, type: 'file', parent_id: parentId, name, size: 0, sha256: null, created_at: createdAt, updated_at: createdAt },
      blob, versions: [],
    };
    files.set(id, file);
    writeVersion(file, blob, 'upload');
    file.item.updated_at = createdAt;
    file.versions[0].createdAt = createdAt;
    return file;
  };
  for (const entry of initialData.folders) {
    folders.set(ids.get(entry.id)!, { id: ids.get(entry.id)!, parent_id: entry.parentId == null ? null : ids.get(entry.parentId)!, name: entry.name, created_at: '2026-10-01T08:00:00+08:00' });
  }
  for (const entry of initialData.files) {
    addFile(ids.get(entry.folderId)!, entry.name, new Blob([entry.content || ''], { type: entry.mime }), entry.updatedAt);
  }
  const pageOf = <T>(all: T[], page: number): Page<T> => ({
    items: all.slice((page - 1) * 100, page * 100), page, page_size: 100,
    total: all.length, total_pages: Math.ceil(all.length / 100),
  });
  const sorted = (items: ContentItem[], options: ContentQuery) => items.sort((a, b) => {
    if (a.type !== b.type) return a.type === 'folder' ? -1 : 1;
    const field = options.sort || 'name';
    const left = field === 'size' ? (a.size || 0) : field === 'name' ? a.name : a[field];
    const right = field === 'size' ? (b.size || 0) : field === 'name' ? b.name : b[field];
    const comparison = typeof left === 'number' && typeof right === 'number' ? left - right : String(left).localeCompare(String(right), 'zh-CN');
    return (options.order === 'desc' ? -comparison : comparison) || a.name.localeCompare(b.name, 'zh-CN') || a.id - b.id;
  });
  const visibleItems = (): ContentItem[] => [
    ...[...folders.values()].filter(folder => folder.id !== rootId && alive(folder.id)).map(folder => ({
      ...copyFolder(folder), type: 'folder' as const, updated_at: folder.created_at,
    })),
    ...[...files.values()].filter(file => !trashedFiles.has(file.item.id) && file.item.parent_id != null && alive(file.item.parent_id)).map(file => copyItem(file.item)),
  ];
  const markTrash = (type: TrashItem['type'], id: number, name: string, parentId: number | null) => {
    const trashedAt = now();
    trash.set(nextTrash, { id: nextTrash, type, resource_id: id, name, original_parent_id: parentId,
      trashed_at: trashedAt, expires_at: new Date(Date.now() + 30 * 86400_000).toISOString() });
    nextTrash += 1;
  };
  const checkSignal = (signal?: AbortSignal) => {
    if (signal?.aborted) throw new DOMException('Upload cancelled', 'AbortError');
  };
  const wait = (signal: AbortSignal, delay: number) => new Promise<void>((resolve, reject) => {
    if (signal.aborted) { reject(new DOMException('Upload cancelled', 'AbortError')); return; }
    const abort = () => { window.clearTimeout(timer); reject(new DOMException('Upload cancelled', 'AbortError')); };
    const timer = window.setTimeout(() => { signal.removeEventListener('abort', abort); resolve(); }, delay);
    signal.addEventListener('abort', abort, { once: true });
  });
  const saved = (file: DemoFile) => ({ id: file.item.id, name: file.item.name, size: file.blob.size,
    sha256: file.item.sha256!, updated_at: file.item.updated_at });
  const assertVersion = (file: DemoFile, expected: string) => {
    if (file.item.sha256 !== expected) fail('文件版本已变化，请重新加载', 412);
  };

  return {
    login: async () => ({ access_token: 'demo-preview' }),
    register: async () => ({ access_token: 'demo-preview' }),
    profile: async () => ({ id: 1, username: '预览用户', used_quota: [...files.values()].reduce((sum, file) => sum + file.blob.size, 0), quota_limit: 1024 ** 3 }),
    quota: async () => {
      const used = [...files.values()].reduce((sum, file) => sum + file.blob.size, 0);
      return { used, limit: 1024 ** 3, remaining: 1024 ** 3 - used };
    },
    root: async () => ({ root: copyFolder(folderAt(rootId)), children: folderChildren(rootId) }),
    children: async id => ({ folder: copyFolder(folderAt(id)), children: folderChildren(id) }),
    contents: async (id, page = 1, options = {}) => {
      folderAt(id);
      return pageOf(sorted(visibleItems().filter(item => item.parent_id === id), options), page);
    },
    search: async (query, page = 1, options: SearchQuery = {}) => pageOf(sorted(visibleItems().filter(item =>
      (options.kind === 'file' ? item.type === 'file' : options.kind === 'folder' ? item.type === 'folder' : true)
      && item.name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())), options), page),
    trash: async (page = 1) => pageOf([...trash.values()].sort((a, b) => b.trashed_at.localeCompare(a.trashed_at)), page),
    createFolder: async (parentId, name) => {
      folderAt(parentId);
      if (folderChildren(parentId).some(folder => folder.name === name)) fail('同名文件夹已存在', 409);
      const folder: ServerFolder = { id: nextId++, parent_id: parentId, name, created_at: now() };
      folders.set(folder.id, folder);
      return copyFolder(folder);
    },
    renameFolder: async (id, name) => {
      const folder = folderAt(id);
      folder.name = name;
      return { id, name };
    },
    moveFolder: async (id, parentId) => {
      folderAt(parentId);
      const folder = folderAt(id);
      let ancestor: number | null = parentId;
      const visited = new Set<number>();
      while (ancestor != null && !visited.has(ancestor)) {
        if (ancestor === id) fail('不能移动到自身或子目录', 400);
        visited.add(ancestor);
        ancestor = folders.get(ancestor)?.parent_id ?? null;
      }
      if (id === rootId) fail('不能移动根目录', 400);
      folder.parent_id = parentId;
      return { id, parent_id: parentId };
    },
    deleteFolder: async id => {
      const folder = folderAt(id);
      if (id === rootId) fail('不能删除根目录', 400);
      trashedFolders.add(id);
      markTrash('folder', id, folder.name, folder.parent_id);
    },
    renameFile: async (id, name) => {
      fileAt(id).item.name = name;
      return { id, name };
    },
    moveFile: async (id, folderId) => {
      folderAt(folderId);
      fileAt(id).item.parent_id = folderId;
      return { id, folder_id: folderId };
    },
    deleteFile: async id => {
      const item = fileAt(id).item;
      trashedFiles.add(id);
      markTrash('file', id, item.name, item.parent_id);
    },
    upload: async (folderId, file, signal) => {
      checkSignal(signal);
      const created = addFile(folderId, file.name, file);
      return { id: created.item.id, name: file.name, folder_id: folderId };
    },
    uploadWithProgress: async (folderId, file, signal, onProgress) => {
      folderAt(folderId);
      checkSignal(signal);
      onProgress(30);
      await wait(signal, 280);
      onProgress(100);
      const created = addFile(folderId, file.name, file);
      return { id: created.item.id, name: file.name, folder_id: folderId };
    },
    download: async (id, signal) => { checkSignal(signal); return fileAt(id).blob; },
    downloadWithMetadata: async (id, signal) => {
      checkSignal(signal);
      const file = fileAt(id);
      return { blob: file.blob, sha256: file.item.sha256 || null, updatedAt: file.item.updated_at };
    },
    updateFileContent: async (id, content, expectedSHA256, createVersion = false) => {
      const file = fileAt(id);
      const oldContent = await file.blob.text();
      const latestContent = await file.versions.at(-1)?.blob.text();
      assertVersion(file, expectedSHA256);
      if (content !== oldContent) {
        file.blob = new Blob([content], { type: file.blob.type || 'text/plain' });
        file.item.size = file.blob.size;
        file.item.sha256 = sha();
        file.item.updated_at = now();
      }
      if (createVersion && content !== latestContent) {
        file.versions.push({ id: nextVersion++, blob: file.blob, sha256: file.item.sha256!, createdAt: now(), source: 'edit' });
      }
      return saved(file);
    },
    fileVersions: async id => {
      const file = fileAt(id);
      return { items: file.versions.slice().reverse().map(version => ({
        id: version.id, size: version.blob.size, sha256: version.sha256,
        source: version.source, created_at: version.createdAt,
        restored_from_version_id: version.restoredFrom,
        current: version.sha256 === file.item.sha256,
      })) };
    },
    downloadFileVersion: async (id, versionId) => fileAt(id).versions.find(version => version.id === versionId)?.blob || fail('版本不存在'),
    restoreFileVersion: async (id, versionId, expectedSHA256) => {
      const file = fileAt(id);
      assertVersion(file, expectedSHA256);
      const version = file.versions.find(entry => entry.id === versionId);
      if (!version) return fail('版本不存在');
      writeVersion(file, version.blob, 'restore', versionId);
      return saved(file);
    },
    restore: async id => {
      const entry = trash.get(id);
      if (!entry) return fail('回收站项目不存在');
      if (entry.type === 'folder') {
        const folder = folders.get(entry.resource_id);
        if (folder) { folder.parent_id = entry.original_parent_id != null && alive(entry.original_parent_id) ? entry.original_parent_id : rootId; trashedFolders.delete(folder.id); }
      } else {
        const file = files.get(entry.resource_id);
        if (file) { file.item.parent_id = entry.original_parent_id != null && alive(entry.original_parent_id) ? entry.original_parent_id : rootId; trashedFiles.delete(file.item.id); }
      }
      trash.delete(id);
    },
    permanentlyDelete: async id => {
      const entry = trash.get(id);
      if (!entry) return fail('回收站项目不存在');
      if (entry.type === 'folder') folders.delete(entry.resource_id);
      else files.delete(entry.resource_id);
      trash.delete(id);
    },
  };
}
