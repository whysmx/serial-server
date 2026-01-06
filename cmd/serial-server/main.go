package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/whysmx/serial-server/config"
	"github.com/whysmx/serial-server/frp"
	"github.com/whysmx/serial-server/listener"
	"github.com/whysmx/serial-server/wizard"
)

const (
	defaultConfigFile = "config.ini"
	version           = "1.2.15"

	// 经典绿风格 - 颜色定义
	colorGreen = "\x1b[32m" // 绿色
	colorRed   = "\x1b[31m" // 红色
	colorReset = "\x1b[0m"  // 重置

	// 经典绿风格 - 状态文字
	emojiYes = "[√]" // 已添加/已配置
	emojiNo  = "[×]" // 未添加/未配置
)

// 运行时命令类型
type runtimeCommand string

const (
	cmdReload   runtimeCommand = "reload"   // 重新加载配置
	cmdAdd      runtimeCommand = "add"      // 添加配置
	cmdModify   runtimeCommand = "modify"   // 修改配置
	cmdDelete   runtimeCommand = "delete"   // 删除配置
	cmdList     runtimeCommand = "list"     // 列出配置
	cmdStatus   runtimeCommand = "status"   // 显示状态
	cmdHelp     runtimeCommand = "help"     // 显示帮助
	cmdFRPMenu  runtimeCommand = "frp"      // FRP 管理
)

// 运行时命令请求
type runtimeRequest struct {
	command runtimeCommand
	data    interface{}
	result  chan error
}

// 运行时管理器
type runtimeManager struct {
	configPath  string
	cfg         *config.Config
	listeners   []*listener.Listener
	requestChan chan runtimeRequest
	stopChan    chan struct{}
}

var (
	configFile  string
	listPorts   bool
	checkConfig bool
	wizardMode  bool
	showConfig  bool
	logFile     string
	logLevel    string
	showVersion bool
	cfg         *config.Config

	// 运行时颜色控制
	useColor bool
)

func init() {
	flag.StringVar(&configFile, "c", defaultConfigFile, "配置文件路径")
	flag.StringVar(&configFile, "config", defaultConfigFile, "配置文件路径")
	flag.BoolVar(&listPorts, "l", false, "列出可用串口设备")
	flag.BoolVar(&listPorts, "list", false, "列出可用串口设备")
	flag.BoolVar(&checkConfig, "check", false, "验证配置文件")
	flag.BoolVar(&wizardMode, "wizard", false, "进入交互式配置向导")
	flag.BoolVar(&showConfig, "show-config", false, "显示配置信息")
	flag.StringVar(&logFile, "log", "", "日志文件路径（默认 serial-server.log）")
	flag.StringVar(&logLevel, "level", "info", "日志级别: debug, info, warn, error")
	flag.BoolVar(&showVersion, "version", false, "显示版本信息")
	flag.BoolVar(&showVersion, "v", false, "显示版本信息")

	// 检测是否应该使用颜色
	// Windows CMD/PowerShell 默认不支持 ANSI 颜色，除非启用 VirtualTerminal
	// 检测环境变量来判断
	useColor = shouldUseColor()
}

// shouldUseColor 检测终端是否支持颜色
func shouldUseColor() bool {
	// 检查 NO_COLOR 环境变量
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// 检查 TERM 环境变量
	term := os.Getenv("TERM")
	if term == "" || term == "dumb" {
		return false
	}

	// Windows 特殊处理
	if runtime.GOOS == "windows" {
		// Windows 10+ 的 CMD/PowerShell 支持 ANSI，但需要检测
		// 如果设置了 TERM=xterm 或其他支持的终端类型，则启用
		if term == "xterm" || term == "xterm-256color" || term == "cygwin" {
			return true
		}
		// 检查是否在支持 ANSI 的终端中运行（如 Windows Terminal, ConEmu 等）
		if os.Getenv("WT_SESSION") != "" || // Windows Terminal
			os.Getenv("ConEmuPID") != "" { // ConEmu
			return true
		}
		// 默认 Windows CMD/PowerShell 不使用颜色
		return false
	}

	// Unix-like 系统通常支持颜色
	return true
}

// setupAutoStart 设置开机自启
func setupAutoStart() {
	fmt.Fprintln(os.Stderr, "正在设置开机自启...")

	// 获取当前可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取可执行文件路径失败: %v\n", err)
		os.Exit(1)
	}

	// 获取配置文件路径
	configPath := findConfigFile(configFile)
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取配置文件绝对路径失败: %v\n", err)
		os.Exit(1)
	}

	// systemd service 文件内容
	serviceContent := fmt.Sprintf(`[Unit]
Description=Serial Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=%s
ExecStart=%s -c %s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`, filepath.Dir(execPath), execPath, absConfigPath)

	// systemd 服务文件路径
	servicePath := "/etc/systemd/system/serial-server.service"

	// 检查是否已存在
	if _, err := os.Stat(servicePath); err == nil {
		fmt.Fprintln(os.Stderr, "检测到已存在开机自启配置")
		fmt.Fprintf(os.Stderr, "  服务文件: %s\n", servicePath)
		fmt.Fprint(os.Stderr, "是否覆盖? [y/N]: ")

		var confirm string
		fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "y" {
			fmt.Fprintln(os.Stderr, "取消设置")
			os.Exit(0)
		}
	}

	// 写入服务文件
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写入服务文件失败: %v\n", err)
		fmt.Fprintf(os.Stderr, "请使用 sudo 权限运行此程序\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "✓ 服务文件已创建: %s\n", servicePath)

	// 重新加载 systemd
	fmt.Fprintln(os.Stderr, "正在重新加载 systemd...")
	reloadCmd := exec.Command("systemctl", "daemon-reload")
	if err := reloadCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 重新加载 systemd 失败: %v\n", err)
	}

	// 启用服务
	fmt.Fprintln(os.Stderr, "正在启用开机自启...")
	enableCmd := exec.Command("systemctl", "enable", "serial-server")
	if err := enableCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "启用开机自启失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "✓ 开机自启已设置")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "管理命令:")
	fmt.Fprintln(os.Stderr, "  启动服务: systemctl start serial-server")
	fmt.Fprintln(os.Stderr, "  停止服务: systemctl stop serial-server")
	fmt.Fprintln(os.Stderr, "  重启服务: systemctl restart serial-server")
	fmt.Fprintln(os.Stderr, "  查看状态: systemctl status serial-server")
	fmt.Fprintln(os.Stderr, "  查看日志: journalctl -u serial-server -f")
	os.Exit(0)
}

