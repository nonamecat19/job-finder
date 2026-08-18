import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

import { adapterForHost } from '@/content/adapters/registry';
import { capabilitiesOf } from '@/content/adapters/types';

function loadFixture(name: string, host: string) {
  document.body.innerHTML = fs.readFileSync(path.join(import.meta.dirname, 'fixtures', name), 'utf8');
  const adapter = adapterForHost(host);
  return { adapter: adapter!, caps: capabilitiesOf(adapter, host) };
}

describe('registry', () => {
  it('matches a host with or without www and on subdomains', () => {
    expect(adapterForHost('djinni.co')?.id).toBe('djinni');
    expect(adapterForHost('www.work.ua')?.id).toBe('workua');
    expect(adapterForHost('jobs.dou.ua')?.id).toBe('dou');
  });

  it('claims nothing on an unrelated host', () => {
    expect(adapterForHost('example.com')).toBeNull();
    expect(capabilitiesOf(null, 'example.com').adapter).toBeNull();
  });
});

describe('djinni adapter', () => {
  it('reports a closed form that can be opened', () => {
    const { caps } = loadFixture('djinni-closed.html', 'djinni.co');

    expect(caps.formOpen).toBe(false);
    expect(caps.canOpenForm).toBe(true);
    expect(caps.hints.title).toBe('Senior Go Engineer');
  });

  it('reports both fields once the form is open', () => {
    const { caps } = loadFixture('djinni-open.html', 'djinni.co');

    expect(caps.formOpen).toBe(true);
    expect(caps.hasFileInput).toBe(true);
    expect(caps.hasLetterField).toBe(true);
    expect(caps.requiresLogin).toBe(false);
  });

  it('finds a display:none file input — hiding one behind a label is the norm, not a reason to skip it', () => {
    const { adapter } = loadFixture('djinni-open.html', 'djinni.co');

    const input = adapter.findFileInput();

    expect(input).not.toBeNull();
    expect(getComputedStyle(input!).display).toBe('none');
  });

  it('still finds both fields when every class and id has been renamed', () => {
    const { caps } = loadFixture('djinni-churn.html', 'djinni.co');

    expect(caps.hasFileInput).toBe(true);
    expect(caps.hasLetterField).toBe(true);
    expect(caps.formOpen).toBe(true);
  });

  it('reports a login wall instead of a broken form', () => {
    const { caps } = loadFixture('djinni-loggedout.html', 'djinni.co');

    expect(caps.requiresLogin).toBe(true);
    expect(caps.formOpen).toBe(false);
  });
});

describe('dou adapter', () => {
  it('reports a fillable on-site form', () => {
    const { caps } = loadFixture('dou-open.html', 'jobs.dou.ua');

    expect(caps.formOpen).toBe(true);
    expect(caps.hasFileInput).toBe(true);
    expect(caps.hasLetterField).toBe(true);
    expect(caps.hints.company).toBe('Acme');
  });

  it('reports no form and no trigger when the vacancy applies by email', () => {
    const { caps } = loadFixture('dou-email-only.html', 'jobs.dou.ua');

    expect(caps.formOpen).toBe(false);
    expect(caps.canOpenForm).toBe(false);
  });
});

describe('work.ua adapter', () => {
  it('reports a letter field with no file upload when the site wants its own resume', () => {
    const { caps } = loadFixture('workua-letter-only.html', 'www.work.ua');

    expect(caps.hasLetterField).toBe(true);
    expect(caps.hasFileInput).toBe(false);
    expect(caps.formOpen).toBe(true);
  });
});
