import type { ContentItem } from '../../services/api';
import Icon from '../../components/Icon';
import { describeFile, iconForFileKind } from './fileUtils';

export default function FileGlyph({ item, size = 18 }: { item: ContentItem; size?: number }) {
  if (item.type === 'folder') return <span className="file-glyph folder"><Icon name="folder" size={size} /></span>;
  const descriptor = describeFile(item.name);
  return <span className={`file-glyph ${descriptor.kind}`} title={descriptor.label}><Icon name={iconForFileKind(descriptor.kind)} size={size} /></span>;
}
