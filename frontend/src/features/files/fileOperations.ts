import type { ContentItem, Folder } from '../../services/api';

export type FileOperationItem = Pick<ContentItem, 'id' | 'type' | 'parent_id' | 'name'>;

const DRAG_MIME = 'application/x-qipan-file-items+json';
let activeDragItems: FileOperationItem[] | null = null;
let dragCleanupTimer: ReturnType<typeof setTimeout> | null = null;

export function fileItemKey(item: Pick<FileOperationItem, 'id' | 'type'>): string {
  return `${item.type}:${item.id}`;
}

export function writeFileDragPayload(dataTransfer: DataTransfer, items: FileOperationItem[]): void {
  const payload = items.map(item => ({ id: item.id, type: item.type, parent_id: item.parent_id, name: item.name }));
  activeDragItems = payload;
  if (dragCleanupTimer) clearTimeout(dragCleanupTimer);
  dragCleanupTimer = setTimeout(clearFileDragPayload, 60_000);
  dataTransfer.effectAllowed = 'move';
  dataTransfer.setData(DRAG_MIME, JSON.stringify(payload));
  dataTransfer.setData('text/plain', payload.map(item => item.name).join('\n'));
}

export function readFileDragPayload(dataTransfer?: DataTransfer | null): FileOperationItem[] | null {
  const raw = dataTransfer?.getData(DRAG_MIME);
  if (raw) {
    try {
      const parsed = JSON.parse(raw) as unknown;
      if (Array.isArray(parsed)) {
        const valid = parsed.filter((item): item is FileOperationItem => {
          if (!item || typeof item !== 'object') return false;
          const value = item as Partial<FileOperationItem>;
          return Number.isInteger(value.id) && value.id! > 0
            && (value.type === 'file' || value.type === 'folder')
            && typeof value.name === 'string';
        });
        if (valid.length > 0) return valid;
      }
    } catch { /* Fall through to the in-memory payload. */ }
  }
  return activeDragItems ? activeDragItems.map(item => ({ ...item })) : null;
}

export function clearFileDragPayload(): void {
  activeDragItems = null;
  if (dragCleanupTimer) clearTimeout(dragCleanupTimer);
  dragCleanupTimer = null;
}

export function dataTransferHasFiles(dataTransfer: DataTransfer): boolean {
  return Array.from(dataTransfer.types || []).includes('Files') || dataTransfer.files.length > 0;
}

function hasSelectedFolderAncestor(item: FileOperationItem, selectedFolderIds: Set<number>, folders: Record<number, Folder>): boolean {
  let parentId = item.parent_id;
  const visited = new Set<number>();
  while (parentId != null && !visited.has(parentId)) {
    if (selectedFolderIds.has(parentId)) return true;
    visited.add(parentId);
    parentId = folders[parentId]?.parent_id ?? null;
  }
  return false;
}

export function compactOperationItems(items: readonly FileOperationItem[], folders: Record<number, Folder>): FileOperationItem[] {
  const unique = new Map(items.map(item => [fileItemKey(item), item]));
  const values = [...unique.values()];
  const selectedFolderIds = new Set(values.filter(item => item.type === 'folder').map(item => item.id));
  return values.filter(item => !hasSelectedFolderAncestor(item, selectedFolderIds, folders));
}

export function folderIsWithin(folderId: number, possibleAncestorId: number, folders: Record<number, Folder>): boolean {
  let currentId: number | null = folderId;
  const visited = new Set<number>();
  while (currentId != null && !visited.has(currentId)) {
    if (currentId === possibleAncestorId) return true;
    visited.add(currentId);
    currentId = folders[currentId]?.parent_id ?? null;
  }
  return false;
}

export function canMoveItemsTo(items: readonly FileOperationItem[], destinationId: number, folders: Record<number, Folder>): boolean {
  if (!items.length || !Number.isInteger(destinationId) || destinationId <= 0) return false;
  const compacted = compactOperationItems(items, folders);
  if (compacted.every(item => item.parent_id === destinationId)) return false;
  return !compacted.some(item => item.type === 'folder' && folderIsWithin(destinationId, item.id, folders));
}