func main() {
	flag.Parse()

	var err error

	if showVersion {
		fmt.Printf("Serial-Server v%s\n", version)
		return
	}

	// 默认日志文件
	if logFile == "" {
		logFile = "serial-server.log"
	}

	setupLogging()

	if listPorts {
		listSerialPorts()
		return
	}

	if checkConfig {
		if err := checkConfiguration(); err != nil {
			fmt.Printf("配置错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("配置检查通过")
		return
	}

	configPath := findConfigFile(configFile)

	cfg, err = loadOrCreateConfig(configPath)
	if err != nil {
		// 检查是否是没有串口的情况
		if strings.Contains(err.Error(), "no serial ports found") || strings.Contains(err.Error(), "没有可用的串口") {
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════")
			fmt.Fprintln(os.Stderr, "  ⚠️  未检测到串口设备")
			fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "可能的原因:")
			fmt.Fprintln(os.Stderr, "  1. 串口设备未连接或未通电")
			fmt.Fprintln(os.Stderr, "  2. 串口驱动未安装")
			fmt.Fprintln(os.Stderr, "  3. 串口被其他程序占用")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "建议操作:")
			fmt.Fprintln(os.Stderr, "  • 检查串口设备连接并通电后重新运行程序")
			fmt.Fprintln(os.Stderr, "  • 使用 --list 参数查看可用串口")
			fmt.Fprintln(os.Stderr, "  • 使用 --wizard 参数手动配置串口参数")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "按回车键退出...")
			_, _ = fmt.Scanln(new(string))
			os.Exit(1)
		}
		log.Fatalf("加载配置失败: %v", err)
	}

	if showConfig {
		printConfigSummary(cfg)
		return
	}

	if cfg == nil || len(cfg.Listeners) == 0 {
		// 无配置时直接进入添加流程
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "未检测到配置，进入添加配置流程...")
		fmt.Fprintln(os.Stderr, "")
		wiz := wizard.NewWizard()
		newCfg, err := wiz.RunAddOnly(cfg)
		if err != nil {
			// 检查是否是没有串口的情况
			if strings.Contains(err.Error(), "no serial ports found") || strings.Contains(err.Error(), "没有可用的串口") {
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════")
				fmt.Fprintln(os.Stderr, "  ⚠️  未检测到串口设备")
				fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════")
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "可能的原因:")
				fmt.Fprintln(os.Stderr, "  1. 串口设备未连接或未通电")
				fmt.Fprintln(os.Stderr, "  2. 串口驱动未安装")
				fmt.Fprintln(os.Stderr, "  3. 串口被其他程序占用")
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "建议操作:")
				fmt.Fprintln(os.Stderr, "  • 检查串口设备连接并通电后重新运行程序")
				fmt.Fprintln(os.Stderr, "  • 使用 --list 参数查看可用串口")
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "按回车键退出...")
				_, _ = fmt.Scanln(new(string))
				os.Exit(1)
			}
			log.Fatalf("配置向导失败: %v", err)
		}
		cfg = newCfg
		if err := config.Save(configPath, cfg); err != nil {
			log.Fatalf("保存配置失败: %v", err)
		}
	}

	// 如果不是特殊模式，显示启动菜单
