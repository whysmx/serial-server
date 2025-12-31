# Serial-Server Claude Code 配置

## Slash Commands

### `/deploy <version>` - 部署到远程设备
下载指定版本的编译产物并部署到 10.10.10.83。

```
/deploy v1.2.1
```

### `/release` - 打 tag 触发编译
创建新版本号并打 tag 推送到远程，触发 GitHub Actions 自动编译。

```
/release
```

## 任务完成通知
任务完成后播放提示音：
```bash
afplay /System/Library/Sounds/Purr.aiff
```

## 工作流程
1. 修改代码
2. 执行 `/release` 打 tag
3. 等待 GitHub Actions 编译完成
4. 执行 `/deploy v1.x.x` 部署到设备
