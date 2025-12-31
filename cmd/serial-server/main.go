package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	version           = "1.12.5"

	// 经典绿风格 - 颜色定义
	colorGreen = "\x1b[32m" // 绿色
	colorRed   = "\x1b[31m" // 红色
	colorReset = "\x1b[0m"  // 重置

	// 经典绿风格 - 状态文字
	emojiYes = "打勾" // 已添加/已配置
	emojiNo  = "打叉" // 未添加/未配置
)

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
			fmt.Scanln(new(string))
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
				fmt.Scanln(new(string))
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
		fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s                    Serial-Server 启动菜单%s\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", colorGreen, colorReset)
		fmt.Fprintln(os.Stderr, "")
		printConfigSummaryToStderr(cfg)
		fmt.Fprintf(os.Stderr, "%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s请选择操作:%s\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s  1 %s- 直接启动程序\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s  2 %s- 添加新配置\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s  3 %s- 修改配置\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s  4 %s- 删除配置\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s  5 %s- FRP 管理\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s  0 %s- 退出\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "\n%s请输入选项 [1/2/3/4/5/0]: %s", colorGreen, colorReset)

		var choice string
		fmt.Scanln(&choice)
		choice = strings.ToLower(strings.TrimSpace(choice))

		fmt.Fprintln(os.Stderr, "")

		switch choice {
		case "1":
			// 直接启动，继续执行
		case "2":
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
					fmt.Scanln(new(string))
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
		case "3":
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
		case "4":
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
			if len(cfg.Listeners) == 0 {
				fmt.Fprintln(os.Stderr, "没有有效配置，请先添加配置")
				os.Exit(1)
			}
		case "5":
			// FRP 管理
			runFRPMenu()
		case "0":
			fmt.Fprintln(os.Stderr, "退出程序")
			return
		default:
			fmt.Fprintln(os.Stderr, "无效选项，直接启动...")
		}
		fmt.Fprintln(os.Stderr, "")
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
			fmt.Scanln(&input)
			choice := strings.ToLower(strings.TrimSpace(input))

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
						fmt.Scanln(&input)
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
					fmt.Scanln(&retryInput)
					continue
				}
				if len(cfg.Listeners) == 0 {
					fmt.Fprintln(os.Stderr, "⚠️  配置文件中没有有效的监听器配置")
					fmt.Fprint(os.Stderr, "按回车键重试，或按 Ctrl+C 退出... ")
					var retryInput string
					fmt.Scanln(&retryInput)
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
		fmt.Scanln(&selection)
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
	fmt.Scanln(&choice)

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
			fmt.Scanln(&selection)
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
		fmt.Scanln(&newVal)
		if newVal > 0 && newVal <= 65535 {
			cfg.Listeners[idx].ListenPort = newVal
		} else {
			return fmt.Errorf("无效的端口号")
		}
	case "3":
		fmt.Fprint(os.Stderr, "新的波特率: ")
		var newVal int
		fmt.Scanln(&newVal)
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
		fmt.Scanln(&newVal)
		newVal = strings.ToUpper(strings.TrimSpace(newVal))
		if newVal == "N" || newVal == "O" || newVal == "E" {
			cfg.Listeners[idx].Parity = newVal
		} else {
			return fmt.Errorf("无效的校验位选项")
		}
	case "5":
		fmt.Fprint(os.Stderr, "新的数据位 (5-8): ")
		var newVal int
		fmt.Scanln(&newVal)
		if newVal >= 5 && newVal <= 8 {
			cfg.Listeners[idx].DataBits = newVal
		} else {
			return fmt.Errorf("无效的数据位")
		}
	case "6":
		fmt.Fprint(os.Stderr, "新的停止位 (1-2): ")
		var newVal int
		fmt.Scanln(&newVal)
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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

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
	fmt.Fprintln(os.Stderr, "║                   Serial-Server 后台运行中                       ║")
	fmt.Fprintf(os.Stderr, "║  日志文件: %-54s ║\n", logFile)
	fmt.Fprintln(os.Stderr, "║                                                                   ║")
	fmt.Fprintln(os.Stderr, "║  按 Ctrl+C 退出程序                                               ║")
	fmt.Fprintln(os.Stderr, "╚═══════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(os.Stderr, "")

	<-sigCh
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "[INFO] 正在关闭...")

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
	fmt.Fprintf(os.Stderr, "%s配置摘要:%s\n", colorGreen, colorReset)
	for i, l := range cfg.Listeners {
		frpStatus := checkFRPStatus(l.SerialPort, l.ListenPort)
		// 根据 FRP 状态选择颜色
		var statusColor string
		if frpStatus == emojiYes {
			statusColor = colorGreen
		} else {
			statusColor = colorRed
		}
		fmt.Fprintf(os.Stderr, "  %d. %s:[%d %s %d %d %s] 端口[%d] frp[%s%s%s]\n",
			i+1, l.SerialPort, l.BaudRate, l.Parity, l.DataBits, l.StopBits, l.DisplayFormat, l.ListenPort,
			statusColor, frpStatus, colorReset)
	}
}

