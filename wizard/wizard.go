// Package main - serial-server
package wizard

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/whysmx/serial-server/config"
	"github.com/whysmx/serial-server/listener"
)

const (
	// Default serial port configuration
	DefaultBaudRate      = 115200
	DefaultDataBits      = 8
	DefaultStopBits      = 1
	DefaultParity        = "N"
	DefaultDisplayFormat = "HEX"

	// Emoji for status display
	emojiYes = "打勾"
	emojiNo  = "打叉"
)

// PortInfo represents information about a serial port.
type PortInfo struct {
	Port string
	Desc string
}

// Wizard provides interactive configuration.
type Wizard struct {
	reader *bufio.Reader
}

// NewWizard creates a new wizard instance.
func NewWizard() *Wizard {
	return &Wizard{
		reader: bufio.NewReader(os.Stdin),
	}
}

// Run runs the configuration wizard.
func (w *Wizard) Run(cfg *config.Config) (*config.Config, error) {
	fmt.Println()
	fmt.Println("  Serial-Server 串口服务器配置向导")
	fmt.Println("  ─────────────────────────────────")

	// Check if we have existing config
	hasExisting := len(cfg.Listeners) > 0

	if hasExisting {
		fmt.Println()
		fmt.Println("  检测到现有配置:")
		for i, l := range cfg.Listeners {
			fmt.Printf("    %d. %s - %s (:%d)\n", i+1, l.Name, l.SerialPort, l.ListenPort)
		}
		fmt.Println()

		fmt.Print("  是否添加新配置? (y/n): ")
		ans, _ := w.reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(ans), "y") {
			return cfg, nil
		}
		fmt.Println()
	}

	// Scan for serial ports
	fmt.Println()
	fmt.Println("  扫描串口设备...")
	ports := w.scanPorts()
	if len(ports) == 0 {
		fmt.Println("  未找到串口设备!")
		return nil, fmt.Errorf("no serial ports found")
	}

	fmt.Println()
	fmt.Println("  可用串口:")
	for i, p := range ports {
		fmt.Printf("    %d. %-15s - %s\n", i+1, p.Port, p.Desc)
	}
	fmt.Println()

	// Add serial listeners
	fmt.Println("  按 Enter 添加第一个串口设备 (或输入 q 退出): ")
	for {
		fmt.Print("  选择串口 (1-" + strconv.Itoa(len(ports)) + "): ")
		choice := w.readLine()
		if choice == "q" || choice == "Q" {
			break
		}

		idx, err := strconv.Atoi(choice)
		if err == nil && idx >= 1 && idx <= len(ports) {
			port := ports[idx-1].Port
			l := w.configureSerialListener(port, len(cfg.Listeners)+1)
			cfg.AddListener(l)
			fmt.Printf("  已添加: %s -> :%d\n", l.SerialPort, l.ListenPort)
		}

		fmt.Println()
		fmt.Println("  已配置监听器:")
		for i, l := range cfg.Listeners {
			fmt.Printf("    %d. %s - %s (:%d)\n", i+1, l.Name, l.SerialPort, l.ListenPort)
		}
		fmt.Println()

		fmt.Print("  添加更多串口? (y/n): ")
		ans := w.readLine()
		if !strings.HasPrefix(strings.ToLower(ans), "y") {
			break
		}
	}

	// 配置完成，等待用户按 Enter 退出
	fmt.Println()
	fmt.Println("  ✓ 配置完成！")
	fmt.Println()
	w.PrintSummary(cfg)
	fmt.Println()
	w.WaitForEnter()

	return cfg, nil
}

// RunAddOnly 添加新配置（跳过确认提示，直接进入添加流程）
func (w *Wizard) RunAddOnly(cfg *config.Config) (*config.Config, error) {
	fmt.Println()
	fmt.Println("  Serial-Server 串口服务器配置向导")
	fmt.Println("  ─────────────────────────────────")

	// 显示现有配置
	if len(cfg.Listeners) > 0 {
		fmt.Println()
		fmt.Println("  检测到现有配置:")
		for i, l := range cfg.Listeners {
			fmt.Printf("    %d. %s - %s (:%d)\n", i+1, l.Name, l.SerialPort, l.ListenPort)
		}
		fmt.Println()
		fmt.Println("  添加新配置...")
		fmt.Println()
	}

	// 直接扫描串口（跳过确认提示）
	return w.runAddPorts(cfg)
}

