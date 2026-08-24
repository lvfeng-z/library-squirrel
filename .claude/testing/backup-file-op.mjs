#!/usr/bin/env node
// 长路径备份文件操作助手（Windows MAX_PATH 规避）：在目录内按 ASCII 通配模式唯一定位文件，
// 两种操作：删除（先复制到临时区保管再删）/ 同目录改名（新名传入后保留原文件）。
// 用法：node backup-file-op.mjs <dir> <asciiNamePattern> [newBaseName]
//   newBaseName 省略 = 删除模式；传入 = 改名模式（保持扩展名）
const [dir, pattern, newBaseName] = process.argv.slice(2);
if (!dir || !pattern) {
  console.error('用法: backup-file-op.mjs <dir> <asciiNamePattern> [newBaseName]');
  process.exit(2);
}
const fs = await import('node:fs');
const path = await import('node:path');
const os = await import('node:os');
// readdir 枚举不受长路径影响（目录本身短）；返回真实完整文件名
const entries = fs.readdirSync(dir);
const hits = entries.filter((n) => {
  const p = pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*').replace(/\?/g, '.');
  return new RegExp(`^${p}$`).test(n);
});
if (hits.length !== 1) {
  console.error(`模式命中 ${hits.length} 个文件（须唯一）: ${hits.join(' | ')}`);
  process.exit(3);
}
const abs = path.join(dir, hits[0]);
const longPath = '\\\\?\\' + abs;
if (newBaseName) {
  const newName = newBaseName + path.extname(hits[0]);
  const newLong = '\\\\?\\' + path.join(dir, newName);
  fs.renameSync(longPath, newLong);
  console.log(JSON.stringify({ renamedFrom: abs, renamedTo: path.join(dir, newName) }));
} else {
  const keep = path.join(os.tmpdir(), 'h-keep-' + Date.now() + path.extname(hits[0]));
  fs.copyFileSync(longPath, keep);
  fs.rmSync(longPath);
  console.log(JSON.stringify({ deleted: abs, keepCopy: keep }));
}