showMenu:
	if !listPorts && !checkConfig && !wizardMode {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s              Serial-Server v%s%s\n", getGreen(), version, getReset())
		fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", getGreen(), getReset())
		fmt.Fprintln(os.Stderr, "")
		printConfigSummaryToStderr(cfg)
		fmt.Fprintf(os.Stderr, "%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s请选择操作:%s\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  1 %s- 直接启动\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  2 %s- 后台运行\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  3 %s- 开机自启\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  4 %s- 添加配置\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  5 %s- 修改配置\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  6 %s- 删除配置\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  7 %s- FRP 管理\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  0 %s- 退出\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "\n%s请输入选项 [1-7/0]: %s", getGreen(), getReset())

		var choice string
		_, _ = fmt.Scanln(&choice)
		choice = strings.ToLower(strings.TrimSpace(choice))

		fmt.Fprintln(os.Stderr, "")

		switch choice {
		case "1":
			// 直接启动，继续执行
		case "2":
			// 后台运行
			startInBackground()
		case "3":
			// 开机自启
			setupAutoStart()
		case "4":
			// 添加新配置（直接进入添加模式，不询问是否添加）
			wiz := wizard.NewWizard()
			newCfg, err := wiz.RunAddOnly(cfg)
			if err != nil {
				// 检查是否是没有串口的情况
				if strings.Contains(err.Error(), "no serial ports found") || strings.Contains(err.Error(), "没有可用的串口") {
					fmt.Fprintln(os.Stderr, "")
					fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════")
					fmt.Fprintln(os.Stderr, "  ⚠️  未检测到串口设备")
					fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════════════════")
					fmt.Fprintln(os.Stderr, "")
					fmt.Fprintln(os.Stderr, "可能的原因:")
					fmt.Fprintln(os.Stderr, "  1. 串口设备未连接或未通电")
					fmt.Fprintln(os.Stderr, "  2. 串口驱动未安装")
					fmt.Fprintln(os.Stderr, "  3. 串口被其他程序占用")
					fmt.Fprintln(os.Stderr, "")
					fmt.Fprintln(os.Stderr, "建议操作:")
					fmt.Fprintln(os.Stderr, "  • 检查串口设备连接并通电后重新运行程序")
					fmt.Fprintln(os.Stderr, "  • 使用 --list 参数查看可用串口")
					fmt.Fprintln(os.Stderr, "")
					fmt.Fprintln(os.Stderr, "按回车键返回主菜单...")
					_, _ = fmt.Scanln(new(string))
					// 返回主菜单重新显示
					goto showMenu
				}
				fmt.Fprintf(os.Stderr, "配置向导失败: %v\n", err)
				os.Exit(1)
			}
			cfg = newCfg
			if err := config.Save(configPath, cfg); err != nil {
				log.Printf("保存配置失败: %v", err)
			}
			fmt.Fprintln(os.Stderr, "配置已保存，重新启动程序...")
			fmt.Fprintln(os.Stderr, "")
			// 重新加载配置并继续
			cfg, err = config.Load(configPath)
			if err != nil {
				log.Fatalf("重新加载配置失败: %v", err)
			}
		case "5":
			// 修改配置
			if len(cfg.Listeners) == 0 {
				fmt.Fprintln(os.Stderr, "没有可修改的配置")
				os.Exit(1)
			}
			if err := modifyConfigInteractively(cfg, configPath); err != nil {
				fmt.Fprintf(os.Stderr, "修改配置失败: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintln(os.Stderr, "配置已保存，重新启动程序...")
			fmt.Fprintln(os.Stderr, "")
			// 重新加载配置并继续
			cfg, err = config.Load(configPath)
			if err != nil {
				log.Fatalf("重新加载配置失败: %v", err)
			}
		case "6":
			// 删除配置
			if len(cfg.Listeners) == 0 {
				fmt.Fprintln(os.Stderr, "没有可删除的配置")
				os.Exit(1)
			}
			if err := deleteConfigInteractively(cfg, configPath); err != nil {
				fmt.Fprintf(os.Stderr, "删除配置失败: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintln(os.Stderr, "配置已删除，重新启动程序...")
			fmt.Fprintln(os.Stderr, "")
			// 重新加载配置并继续
			cfg, err = config.Load(configPath)
			if err != nil {
				log.Fatalf("重新加载配置失败: %v", err)
			}
		case "7":
			// FRP 管理
			runFRPMenu()
			goto showMenu
		case "0":
			fmt.Fprintln(os.Stderr, "退出")
			os.Exit(0)
		default:
			fmt.Fprintln(os.Stderr, "无效选项")
			os.Exit(1)
		}
	}

	// 启动应用，如果失败则允许用户修改配置后重试
	configPath = findConfigFile(configFile)
	for {
		if err := runApp(cfg); err != nil {
			fmt.Fprintln(os.Stderr, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Fprintln(os.Stderr, "❌ 启动失败")
			fmt.Fprintf(os.Stderr, "错误: %v\n\n", err)
			fmt.Fprintf(os.Stderr, "💡 提示:\n")
			fmt.Fprintf(os.Stderr, "  1. 串口被占用? 先关闭占用串口的程序\n")
			fmt.Fprintf(os.Stderr, "  2. 串口名称错误? 修改配置文件: %s\n", configPath)
			fmt.Fprintf(os.Stderr, "  3. 查看可用串口: ./serial-server --list\n\n")
			fmt.Fprintln(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Fprintln(os.Stderr, "请选择操作:")
			fmt.Fprintln(os.Stderr, "  1 - 交互式修改配置")
			fmt.Fprintln(os.Stderr, "  2 - 编辑配置文件")
			fmt.Fprintln(os.Stderr, "  0 - 重新加载配置")
			fmt.Fprint(os.Stderr, "选择 [1/2/0]: ")

			// 等待用户输入
			var input string
			_, _ = fmt.Scanln(&input)
			choice := strings.ToLower(strings.TrimSpace(input))

			//nolint:gocritic // Complex menu logic - if-else chain is appropriate
			if choice == "1" || choice == "m" {
				// 交互式修改配置
				fmt.Fprintln(os.Stderr)
				if err := modifyConfigInteractively(cfg, configPath); err != nil {
					fmt.Fprintf(os.Stderr, "⚠️  修改配置失败: %v\n", err)
				} else {
					fmt.Fprintln(os.Stderr, "✓ 配置已保存")
				}
			} else if choice == "2" || choice == "e" {
				// 编辑配置文件
				editor := os.Getenv("EDITOR")
				if editor == "" {
					if _, err := exec.LookPath("nano"); err == nil {
						editor = "nano"
					} else if _, err := exec.LookPath("vi"); err == nil {
						editor = "vi"
					} else {
						fmt.Fprintln(os.Stderr, "\n⚠️  未找到可用的编辑器 (nano/vi)")
						fmt.Fprintln(os.Stderr, "请手动编辑配置文件后按回车继续...")
						_, _ = fmt.Scanln(&input)
					}
				}

				if editor != "" {
					fmt.Fprintf(os.Stderr, "\n正在使用 %s 编辑配置文件...\n", editor)
					editCmd := exec.Command(editor, configPath)
					editCmd.Stdin = os.Stdin
					editCmd.Stdout = os.Stdout
					editCmd.Stderr = os.Stderr
					if err := editCmd.Run(); err != nil {
						fmt.Fprintf(os.Stderr, "⚠️  编辑器启动失败: %v\n", err)
					} else {
						fmt.Fprintln(os.Stderr, "✓ 配置文件已保存")
					}
				}
			} else if choice == "0" || choice == "" {
				// 重新加载配置
				fmt.Fprintln(os.Stderr, "\n正在重新加载配置...")
				cfg, err = config.Load(configPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "⚠️  加载配置失败: %v\n", err)
					fmt.Fprintln(os.Stderr, "请检查配置文件格式是否正确")
					fmt.Fprint(os.Stderr, "按回车键重试，或按 Ctrl+C 退出... ")
					var retryInput string
					_, _ = fmt.Scanln(&retryInput)
					continue
				}
				if len(cfg.Listeners) == 0 {
					fmt.Fprintln(os.Stderr, "⚠️  配置文件中没有有效的监听器配置")
					fmt.Fprint(os.Stderr, "按回车键重试，或按 Ctrl+C 退出... ")
					var retryInput string
					_, _ = fmt.Scanln(&retryInput)
					continue
				}
				fmt.Fprintln(os.Stderr, "✓ 配置已重新加载")
				continue
			} else {
				fmt.Fprintln(os.Stderr, "无效选项")
			}
		}
		break
	}
}

// modifyConfigInteractively 交互式修改配置
func modifyConfigInteractively(cfg *config.Config, configPath string) error {
	if len(cfg.Listeners) == 0 {
		return fmt.Errorf("没有可修改的监听器配置")
	}

	var idx int
	if len(cfg.Listeners) > 1 {
		// 显示所有监听器列表
		fmt.Fprintln(os.Stderr, "可用的监听器配置:")
		for i, l := range cfg.Listeners {
			fmt.Fprintf(os.Stderr, "  %d. %s - 端口:%d 串口:%s\n",
				i+1, l.Name, l.ListenPort, l.SerialPort)
		}
		fmt.Fprintln(os.Stderr)

		// 选择要编辑的监听器
		fmt.Fprintf(os.Stderr, "选择要编辑的监听器 (1-%d): ", len(cfg.Listeners))
		var selection int
		_, _ = fmt.Scanln(&selection)
		if selection < 1 || selection > len(cfg.Listeners) {
			return fmt.Errorf("无效的选择")
		}
		idx = selection - 1
		fmt.Fprintln(os.Stderr)
	} else {
		idx = 0
	}

	// 显示当前配置
	fmt.Fprintln(os.Stderr, "当前配置:")
	fmt.Fprintf(os.Stderr, "  1. 串口: %s\n", cfg.Listeners[idx].SerialPort)
	fmt.Fprintf(os.Stderr, "  2. 监听端口: %d\n", cfg.Listeners[idx].ListenPort)
	fmt.Fprintf(os.Stderr, "  3. 波特率: %d\n", cfg.Listeners[idx].BaudRate)
	fmt.Fprintf(os.Stderr, "  4. 校验位: %s\n", cfg.Listeners[idx].Parity)
	fmt.Fprintf(os.Stderr, "  5. 数据位: %d\n", cfg.Listeners[idx].DataBits)
	fmt.Fprintf(os.Stderr, "  6. 停止位: %d\n", cfg.Listeners[idx].StopBits)
	fmt.Fprintln(os.Stderr)

	// 询问要修改哪项
	fmt.Fprint(os.Stderr, "请输入要修改的项编号 (1-6，直接回车跳过): ")
	var choice string
	_, _ = fmt.Scanln(&choice)

	choice = strings.TrimSpace(choice)
	if choice == "" {
		return nil
	}

	switch choice {
	case "1":
		// 列出可用串口并让用户选择（标记已配置的）
		fmt.Fprintln(os.Stderr, "\n可用的串口设备:")
		ports := scanSerialPorts()
		if len(ports) == 0 {
			fmt.Fprintln(os.Stderr, "  未找到可用的串口设备")
			return fmt.Errorf("没有可用的串口设备")
		}
		for i, p := range ports {
			// 检查是否已配置（排除当前修改的配置）
			used := false
			for j, l := range cfg.Listeners {
				if j != idx && l.SerialPort == p.Port {
					used = true
					break
				}
			}
			if used {
				fmt.Fprintf(os.Stderr, "  %d. %-20s - 已配置 %s\n", i+1, p.Port, emojiYes)
			} else {
				fmt.Fprintf(os.Stderr, "  %d. %-20s\n", i+1, p.Port)
			}
		}
		fmt.Fprintln(os.Stderr)

		// 循环直到选择有效的串口
		for {
			fmt.Fprintf(os.Stderr, "选择串口 (1-%d): ", len(ports))
			var selection int
			_, _ = fmt.Scanln(&selection)
			if selection < 1 || selection > len(ports) {
				return fmt.Errorf("无效的选择")
			}
			newPort := ports[selection-1].Port

			// 检查是否已配置
			used := false
			for j, l := range cfg.Listeners {
				if j != idx && l.SerialPort == newPort {
					used = true
					break
				}
			}
			if used {
				fmt.Fprintf(os.Stderr, "  串口 %s %s已被其他配置使用，请重新选择\n", newPort, emojiYes)
				continue
			}
			cfg.Listeners[idx].SerialPort = newPort
			break
		}
	case "2":
		fmt.Fprint(os.Stderr, "新的监听端口: ")
		var newVal int
		_, _ = fmt.Scanln(&newVal)
		if newVal > 0 && newVal <= 65535 {
			cfg.Listeners[idx].ListenPort = newVal
		} else {
			return fmt.Errorf("无效的端口号")
		}
	case "3":
		fmt.Fprint(os.Stderr, "新的波特率: ")
		var newVal int
		_, _ = fmt.Scanln(&newVal)
		if newVal > 0 {
			cfg.Listeners[idx].BaudRate = newVal
		}
	case "4":
		fmt.Fprintln(os.Stderr, "校验位选项:")
		fmt.Fprintln(os.Stderr, "  N - 无校验 (None)")
		fmt.Fprintln(os.Stderr, "  O - 奇校验 (Odd)")
		fmt.Fprintln(os.Stderr, "  E - 偶校验 (Even)")
		fmt.Fprint(os.Stderr, "选择 [N/O/E]: ")
		var newVal string
		_, _ = fmt.Scanln(&newVal)
		newVal = strings.ToUpper(strings.TrimSpace(newVal))
		if newVal == "N" || newVal == "O" || newVal == "E" {
			cfg.Listeners[idx].Parity = newVal
		} else {
			return fmt.Errorf("无效的校验位选项")
		}
	case "5":
		fmt.Fprint(os.Stderr, "新的数据位 (5-8): ")
		var newVal int
		_, _ = fmt.Scanln(&newVal)
		if newVal >= 5 && newVal <= 8 {
			cfg.Listeners[idx].DataBits = newVal
		} else {
			return fmt.Errorf("无效的数据位")
		}
	case "6":
		fmt.Fprint(os.Stderr, "新的停止位 (1-2): ")
		var newVal int
		_, _ = fmt.Scanln(&newVal)
		if newVal == 1 || newVal == 2 {
			cfg.Listeners[idx].StopBits = newVal
		} else {
			return fmt.Errorf("无效的停止位")
		}
	default:
		return fmt.Errorf("无效的选择")
	}

	// 保存配置
	return config.Save(configPath, cfg)
}

func setupLogging() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("[WARN] 无法打开日志文件: %v", err)
		} else {
			log.SetOutput(f)
		}
	}

	log.Printf("[INFO] Serial-Server v%s 启动", version)
}

func findConfigFile(name string) string {
	if _, err := os.Stat(name); err == nil {
		return name
	}

	locations := []string{
		name,
		filepath.Join(".", name),
		filepath.Join("..", name),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return name
}

func loadOrCreateConfig(path string) (*config.Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if !wizardMode {
			fmt.Println("未找到配置文件，进入配置向导...")
			wizardMode = true
		}
	}

	var cfg *config.Config
	var err error

	if wizardMode {
		wiz := wizard.NewWizard()
		cfg, err = wiz.Run(&config.Config{})
		if err != nil {
			return nil, fmt.Errorf("配置向导失败: %w", err)
		}

		if cfg != nil && len(cfg.Listeners) > 0 {
			if err := config.Save(path, cfg); err != nil {
				log.Printf("[WARN] 保存配置失败: %v", err)
			} else {
				fmt.Printf("配置已保存到 %s\n", path)
			}
		}
	} else {
		cfg, err = config.Load(path)
		if err != nil {
			return nil, fmt.Errorf("加载配置失败: %w", err)
		}
	}

	return cfg, nil
}

func runApp(cfg *config.Config) error {
	configPath := findConfigFile(configFile)

	listeners := make([]*listener.Listener, 0, len(cfg.Listeners))

	for _, lcfg := range cfg.Listeners {
		l := listener.NewListener(
			lcfg.Name,
			lcfg.ListenPort,
			lcfg.SerialPort,
			lcfg.BaudRate,
			lcfg.DataBits,
			lcfg.StopBits,
			lcfg.Parity,
			listener.DisplayFormat(lcfg.DisplayFormat),
		)
		listeners = append(listeners, l)
	}

	// 先显示配置摘要，让用户知道监听端口
	printConfigSummary(cfg)

	startedCount := 0
	for _, l := range listeners {
		if err := l.Start(); err != nil {
			// 启动失败时，先停止已启动的监听器
			for i := 0; i < startedCount; i++ {
				listeners[i].Stop()
			}
			return fmt.Errorf("启动监听器 %s 失败: %w", l.GetName(), err)
		}
		startedCount++
	}

	log.Printf("[INFO] 已启动 %d 个监听器", len(listeners))

	// 创建运行时管理器
	manager := &runtimeManager{
		configPath:  configPath,
		cfg:         cfg,
		listeners:   listeners,
		requestChan: make(chan runtimeRequest, 10),
		stopChan:    make(chan struct{}),
	}

	// 启动运行时命令监听
	go manager.listenForCommands()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// 处理运行时命令的 goroutine
	go func() {
		for req := range manager.requestChan {
			manager.handleRuntimeCommand(req)
		}
	}()

	// 记录启动信息到日志文件
	log.Println("╔═══════════════════════════════════════════════════════════════")
	log.Println("║                    Serial-Server 后台模式启动                     ")
	log.Println("╚═══════════════════════════════════════════════════════════════")
	log.Printf("[INFO] 版本: %s", version)
	log.Printf("[INFO] 启动时间: %s", time.Now().Format("2006-01-02 15:04:05"))
	log.Printf("[INFO] 日志文件: %s", logFile)
	log.Println("")

	// 记录配置摘要到日志
	log.Println("─────────────────────────────────────────────────────────────────")
	log.Println("配置摘要:")
	log.Println("─────────────────────────────────────────────────────────────────")
	for i, lcfg := range cfg.Listeners {
		log.Printf("  [%d] %s", i+1, lcfg.Name)
		log.Printf("      串口: %s", lcfg.SerialPort)
		log.Printf("      监听端口: %d", lcfg.ListenPort)
		log.Printf("      波特率: %d, 校验位: %s, 数据位: %d, 停止位: %d",
			lcfg.BaudRate, lcfg.Parity, lcfg.DataBits, lcfg.StopBits)
		log.Printf("      显示格式: %s", lcfg.DisplayFormat)
		log.Println("")
	}
	log.Println("─────────────────────────────────────────────────────────────────")
	log.Println("")
	log.Println("[INFO] 监听器启动中...")
	log.Println("")

	// 为每个监听器创建数据缓冲器，避免单字节一行
	type dataBuffer struct {
		buffer    []byte
		direction string
		lastTime  time.Time
		timer     *time.Timer
		mu        sync.Mutex
	}

	buffers := make(map[string]*dataBuffer)
	buffersMutex := sync.Mutex{}
	flushInterval := 50 * time.Millisecond // 50ms内的数据合并显示

	for _, l := range listeners {
		l := l
		l.SetOnData(func(data []byte, direction string, clientID string) {
			// 为每个客户端创建独立缓冲
			bufferKey := l.GetName() + ":" + clientID

			buffersMutex.Lock()
			buf, exists := buffers[bufferKey]
			if !exists {
				buf = &dataBuffer{
					buffer:   make([]byte, 0, 256),
					lastTime: time.Now(),
				}
				buffers[bufferKey] = buf
			}
			buffersMutex.Unlock()

			buf.mu.Lock()
			defer buf.mu.Unlock()

			// 合并设备名和客户端ID: device_1_#1
			deviceTag := l.GetName() + "_" + clientID

			// 转换方向为箭头显示
			directionArrow := direction
			if direction == "tx" {
				directionArrow = "→"
			} else if direction == "rx" {
				directionArrow = "←"
			}

			// 如果方向改变，先刷新旧数据
			if buf.direction != "" && buf.direction != direction && len(buf.buffer) > 0 {
				oldArrow := buf.direction
				if oldArrow == "tx" {
					oldArrow = "→"
				} else if oldArrow == "rx" {
					oldArrow = "←"
				}
				formatted := listener.FormatForDisplayCompact(buf.buffer, l.GetDisplayFormat())
				log.Printf("[%s] [%s] [%d] %s", deviceTag, oldArrow, len(buf.buffer), formatted)
				buf.buffer = buf.buffer[:0]
			}

			buf.direction = direction
			buf.buffer = append(buf.buffer, data...)

			// 重置定时器
			if buf.timer != nil {
				buf.timer.Stop()
			}
			buf.timer = time.AfterFunc(flushInterval, func() {
				buf.mu.Lock()
				defer buf.mu.Unlock()
				if len(buf.buffer) > 0 {
					formatted := listener.FormatForDisplayCompact(buf.buffer, l.GetDisplayFormat())
					log.Printf("[%s] [%s] [%d] %s", deviceTag, directionArrow, len(buf.buffer), formatted)
					buf.buffer = buf.buffer[:0]
				}
			})
		})
	}

	// 在控制台只显示简洁提示
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "╔═══════════════════════════════════════════════════════════════╗")
	fmt.Fprintf(os.Stderr, "║           Serial-Server v%s 后台运行中              ║\n", version)
	fmt.Fprintf(os.Stderr, "║  日志文件: %-54s ║\n", logFile)
	fmt.Fprintln(os.Stderr, "║                                                                   ║")
	fmt.Fprintln(os.Stderr, "║  运行时管理命令:                                                   ║")
	fmt.Fprintln(os.Stderr, "║    1 - 显示帮助                                                    ║")
	fmt.Fprintln(os.Stderr, "║    2 - 重新加载配置文件                                            ║")
	fmt.Fprintln(os.Stderr, "║    3 - 添加新配置                                                  ║")
	fmt.Fprintln(os.Stderr, "║    4 - 修改现有配置                                                ║")
	fmt.Fprintln(os.Stderr, "║    5 - 删除配置                                                    ║")
	fmt.Fprintln(os.Stderr, "║    6 - 列出当前配置                                                ║")
	fmt.Fprintln(os.Stderr, "║    7 - 显示运行状态                                                ║")
	fmt.Fprintln(os.Stderr, "║    8 - FRP 管理                                                   ║")
	fmt.Fprintln(os.Stderr, "║                                                                   ║")
	fmt.Fprintln(os.Stderr, "║  提示: 输入数字即可执行，无需重启程序                              ║")
	fmt.Fprintln(os.Stderr, "║  按 Ctrl+C 退出程序                                               ║")
	fmt.Fprintln(os.Stderr, "╚═══════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(os.Stderr, "")

	<-sigCh
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "[INFO] 正在关闭...")

	// 停止运行时管理器
	close(manager.stopChan)
	close(manager.requestChan)

	// 停止所有定时器
	for _, buf := range buffers {
		buf.mu.Lock()
		if buf.timer != nil {
			buf.timer.Stop()
			buf.timer = nil
		}
		buf.mu.Unlock()
	}

	fmt.Fprintln(os.Stderr, "[INFO] 正在停止监听器...")

	log.Println("")
	log.Println("─────────────────────────────────────────────────────────────────")
	log.Println("[INFO] 收到退出信号，正在关闭...")
	log.Printf("[INFO] 关闭时间: %s", time.Now().Format("2006-01-02 15:04:05"))

	// 记录统计信息
	for _, l := range listeners {
		stats := l.GetStats()
		log.Printf("[STATS] %s:", l.GetName())
		log.Printf("    接收字节数: %d", stats.RxBytes)
		log.Printf("    发送字节数: %d", stats.TxBytes)
		log.Printf("    接收包数: %d", stats.RxPackets)
		log.Printf("    发送包数: %d", stats.TxPackets)
		log.Printf("    当前客户端数: %d", stats.Clients)
	}

	log.Println("─────────────────────────────────────────────────────────────────")
	log.Println("╚═══════════════════════════════════════════════════════════════")
	log.Println("")

	// 在 goroutine 中停止监听器，避免阻塞
	done := make(chan struct{})
	go func() {
		for _, l := range listeners {
			l.Stop()
		}
		close(done)
	}()

	// 等待停止完成，最多 2 秒
	select {
	case <-done:
		log.Println("[INFO] 所有监听器已停止")
		fmt.Fprintln(os.Stderr, "[INFO] 已退出")
	case <-time.After(2 * time.Second):
		log.Println("[WARN] 停止超时，强制退出")
		fmt.Fprintln(os.Stderr, "[WARN] 停止超时，已强制退出")
	}

	return nil
}

func printConfigSummary(cfg *config.Config) {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "%s配置摘要:%s\n", getGreen(), getReset())
	for i, l := range cfg.Listeners {
		frpStatus := checkFRPStatus(l.SerialPort, l.ListenPort)
		// 根据 FRP 状态选择颜色
		var statusColor string
		if frpStatus == emojiYes {
			statusColor = getGreen()
		} else {
			statusColor = getRed()
		}
		fmt.Fprintf(os.Stderr, "  %d. %s:[%d %s %d %d %s] 端口[%d] frp[%s%s%s]\n",
			i+1, l.SerialPort, l.BaudRate, l.Parity, l.DataBits, l.StopBits, l.DisplayFormat, l.ListenPort,
			statusColor, frpStatus, getReset())
	}
}

