import { expect, test, type Page } from '@playwright/test';
import { readFile } from 'node:fs/promises';

async function openEditor(page: Page, source: string) {
  await page.goto('/');
  await expect(page.getByText('暂无预览内容')).toBeVisible();
  await page.locator('input[type=file]').setInputFiles({ name: '编辑验收.md', mimeType: 'text/markdown', buffer: Buffer.from(source) });
  await page.getByRole('treeitem', { name: '编辑验收.md', exact: true }).click();
  await page.getByRole('button', { name: '编辑', exact: true }).click();
  await expect(page.locator('.cm-editor')).toBeVisible();
  const uploadClose = page.locator('.upload-status').getByRole('button', { name: '关闭', exact: true });
  if (await uploadClose.isVisible()) await uploadClose.click();
  return page.locator('.cm-content');
}

async function savedSource(page: Page) {
  await expect(page.locator('.save-state')).toHaveText('已保存');
  const downloading = page.waitForEvent('download');
  await page.getByRole('button', { name: '下载文件', exact: true }).click();
  const download = await downloading;
  return readFile((await download.path())!, 'utf8');
}

test('editor matches reference typography, has no floating counter and preserves source on open', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  const source = '# 项目计划\n\n把存储服务变成一个真正好用的文件工作区。\n\n## 第一阶段 · 前端体验\n\n- [x] 文件树和工作区布局\n- [ ] Markdown 编辑与预览\n\n## 第二阶段 · 在线保存\n\n支持 **加粗**、*斜体*、~~删除线~~和 `code`。\n\n```ts\nconst name = "disk";\n```\n\n| 名称 | 状态 |\n| --- | --- |\n| 文件 | 就绪 |\n\n结束。';
  await openEditor(page, source);
  await expect(page.locator('.markdown-floating-actions,.editor-stats')).toHaveCount(0);
  const heading = page.locator('.cm-line').filter({ hasText: '第一阶段' });
  expect(await heading.evaluate(el => [...el.querySelectorAll('*')].every(child => getComputedStyle(child).textDecorationLine === 'none'))).toBe(true);
  await expect(page.locator('.cm-codeblock-lang')).toHaveText('ts');
  await expect(page.locator('.cm-table-widget table')).toBeVisible();
  await page.screenshot({ path: test.info().outputPath('editor-desktop.png') });
  expect(await savedSource(page)).toBe(source);
  expect(errors).toEqual([]);
});

test('typing, Chinese composition, task toggles, list continuation and undo survive autosave', async ({ page }) => {
  const source = '# 编辑检查\n\n- [ ] 第一项\n- [x] 第二项\n\n正文';
  const editor = await openEditor(page, source);
  await page.getByRole('checkbox', { name: '标记为已完成' }).click();
  expect(await savedSource(page)).toContain('- [x] 第一项');
  await page.getByRole('checkbox', { name: '标记为未完成' }).first().focus();
  await page.keyboard.press('Space');
  expect(await savedSource(page)).toContain('- [ ] 第一项');
  await editor.click();
  await editor.press('Control+End');
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Input.imeSetComposition', { text: '中文输入', selectionStart: 4, selectionEnd: 4 });
  await cdp.send('Input.insertText', { text: '中文输入' });
  await expect(editor).toContainText('正文中文输入');
  await expect(page.locator('.save-state')).toHaveText('已保存');
  await page.keyboard.insertText('继续');
  await expect(editor).toContainText('正文中文输入继续');
  await editor.press('Control+z');
  await expect(editor).not.toContainText('继续');
  await editor.press('Control+Shift+Z');
  await expect(editor).toContainText('继续');
  await editor.press('Control+End');
  await editor.press('Enter');
  await page.keyboard.insertText('- 项目一');
  await editor.press('Enter');
  await page.keyboard.insertText('项目二');
  const result = await savedSource(page);
  expect(result).toContain('正文中文输入继续');
  expect(result).toContain('- 项目一\n- 项目二');
});

test('mobile editor supports formulas and toolbar find without bottom overlays', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  // Upload through the desktop explorer, then verify the editor at phone width.
  await page.setViewportSize({ width: 1440, height: 900 });
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await openEditor(page, '# 数学笔记\n\n公式 $x^2$。\n\n$$\nx+y=z\n$$\n\n> 引用内容\n\n```ts\nconst value = 42;\n```\n\n结束。');
  await expect(page.locator('.cm-math-widget .katex').first()).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator('.markdown-floating-actions,.editor-stats')).toHaveCount(0);
  await page.getByRole('button', { name: '在文档中查找', exact: true }).click();
  await page.getByRole('textbox', { name: '在文档中查找' }).fill('引用');
  await expect(page.locator('.preview-find-box')).toContainText('1/1');
  await page.getByRole('button', { name: '关闭查找' }).click();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath('editor-mobile.png') });
  expect(errors).toEqual([]);
});

test('formatting toggles without accumulating markers, and table edits persist', async ({ page }) => {
  const editor = await openEditor(page, '# 格式检查\n\n| 名称 | 状态 |\n| --- | --- |\n| 文件 | 就绪 |\n\n选择文字');
  const cell = page.locator('.cm-table-widget td').last();
  await cell.click();
  await page.keyboard.insertText('已更新');
  await page.keyboard.press('Enter');
  expect(await savedSource(page)).toContain('已更新');
  await editor.click();
  await editor.press('Control+End');
  await editor.press('Home');
  await editor.press('Shift+End');
  await editor.press('Control+b');
  expect(await savedSource(page)).toContain('**选择文字**');
  // Returning from a toolbar action preserves the editor selection.
  await editor.press('Control+b');
  expect(await savedSource(page)).toContain('\n选择文字');
  expect(await savedSource(page)).not.toContain('**');
  await page.getByRole('button', { name: '预览', exact: true }).click();
  await expect(page.locator('.cm-editor')).toBeHidden();
  await expect(page.locator('.markdown-floating-actions')).toHaveCount(0);
  await page.getByRole('button', { name: '编辑', exact: true }).click();
  await editor.press('Control+z');
  expect(await savedSource(page)).toContain('**选择文字**');
  await page.keyboard.press('Control+f');
  await expect(page.getByRole('textbox', { name: '在文档中查找' })).toBeFocused();
  await page.getByRole('textbox', { name: '在文档中查找' }).fill('选择文字');
  await expect(page.locator('.preview-find-box')).toContainText('1/1');
  await page.keyboard.press('Escape');
  await expect(page.locator('.preview-find-box')).toHaveCount(0);
});

test('table navigation keeps focus, escaped pipes and column alignment', async ({ page }) => {
  await openEditor(page, '# 表格检查\n\n| 名称 | 状态 |\n| :--- | ---: |\n| A\\|B | 就绪 |\n\n正文');
  const firstCell = page.locator('.cm-table-widget td').first();
  await expect(firstCell).toHaveText('A|B');
  await firstCell.click();
  await page.keyboard.insertText('C|D');
  await page.keyboard.press('Tab');
  await expect(page.locator('.cm-table-widget td').last()).toBeFocused();
  await page.keyboard.insertText('完成');
  await page.keyboard.press('Enter');
  const saved = await savedSource(page);
  expect(saved).toContain('C\\|D');
  expect(saved).toContain('| :--- | ---: |');
  expect(saved).toContain('完成');
});
