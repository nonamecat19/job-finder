import { base64ToBytes } from '@/shared/base64';

/**
 * Puts a File into an <input type=file> the way a real file picker does.
 *
 * `input.files` is only assignable from a real FileList, so the file goes in
 * through a DataTransfer. The two dispatched events are what every upload
 * widget listens for — without them the site's own state never learns a file
 * arrived.
 */
export function injectFile(input: HTMLInputElement, file: File): void {
  if (!assignThroughDataTransfer(input, file)) {
    // No usable DataTransfer (jsdom, and anything that rejects a synthesised
    // FileList): define the property instead. Every consumer we care about
    // reads input.files[0] and never checks the prototype.
    Object.defineProperty(input, 'files', { value: fileListOf(file), configurable: true });
  }
  input.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
  input.dispatchEvent(new Event('change', { bubbles: true, composed: true }));
}

function assignThroughDataTransfer(input: HTMLInputElement, file: File): boolean {
  if (typeof DataTransfer !== 'function') return false;
  try {
    const dt = new DataTransfer();
    dt.items.add(file);
    input.files = dt.files;
    return input.files?.[0] === file;
  } catch {
    return false;
  }
}

export function fileFromBase64(base64: string, name: string, mime = 'application/pdf'): File {
  const bytes = base64ToBytes(base64);
  return new File([bytes as BlobPart], name, { type: mime });
}

function fileListOf(file: File): FileList {
  const list = {
    0: file,
    length: 1,
    item: (i: number) => (i === 0 ? file : null),
    [Symbol.iterator]: function* () {
      yield file;
    },
  };
  return list as unknown as FileList;
}