// checkFRPStatus 检查端口是否已在 FRP 中添加代理
func checkFRPStatus(_ string, port int) string {
	client := frp.NewClient()
	proxyNames, proxyPorts, err := client.GetAllSerialServerProxies()
	if err != nil {
		return emojiNo
	}

	// 检查是否有 local_port = port 的代理
	for _, name := range proxyNames {
		if proxyPorts[name] == port {
			return emojiYes
		}
	}
	return emojiNo
}

func printConfigSummaryToStderr(cfg *config.Config) {
	printConfigSummary(cfg)
}

// deleteConfigInteractively 交互式删除配置
func deleteConfigInteractively(cfg *config.Config, configPath string) error {
	if len(cfg.Listeners) == 0 {
		return fmt.Errorf("没有可删除的配置")
	}

	fmt.Fprintln(os.Stderr, "当前配置:")
	for i, l := range cfg.Listeners {
		fmt.Fprintf(os.Stderr, "  %d. %s - %s (:%d)\n", i+1, l.Name, l.SerialPort, l.ListenPort)
	}
	fmt.Fprintln(os.Stderr)

	fmt.Fprintf(os.Stderr, "请输入要删除的配置编号 (1-%d): ", len(cfg.Listeners))
	var choice string
	_, _ = fmt.Scanln(&choice)

	choice = strings.TrimSpace(choice)
	if choice == "" {
		return nil
	}

	idx, err := strconv.Atoi(choice)
	if err != nil || idx < 1 || idx > len(cfg.Listeners) {
		return fmt.Errorf("无效的选择")
	}

	// 确认删除
	deletedCfg := cfg.Listeners[idx-1]
	fmt.Fprintf(os.Stderr, "\n确认删除配置: %s - %s (:%d)? [y/n]: ",
		deletedCfg.Name, deletedCfg.SerialPort, deletedCfg.ListenPort)
	var confirm string
	_, _ = fmt.Scanln(&confirm)
	if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
		return fmt.Errorf("已取消删除")
	}

	// 删除配置
	cfg.Listeners = append(cfg.Listeners[:idx-1], cfg.Listeners[idx:]...)

	// 保存配置
	if err := config.Save(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "已删除配置: %s\n", deletedCfg.Name)
	return nil
}

