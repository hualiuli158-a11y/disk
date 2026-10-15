export type FileKind = 'markdown' | 'code' | 'text' | 'csv' | 'image' | 'video' | 'audio' | 'pdf' | 'office' | 'archive' | 'other';
export type PreviewStrategy = 'markdown' | 'text' | 'csv' | 'image' | 'video' | 'audio' | 'pdf' | 'unsupported';

export interface FileDescriptor {
  ext?: string;
  kind: FileKind;
  label: string;
  mime: string;
  preview: PreviewStrategy;
  maxPreviewBytes: number | null;
}

export type PreviewUnavailableReason = 'unsupported' | 'too-large' | 'binary' | 'load-error';
export type Preview =
  | { fileId: number; status: 'loading'; descriptor: FileDescriptor }
  | { fileId: number; status: 'ready'; descriptor: FileDescriptor; text?: string; url?: string; sha256?: string | null; size?: number; updatedAt?: string | null }
  | { fileId: number; status: 'unavailable'; descriptor: FileDescriptor; reason: PreviewUnavailableReason; message: string };

const MB = 1024 * 1024;

const EXT_TO_KIND: Record<string, FileKind> = {
  md: 'markdown', markdown: 'markdown',
  csv: 'csv', tsv: 'csv',
  txt: 'text', log: 'text', ini: 'text', conf: 'text', env: 'text', properties: 'text',
  js: 'code', jsx: 'code', ts: 'code', tsx: 'code', mjs: 'code', cjs: 'code',
  css: 'code', scss: 'code', less: 'code', html: 'code', htm: 'code', xml: 'code',
  json: 'code', jsonl: 'code', yaml: 'code', yml: 'code', toml: 'code', sql: 'code',
  go: 'code', py: 'code', rb: 'code', rs: 'code', java: 'code', kt: 'code', swift: 'code',
  c: 'code', h: 'code', cpp: 'code', hpp: 'code', cs: 'code', php: 'code', sh: 'code', bash: 'code', zsh: 'code',
  ps1: 'code', psm1: 'code', vue: 'code', svelte: 'code', dart: 'code', lua: 'code', r: 'code',
  mod: 'text', sum: 'text', lock: 'text', gitignore: 'text', dockerignore: 'text',
  png: 'image', jpg: 'image', jpeg: 'image', gif: 'image', webp: 'image', bmp: 'image',
  avif: 'image', ico: 'image', tif: 'image', tiff: 'image', heic: 'image', heif: 'image', svg: 'image',
  mp4: 'video', webm: 'video', mov: 'video', m4v: 'video', mkv: 'video',
  mp3: 'audio', wav: 'audio', ogg: 'audio', flac: 'audio', m4a: 'audio', weba: 'audio', aac: 'audio',
  pdf: 'pdf',
  doc: 'office', docx: 'office', xls: 'office', xlsx: 'office', ppt: 'office', pptx: 'office',
  odt: 'office', ods: 'office', odp: 'office',
  zip: 'archive', rar: 'archive', '7z': 'archive', tar: 'archive', gz: 'archive', bz2: 'archive', xz: 'archive',
};

const EXT_TO_MIME: Record<string, string> = {
  md: 'text/markdown', markdown: 'text/markdown', txt: 'text/plain', log: 'text/plain',
  csv: 'text/csv', tsv: 'text/tab-separated-values', json: 'application/json', jsonl: 'application/x-ndjson',
  html: 'text/html', htm: 'text/html', css: 'text/css', xml: 'application/xml', yaml: 'application/yaml', yml: 'application/yaml',
  png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif', webp: 'image/webp',
  bmp: 'image/bmp', avif: 'image/avif', ico: 'image/x-icon', svg: 'image/svg+xml', tif: 'image/tiff', tiff: 'image/tiff',
  mp4: 'video/mp4', webm: 'video/webm', mov: 'video/quicktime', m4v: 'video/x-m4v', mkv: 'video/x-matroska',
  mp3: 'audio/mpeg', wav: 'audio/wav', ogg: 'audio/ogg', flac: 'audio/flac', m4a: 'audio/mp4', weba: 'audio/webm', aac: 'audio/aac',
  pdf: 'application/pdf', zip: 'application/zip',
};

const KIND_LABELS: Record<FileKind, string> = {
  markdown: 'Markdown', code: '代码', text: '文本', csv: '表格', image: '图片', video: '视频',
  audio: '音频', pdf: 'PDF', office: 'Office 文档', archive: '压缩包', other: '文件',
};

