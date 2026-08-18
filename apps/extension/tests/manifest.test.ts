import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

const ROOT = path.resolve(import.meta.dirname, '..');
const manifest = JSON.parse(fs.readFileSync(path.join(ROOT, 'public/manifest.json'), 'utf8'));

describe('manifest', () => {
  it('names files the vite builds actually emit', () => {
    const mainConfig = fs.readFileSync(path.join(ROOT, 'vite.config.ts'), 'utf8');
    const contentConfig = fs.readFileSync(path.join(ROOT, 'vite.content.config.ts'), 'utf8');

    expect(manifest.background.service_worker).toBe('background.js');
    expect(mainConfig).toContain("background: path.resolve(import.meta.dirname, 'src/background/index.ts')");

    expect(manifest.action.default_popup).toBe('popup.html');
    expect(manifest.options_ui.page).toBe('options.html');
    expect(mainConfig).toContain("for (const entry of ['popup', 'options'])");

    expect(manifest.content_scripts[0].js).toEqual(['content.js']);
    expect(contentConfig).toContain("fileName: () => 'content.js'");
  });

  it('ships icons that exist', () => {
    for (const file of Object.values(manifest.icons) as string[]) {
      expect(fs.existsSync(path.join(ROOT, 'public', file))).toBe(true);
    }
  });

  it('grants host permissions to the API only, never to a job board', () => {
    expect(manifest.host_permissions).toEqual(['http://localhost:3000/*', 'http://127.0.0.1:3000/*']);
    for (const origin of manifest.host_permissions as string[]) {
      expect(origin).not.toMatch(/djinni|dou\.ua|work\.ua/);
    }
  });

  it('declares the three supported boards as content scripts', () => {
    expect(manifest.content_scripts[0].matches).toEqual([
      'https://djinni.co/*',
      'https://jobs.dou.ua/*',
      'https://www.work.ua/*',
      'https://work.ua/*',
    ]);
  });
});
