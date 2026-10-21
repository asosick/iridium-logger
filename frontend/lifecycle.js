// HTMX 4 exposes resolved swap targets on tasks, including out-of-band swaps.
export function registerLifecycle(document, initialize, destroyWithin) {
  // A settled task can name roots that are not on the page: an outerHTML
  // target is the element just removed, and a morph's newContent holds the
  // parsed copy whose nodes were merged into the existing ones. A viewer
  // mounted there opens a stream nothing ever closes, and once the leaked
  // streams fill the browser's connections to the host every request hangs.
  const initializeAttached = (root) => {
    if (root?.isConnected !== false) initialize(root);
  };
  document.addEventListener("DOMContentLoaded", () => initialize(document));
  document.addEventListener("htmx:before:swap", (event) => {
    if (event.defaultPrevented) return;
    for (const task of event.detail?.tasks || []) {
      const style = task.swapSpec?.style;
      if (["none", "beforebegin", "afterbegin", "beforeend", "afterend"].includes(style)) continue;
      const target = typeof task.target === "string" ? document.querySelector(task.target) : task.target;
      if (target) destroyWithin(target);
    }
  });
  document.addEventListener("htmx:after:swap", () => initialize(document));
  document.addEventListener("htmx:after:settle", (event) => {
    if (event.detail?.task?.target) initializeAttached(event.detail.task.target);
    for (const root of event.detail?.newContent || []) initializeAttached(root);
  });
  document.addEventListener("htmx:before:cleanup", (event) => {
    if (event.target !== document) destroyWithin(event.target);
  });
}
