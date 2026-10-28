import test from 'node:test';
import assert from 'node:assert/strict';
import { parseEntry } from '../frontend/entry.js';

test('a logfmt line has its time, level and message', () => {
  const entry = parseEntry('time=2026-10-28T09:01:29.113-04:00 level=warning msg="Disk \\"low\\""');
  assert.equal(entry.structured, true);
  assert.equal(entry.time, '2026-10-28T09:01:29.113-04:00');
  assert.equal(entry.level, 'WARN');
  assert.equal(entry.message, 'Disk "low"');
});

// A CAP-1000 request is logged as XML, whose time attribute is not a log time.
test('a line that only contains key=value pairs is plain text', () => {
  const entry = parseEntry('<request id="1000" command="add" time="Fri Oct 09 23:59:55 EDT 2026" type="push">');
  assert.equal(entry.structured, false);
  assert.equal(entry.time, '');
  assert.equal(entry.level, '');
});