const KIND_PREVIEW: Record<FileKind, PreviewStrategy> = {
  markdown: 'markdown', code: 'text', text: 'text', csv: 'csv', image: 'image', video: 'video',
  audio: 'audio', pdf: 'pdf', office: 'unsupported', archive: 'unsupported', other: 'unsupported',
};

const PREVIEW_LIMITS: Record<PreviewStrategy, number | null> = {
  markdown: 5 * MB,
  text: 5 * MB,
  csv: 5 * MB,
  image: 40 * MB,
  video: 80 * MB,
  audio: 80 * MB,
  pdf: 60 * MB,
  unsupported: null,
};

export function extOfName(name: string): string | undefined {
  const base = name.split(/[\\/]/).pop() || '';
  const dot = base.lastIndexOf('.');
  if (dot < 0 || dot === base.length - 1) return undefined;
  return base.slice(dot + 1).toLowerCase();
}

function kindFromMime(mimeType?: string): FileKind | null {
  const mime = String(mimeType || '').toLowerCase().split(';')[0].trim();
  if (!mime || mime === 'application/octet-stream') return null;
  if (mime === 'text/markdown') return 'markdown';
  if (mime === 'text/csv' || mime === 'text/tab-separated-values') return 'csv';
  if (mime.startsWith('text/')) return 'text';
  if (mime.startsWith('image/')) return 'image';
  if (mime.startsWith('video/')) return 'video';
  if (mime.startsWith('audio/')) return 'audio';
  if (mime === 'application/pdf') return 'pdf';
  return null;
}

export function describeFile(name: string, mimeType?: string): FileDescriptor {
  const ext = extOfName(name);
  const baseName = name.split(/[\\/]/).pop()?.toLowerCase();
  const kind = (ext ? EXT_TO_KIND[ext] : undefined) || (baseName === 'dockerfile' || baseName === 'makefile' ? 'code' : undefined) || kindFromMime(mimeType) || 'other';
  const preview = KIND_PREVIEW[kind];
  const suppliedMime = String(mimeType || '').toLowerCase().split(';')[0].trim();
  const mime = suppliedMime && suppliedMime !== 'application/octet-stream'
    ? suppliedMime
    : (ext ? EXT_TO_MIME[ext] : undefined) || (kind === 'text' || kind === 'code' ? 'text/plain' : 'application/octet-stream');
  return { ext, kind, label: KIND_LABELS[kind], mime, preview, maxPreviewBytes: PREVIEW_LIMITS[preview] };
}

export function kindOf(name: string): FileKind {
  return describeFile(name).kind;
}

export function mimeOf(name: string): string {
  return describeFile(name).mime;
}

export function iconForFileKind(kind: FileKind): string {
  if (kind === 'image') return 'image';
  if (kind === 'video') return 'video';
  if (kind === 'audio') return 'audio';
  if (kind === 'markdown') return 'markdown';
  if (kind === 'code' || kind === 'text' || kind === 'csv') return 'code';
  if (kind === 'pdf') return 'pdf';
  if (kind === 'archive') return 'archive';
  return 'file';
}

export function previewVersionKey(file: { id: number; name: string; size?: number | null; sha256?: string | null; updated_at?: string; created_at?: string }): string {
  return [file.id, file.name, file.size ?? '', file.sha256 || '', file.updated_at || file.created_at || ''].join(':');
}

export function formatSize(size?: number | null): string {
  if (size == null) return '—';
  if (size < 1024) return `${size} B`;
  if (size < 1024 ** 2) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 ** 3) return `${(size / 1024 ** 2).toFixed(1)} MB`;
  return `${(size / 1024 ** 3).toFixed(1)} GB`;
}

export function formatDate(value?: string): string {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? '—' : date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

export function formatTrashExpiry(value?: string): string {
  if (!value) return '到期时间未知';
  const expiresAt = new Date(value);
  if (Number.isNaN(expiresAt.valueOf())) return '到期时间未知';
  const remaining = expiresAt.getTime() - Date.now();
  if (remaining <= 0) return '即将清理';
  const hours = Math.ceil(remaining / (60 * 60 * 1000));
  if (hours <= 24) return `${hours} 小时后清理`;
  return `${Math.ceil(hours / 24)} 天后清理`;
}
