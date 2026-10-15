export interface ParsedTable {
  rows: string[][];
  truncated: boolean;
}

export const MAX_CSV_PREVIEW_ROWS = 500;
const MAX_CSV_PREVIEW_COLUMNS = 60;

export function parseDelimitedText(source: string, delimiter: ',' | '\t', maxRows = MAX_CSV_PREVIEW_ROWS): ParsedTable {
  const rows: string[][] = [];
  let row: string[] = [];
  let field = '';
  let quoted = false;
  let truncated = false;

  const pushField = () => {
    if (row.length < MAX_CSV_PREVIEW_COLUMNS) row.push(field);
    field = '';
  };
  const pushRow = () => {
    pushField();
    if (rows.length < maxRows) rows.push(row);
    else truncated = true;
    row = [];
  };

  for (let index = 0; index < source.length; index += 1) {
    const char = source[index];
    if (quoted) {
      if (char === '"' && source[index + 1] === '"') {
        field += '"';
        index += 1;
      } else if (char === '"') {
        quoted = false;
      } else {
        field += char;
      }
      continue;
    }
    if (char === '"' && field.length === 0) {
      quoted = true;
    } else if (char === delimiter) {
      pushField();
    } else if (char === '\n') {
      pushRow();
      if (truncated) break;
    } else if (char !== '\r') {
      field += char;
    }
  }

  if (!truncated && (field.length > 0 || row.length > 0)) pushRow();
  return { rows, truncated };
}