// runFRPMenu FRP 管理菜单
func runFRPMenu() {
	for {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s                    FRP (v%s)%s\n", getGreen(), version, getReset())
		fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", getGreen(), getReset())
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "%s操作:%s\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  1 %s- 添加代理\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  2 %s- 查看配置\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  3 %s- 清理代理\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "%s  0 %s- 返回\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "\n%s请输入 [1/2/3/0]: %s", getGreen(), getReset())

		var choice string
		_, _ = fmt.Scanln(&choice)
		choice = strings.ToLower(strings.TrimSpace(choice))
		fmt.Fprintln(os.Stderr, "")

		switch choice {
		case "1":
			// 添加 STCP 代理
			frpAddProxy()
		case "2":
			// 查看当前配置
			frpShowConfig()
		case "3":
			// 清理所有串口代理
			frpCleanupProxies()
		case "0":
			fmt.Fprintln(os.Stderr, "返回上级菜单")
			return
		default:
			fmt.Fprintln(os.Stderr, "无效选项")
		}
	}
}

// frpAddProxy 添加 STCP 代理
func frpAddProxy() {
	if len(cfg.Listeners) == 0 {
		fmt.Fprintln(os.Stderr, "没有可用的监听配置")
		return
	}

	fmt.Fprintln(os.Stderr, "添加 STCP 代理")
	fmt.Fprintln(os.Stderr, "━━━━━━━━━━━━━━━")

	// 列出所有监听器供选择
	for i, l := range cfg.Listeners {
		fmt.Fprintf(os.Stderr, "  %d. %s - 端口 %d\n", i+1, l.Name, l.ListenPort)
	}

	fmt.Fprint(os.Stderr, "\n请选择要添加代理的监听器: ")

	var choice string
	_, _ = fmt.Scanln(&choice)
	choice = strings.TrimSpace(choice)

	idx, err := strconv.Atoi(choice)
	if err != nil || idx < 1 || idx > len(cfg.Listeners) {
		fmt.Fprintln(os.Stderr, "无效的选择")
		return
	}

	listener := cfg.Listeners[idx-1]
	port := listener.ListenPort

	proxyName := frp.SafeProxyName(listener.SerialPort, port)
	fmt.Fprintf(os.Stderr, "正在添加 STCP 代理 [%s]...\n", proxyName)

	client := frp.NewClient()
	if err := client.AddSTCPProxy(listener.SerialPort, port); err != nil {
		fmt.Fprintf(os.Stderr, "%s失败%s 添加失败: %v\n", getRed(), getReset(), err)
	} else {
		fmt.Fprintf(os.Stderr, "%s成功%s 成功添加 STCP 代理 [%s]\n", getGreen(), getReset(), proxyName)
	}
}

