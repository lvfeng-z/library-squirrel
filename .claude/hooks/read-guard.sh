#!/usr/bin/env bash
# PreToolUse 钩子：大文件整 Read 拦截。
# 目标文件超过阈值（>500 行 或 >30KB）且未带 limit（未按片段读取）时，以退出码 2 + stderr 提示阻断，
# 迫使模型先 Grep 定位、再按片段读取；通过时静默退出。

input=$(cat)

# 从 stdin JSON 提取 file_path / limit / offset（本机无 jq，用 node 解析）
mapfile -t fields < <(printf '%s' "$input" | node -e "let d='';process.stdin.on('data',c=>d+=c);process.stdin.on('end',()=>{try{const i=JSON.parse(d).tool_input||{};console.log(i.file_path||'');console.log(i.limit||'');console.log(i.offset||'')}catch(e){console.log('');console.log('');console.log('')}})")
file="${fields[0]}"; limit="${fields[1]}"; offset="${fields[2]}"

# 无路径，或已带 limit（片段读取），直接放行
[ -n "$file" ] || exit 0
[ -n "$limit" ] && exit 0

# Windows 反斜杠路径统一为正斜杠
file=${file//\\//}

# 相对路径基于项目目录解析
case "$file" in
  /* | [A-Za-z]:/*) ;;
  *) file="${CLAUDE_PROJECT_DIR:-$PWD}/$file" ;;
esac

[ -f "$file" ] || exit 0

lines=$(wc -l < "$file" 2>/dev/null || echo 0)
bytes=$(stat -c %s "$file" 2>/dev/null || echo 0)

if [ "${lines:-0}" -gt 500 ] || [ "${bytes:-0}" -gt 30720 ]; then
  printf '大文件（%s 行 / %s KB）禁止整 Read：先 Grep 定位目标行，再用 Read 带 offset/limit 读片段；确需全文件时分多次 offset/limit 读取。优先读模块 README.md 定位。\n' "$lines" "$(( (bytes + 1023) / 1024 ))" >&2
  exit 2
fi
exit 0
