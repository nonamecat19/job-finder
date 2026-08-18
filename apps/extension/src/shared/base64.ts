/**
 * Extension messaging JSON-serialises its payloads: ArrayBuffer, Blob and File
 * do not survive the trip and there are no transferables. PDFs therefore cross
 * the worker/content-script boundary as base64.
 */

// Encoding the whole buffer in one String.fromCharCode.apply call overflows the
// argument stack somewhere around a few hundred KB, so walk it in slices.
const CHUNK = 0x4000;

export function bytesToBase64(bytes: Uint8Array): string {
  let binary = '';
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode(...bytes.subarray(i, i + CHUNK));
  }
  return btoa(binary);
}

export function base64ToBytes(b64: string): Uint8Array {
  const binary = atob(b64);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}