// frpShowConfig 查看当前 FRP 配置
func frpShowConfig() {
	fmt.Fprintf(os.Stderr, "%s当前 FRP 配置%s\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s━━━━━━━━━━━━━━━%s\n", getGreen(), getReset())

	client := frp.NewClient()
	config, err := client.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s失败%s 获取配置失败: %v\n", getRed(), getReset(), err)
		return
	}

	fmt.Fprintln(os.Stderr, config)
}

// frpCleanupProxies 清理所有串口代理
func frpCleanupProxies() {
	fmt.Fprintf(os.Stderr, "%s清理所有串口代理%s\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s━━━━━━━━━━━━━━━━━━━━%s\n", getGreen(), getReset())

	client := frp.NewClient()
	proxyNames, proxyPorts, err := client.GetAllSerialServerProxies()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s失败%s 获取配置失败: %v\n", getRed(), getReset(), err)
		return
	}

	if len(proxyNames) == 0 {
		fmt.Fprintln(os.Stderr, "未找到串口代理配置")
		return
	}

	// 显示要删除的代理列表
	fmt.Fprintf(os.Stderr, "找到 %d 个串口代理配置:\n", len(proxyNames))
	for i, name := range proxyNames {
		fmt.Fprintf(os.Stderr, "  %d. [%s] 端口: %d\n", i+1, name, proxyPorts[name])
	}
	fmt.Fprintln(os.Stderr, "")

	fmt.Fprint(os.Stderr, "确认清理? (输入 y 确认，其他取消): ")
	var confirm string
	_, _ = fmt.Scanln(&confirm)
	if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
		fmt.Fprintln(os.Stderr, "已取消")
		return
	}

	// 逐个移除代理
	successCount := 0
	for _, name := range proxyNames {
		if err := client.RemoveSerialServerProxy(name); err != nil {
			fmt.Fprintf(os.Stderr, "失败 移除 [%s] 失败: %v\n", name, err)
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		fmt.Fprintf(os.Stderr, "%s成功%s 已清理 %d 个串口代理配置\n", getGreen(), getReset(), successCount)
	} else {
		fmt.Fprintf(os.Stderr, "%s失败%s 清理失败\n", getRed(), getReset())
	}
}