// runAddPorts 执行添加串口的逻辑
func (w *Wizard) runAddPorts(cfg *config.Config) (*config.Config, error) {
	// Scan for serial ports
	fmt.Println("  扫描串口设备...")
	ports := w.scanPorts()
	if len(ports) == 0 {
		fmt.Println("  未找到串口设备!")
		return cfg, fmt.Errorf("no serial ports found")
	}

	fmt.Println()
	fmt.Println("  可用串口:")
	for i, p := range ports {
		// 检查是否已配置
		used := false
		for _, l := range cfg.Listeners {
			if l.SerialPort == p.Port {
				used = true
				break
			}
		}
		if used {
			fmt.Printf("    %d. %-15s - 已配置 %s\n", i+1, p.Port, emojiYes)
		} else {
			fmt.Printf("    %d. %-15s\n", i+1, p.Port)
		}
	}
	fmt.Println()

	// Add serial listeners
	for {
		fmt.Print("  选择串口 (1-" + strconv.Itoa(len(ports)) + ", q 退出): ")
		choice := w.readLine()
		if choice == "q" || choice == "Q" {
			break
		}

		idx, err := strconv.Atoi(choice)
		if err == nil && idx >= 1 && idx <= len(ports) {
			port := ports[idx-1].Port

			// 检查是否已配置
			for _, l := range cfg.Listeners {
				if l.SerialPort == port {
					fmt.Printf("  串口 %s %s已配置，请选择其他串口\n", port, emojiYes)
					port = ""
					break
				}
			}

			if port == "" {
				continue
			}

			l := w.configureSerialListener(port, len(cfg.Listeners)+1)
			cfg.AddListener(l)
			fmt.Printf("  已添加: %s -> :%d\n", l.SerialPort, l.ListenPort)
		}

		fmt.Println()
		fmt.Println("  已配置监听器:")
		for i, l := range cfg.Listeners {
			fmt.Printf("    %d. %s - %s (:%d)\n", i+1, l.Name, l.SerialPort, l.ListenPort)
		}
		fmt.Println()

		fmt.Print("  添加更多串口? (y/n): ")
		ans := w.readLine()
		if !strings.HasPrefix(strings.ToLower(ans), "y") {
			break
		}
	}

	// 配置完成
	fmt.Println()
	fmt.Println("  ✓ 配置完成！")
	fmt.Println()
	w.PrintSummary(cfg)
	fmt.Println()
	w.WaitForEnter()

	return cfg, nil
}

// selectPort lets user select a serial port.
func (w *Wizard) selectPort(ports []PortInfo) string {
	fmt.Print("  选择串口 (1-" + strconv.Itoa(len(ports)) + "): ")
	ans := w.readLine()

	idx, err := strconv.Atoi(ans)
	if err != nil || idx < 1 || idx > len(ports) {
		return ""
	}

	return ports[idx-1].Port
}

// configureSerialListener configures a serial listener.
func (w *Wizard) configureSerialListener(port string, num int) *config.ListenerConfig {
	l := &config.ListenerConfig{
		Name:          fmt.Sprintf("device_%d", num),
		SerialPort:    port,
		ListenPort:    8000 + num,
		BaudRate:      DefaultBaudRate,
		DataBits:      DefaultDataBits,
		StopBits:      DefaultStopBits,
		Parity:        DefaultParity,
		DisplayFormat: DefaultDisplayFormat,
	}

	fmt.Println()
	fmt.Printf("  配置串口: %s\n", port)
	fmt.Println()

	// Listen port
	fmt.Printf("  监听端口 (默认 %d，直接回车使用默认): ", l.ListenPort)
	if ans := w.readLine(); ans != "" {
		if port, err := strconv.Atoi(ans); err == nil && port > 0 && port <= 65535 {
			l.ListenPort = port
		}
	}
	fmt.Printf("  -> 使用: %d\n", l.ListenPort)

	// Baud rate
	fmt.Printf("  波特率 (默认 %d，直接回车使用默认): ", l.BaudRate)
	if ans := w.readLine(); ans != "" {
		if rate, err := strconv.Atoi(ans); err == nil && rate > 0 {
			l.BaudRate = rate
		}
	}
	fmt.Printf("  -> 使用: %d\n", l.BaudRate)

	// Parity
	fmt.Printf("  校验位 (默认 %s，直接回车使用默认):\n", l.Parity)
	fmt.Println("    N - 无校验 (None)")
	fmt.Println("    O - 奇校验 (Odd)")
	fmt.Println("    E - 偶校验 (Even)")
	fmt.Print("    选择: ")
	ans := w.readLine()
	if ans != "" {
		upper := strings.ToUpper(ans)
		if upper == "N" || upper == "O" || upper == "E" {
			l.Parity = upper
		}
	}
	fmt.Printf("  -> 使用: %s\n", l.Parity)

	// Data bits
	fmt.Printf("  数据位 (默认 %d，直接回车使用默认): ", l.DataBits)
	if ans := w.readLine(); ans != "" {
		if bits, err := strconv.Atoi(ans); err == nil && bits >= 5 && bits <= 8 {
			l.DataBits = bits
		}
	}
	fmt.Printf("  -> 使用: %d\n", l.DataBits)

	// Stop bits
	fmt.Printf("  停止位 (默认 %d，直接回车使用默认): ", l.StopBits)
	if ans := w.readLine(); ans != "" {
		if bits, err := strconv.Atoi(ans); err == nil && bits >= 1 && bits <= 2 {
			l.StopBits = bits
		}
	}
	fmt.Printf("  -> 使用: %d\n", l.StopBits)

	// Display format
	fmt.Println()
	fmt.Printf("  显示格式 (默认 %s，直接回车使用默认):\n", l.DisplayFormat)
	fmt.Println("    1. HEX")
	fmt.Println("    2. UTF8")
	fmt.Println("    3. GB2312")
	fmt.Print("    选择: ")
	ans = w.readLine()
	switch ans {
	case "2":
		l.DisplayFormat = "UTF8"
	case "3":
		l.DisplayFormat = "GB2312"
	case "1":
		l.DisplayFormat = "HEX"
	}
	fmt.Printf("  -> 使用: %s\n", l.DisplayFormat)

	// Max clients
	fmt.Println()

	return l
}

