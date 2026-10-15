import { expect, test, type Page } from '@playwright/test';

async function openNote(page: Page) {
  await page.goto('/');
  await page.getByRole('treeitem', { name: '我的笔记', exact: true }).click();
  await page.getByRole('treeitem', { name: '欢迎使用栖盘.md', exact: true }).click();
  await page.getByRole('button', { name: '编辑', exact: true }).click();
  await expect(page.locator('.cm-editor')).toBeVisible();
  return page.locator('.cm-content');
}
async function historyCount(page: Page, count: number, current = true) {
  await page.getByRole('button', { name: '版本历史', exact: true }).click();
  await expect(page.locator('.history-version')).toHaveCount(count);
  await expect(page.locator('.history-version.current')).toHaveCount(current ? 1 : 0);
  await page.getByRole('button', { name: '关闭版本历史' }).click();
}

test('autosaves do not create history; Ctrl+S snapshots clean and dirty content once', async ({ page }) => {
  const editor = await openNote(page);
  await historyCount(page, 1);
  await editor.click();
  await editor.press('Control+End');
  await page.keyboard.insertText('\n自动保存第一段');
  await expect(page.locator('.save-state')).toHaveText('已保存');
  await historyCount(page, 1, false);
  await editor.press('Control+End');
  await page.keyboard.insertText('\n自动保存第二段');
  await expect(page.locator('.save-state')).toHaveText('已保存');
  await historyCount(page, 1, false);
  // The editor is clean; this must still create a snapshot.
  await editor.press('Control+s');
  await expect(page.getByText('已保存版本快照', { exact: true })).toBeVisible();
  await historyCount(page, 2);
  await editor.press('Control+s');
  await historyCount(page, 2);
  await editor.press('Control+End');
  await page.keyboard.insertText('\n手动保存第三段');
  await editor.press('Control+s');
  await expect(page.locator('.save-state')).toHaveText('已保存');
  await historyCount(page, 3);
  await editor.press('Control+End');
  await page.keyboard.insertText('\n未建快照的第四段');
  await expect(page.locator('.save-state')).toHaveText('已保存');
  await historyCount(page, 3, false);
  await page.getByRole('button', { name: '版本历史', exact: true }).click();
  page.once('dialog', dialog => dialog.accept());
  await page.locator('.history-version').first().getByRole('button', { name: '恢复此版本' }).click();
  await expect(page.locator('.history-version')).toHaveCount(4);
  await page.getByRole('button', { name: '关闭版本历史' }).click();
  await expect(editor).toContainText('手动保存第三段');
  await expect(editor).not.toContainText('未建快照的第四段');
});

test('Ctrl+S during an in-flight autosave queues a snapshot with the new version token', async ({ page }) => {
  const editor = await openNote(page);
  await page.evaluate(async () => {
    const path = performance.getEntriesByType('resource').map(entry => entry.name).find(url => new URL(url).pathname === '/src/services/api.ts')!;
    const { api } = await import(path);
    const save = api.updateFileContent;
    let first = true;
    api.updateFileContent = async (...args: Parameters<typeof save>) => {
      if (first) {
        first = false;
        await new Promise<void>(resolve => { (window as any).releaseAutosave = resolve; });
      }
      return save(...args);
    };
  });
  await editor.click();
  await editor.press('Control+End');
  await page.keyboard.insertText('\n自动保存中');
  await expect(page.locator('.save-state')).toHaveText('保存中…');
  await page.keyboard.insertText('追加内容');
  await editor.press('Control+s');
  await page.evaluate(() => (window as any).releaseAutosave());
  await expect(page.getByText('已保存版本快照', { exact: true })).toBeVisible();
  await expect(page.locator('.save-state')).toHaveText('已保存');
  await expect(page.locator('.editor-notice')).toHaveCount(0);
  await historyCount(page, 2);
  await expect(editor).toContainText('自动保存中追加内容');
});

test('retry after a failed manual snapshot still creates history for already autosaved content', async ({ page }) => {
  const editor = await openNote(page);
  await editor.click();
  await editor.press('Control+End');
  await page.keyboard.insertText('\n已自动保存的内容');
  await expect(page.locator('.save-state')).toHaveText('已保存');
  await page.evaluate(async () => {
    const path = performance.getEntriesByType('resource').map(entry => entry.name).find(url => new URL(url).pathname === '/src/services/api.ts')!;
    const { api, ApiError } = await import(path);
    const save = api.updateFileContent;
    let fail = true;
    api.updateFileContent = async (...args: Parameters<typeof save>) => {
      if (args[3] && fail) { fail = false; throw new ApiError('网络暂时不可用', 503); }
      return save(...args);
    };
  });
  await editor.press('Control+s');
  await expect(page.locator('.editor-notice.error')).toBeVisible();
  await page.locator('.editor-notice').getByRole('button', { name: '重试' }).click();
  await expect(page.getByText('已保存版本快照', { exact: true })).toBeVisible();
  await expect(page.locator('.editor-notice.error')).toHaveCount(0);
  await historyCount(page, 2);
});
