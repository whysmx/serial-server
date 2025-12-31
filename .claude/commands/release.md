---
description: 打 tag 触发编译，等待完成后自动部署
---

你是一个自动化发布助手。请执行以下完整的发布流程：

1. **显示历史版本**：运行 `git tag --sort=-version:refname | head -10` 显示最近 10 个版本，让用户了解版本历史
2. **获取当前版本**：运行 `git describe --tags --abbrev=0` 获取最新版本号
3. **创建新版本号**：分析当前更改自动决定版本号递增类型：
   - 如果有新功能（major/minor 功能）：递增 minor 版本（v1.9.0 → v1.10.0）
   - 如果只是 bug 修复：递增 patch 版本（v1.9.0 → v1.9.1）
4. **生成更新日志**：从上一个 tag 到现在的 git commits 生成更新日志，格式：
   ```markdown
   ## [vX.X.X] - YYYY-MM-DD

   ### ✨ 新功能
   - 新功能1
   - 新功能2

   ### 🐛 Bug 修复
   - 修复1
   - 修复2

   ### 🔧 改进
   - 改进1
   ```
5. **更新 CHANGELOG.md**：
   - 在文件顶部插入新版本日志
   - 保留文件的其他内容（版本号说明、问题分类符号等）
6. **提交 CHANGELOG.md**：使用 `git add CHANGELOG.md && git commit -m "docs: 更新 CHANGELOG for vX.X.X"`
7. **打 tag 并推送**：创建新 tag 并推送到远程，触发 GitHub Actions 编译
8. **等待编译完成**：使用 `gh run watch` 监控编译状态，直到编译完成
   - 如果编译失败，立即停止并报告错误
   - 如果编译成功，继续下一步
9. **自动部署**：编译成功后，自动运行 `./scripts/deploy.sh <新版本号>` 部署到远程设备
10. **完成通知**：部署成功后，运行 `afplay /System/Library/Sounds/Purr.aiff` 播放提示音

**重要提醒：**
- 整个过程必须全自动完成，不需要用户干预
- 每个步骤都要显示进度信息
- 如果任何步骤失败，立即停止并报告错误
- 版本号格式必须严格遵守 v{{major}}.{{minor}}.{{patch}} 格式
- 更新日志必须遵循 CHANGELOG.md 的格式规范
- 使用适当的 emoji 符号：✨新功能 🐛Bug修复 🔧改进 📝文档 ♻️重构 ⚡性能 🔒安全

**更新日志生成指南：**
分析 git commit messages，按以下规则分类：
- 包含 "feat", "新功能", "添加" → ✨ 新功能
- 包含 "fix", "修复", "bug" → 🐛 Bug 修复
- 包含 "refactor", "重构" → ♻️ 重构
- 包含 "perf", "性能", "优化" → ⚡ 性能
- 包含 "doc", "文档" → 📝 文档
- 包含 "security", "安全" → 🔒 安全
- 其他 → 🔧 改进

现在开始执行发布流程。