// removeSections 从配置中移除指定的 sections
func removeSections(config string, sectionsToRemove []string) string {
	sectionSet := make(map[string]bool)
	for _, s := range sectionsToRemove {
		sectionSet[strings.ToLower(s)] = true
	}

	var result []string
	inSectionToRemove := false
	currentSection := ""

	lines := strings.Split(config, "\n")
	//nolint:gocritic // Complex menu logic - if-else chain is appropriate
	for _, line := range lines {
		lineStr := strings.TrimSpace(line)

		if strings.HasPrefix(lineStr, "[") && strings.HasSuffix(lineStr, "]") {
			// 切换 section
			currentSection = strings.Trim(lineStr, "[]")
			inSectionToRemove = sectionSet[strings.ToLower(currentSection)]

			if !inSectionToRemove {
				result = append(result, line)
			}
		} else if inSectionToRemove {
			// 在要移除的 section 内，跳过所有行
			continue
		} else {
			result = append(result, line)
		}
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

func listSerialPorts() {
	fmt.Println("可用串口设备:")
	fmt.Println()

	ports := scanSerialPorts()
	if len(ports) == 0 {
		fmt.Println("  未找到串口设备")
		return
	}

	for _, p := range ports {
		fmt.Printf("  %-20s - %s\n", p.Port, p.Desc)
	}
}

func scanSerialPorts() []wizard.PortInfo {
	ports := make([]wizard.PortInfo, 0, 10)

	// 扫描可用串口
	availablePorts := listener.ScanAvailablePorts()

	for _, p := range availablePorts {
		desc := getPortDescription(p)
		ports = append(ports, wizard.PortInfo{Port: p, Desc: desc})
	}

	return ports
}

func getPortDescription(port string) string {
	if contains(port, "usb") {
		return "USB 串口设备"
	}
	if contains(port, "ttyS") {
		return "标准串口"
	}
	if contains(port, "ttyACM") {
		return "USB CDC 设备"
	}
	return "串口设备"
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func checkConfiguration() error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}

	if len(cfg.Listeners) == 0 {
		return fmt.Errorf("配置文件中没有监听器")
	}

	for _, l := range cfg.Listeners {
		if l.SerialPort == "" {
			return fmt.Errorf("[%s] serial_port 未设置", l.Name)
		}
		if l.ListenPort <= 0 || l.ListenPort > 65535 {
			return fmt.Errorf("[%s] listen_port 无效: %d", l.Name, l.ListenPort)
		}
	}

	return nil
}

// ScanAvailablePorts 扫描可用串口（包装函数）
func ScanAvailablePorts() []string {
	return listener.ScanAvailablePorts()
}

// getGreen 返回绿色代码（如果支持颜色）
func getGreen() string {
	if useColor {
		return colorGreen
	}
	return ""
}

// getRed 返回红色代码（如果支持颜色）
func getRed() string {
	if useColor {
		return colorRed
	}
	return ""
}

// getReset 返回重置代码（如果支持颜色）
func getReset() string {
	if useColor {
		return colorReset
	}
	return ""
}

// listenForCommands 监听用户输入的运行时命令
func (m *runtimeManager) listenForCommands() {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		select {
		case <-m.stopChan:
			return
		default:
			if !scanner.Scan() {
				return
			}

			input := strings.TrimSpace(scanner.Text())
			if input == "" {
				continue
			}

			// 只支持数字输入
			var cmd runtimeCommand
			var cmdName string
			var needConfirm bool // 是否需要确认

			switch input {
			case "1":
				cmd = cmdHelp
				cmdName = "显示帮助"
				needConfirm = false
			case "2":
				cmd = cmdReload
				cmdName = "重新加载配置"
				needConfirm = false
			case "3":
				cmd = cmdAdd
				cmdName = "添加新配置"
				needConfirm = false
			case "4":
				cmd = cmdModify
				cmdName = "修改配置"
				needConfirm = false
			case "5":
				cmd = cmdDelete
				cmdName = "删除配置"
				needConfirm = true // 危险操作，需要确认
			case "6":
				cmd = cmdList
				cmdName = "列出配置"
				needConfirm = false
			case "7":
				cmd = cmdStatus
				cmdName = "显示状态"
				needConfirm = false
			case "8":
				cmd = cmdFRPMenu
				cmdName = "FRP 管理"
				needConfirm = false
			default:
				fmt.Fprintf(os.Stderr, "\n[WARN] 无效输入: %s，请输入数字 1-8\n\n", input)
				continue
			}

			// 危险操作需要确认
			if needConfirm {
				fmt.Fprintf(os.Stderr, "\n[确认] 即将执行: %s\n", cmdName)
				fmt.Fprint(os.Stderr, "确认执行? [y/N]: ")

				var confirm string
				if scanner.Scan() {
					confirm = strings.TrimSpace(strings.ToLower(scanner.Text()))
				}

				if confirm != "y" && confirm != "yes" {
					fmt.Fprintln(os.Stderr, "[INFO] 已取消操作\n")
					continue
				}
			}

			req := runtimeRequest{
				command: cmd,
				data:    []string{input},
				result:  make(chan error, 1),
			}

			// 发送请求到主循环处理
			select {
			case m.requestChan <- req:
				// 等待处理结果
				if err := <-req.result; err != nil {
					fmt.Fprintf(os.Stderr, "\n[ERROR] %v\n\n", err)
				}
			case <-m.stopChan:
				return
			}
		}
	}
}

