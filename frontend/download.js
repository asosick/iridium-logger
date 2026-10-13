// Fetches a complete log file before handing it to the browser, so a refused
// or missing download reports an error instead of saving the error response.
export async function downloadLog(url, { fetch = globalThis.fetch, document = globalThis.document, URL = globalThis.URL } = {}) {
  let response;
  try {
    response = await fetch(url, { credentials: "same-origin" });
  } catch {
    return { ok: false, message: "Download failed: the server could not be reached" };
  }
  if (!response.ok) {
    return { ok: false, message: downloadError(response.status) };
  }
  const blob = await response.blob();
  const objectURL = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = objectURL;
  link.download = filenameFrom(response.headers.get("Content-Disposition")) || filenameFromURL(url) || "log.txt";
  link.hidden = true;
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(objectURL), 0);
  return { ok: true };
}

function downloadError(status) {
  if (status === 404) return "Download failed: the log file is unavailable or downloads are disabled";
  if (status === 401 || status === 403) return "Download failed: not permitted";
  return `Download failed (${status})`;
}

export function filenameFrom(disposition) {
  if (!disposition) return "";
  const encoded = disposition.match(/filename\*\s*=\s*(?:UTF-8|utf-8)''([^;]+)/);
  if (encoded) {
    try { return decodeURIComponent(encoded[1].trim()); }
    catch { /* fall through to the plain filename */ }
  }
  const quoted = disposition.match(/filename\s*=\s*"((?:\\.|[^"\\])*)"/);
  if (quoted) return quoted[1].replace(/\\(.)/g, "$1");
  return disposition.match(/filename\s*=\s*([^;]+)/)?.[1].trim() || "";
}

function filenameFromURL(url) {
  try { return new URL(url, "http://localhost").searchParams.get("source") || ""; }
  catch { return ""; }
}
