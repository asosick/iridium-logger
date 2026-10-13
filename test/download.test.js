import test from 'node:test';
import assert from 'node:assert/strict';
import { downloadLog, filenameFrom } from '../frontend/download.js';

function fakeDocument() {
  const clicked = [];
  return {
    clicked,
    body: { append() {} },
    createElement: () => ({ click() { clicked.push({ href: this.href, download: this.download }); }, remove() {} }),
  };
}

const fakeURL = { createObjectURL: () => 'blob:log', revokeObjectURL() {} };

test('an error response is reported instead of being saved as a file', async () => {
  const document = fakeDocument();
  const fetch = async () => ({ ok: false, status: 404 });
  const result = await downloadLog('/admin/logs/stream?source=a.log&download=1', { fetch, document, URL: fakeURL });
  assert.equal(result.ok, false);
  assert.match(result.message, /unavailable or downloads are disabled/);
  assert.deepEqual(document.clicked, []);
});

test('a successful response is saved under the server filename', async () => {
  const document = fakeDocument();
  const fetch = async () => ({
    ok: true,
    status: 200,
    blob: async () => ({}),
    headers: { get: () => 'attachment; filename="Cap-1000 Log For Unit # 0.TXT"' },
  });
  const result = await downloadLog('/admin/logs/stream?source=x&download=1', { fetch, document, URL: fakeURL });
  assert.equal(result.ok, true);
  assert.deepEqual(document.clicked, [{ href: 'blob:log', download: 'Cap-1000 Log For Unit # 0.TXT' }]);
});

test('filenames are read from plain, quoted and encoded dispositions', () => {
  assert.equal(filenameFrom('attachment; filename=app.log'), 'app.log');
  assert.equal(filenameFrom('attachment; filename="a \\"b\\".log"'), 'a "b".log');
  assert.equal(filenameFrom("attachment; filename*=utf-8''caf%C3%A9.log"), 'café.log');
});
