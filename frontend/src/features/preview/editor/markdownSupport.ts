import MarkdownIt from 'markdown-it';
import DOMPurify from 'dompurify';

// Browser adapters for the upstream editor's desktop Markdown utilities.
const inlineParser = new MarkdownIt({ html: false, breaks: false });
export function getMd() {
  return { renderInline: (source: string) => DOMPurify.sanitize(inlineParser.renderInline(source)) };
}
export interface ImageDimensions { width?: string; height?: string }
export interface MarkdownImageContext { filePath?: string | null; getFileUrl?: ((path: string) => string | undefined) | null }
export function parseImageLabel(raw: string): { alt: string; dimensions: ImageDimensions | null } {
  const match = /(?:^|\|)([1-9]\d{0,4})(?:x([1-9]\d{0,4}))?$/.exec(raw.trim());
  return match ? { alt: raw.slice(0, match.index).trim(), dimensions: { width: match[1], height: match[2] } } : { alt: raw, dimensions: null };
}
export function parseObsidianImageEmbed(raw: string) {
  const [src, ...label] = raw.split('|');
  if (!/\.(?:png|jpe?g|gif|webp|avif|svg|bmp)(?:[?#]|$)/i.test(src)) return null;
  return { src, ...parseImageLabel(label.join('|') || src) };
}
export function resolveMarkdownImageSrc(src: string, context?: MarkdownImageContext): string {
  if (/^(?:https?:|blob:|data:image\/)/i.test(src)) return src;
  return context?.getFileUrl?.(src) || src;
}
