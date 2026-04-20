# AI Skills 目录

本目录包含 LibrarySquirrel 项目的 AI 技能定义，将原有的 Agent 转换为 Skill 格式。

## Skill 列表

| Skill                                                       | 名称           | 描述                         |
| ----------------------------------------------------------- | -------------- | ---------------------------- |
| [workflow-coordinator-skill](./workflow-coordinator-skill/) | 工作流协调员   | 统筹协调所有技能的工作流程   |
| [requirements-analyst-skill](./requirements-analyst-skill/) | 需求分析师     | 与用户交流需求，生成开发计划 |
| [code-writer-skill](./code-writer-skill/)                   | 代码工程师     | 按照规范编写高质量代码       |
| [test-engineer-skill](./test-engineer-skill/)               | 测试工程师     | 验证功能，编写和执行测试     |
| [doc-maintainer-skill](./doc-maintainer-skill/)             | 文档维护工程师 | 维护开发辅助文档             |

## Skill 结构

每个 Skill 包含以下内容：

```
skill-name/
├── SKILL.md          # 【核心】技能的元数据、触发条件、核心指令
├── rules/            # 【规则库】详细的代码规范、命名约定
├── examples/         # 【少样本学习】优秀的代码范例
├── scripts/          # 【执行逻辑】可选的辅助脚本
└── tools.json        # 【工具依赖】声明需要调用的工具
```

## 使用方式

直接与 **workflow-coordinator** 开始交互，它会自动协调其他技能完成工作。

## 与原 Agent 的对应关系

| 原 Agent             | 转换为 Skill               |
| -------------------- | -------------------------- |
| workflow-coordinator | workflow-coordinator-skill |
| requirements-analyst | requirements-analyst-skill |
| code-writer          | code-writer-skill          |
| test-engineer        | test-engineer-skill        |
| doc-maintainer       | doc-maintainer-skill       |

---

**最后更新**: 2026-03-15
