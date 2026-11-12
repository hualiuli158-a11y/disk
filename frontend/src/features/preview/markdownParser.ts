import MarkdownIt from 'markdown-it';
import footnote from 'markdown-it-footnote';

// The footnote declarations use the CJS entry for the same runtime plugin API.
export const parser = new MarkdownIt({ html: true, linkify: true, typographer: true, breaks: false })
  .use(footnote as unknown as (md: MarkdownIt) => void);

export interface Heading {
  level: number;
  text: string;
  index: number;
  line: number;
  id: string;
}

export function getOutline(source: string, fileId = ''): Heading[] {
  // Use the same Markdown parser as the rendered document so nested headings,
  // fenced code and setext headings share one order and one set of anchor IDs.
  const tokens = parser.parse(source, { fileId, headingIndex: 0 });
  const headings: Heading[] = [];
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token.type !== 'heading_open') continue;
    const headingIndex = headings.length;
    const inline = tokens[index + 1];
    const text = inline?.children?.map(child => {
      if (child.type === 'image') return child.content;
      if (child.type === 'text' || child.type === 'code_inline') return child.content;
      if (child.type === 'softbreak' || child.type === 'hardbreak') return ' ';
      return '';
    }).join('').trim() || inline?.content || '';
    headings.push({
      level: Number(token.tag.slice(1)), text, index: headingIndex,
      line: (token.map?.[0] ?? 0) + 1, id: `heading-${fileId}-${headingIndex}`,
    });
  }
  return headings;
}

