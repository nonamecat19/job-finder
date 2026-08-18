import type { CsRequest, CsResponse, FillReport } from '@/shared/messages';
import { attempt, err, ok, type Result } from '@/shared/result';

import { adapterForHost } from './adapters/registry';
import { capabilitiesOf } from './adapters/types';
import { waitForElement } from './dom';
import { fileFromBase64, injectFile } from './inject-file';
import { hasExistingText, setFieldText } from './set-text';

chrome.runtime.onMessage.addListener((req: CsRequest, _sender, sendResponse) => {
  attempt(() => handle(req)).then(sendResponse);
  return true;
});

async function handle(req: CsRequest): Promise<Result<CsResponse>> {
  const adapter = adapterForHost(location.hostname);
  const host = location.hostname;

  switch (req.kind) {
    case 'cs/probe':
      return ok({ kind: 'cs/capabilities', caps: capabilitiesOf(adapter, host) });

    case 'cs/openApplyForm': {
      if (!adapter) return err('no_adapter', 'This site is not supported yet.');
      const trigger = adapter.applyTrigger();
      if (!trigger) return err('form_not_open', 'No apply button found on this page.');
      trigger.click();
      // The form usually mounts asynchronously; give it a moment before reporting back.
      await waitForElement(['textarea', 'input[type="file"]'], 3000);
      return ok({ kind: 'cs/opened', caps: capabilitiesOf(adapter, host) });
    }

    case 'cs/fill': {
      if (!adapter) return err('no_adapter', 'This site is not supported yet.');
      const report: FillReport = { fileAttached: false, letterFilled: false, warnings: [] };

      if (req.payload.file) {
        const input = adapter.findFileInput();
        if (!input) return err('no_file_input', 'No file field on this apply form.');
        injectFile(input, fileFromBase64(req.payload.file.base64, req.payload.file.name, req.payload.file.mime));
        report.fileAttached = true;
      }

      if (req.payload.letter !== undefined) {
        const field = adapter.findLetterField();
        if (!field) return err('no_letter_field', 'No cover-letter field on this apply form.');
        if (hasExistingText(field)) report.warnings.push('The letter field already had text — it was replaced.');
        if (!setFieldText(field, req.payload.letter)) {
          return err('no_letter_field', 'The cover-letter field could not be written to.');
        }
        report.letterFilled = true;
      }

      return ok({ kind: 'cs/filled', report });
    }
  }
}
