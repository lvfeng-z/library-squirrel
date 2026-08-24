#!/usr/bin/env node
// CDP 求值桥：在应用真实前端页面上下文中求值 JS 表达式（实机测试的驱动通道）。
// 用法：node cdp-eval.mjs '<js表达式>'   （表达式可为 async/awaitPromise；CDP_PORT 默认 9222）
// 输出：表达式的值（returnByValue）JSON；页面异常时输出异常详情并以 1 退出
const port = process.env.CDP_PORT || '9222';
const expr = process.argv[2];
if (!expr) {
  console.error('用法: cdp-eval.mjs <js表达式>');
  process.exit(2);
}
const list = await (await fetch(`http://127.0.0.1:${port}/json`)).json();
const page = list.find((t) => t.type === 'page');
if (!page) {
  console.error('无 page target（应用未运行或 CDP 未开启）');
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
  const r = await send('Runtime.evaluate', {
    expression: expr,
    awaitPromise: true,
    returnByValue: true,
  });
  if (r.exceptionDetails) {
    console.error('页面异常:', JSON.stringify(r.exceptionDetails, null, 2));
    process.exit(1);
  }
  const v = r.result?.value;
  console.log(typeof v === 'string' ? v : JSON.stringify(v, null, 2));
} finally {
  ws.close();
}
