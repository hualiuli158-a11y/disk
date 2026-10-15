import { expect, test, type Page } from '@playwright/test';

async function upload(page: Page, name: string, content: string, mimeType = 'text/plain') {
  await page.locator('input[type=file]').setInputFiles({ name, mimeType, buffer: Buffer.from(content) });
  await expect(page.getByRole('treeitem', { name, exact: true })).toBeVisible();
  await page.getByRole('treeitem', { name, exact: true }).click();
}

test('folder navigation leaves the reader empty; selecting files opens reading tabs', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByText('暂无预览内容')).toBeVisible();
  await page.getByRole('treeitem', { name: '我的笔记', exact: true }).click();
  await expect(page.getByText('暂无预览内容')).toBeVisible();
  await page.getByRole('treeitem', { name: '欢迎使用栖盘.md', exact: true }).click();
  await expect(page.locator('.markdown-body h1')).toBeVisible();
  await expect(page.locator('.cm-editor')).toHaveCount(0);
  await expect(page.locator('.code-block-heading')).toContainText('js');
  await expect(page.locator('.hljs-keyword').first()).toBeVisible();
  await page.screenshot({ path: test.info().outputPath('markdown-desktop.png') });
  const before = await page.locator('.document-page').boundingBox();
  await page.getByRole('button', { name: '沉浸阅读', exact: true }).click();
  await expect(page.locator('.explorer')).toBeHidden();
  const after = await page.locator('.document-page').boundingBox();
  expect(after!.width).toBeGreaterThan(before!.width + 200);
  await page.getByRole('button', { name: '退出沉浸阅读', exact: true }).click();
  await page.getByRole('button', { name: '关闭 欢迎使用栖盘.md', exact: true }).click();
  await expect(page.getByText('暂无预览内容')).toBeVisible();
});

test('Markdown renders tables, tasks, formulas, footnotes and sanitized HTML', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByText('暂无预览内容')).toBeVisible();
  await upload(page, '阅读回归.md', '# 阅读回归\n\n## 章节\n\n[跳到章节](#章节)\n\n- [x] 完成\n\n| 名称 | 状态 |\n| --- | --- |\n| 文件 | 就绪 |\n\n公式 $x^2$ 与脚注[^1]。\n\n[^1]: 说明。\n\n```go\npackage main\nfunc main() {}\n```\n\n<img src=x onerror="window.previewInjected=true"><script>window.previewInjected=true</script>', 'text/markdown');
  await expect(page.locator('.markdown-body h1')).toHaveText('阅读回归');
  await expect(page.locator('.task-checkbox')).toBeChecked();
  await expect(page.locator('.markdown-table-scroll table')).toBeVisible();
  await expect(page.locator('.katex')).toBeVisible();
  await expect(page.locator('.footnotes')).toContainText('说明');
  expect(await page.evaluate(() => (window as any).previewInjected)).toBeUndefined();
  await page.getByRole('button', { name: '编辑', exact: true }).click();
  await expect(page.locator('.cm-editor')).toBeVisible();
  await page.getByRole('button', { name: '预览', exact: true }).click();
  await expect(page.locator('.markdown-body h1')).toHaveText('阅读回归');
});

test('source and PDF use the central height, unsupported files have a download fallback', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByText('暂无预览内容')).toBeVisible();
  await upload(page, 'router.go', 'package main\n\nfunc main() {\n  println("hello")\n}');
  await expect(page.locator('.source-code .hljs-keyword').first()).toBeVisible();
  await expect(page.locator('.source-gutter')).toContainText('5');
  await page.getByRole('button', { name: '自动换行' }).click();
  await expect(page.locator('.source-scroll')).toHaveClass(/wrap/);
  await upload(page, 'test.pdf', '%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Count 0/Kids[]>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF', 'application/pdf');
  await expect(page.locator('object.pdf-preview')).toBeVisible();
  const pdf = await page.locator('object.pdf-preview').boundingBox();
  expect(pdf!.height).toBeGreaterThan(600);
  await upload(page, 'archive.zip', 'unsupported', 'application/zip');
  await expect(page.getByText('预览不可用', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '下载文件', exact: true }).last()).toBeVisible();
});

test('mobile directory stays open while navigating folders and closes for reading', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await expect(page.getByText('暂无预览内容')).toBeVisible();
  await page.getByRole('button', { name: '文件', exact: true }).click();
  await page.getByRole('treeitem', { name: '我的笔记', exact: true }).click();
  await expect(page.locator('.explorer')).toBeVisible();
  await page.getByRole('treeitem', { name: '欢迎使用栖盘.md', exact: true }).click();
  await expect(page.locator('.explorer')).toBeHidden();
  await expect(page.locator('.markdown-body h1')).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath('markdown-mobile.png') });
});

test('file management uses compact columns and preserves file operations', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('treeitem', { name: '项目资料', exact: true }).click();
  await page.getByRole('button', { name: '管理文件', exact: true }).click();
  await expect(page.locator('.listing-heading h1')).toHaveText('项目资料');
  await expect(page.locator('.list-header')).toContainText('修改时间');
  await expect(page.locator('.tabbar')).toHaveCount(0);
  const heading = await page.locator('.listing-heading h1').evaluate(element => getComputedStyle(element).fontSize);
  expect(parseFloat(heading)).toBeLessThanOrEqual(16);
  await page.screenshot({ path: test.info().outputPath('management-desktop.png') });
  await page.locator('.listing-page').getByRole('button', { name: '文件夹', exact: true }).click();
  const input = page.locator('.inline-create-row').getByRole('textbox', { name: '名称' });
  await input.fill('新资料');
  await input.press('Enter');
  await expect(page.locator('.listing-page').getByRole('button', { name: '新资料', exact: true })).toBeVisible();
  await expect(page.getByRole('treeitem', { name: '新资料', exact: true })).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator('.listing-heading')).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath('management-mobile.png') });
});

