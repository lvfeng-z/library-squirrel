# 插件开发指南（已迁移）

> ⚠️ 本文档已过时，**不再维护**。其内容基于旧的 DLL/plugin.Open 架构，与当前「hashicorp/go-plugin 子进程 + 统一 KV API」架构不符，请勿依据此处描述开发。

## 权威文档

插件开发的权威指南已迁移至：

👉 **[doc/plugin-dev-guide.md](../plugin-dev-guide.md)**

新文档与 `library-squirrel-sdk` 的 `dto.PluginContext`、`.claude/rules/plugin.md`、pixiv/local-import 示例严格对齐，涵盖：

- plugin.json 完整 schema（含 settings）
- 插件入口 `plugin.Serve` + Activate 标准签名
- PluginContext 全部 21 个 API
- 扩展点（TaskHandler/SiteBrowser/Slot）
- 插件自存信息（统一 KV）
- 前端通信、原生窗口、静态资源、打包发布

请前往新文档查阅。
