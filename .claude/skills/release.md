# 自动化发布流程

完整执行从打 tag 到部署的全流程自动化。

## 执行步骤

### 1. 获取当前版本号
使用 `git describe --tags --abbrev=0` 获取当前最新的版本号。

### 2. 递增版本号
自动递增 patch 版本号，例如：
- v1.3.4 -> v1.3.5
- v2.0.9 -> v2.0.10

使用版本号计算逻辑（将版本号拆分后递增最后一位）。

### 3. 创建并推送 tag
```bash
git tag -a <NEW_VERSION> -m "Release <NEW_VERSION>"
git push origin <NEW_VERSION>
```

### 4. 监控 GitHub Actions 编译状态
使用 `gh run` 命令监控编译进度：

```bash
# 获取最新的 workflow run
gh run list --limit 1 --json databaseId,status,conclusion --jq '.[0]'

# 每 10 秒检查一次状态，直到 status 为 "completed"
# 如果 conclusion 不是 "success"，立即停止并报告错误
```

显示进度信息：
- 开始编译时：显示 "等待 GitHub Actions 编译..."
- 每 10 秒：显示已等待时间
- 编译完成：显示总耗时和结果

### 5. 部署到远程设备
编译成功后，执行部署脚本：
```bash
./scripts/deploy.sh <NEW_VERSION> --wait
```

部署脚本会：
- 下载 linux-arm64 平台的编译产物
- 通过 sshpass 上传到 10.10.10.83:28024
- 设置可执行权限

### 6. 完成通知
成功完成后播放提示音：
```bash
afplay /System/Library/Sounds/Purr.aiff
```

## 错误处理

如果任何步骤失败，必须：
1. 立即停止执行
2. 显示详细的错误信息
3. 指出失败的具体步骤和原因
4. 不要播放完成提示音

## 输出格式

每个步骤都要显示清晰的进度信息：

```
=== 自动化发布流程 ===

[步骤 1] 获取当前版本
  当前版本: v1.3.4

[步骤 2] 计算新版本号
  新版本: v1.3.5

[步骤 3] 创建并推送 tag
  已创建 tag v1.3.5
  已推送到远程

[步骤 4] 等待 GitHub Actions 编译
  编译中... (10s)
  编译中... (20s)
  编译完成 (总耗时: 45s)
  状态: success

[步骤 5] 部署到远程设备
  正在下载编译产物...
  正在上传到 10.10.10.83...
  部署完成

=== 发布完成 ===
```

## 重要提示

- 确保工作目录是干净的（没有未提交的更改）
- 确保已安装 `gh` 命令行工具并已登录
- 确保已安装 `sshpass` 用于部署
- 确保网络连接正常
