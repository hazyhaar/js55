'use strict';

const vm = require('node:vm');

function walk(v, seen) {
  if (v === undefined) return { t: 'u' };
  if (v === null) return { t: 'n' };
  const ty = typeof v;
  if (ty === 'boolean') return { t: 'b', v: v };
  if (ty === 'number') {
    if (Object.is(v, -0)) return { t: 'num', v: '-0' };
    if (Number.isNaN(v)) return { t: 'num', v: 'NaN' };
    if (v === Infinity) return { t: 'num', v: 'Infinity' };
    if (v === -Infinity) return { t: 'num', v: '-Infinity' };
    return { t: 'num', v: String(v) };
  }
  if (ty === 'string') return { t: 's', v: v };
  if (ty === 'bigint') return { t: 'bi', v: String(v) };
  if (ty === 'symbol') return { t: 'sym', v: String(v) };
  if (ty === 'function') {
    return { t: 'fn', name: v.name, length: v.length };
  }
  if (ty === 'object') {
    if (seen.has(v)) return { t: 'cycle' };
    seen.add(v);
    if (typeof v.then === 'function' && Object.prototype.toString.call(v) === '[object Promise]') {
      return { t: 'promise', s: 'pending' };
    }
    if (Array.isArray(v)) {
      const els = [];
      for (let i = 0; i < v.length; i++) {
        els.push(walk(v[i], seen));
      }
      const extraKeys = [];
      const extra = [];
      for (const k of Object.keys(v)) {
        if (k === 'length' || /^(0|[1-9]\d*)$/.test(k)) continue;
        extraKeys.push(k);
        extra.push(walk(v[k], seen));
      }
      return { t: 'arr', els: els, extraKeys: extraKeys, extra: extra };
    }
    const keys = Object.keys(v);
    const vals = keys.map((k) => walk(v[k], seen));
    return { t: 'obj', keys: keys, vals: vals };
  }
  return { t: 'other', v: String(v) };
}

function display(v) {
  if (typeof v === 'string') return v;
  if (v === undefined) return 'undefined';
  if (v === null) return 'null';
  if (typeof v === 'function') {
    return 'function ' + (v.name || 'anonymous') + '() { [bytecode] }';
  }
  if (Array.isArray(v)) {
    return v.map((e) => (e === undefined || e === null ? '' : display(e))).join(',');
  }
  if (typeof v === 'object') return '[object Object]';
  return String(v);
}

function encode(v) {
  return JSON.stringify(walk(v, new WeakSet()));
}

function drain() {
  return new Promise((resolve) => setImmediate(resolve));
}

function fail(e) {
  const name = (e && e.name) || (e && e.constructor && e.constructor.name) || 'Error';
  const message = (e && typeof e.message === 'string') ? e.message : String(e);
  return { error: name, name: name, message: message };
}

function isThenable(v) {
  return v != null && (typeof v === 'object' || typeof v === 'function') && typeof v.then === 'function';
}

async function runOne(src) {
  const context = vm.createContext({});
  try {
    let value = vm.runInContext(src, context, { timeout: 2000 });
    if (isThenable(value)) {
      const settled = Promise.resolve(value).then(
        (v) => ({ ok: true, v: v }),
        (e) => ({ ok: false, e: e })
      );
      await drain();
      let r;
      try {
        r = await Promise.race([
          settled,
          new Promise((_, rej) => setTimeout(() => rej(new Error('promise timeout')), 1500)),
        ]);
      } catch (e) {
        return fail(e);
      }
      if (!r.ok) {
        return fail(r.e);
      }
      value = r.v;
    } else {
      await drain();
    }
    try {
      return { value: encode(value), display: display(value) };
    } catch (e) {
      return fail(e);
    }
  } catch (e) {
    return fail(e);
  }
}

let input = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', (d) => { input += d; });
process.stdin.on('end', async () => {
  const { programs } = JSON.parse(input);
  const results = [];
  for (const src of programs) {
    results.push(await runOne(src));
  }
  process.stdout.write(JSON.stringify({ results }));
});
