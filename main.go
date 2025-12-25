package main

import (
	"serial-server/config"
	"serial-server/listener"
	"serial-server/tui"
	"serial-server/wizard"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const (
	defaultConfigFile = "config.ini"
	version           = "1.0.0"
)

var (
	configFile  string
	listPorts   bool
	checkConfig bool
	wizardMode  bool
	logFile     string
	logLevel    string
	showVersion bool
)

func init() {
	flag.StringVar(&configFile, "c", defaultConfigFile, "配置文件路径")
	flag.StringVar(&configFile, "config", defaultConfigFile, "配置文件路径")
	flag.BoolVar(&listPorts, "l", false, "列出可用串口设备")
	flag.BoolVar(&listPorts, "list", false, "列出可用串口设备")
	flag.BoolVar(&checkConfig, "check", false, "验证配置文件")
	flag.BoolVar(&wizardMode, "wizard", false, "进入交互式配置向导")
	flag.StringVar(&logFile, "log", "", "日志文件路径")
	flag.StringVar(&logLevel, "level", "info", "日志级别: debug, info, warn, error")
	flag.BoolVar(&showVersion, "version", false, "显示版本信息")
	flag.BoolVar(&showVersion, "v", false, "显示版本信息")
}

func main() {
	flag.Parse()

	if showVersion {
		fmt.Printf("Serial-Server v%s\n", version)
		return
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

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	if cfg == nil || len(cfg.Listeners) == 0 {
		fmt.Println("未找到有效配置，请先运行配置向导:")
		fmt.Println("  ./serial-server --wizard")
		return
	}

	if err := runApp(cfg); err != nil {
		log.Fatalf("运行失败: %v", err)
	}
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
			listener.DisplayFormat(lcfg.DisplayFormat),
			lcfg.MaxClients,
		)
		listeners = append(listeners, l)
	}

	for _, l := range listeners {
		if err := l.Start(); err != nil {
			return fmt.Errorf("启动监听器 %s 失败: %w", l.GetName(), err)
		}
	}

	log.Printf("[INFO] 已启动 %d 个监听器", len(listeners))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	tuiInstance, err := tui.NewTUI(listeners)
	if err != nil {
		log.Printf("[WARN] 无法创建 TUI: %v，使用日志模式", err)

		for _, l := range listeners {
			l.SetOnData(func(data []byte, direction string) {
				formatted := listener.FormatForDisplay(data, l.GetDisplayFormat())
				log.Printf("[%s] [%s] %s", l.GetName(), direction, formatted)
			})
		}

		<-sigCh
		log.Println("[INFO] 正在关闭...")
		for _, l := range listeners {
			l.Stop()
		}
		return nil
	}

	for i, l := range listeners {
		idx := i
		l.SetOnData(func(data []byte, direction string) {
			tuiInstance.AddData(data, direction, idx)
		})
	}

	go func() {
		if err := tuiInstance.Run(); err != nil {
			log.Printf("[ERROR] TUI 错误: %v", err)
		}
	}()

	<-sigCh
	log.Println("[INFO] 正在关闭...")

	tuiInstance.Stop()
	time.Sleep(100 * time.Millisecond)

	for _, l := range listeners {
		l.Stop()
	}

	return nil
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

	possiblePorts := []string{
		"/dev/ttyUSB0", "/dev/ttyUSB1", "/dev/ttyUSB2",
		"/dev/ttyS0", "/dev/ttyS1",
		"/dev/ttyACM0",
		"/dev/cu.usbserial-1", "/dev/cu.usbserial-2",
	}

	for _, p := range possiblePorts {
		if _, err := os.Stat(p); err == nil {
			desc := getPortDescription(p)
			ports = append(ports, wizard.PortInfo{Port: p, Desc: desc})
		}
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
