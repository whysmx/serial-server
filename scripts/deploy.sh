#!/bin/bash
# 部署脚本：下载指定版本并发送到远程设备

set -e

REPO="whysmx/serial-server"
HOST="forlinx@10.10.10.83"
PORT="28024"
PASSWORD="forlinx"
REMOTE_DIR="/home/forlinx/Downloads"
VERSION="$1"
TARGET_OS="linux"
TARGET_ARCH="arm64"  # 固定目标平台
MAX_WAIT=300
CHECK_INTERVAL=10

if [ -z "$VERSION" ]; then
    echo "用法: $0 <版本号> [--wait]"
    echo "  例如: $0 v1.2.0"
    echo "  等待编译: $0 v1.2.0 --wait"
    exit 1
fi

wait_mode=false
if [ "$2" = "--wait" ]; then
    wait_mode=true
fi

echo "版本: $VERSION"
echo "目标平台: $TARGET_OS-$TARGET_ARCH"

FILENAME="serial-server-$VERSION-$TARGET_OS-$TARGET_ARCH"
TEMP_DIR=$(mktemp -d)

# 检查文件是否存在
if ! gh release view "$VERSION" --repo "$REPO" --json tagName --jq '.tagName' >/dev/null 2>&1; then
    echo "版本 $VERSION 不存在"
    rm -rf "$TEMP_DIR"
    exit 1
fi

# 尝试下载，带等待模式
download_with_wait() {
    local elapsed=0
    while [ $elapsed -lt $MAX_WAIT ]; do
        if gh release download "$VERSION" --repo "$REPO" -p "*$TARGET_OS*$TARGET_ARCH*" -D "$TEMP_DIR" --clobber 2>/dev/null; then
            return 0
        fi
        if [ "$wait_mode" = "false" ]; then
            return 1
        fi
        echo "等待编译... ($elapsed/$MAX_WAIT 秒)"
        sleep $CHECK_INTERVAL
        elapsed=$((elapsed + CHECK_INTERVAL))
    done
    return 1
}

echo "下载: $FILENAME"
if ! download_with_wait; then
    echo "未找到对应平台的编译文件或等待超时"
    rm -rf "$TEMP_DIR"
    exit 1
fi

# 清理远程目录中的所有旧版本
echo "清理远程目录旧版本..."
sshpass -p "$PASSWORD" ssh -p $PORT "$HOST" "
    cd $REMOTE_DIR
    # 删除所有旧版本文件
    rm -f serial-server-v*-linux-arm64 2>/dev/null && echo '已清理所有旧版本' || echo '没有旧版本需要清理'
"

# 发送到远程
echo "发送到远程: $HOST:$REMOTE_DIR"
sshpass -p "$PASSWORD" scp -P $PORT "$TEMP_DIR/$FILENAME" "$HOST:$REMOTE_DIR/"

# 给可执行权限
sshpass -p "$PASSWORD" ssh -p $PORT "$HOST" "chmod +x $REMOTE_DIR/$FILENAME && ls -la $REMOTE_DIR/$FILENAME"

# 清理
rm -rf "$TEMP_DIR"

echo "部署完成!"
