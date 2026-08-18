import { afterEach, beforeEach } from 'vitest';

import { installFakeChrome, uninstallFakeChrome } from './fake-chrome';

beforeEach(() => {
  installFakeChrome();
});

afterEach(() => {
  uninstallFakeChrome();
});
