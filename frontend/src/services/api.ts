/// <reference types="vite/client" />

import { createDemoApi } from '../demo/data';

export interface Profile {
  id: number;
  username: string;
  used_quota: number;
  quota_limit: number;
}

export interface Quota { used: number; limit: number; remaining: number }

export interface Folder {
  id: number;
  parent_id: number | null;
  name: string;
  created_at: string;
}

export interface ContentItem {
  id: number;
  type: 'file' | 'folder';
  parent_id: number | null;
  name: string;
  size?: number | null;
  sha256?: string | null;
  created_at: string;
  updated_at: string;
}

export interface Page<T> {
  items: T[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface TrashItem {
  id: number;
  type: 'file' | 'folder';
  resource_id: number;
  original_parent_id: number | null;
  name: string;
  trashed_at: string;
  expires_at: string;
}

export type ContentSortField = 'name' | 'size' | 'created_at' | 'updated_at';
export type SortOrder = 'asc' | 'desc';
export type SearchKind = 'all' | 'file' | 'folder';

export interface ContentQuery {
  sort?: ContentSortField;
  order?: SortOrder;
}

export interface SearchQuery extends ContentQuery {
  kind?: SearchKind;
}

const API_BASE = (import.meta.env.VITE_API_BASE_URL || '').replace(/\/$/, '');
const TOKEN_KEY = 'disk-front-access-token';
export const isOfflinePreview = import.meta.env.DEV && import.meta.env.MODE !== 'integration';

export function getToken(): string | null { return isOfflinePreview ? 'demo-preview' : sessionStorage.getItem(TOKEN_KEY); }
export function setToken(token: string | null): void {
  if (isOfflinePreview) return;
  if (token) sessionStorage.setItem(TOKEN_KEY, token);
  else sessionStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  constructor(message: string, public status: number, public code?: number) { super(message); }
}

async function requestResponse(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  const token = getToken();
  if (token) headers.set('Authorization', `Bearer ${token}`);
  const response = await fetch(`${API_BASE}${path}`, { ...init, headers });
  if (!response.ok) {
    let message = `请求失败（HTTP ${response.status}）`;
    let code: number | undefined;
    try {
      const body = await response.json() as { code?: number; msg?: string; details?: string[] };
      code = body.code;
      message = body.msg || message;
      if (body.details?.length) message += `：${body.details.join('、')}`;
    } catch { /* Some errors have no JSON body. */ }
    throw new ApiError(message, response.status, code);
  }
  return response;
}

async function request<T>(path: string, init: RequestInit = {}, responseType: 'json' | 'blob' = 'json'): Promise<T> {
  const response = await requestResponse(path, init);
  if (responseType === 'blob') return response.blob() as Promise<T>;
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

function sha256FromResponse(response: Response): string | null {
  const explicit = response.headers.get('X-File-SHA256')?.trim();
  if (explicit) return explicit.toLowerCase();
  const etag = response.headers.get('ETag')?.trim();
  if (!etag || etag.startsWith('W/')) return null;
  return etag.replace(/^"|"$/g, '').toLowerCase() || null;
}

function json(method: string, body: object): RequestInit {
  return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
}

function uploadWithProgress(folderId: number, file: File, signal: AbortSignal, onProgress: (percent: number) => void): Promise<{ id: number; name: string; folder_id: number }> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) { reject(new DOMException('Upload cancelled', 'AbortError')); return; }
    const body = new FormData();
    body.set('folder_id', String(folderId));
    body.set('file', file);
    const xhr = new XMLHttpRequest();
    let settled = false;
    const finish = (callback: () => void) => {
      if (settled) return;
      settled = true;
      signal.removeEventListener('abort', abort);
      callback();
    };
    const abort = () => xhr.abort();
    xhr.open('POST', `${API_BASE}/api/files`);
    const token = getToken();
    if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`);
    xhr.upload.onprogress = event => {
      if (event.lengthComputable && event.total > 0) onProgress(Math.min(100, Math.round(event.loaded / event.total * 100)));
    };
    xhr.onload = () => finish(() => {
      let data: { id: number; name: string; folder_id: number; code?: number; msg?: string; details?: string[] } | null = null;
      try { data = JSON.parse(xhr.responseText); } catch { /* Handle invalid server responses below. */ }
      if (xhr.status >= 200 && xhr.status < 300 && data && typeof data.id === 'number') {
        onProgress(100);
        resolve(data);
        return;
      }
      const message = data?.msg || (xhr.status >= 200 && xhr.status < 300 ? '上传响应无效，请刷新列表确认文件是否已上传。' : `请求失败（HTTP ${xhr.status}）`);
      reject(new ApiError(data?.details?.length ? `${message}：${data.details.join('、')}` : message, xhr.status, data?.code));
    });
    xhr.onerror = () => finish(() => reject(new TypeError('Network request failed')));
    xhr.onabort = () => finish(() => reject(new DOMException('Upload cancelled', 'AbortError')));
    signal.addEventListener('abort', abort, { once: true });
    xhr.send(body);
  });
}

export interface DownloadedFile {
  blob: Blob;
  sha256: string | null;
  updatedAt: string | null;
}

export interface UpdatedFileContent {
  id: number;
  name: string;
  size: number;
  sha256: string;
  updated_at: string;
}

export interface FileVersion {
  id: number;
  size: number;
  sha256: string;
  source: 'upload' | 'edit' | 'restore';
  restored_from_version_id?: number;
  created_at: string;
  current: boolean;
}

const backendApi = {
  login: (username: string, password: string) => request<{ access_token: string }>('/login', json('POST', { username, password })),
  register: (username: string, password: string) => request<{ access_token: string }>('/register', json('POST', { username, password })),
  profile: () => request<Profile>('/api/profile'),
  quota: () => request<Quota>('/api/quota'),
  root: () => request<{ root: Folder; children: Folder[] }>('/api/folders/root/children'),
  children: (id: number) => request<{ folder: Folder; children: Folder[] }>(`/api/folders/${id}/children`),
  contents: (id: number, page = 1, options: ContentQuery = {}) => {
    const params = new URLSearchParams({
      page: String(page),
      page_size: '100',
      sort: options.sort || 'name',
      order: options.order || 'asc',
    });
    return request<Page<ContentItem>>(`/api/folders/${id}/contents?${params.toString()}`);
  },
  search: (query: string, page = 1, options: SearchQuery = {}) => {
    const params = new URLSearchParams({
      q: query,
      page: String(page),
      page_size: '100',
      type: options.kind || 'all',
      sort: options.sort || 'name',
      order: options.order || 'asc',
    });
    return request<Page<ContentItem>>(`/api/search?${params.toString()}`);
  },
  trash: (page = 1) => request<Page<TrashItem>>(`/api/trash?page=${page}&page_size=100`),
  createFolder: (parentId: number, name: string) => request<Folder>('/api/folders', json('POST', { parent_id: parentId, name })),
  renameFolder: (id: number, name: string) => request<{ id: number; name: string }>(`/api/folders/${id}`, json('PATCH', { name })),
  moveFolder: (id: number, parentId: number) => request<{ id: number; parent_id: number }>(`/api/folders/${id}/move`, json('PATCH', { parent_id: parentId })),
  deleteFolder: (id: number) => request<void>(`/api/folders/${id}`, { method: 'DELETE' }),
  renameFile: (id: number, name: string) => request<{ id: number; name: string }>(`/api/files/${id}`, json('PATCH', { name })),
  moveFile: (id: number, folderId: number) => request<{ id: number; folder_id: number }>(`/api/files/${id}/move`, json('PATCH', { folder_id: folderId })),
  deleteFile: (id: number) => request<void>(`/api/files/${id}`, { method: 'DELETE' }),
  upload: (folderId: number, file: File, signal?: AbortSignal) => {
    const body = new FormData();
    body.set('folder_id', String(folderId));
    body.set('file', file);
    return request<{ id: number; name: string; folder_id: number }>('/api/files', { method: 'POST', body, signal });
  },
  uploadWithProgress,
  download: (id: number, signal?: AbortSignal) => request<Blob>(`/api/files/${id}/download`, { signal }, 'blob'),
  downloadWithMetadata: async (id: number, signal?: AbortSignal): Promise<DownloadedFile> => {
    const response = await requestResponse(`/api/files/${id}/download`, { signal });
    return {
      blob: await response.blob(),
      sha256: sha256FromResponse(response),
      updatedAt: response.headers.get('Last-Modified'),
    };
  },
  updateFileContent: (id: number, content: string, expectedSHA256: string, createVersion = false) => request<UpdatedFileContent>(`/api/files/${id}/content?create_version=${createVersion}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'text/plain; charset=utf-8',
      'If-Match': `"${expectedSHA256}"`,
    },
    body: content,
  }),
  fileVersions: (id: number) => request<{ items: FileVersion[] }>(`/api/files/${id}/versions`),
  downloadFileVersion: (id: number, versionId: number) => request<Blob>(`/api/files/${id}/versions/${versionId}/download`, {}, 'blob'),
  restoreFileVersion: (id: number, versionId: number, expectedSHA256: string) => request<UpdatedFileContent>(`/api/files/${id}/versions/${versionId}/restore`, {
    method: 'POST',
    headers: { 'If-Match': `"${expectedSHA256}"` },
  }),
  restore: (id: number) => request<void>(`/api/trash/${id}/restore`, { method: 'POST' }),
  permanentlyDelete: (id: number) => request<void>(`/api/trash/${id}`, { method: 'DELETE' }),
};

export type BackendApi = typeof backendApi;
export const api: BackendApi = isOfflinePreview ? createDemoApi() : backendApi;
