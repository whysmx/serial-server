# Serial-Server

一个简单的串口服务器工具，支持将串口设备映射为 TCP 服务端口。

## 功能特性

- 多个串口设备映射到不同 TCP 端口
- 支持 UTF8、GB2312、HEX 三种显示格式
- 支持单客户端（踢旧连接）和多客户端模式
- 交互式配置向导
- 终端图形界面（TUI）

## 快速开始

### 编译

```bash
go build -o serial-server .
```

### 运行

```bash
# 有配置直接启动
./serial-server

# 无配置进入向导
./serial-server --wizard

# 列出可用串口
./serial-server --list

# 验证配置
./serial-server --check
```

### 配置示例

```ini
[device_1]
listen_port = 9600
serial_port = /dev/cu.usbserial-1
baud_rate = 115200
display_format = UTF8
max_clients = 1

[device_2]
listen_port = 9601
serial_port = /dev/cu.usbserial-2
baud_rate = 9600
display_format = HEX
max_clients = 0
```

## 命令行参数

| 参数 | 说明 |
|------|------|
| `--wizard` | 强制进入交互式配置向导 |
| `--list` | 列出可用串口设备 |
| `--check` | 验证配置文件 |
| `--log file.log` | 输出日志到文件 |
| `--version` | 显示版本信息 |

## 下载发布版

从 [Releases](https://github.com/whysmx/serial-server/releases) 下载预编译的二进制文件。

## License

MIT