// checkFRPStatus 检查端口是否已在 FRP 中添加代理
func checkFRPStatus(serialPort string, port int) string {
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
	fmt.Scanln(&choice)

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
	fmt.Scanln(&confirm)
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
		fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s                    FRP 管理菜单%s\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s═══════════════════════════════════════════════════════%s\n", colorGreen, colorReset)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "%s请选择操作:%s\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s  1 %s- 添加 STCP 代理\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s  2 %s- 查看当前配置\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s  3 %s- 清理所有串口代理\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "%s  0 %s- 返回上级菜单\n", colorGreen, colorReset)
		fmt.Fprintf(os.Stderr, "\n%s请输入选项 [1/2/3/0]: %s", colorGreen, colorReset)

		var choice string
		fmt.Scanln(&choice)
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
	fmt.Scanln(&choice)
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
		fmt.Fprintf(os.Stderr, "%s打叉 %s添加失败: %v\n", colorRed, colorReset, err)
	} else {
		fmt.Fprintf(os.Stderr, "%s打勾 %s成功添加 STCP 代理 [%s]\n", colorGreen, colorReset, proxyName)
	}
}

// frpShowConfig 查看当前 FRP 配置
func frpShowConfig() {
	fmt.Fprintf(os.Stderr, "%s当前 FRP 配置%s\n", colorGreen, colorReset)
	fmt.Fprintf(os.Stderr, "%s━━━━━━━━━━━━━━━%s\n", colorGreen, colorReset)

	client := frp.NewClient()
	config, err := client.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s打叉 %s获取配置失败: %v\n", colorRed, colorReset, err)
		return
	}

	fmt.Fprintln(os.Stderr, config)
}

// frpCleanupProxies 清理所有串口代理
func frpCleanupProxies() {
	fmt.Fprintf(os.Stderr, "%s清理所有串口代理%s\n", colorGreen, colorReset)
	fmt.Fprintf(os.Stderr, "%s━━━━━━━━━━━━━━━━━━━━%s\n", colorGreen, colorReset)

	client := frp.NewClient()
	proxyNames, proxyPorts, err := client.GetAllSerialServerProxies()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s打叉 %s获取配置失败: %v\n", colorRed, colorReset, err)
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
	fmt.Scanln(&confirm)
	if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
		fmt.Fprintln(os.Stderr, "已取消")
		return
	}

	// 逐个移除代理
	successCount := 0
	for _, name := range proxyNames {
		if err := client.RemoveSerialServerProxy(name); err != nil {
			fmt.Fprintf(os.Stderr, "打叉 移除 [%s] 失败: %v\n", name, err)
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		fmt.Fprintf(os.Stderr, "%s打勾 %s已清理 %d 个串口代理配置\n", colorGreen, colorReset, successCount)
	} else {
		fmt.Fprintf(os.Stderr, "%s打叉 %s清理失败\n", colorRed, colorReset)
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
	for _, line := range lines {
		lineStr := strings.TrimSpace(line)

		if strings.HasPrefix(lineStr, "[") && strings.HasSuffix(lineStr, "]") {
			// 切换 section
			if inSectionToRemove {
				inSectionToRemove = false
			}
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
	var ports []wizard.PortInfo

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
