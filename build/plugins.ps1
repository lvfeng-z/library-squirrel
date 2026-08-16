# 一键构建全部插件并把安装包复制到主仓库 resources/bundled-plugins/
# 复用各插件仓库自带的 build.ps1（以各自仓库根为工作目录运行）。用法：task build:plugins
# 插件仓库默认位于主仓库同级目录；不同开发环境位置不同时，用本地配置 build/plugins.local.json 覆盖
# （不入库，格式见 build/plugins.local.example.json）：reposRoot 改全部仓库的共同父目录，
# plugins 按仓库名逐个覆盖（优先级更高）；两者均可缺省/留空，未覆盖的插件回落同级目录默认
$ErrorActionPreference = "Stop"
# 重定向/管道场景下 PS 5.1 默认按 OEM 代码页输出中文会乱码，强制 UTF-8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$mainRepo = Split-Path $PSScriptRoot -Parent
$siblingRoot = Split-Path $mainRepo -Parent
$targetDir = Join-Path $mainRepo 'resources\bundled-plugins'

# 加载本地配置（文件不存在则全部走同级目录默认）
$configPath = Join-Path $PSScriptRoot 'plugins.local.json'
$config = $null
if (Test-Path $configPath) {
    try {
        $config = Get-Content $configPath -Raw -Encoding UTF8 | ConvertFrom-Json
    } catch {
        throw "plugins.local.json 解析失败：$($_.Exception.Message)"
    }
    Write-Host "已加载本地插件配置：$configPath" -ForegroundColor DarkCyan
}

# 仓库定位优先级：plugins.{仓库名} > reposRoot/{仓库名} > 主仓库同级目录/{仓库名}；
# 配置值为权威路径（不存在的仓库按缺失跳过，不静默回落低优先级位置，避免误用旧克隆）；相对路径以主仓库根为基准
function Resolve-RepoDir([string]$repoName) {
    $dir = $null
    if ($config -and $config.plugins) {
        $prop = $config.plugins.PSObject.Properties[$repoName]
        if ($prop -and $prop.Value) { $dir = $prop.Value }
    }
    if (-not $dir -and $config -and $config.reposRoot) { $dir = Join-Path $config.reposRoot $repoName }
    if (-not $dir) { $dir = Join-Path $siblingRoot $repoName }
    if (-not [System.IO.Path]::IsPathRooted($dir)) { $dir = Join-Path $mainRepo $dir }
    return $dir
}

# 插件清单：Repo = 同级仓库目录名；Zip = 安装包文件名；
# Direct = $true 表示该插件的 build.ps1 直接打包到主仓库目标目录（无 dist 中转），构建后仅校验产物存在
$plugins = @(
    @{ Repo = 'library-squirrel-plugin-local';    Zip = 'local-plugin.zip' }
    @{ Repo = 'library-squirrel-plugin-pixiv';    Zip = 'pixiv-plugin.zip' }
    @{ Repo = 'library-squirrel-plugin-bilibili'; Zip = 'bilibili-plugin.zip' }
    @{ Repo = 'library-squirrel-plugin-test';     Zip = 'test-plugin.zip'; Direct = $true }
)

New-Item -ItemType Directory -Path $targetDir -Force | Out-Null

# 校验产物 zip 内 plugin.json 带非空 buildId（构建身份标识，主程序升级判据依赖）且无 BOM（Go json 拒绝 BOM）。
# 集中兜底各仓库打标遗漏；校验失败按该插件构建失败处理
Add-Type -AssemblyName System.IO.Compression.FileSystem
function Assert-BuildId([string]$zipPath, [string]$repoName) {
    $zip = [System.IO.Compression.ZipFile]::OpenRead($zipPath)
    try {
        $entry = $zip.GetEntry('plugin.json')
        if ($null -eq $entry) { throw "zip 内缺少 plugin.json" }
        $reader = New-Object System.IO.StreamReader($entry.Open())
        try { $text = $reader.ReadToEnd() } finally { $reader.Dispose() }
        if ($text.Length -gt 0 -and $text[0] -eq [char]0xFEFF) { throw "plugin.json 带 BOM" }
        $manifest = $text | ConvertFrom-Json
        if ([string]::IsNullOrWhiteSpace($manifest.buildId)) { throw "plugin.json 缺少 buildId" }
        Write-Host "  [校验通过] $repoName buildId=$($manifest.buildId)" -ForegroundColor DarkGreen
    } finally {
        $zip.Dispose()
    }
}

$failedPlugins = @()
$skippedPlugins = @()

foreach ($plugin in $plugins) {
    $repoName = $plugin.Repo
    $zipName = $plugin.Zip
    $repoDir = Resolve-RepoDir $repoName
    $buildScript = Join-Path $repoDir 'build.ps1'

    if (-not (Test-Path $buildScript)) {
        Write-Host "[跳过] $repoName ：未找到 $buildScript" -ForegroundColor Yellow
        $skippedPlugins += $repoName
        continue
    }

    Write-Host ""
    Write-Host "===== $repoName =====" -ForegroundColor Cyan

    # 子进程运行各仓库 build.ps1：退出码判定清晰，且各脚本内部的变量/错误偏好设置不泄漏到本脚本
    Push-Location $repoDir
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $buildScript
        if ($LASTEXITCODE -ne 0) { throw "build.ps1 退出码 $LASTEXITCODE" }
    } catch {
        Write-Host "[失败] $repoName ：$($_.Exception.Message)" -ForegroundColor Red
        $failedPlugins += $repoName
        continue
    } finally {
        Pop-Location
    }

    if ($plugin.Direct) {
        $directZip = Join-Path $targetDir $zipName
        if (-not (Test-Path $directZip)) {
            Write-Host "[失败] $repoName ：目标目录未生成 $zipName" -ForegroundColor Red
            $failedPlugins += $repoName
            continue
        }
        try {
            Assert-BuildId $directZip $repoName
            Write-Host "[完成] $zipName 已直接打包到目标目录" -ForegroundColor Green
        } catch {
            Write-Host "[失败] $repoName ：产物校验未通过（$($_.Exception.Message)）" -ForegroundColor Red
            $failedPlugins += $repoName
        }
        continue
    }

    $zipPath = Join-Path $repoDir "dist\$zipName"
    if (-not (Test-Path $zipPath)) {
        Write-Host "[失败] $repoName ：未找到构建产物 $zipPath" -ForegroundColor Red
        $failedPlugins += $repoName
        continue
    }
    try {
        Assert-BuildId $zipPath $repoName
    } catch {
        Write-Host "[失败] $repoName ：产物校验未通过（$($_.Exception.Message)）" -ForegroundColor Red
        $failedPlugins += $repoName
        continue
    }
    Copy-Item $zipPath (Join-Path $targetDir $zipName) -Force
    Write-Host "[完成] $zipName -> resources\bundled-plugins\" -ForegroundColor Green
}

Write-Host ""
if ($failedPlugins.Count -gt 0) {
    Write-Host "构建失败：$($failedPlugins -join ', ')" -ForegroundColor Red
    exit 1
}
# 全部跳过通常意味着本地配置的仓库路径全都不对，按失败处理以免误以为构建成功
if ($skippedPlugins.Count -eq $plugins.Count) {
    Write-Host "未构建任何插件（全部跳过）：请检查 build/plugins.local.json 中配置的仓库路径" -ForegroundColor Red
    exit 1
}
Write-Host "全部插件构建完成，安装包已就位于 resources\bundled-plugins\" -ForegroundColor Green
