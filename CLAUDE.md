# Claude Code 配置

## 任务完成通知
任务完成后播放提示音：
```bash
afplay /System/Library/Sounds/Purr.aiff
```

## 部署 Skill
推送代码后，打 tag 触发编译，然后运行部署脚本：
```bash
# 手动运行
./scripts/deploy.sh

# 或通过 Claude Code skill
# 直接说 "部署" 或 "deploy"
```
