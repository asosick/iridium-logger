import test from 'node:test';
import assert from 'node:assert/strict';
import { registerLifecycle } from '../frontend/lifecycle.js';

function lifecycle() {
  const document = new EventTarget();
  const active = new Set();
  registerLifecycle(document, root => active.add(root), root => active.delete(root));
  const dispatch = (name, detail, cancelled = false) => {
    const event = new CustomEvent(name, {detail, cancelable: true});
    if (cancelled) event.preventDefault();
    document.dispatchEvent(event);
  };
  return {document, active, dispatch};
}

test('a live form switch closes both main and out-of-band streams before mounting replacement viewers', () => {
  const {active, dispatch} = lifecycle();
  const main = {}, sidebar = {}, unrelated = {}, replacement = {};
  active.add(main); active.add(sidebar); active.add(unrelated);
  dispatch('htmx:before:swap', {ctx: {target: '#form'}, tasks: [
    {target: main, swapSpec: {style: 'morph'}},
    {target: sidebar, swapSpec: {style: 'innerHTML'}},
  ]});
  assert.deepEqual([...active], [unrelated]);
  dispatch('htmx:after:settle', {task: {target: replacement}, newContent: [replacement]});
  assert.deepEqual([...active], [unrelated, replacement]);
});

test('cancelled and append-only swaps keep the existing streams alive', () => {
  const {active, dispatch} = lifecycle();
  const root = {};
  active.add(root);
  dispatch('htmx:before:swap', {tasks: [{target: root, swapSpec: {style: 'morph'}}]}, true);
  for (const style of ['none', 'beforebegin', 'afterbegin', 'beforeend', 'afterend']) {
    dispatch('htmx:before:swap', {tasks: [{target: root, swapSpec: {style}}]});
  }
  assert.deepEqual([...active], [root]);
});

test('outer and sibling swaps initialize the inserted roots even when the source is detached', () => {
  const {active, document, dispatch} = lifecycle();
  const target = {}, sibling = {};
  dispatch('htmx:after:settle', {task: {target}, newContent: [sibling]});
  assert.deepEqual([...active], [target, sibling]);
  dispatch('htmx:after:swap', {ctx: {target: '#old-target'}});
  assert.ok(active.has(document));
  dispatch('htmx:before:cleanup');
  assert.ok(active.has(document), 'detached cleanup events must not close unrelated streams');
});