// scanPorts scans for available serial ports.
func (w *Wizard) scanPorts() []PortInfo {
	var ports []PortInfo

	// 使用 serialhelper 扫描可用串口
	availablePorts := ScanAvailablePorts()

	for _, p := range availablePorts {
		desc := getPortDescription(p)
		ports = append(ports, PortInfo{Port: p, Desc: desc})
	}

	return ports
}

// readLine reads a line from stdin.
func (w *Wizard) readLine() string {
	line, _ := w.reader.ReadString('\n')
	line = strings.TrimSpace(line)
	return line
}

// readInt reads an integer from stdin.
func (w *Wizard) readInt(defaultVal int) int {
	line := w.readLine()
	if line == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(line)
	if err != nil {
		return defaultVal
	}
	return val
}

// SelectPortInteractive provides interactive port selection with auto-refresh.
func (w *Wizard) SelectPortInteractive() (string, error) {
	fmt.Println()
	fmt.Println("  扫描串口设备...")
	fmt.Println()
	fmt.Println("  可用串口:")
	fmt.Println("    (扫描中...)")
	fmt.Println()

	// Initial scan
	ports := w.scanPorts()

	fmt.Print("\r")
	for i := 0; i < 50; i++ {
		fmt.Print(" ")
	}
	fmt.Print("\r")

	if len(ports) == 0 {
		fmt.Println("  未找到串口设备")
		fmt.Println()
		fmt.Print("  请手动输入串口路径 (或直接回车跳过): ")
		port := w.readLine()
		if port == "" {
			return "", fmt.Errorf("no port selected")
		}
		return port, nil
	}

	fmt.Println("  可用串口:")
	for i, p := range ports {
		fmt.Printf("    %d. %-20s - %s\n", i+1, p.Port, p.Desc)
	}
	fmt.Println()

	fmt.Print("  选择串口 (1-" + strconv.Itoa(len(ports)) + "): ")
	ans := w.readLine()

	idx, err := strconv.Atoi(ans)
	if err != nil || idx < 1 || idx > len(ports) {
		return "", fmt.Errorf("invalid selection")
	}

	return ports[idx-1].Port, nil
}

// WaitForEnter waits for user to press Enter.
func (w *Wizard) WaitForEnter() {
	fmt.Print("  按 Enter 继续...")
	w.readLine()
}

// PrintSummary prints a summary of the configuration.
func (w *Wizard) PrintSummary(cfg *config.Config) {
	fmt.Println()
	fmt.Println("  配置摘要:")
	fmt.Println("  ───────")

	for i, l := range cfg.Listeners {
		fmt.Printf("    %d. %s\n", i+1, l.Name)
		fmt.Printf("       串口: %s\n", l.SerialPort)
		fmt.Printf("       监听端口: %d\n", l.ListenPort)
		fmt.Printf("       波特率: %d\n", l.BaudRate)
		fmt.Printf("       校验位: %s\n", l.Parity)
		fmt.Printf("       数据位: %d\n", l.DataBits)
		fmt.Printf("       停止位: %d\n", l.StopBits)
		fmt.Printf("       显示格式: %s\n", l.DisplayFormat)
		fmt.Println()
	}
}

// ScanAvailablePorts scans for available serial ports
func ScanAvailablePorts() []string {
	return listener.ScanAvailablePorts()
}

// getPortDescription returns a description for a serial port
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

// contains checks if substr is in s (case sensitive)
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
