import type { ContentItem } from '../../services/api';
import { api } from '../../services/api';
import { describeFile, formatSize } from '../files/fileUtils';
import type { Preview } from '../files/fileUtils';

export interface LoadedPreview {
  preview: Preview;
  release: () => void;
}

const noop = () => {};

function unavailable(file: ContentItem, reason: Extract<Preview, { status: 'unavailable' }>['reason'], message: string): LoadedPreview {
  return {
    preview: { fileId: file.id, status: 'unavailable', descriptor: describeFile(file.name), reason, message },
    release: noop,
  };
}

function throwIfAborted(signal: AbortSignal): void {
  if (signal.aborted) throw new DOMException('Preview request aborted', 'AbortError');
}

function textEncoding(bytes: Uint8Array): 'utf-8' | 'utf-16le' | 'utf-16be' {
  if (bytes.length >= 2 && bytes[0] === 0xff && bytes[1] === 0xfe) return 'utf-16le';
  if (bytes.length >= 2 && bytes[0] === 0xfe && bytes[1] === 0xff) return 'utf-16be';
  return 'utf-8';
}

function appearsBinary(bytes: Uint8Array, encoding: ReturnType<typeof textEncoding>): boolean {
  if (encoding !== 'utf-8') return false;
  const sampleLength = Math.min(bytes.length, 8192);
  for (let index = 0; index < sampleLength; index += 1) {
    if (bytes[index] === 0) return true;
  }
  return false;
}

async function decodeText(blob: Blob, signal: AbortSignal): Promise<string | null> {
  const bytes = new Uint8Array(await blob.arrayBuffer());
  throwIfAborted(signal);
  const encoding = textEncoding(bytes);
  if (appearsBinary(bytes, encoding)) return null;
  return new TextDecoder(encoding).decode(bytes);
}

function blobWithMime(blob: Blob, mime: string): Blob {
  if (blob.type && blob.type !== 'application/octet-stream') return blob;
  return new Blob([blob], { type: mime });
}

export function previewLoadingState(file: ContentItem): Preview {
  return { fileId: file.id, status: 'loading', descriptor: describeFile(file.name) };
}

export async function loadFilePreview(file: ContentItem, signal: AbortSignal): Promise<LoadedPreview> {
  const descriptor = describeFile(file.name);
  if (descriptor.preview === 'unsupported') {
    return unavailable(file, 'unsupported', `${descriptor.label}暂不支持在线预览。`);
  }

  if (descriptor.maxPreviewBytes != null && (file.size || 0) > descriptor.maxPreviewBytes) {
    return unavailable(file, 'too-large', `文件超过 ${formatSize(descriptor.maxPreviewBytes)}，请下载后打开。`);
  }

  const downloaded = await api.downloadWithMetadata(file.id, signal);
  const { blob } = downloaded;
  const sha256 = downloaded.sha256 || file.sha256 || null;
  const parsedUpdatedAt = downloaded.updatedAt ? new Date(downloaded.updatedAt) : null;
  const updatedAt = parsedUpdatedAt && !Number.isNaN(parsedUpdatedAt.valueOf()) ? parsedUpdatedAt.toISOString() : null;
  throwIfAborted(signal);

  if (descriptor.maxPreviewBytes != null && blob.size > descriptor.maxPreviewBytes) {
    return unavailable(file, 'too-large', `文件超过 ${formatSize(descriptor.maxPreviewBytes)}，请下载后打开。`);
  }

  if (descriptor.preview === 'markdown' || descriptor.preview === 'text' || descriptor.preview === 'csv') {
    const text = await decodeText(blob, signal);
    if (text == null) {
      return unavailable(file, 'binary', '文件内容包含二进制数据，无法作为文本安全预览。');
    }
    return {
      preview: { fileId: file.id, status: 'ready', descriptor, text, sha256, size: blob.size, updatedAt },
      release: noop,
    };
  }

  const url = URL.createObjectURL(blobWithMime(blob, descriptor.mime));
  return {
    preview: { fileId: file.id, status: 'ready', descriptor, url, sha256, size: blob.size, updatedAt },
    release: () => URL.revokeObjectURL(url),
  };
}

export function isPreviewAbort(cause: unknown): boolean {
  return cause instanceof DOMException && cause.name === 'AbortError';
}