// handleRuntimeCommand 处理运行时命令
func (m *runtimeManager) handleRuntimeCommand(req runtimeRequest) {
	switch req.command {
	case cmdHelp:
		m.showHelp()
		req.result <- nil
	case cmdReload:
		req.result <- m.reloadConfig()
	case cmdList:
		m.listConfigs()
		req.result <- nil
	case cmdStatus:
		m.showStatus()
		req.result <- nil
	case cmdAdd:
		req.result <- m.addConfig()
	case cmdModify:
		req.result <- m.modifyConfig()
	case cmdDelete:
		req.result <- m.deleteConfig()
	case cmdFRPMenu:
		m.runFRPMenuRuntime()
		req.result <- nil
	default:
		fmt.Fprintf(os.Stderr, "[WARN] 未知命令: %s\n", req.command)
		fmt.Fprintf(os.Stderr, "[INFO] 输入 'help' 查看可用命令\n")
		req.result <- nil
	}
}

// showHelp 显示帮助信息
func (m *runtimeManager) showHelp() {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s                    帮助 (v%s)%s\n", getGreen(), version, getReset())
	fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", getGreen(), getReset())
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "%s命令:%s\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s  1%s - 帮助\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s  2%s - 重载配置\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s  3%s - 添加配置\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s  4%s - 修改配置\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s  5%s - 删除配置\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s  6%s - 配置列表\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s  7%s - 运行状态\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s  8%s - FRP\n", getGreen(), getReset())
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "%s提示:%s\n", getGreen(), getReset())
	fmt.Fprintln(os.Stderr, "  配置修改即时生效，无需重启")
	fmt.Fprintln(os.Stderr, "")
}

// reloadConfig 重新加载配置文件
func (m *runtimeManager) reloadConfig() error {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "[INFO] 正在重新加载配置文件...")

	newCfg, err := config.Load(m.configPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	if len(newCfg.Listeners) == 0 {
		return fmt.Errorf("配置文件中没有监听器")
	}

	// 更新配置
	m.cfg = newCfg
	fmt.Fprintln(os.Stderr, "[INFO] 配置已重新加载")

	// 显示新配置
	m.listConfigs()

	return nil
}

// listConfigs 列出当前配置
func (m *runtimeManager) listConfigs() {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s                    (v%s)%s\n", getGreen(), version, getReset())
	fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", getGreen(), getReset())
	fmt.Fprintln(os.Stderr, "")

	if len(m.cfg.Listeners) == 0 {
		fmt.Fprintln(os.Stderr, "  没有配置")
		fmt.Fprintln(os.Stderr, "")
		return
	}

	for i, l := range m.cfg.Listeners {
		frpStatus := checkFRPStatus(l.SerialPort, l.ListenPort)
		var statusColor string
		if frpStatus == emojiYes {
			statusColor = getGreen()
		} else {
			statusColor = getRed()
		}

		// 检查是否正在运行
		running := "●"
		runningColor := getRed()
		for _, listener := range m.listeners {
			if listener.GetName() == l.Name {
				running = "●"
				runningColor = getGreen()
				break
			}
		}

		fmt.Fprintf(os.Stderr, "  %d. %s%s%s %s:[%d %s %d %d %s] 端口[%d] frp[%s%s%s]\n",
			i+1, runningColor, running, getReset(),
			l.SerialPort, l.BaudRate, l.Parity, l.DataBits, l.StopBits, l.DisplayFormat,
			l.ListenPort, statusColor, frpStatus, getReset())
	}
	fmt.Fprintln(os.Stderr, "")
}

// showStatus 显示运行状态
func (m *runtimeManager) showStatus() {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", getGreen(), getReset())
	fmt.Fprintf(os.Stderr, "%s                    状态 (v%s)%s\n", getGreen(), version, getReset())
	fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", getGreen(), getReset())
	fmt.Fprintln(os.Stderr, "")

	fmt.Fprintf(os.Stderr, "%s运行中的监听器:%s\n", getGreen(), getReset())
	for i, l := range m.listeners {
		stats := l.GetStats()
		fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, l.GetName())
		fmt.Fprintf(os.Stderr, "     状态: %s● 运行中%s\n", getGreen(), getReset())
		fmt.Fprintf(os.Stderr, "     客户端: %d\n", stats.Clients)
		fmt.Fprintf(os.Stderr, "     接收: %d 字节 (%d 包)\n", stats.RxBytes, stats.RxPackets)
		fmt.Fprintf(os.Stderr, "     发送: %d 字节 (%d 包)\n", stats.TxBytes, stats.TxPackets)
		fmt.Fprintln(os.Stderr, "")
	}
}

// addConfig 添加新配置
func (m *runtimeManager) addConfig() error {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "[INFO] 添加新配置...")

	wiz := wizard.NewWizard()
	newCfg, err := wiz.RunAddOnly(m.cfg)
	if err != nil {
		if strings.Contains(err.Error(), "no serial ports found") || strings.Contains(err.Error(), "没有可用的串口") {
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "⚠️  未检测到串口设备")
			return nil
		}
		return fmt.Errorf("添加配置失败: %w", err)
	}

	m.cfg = newCfg
	if err := config.Save(m.configPath, m.cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	fmt.Fprintln(os.Stderr, "[INFO] 配置已添加并保存")
	m.listConfigs()

	return nil
}

// modifyConfig 修改配置
func (m *runtimeManager) modifyConfig() error {
	if len(m.cfg.Listeners) == 0 {
		return fmt.Errorf("没有可修改的配置")
	}

	return modifyConfigInteractively(m.cfg, m.configPath)
}

// deleteConfig 删除配置
func (m *runtimeManager) deleteConfig() error {
	if len(m.cfg.Listeners) == 0 {
		return fmt.Errorf("没有可删除的配置")
	}

	return deleteConfigInteractively(m.cfg, m.configPath)
}

// runFRPMenuRuntime 运行时 FRP 菜单
func (m *runtimeManager) runFRPMenuRuntime() {
	// 使用全局 cfg 变量
	oldCfg := cfg
	cfg = m.cfg
	defer func() { cfg = oldCfg }()

	runFRPMenu()
}
