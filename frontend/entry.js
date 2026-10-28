import { stripAnsi } from "fancy-ansi";

const pair = /(?:^|\s)([A-Za-z_][\w.-]*)=("(?:\\.|[^"\\])*"|[^\s]+)/g;
const leadingPair = /^\s*[A-Za-z_][\w.-]*=/;

// parseEntry reads a logfmt line's time, level and message. Only a line that
// begins with a key=value pair is logfmt: other text that merely contains one,
// such as an XML element with a time attribute, stays unstructured.
export function parseEntry(raw) {
  const plain = stripAnsi(raw);
  const fields = {};
  if (leadingPair.test(plain)) {
    let match;
    pair.lastIndex = 0;
    while ((match = pair.exec(plain)) !== null) {
      let value = match[2];
      if (value.startsWith('"')) {
        try { value = JSON.parse(value); }
        catch { value = value.slice(1, -1); }
      }
      fields[match[1]] = value;
    }
  }
  const time = fields.time || fields.timestamp || "";
  const level = normalizeLevel(fields.level || "");
  const message = fields.msg || fields.message || "";
  return {
    raw,
    plain,
    time,
    level,
    message,
    structured: Boolean(time || level || message),
  };
}

export function normalizeLevel(level) {
  const normalized = level.toUpperCase();
  if (normalized === "WARNING") return "WARN";
  return normalized;
}
