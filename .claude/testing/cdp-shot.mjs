#!/usr/bin/env node
// CDP 截图：截取应用页面写入 PNG。用法：node cdp-shot.mjs <输出路径>   （CDP_PORT 默认 9222）
const port = process.env.CDP_PORT || '9222';
const out = process.argv[2];
if (!out) {
  console.error('用法: cdp-shot.mjs <输出.png>');
  process.exit(2);
}
const fs = await import('node:fs');
const list = await (await fetch(`http://127.0.0.1:${port}/json`)).json();
const page = list.find((t) => t.type === 'page');
if (!page) {
  console.error('无 page target');
  process.exit(3);
}
const ws = new WebSocket(page.webSocketDebuggerUrl);
let nextId = 0;
const pending = new Map();
function send(method, params) {
  return new Promise((resolve, reject) => {
    const id = ++nextId;
    pending.set(id, { resolve, reject });
    ws.send(JSON.stringify({ id, method, params }));
  });
}
ws.onmessage = (ev) => {
  const msg = JSON.parse(ev.data);
  if (msg.id && pending.has(msg.id)) {
    const { resolve, reject } = pending.get(msg.id);
    pending.delete(msg.id);
    msg.error ? reject(new Error(JSON.stringify(msg.error))) : resolve(msg.result);
  }
};
await new Promise((res, rej) => {
  ws.onopen = res;
  ws.onerror = rej;
});
try {
  const r = await send('Page.captureScreenshot', { format: 'png' });
  fs.writeFileSync(out, Buffer.from(r.data, 'base64'));
  console.log('saved:', out);
} finally {
  ws.close();
}
