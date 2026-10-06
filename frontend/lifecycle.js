// HTMX 4 exposes resolved swap targets on tasks, including out-of-band swaps.
export function registerLifecycle(document, initialize, destroyWithin) {
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
    if (event.detail?.task?.target) initialize(event.detail.task.target);
    for (const root of event.detail?.newContent || []) initialize(root);
  });
  document.addEventListener("htmx:before:cleanup", (event) => {
    if (event.target !== document) destroyWithin(event.target);
  });
}
