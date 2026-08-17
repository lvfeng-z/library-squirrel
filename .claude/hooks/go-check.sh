#!/usr/bin/env bash
# PostToolUse 钩子：Go 源文件被编辑后，在其所属仓库内即时运行格式与编译检查。
# 发现问题时以退出码 2 + stderr 回传（配合 asyncRewake 后台执行，失败时唤醒模型立即修正）；通过时静默退出。

input=$(cat)

# 从 stdin JSON 提取被编辑文件路径（本机无 jq，用 node 解析）
file=$(printf '%s' "$input" | node -e "let d='';process.stdin.on('data',c=>d+=c);process.stdin.on('end',()=>{try{console.log(JSON.parse(d).tool_input.file_path||'')}catch(e){console.log('')}})")

# 仅检查 Go 源文件，其余编辑直接放行
case "$file" in
  *.go) ;;
  *) exit 0 ;;
esac

# Windows 反斜杠路径统一为正斜杠，供 dirname/git 使用
file=${file//\\//}

# 多仓库工作区：定位被编辑文件所属的 git 仓库根（主程序/SDK/插件各自独立 Go 模块）
repo=$(cd "$(dirname "$file")" 2>/dev/null && git rev-parse --show-toplevel 2>/dev/null)
if [ -z "$repo" ] || [ ! -f "$repo/go.mod" ]; then
  exit 0
fi
cd "$repo" || exit 0

errors=""

# 格式检查：仅查本次编辑的文件（仓库历史遗留的未格式化文件不在本钩子职责内）
unformatted=$(gofmt -l "$file")
if [ -n "$unformatted" ]; then
  errors="${errors}gofmt 未格式化：${unformatted}
"
fi

# 编译检查：CGO_ENABLED=0 绕开 Windows 下 CGO 工具链兼容问题。
# 构建目标排除 build/ 子树——Wails 各平台脚手架目录（ios/darwin 等）是按目标平台打
# build tag 的 package main，宿主机全量构建时缺少本平台入口必然报错，不属业务代码缺陷。
pkgs=$(go list ./... 2>/dev/null | grep -v '/build/')
[ -n "$pkgs" ] || pkgs="./..."
out=$(CGO_ENABLED=0 go build $pkgs 2>&1)
if [ $? -ne 0 ]; then
  errors="${errors}${out}
"
fi

if [ -n "$errors" ]; then
  printf '%s\n' "$errors" >&2
  exit 2
fi
exit 0
