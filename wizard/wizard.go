// Package wizard provides interactive configuration wizard for serial-server.
package wizard

import (
	"serial-server/config"
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
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

		fmt.Print("  是否修改现有配置? (y/n): ")
		ans, _ := w.reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(ans), "y") {
			return cfg, nil
		}

		// Clear existing listeners
		cfg.Listeners = make([]*config.ListenerConfig, 0)
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
		BaudRate:      115200,
		DisplayFormat: "UTF8",
		MaxClients:    1,
	}

	fmt.Println()
	fmt.Printf("  配置串口: %s\n", port)
	fmt.Println()

	// Baud rate
	fmt.Printf("  波特率 (默认 %d): ", l.BaudRate)
	if ans := w.readLine(); ans != "" {
		if rate, err := strconv.Atoi(ans); err == nil && rate > 0 {
			l.BaudRate = rate
		}
	}

	// Listen port
	fmt.Printf("  监听端口 (默认 %d): ", l.ListenPort)
	if ans := w.readLine(); ans != "" {
		if port, err := strconv.Atoi(ans); err == nil && port > 0 && port <= 65535 {
			l.ListenPort = port
		}
	}

	// Display format
	fmt.Println()
	fmt.Println("  显示格式:")
	fmt.Println("    1. UTF8")
	fmt.Println("    2. GB2312")
	fmt.Println("    3. HEX")
	fmt.Print("  选择 (默认 1): ")
	ans := w.readLine()
	switch ans {
	case "2":
		l.DisplayFormat = "GB2312"
	case "3":
		l.DisplayFormat = "HEX"
	default:
		l.DisplayFormat = "UTF8"
	}

	// Max clients
	fmt.Println()
	fmt.Println("  客户端连接:")
	fmt.Println("    1. 单客户端 (新连接踢掉旧连接)")
	fmt.Println("    2. 多客户端 (允许多个连接)")
	fmt.Print("  选择 (默认 1): ")
	ans = w.readLine()
	if ans == "2" {
		l.MaxClients = 0
	}

	return l
}

// scanPorts scans for available serial ports.
func (w *Wizard) scanPorts() []PortInfo {
	var ports []PortInfo

	// Try to read from common port listing
	// This is a basic implementation - real world would use platform-specific methods

	// Check common locations
	possiblePorts := []string{
		"/dev/ttyUSB0",
		"/dev/ttyUSB1",
		"/dev/ttyUSB2",
		"/dev/ttyS0",
		"/dev/ttyS1",
		"/dev/ttyACM0",
		"/dev/cu.usbserial-1",
		"/dev/cu.usbserial-2",
		"/dev/cu.usbserial-3",
	}

	for _, p := range possiblePorts {
		if _, err := os.Stat(p); err == nil {
			desc := w.getPortDescription(p)
			ports = append(ports, PortInfo{Port: p, Desc: desc})
		}
	}

	return ports
}

// getPortDescription returns a description for a port.
func (w *Wizard) getPortDescription(port string) string {
	// Basic descriptions based on port name
	if strings.Contains(port, "usb") {
		return "USB 串口设备"
	}
	if strings.Contains(port, "ttyS") {
		return "标准串口"
	}
	if strings.Contains(port, "ttyACM") {
		return "USB CDC 设备"
	}
	return "串口设备"
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
		fmt.Printf("       波特率: %d\n", l.BaudRate)
		fmt.Printf("       监听端口: %d\n", l.ListenPort)
		fmt.Printf("       显示格式: %s\n", l.DisplayFormat)
		if l.MaxClients == 0 {
			fmt.Printf("       客户端: 多客户端\n")
		}
		fmt.Println()
	}
}
