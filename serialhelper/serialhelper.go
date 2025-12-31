// Package serialhelper provides serial port utilities for cross-platform support.
package serialhelper

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/tarm/serial"
)

// ComUsbPair 串口与USB设备映射关系
type ComUsbPair struct {
	Lock sync.RWMutex
	Data map[string]string // COM号 -> USB路径 (如 COM1 -> /dev/ttyUSB0)
}

var Default = NewComUsbPair()

func NewComUsbPair() *ComUsbPair {
	return &ComUsbPair{
		Data: make(map[string]string),
	}
}

func IsLinux() bool {
	return strings.Contains(strings.ToLower(runtime.GOOS), "linux")
}

func IsWindows() bool {
	return strings.Contains(strings.ToLower(runtime.GOOS), "windows")
}

// UpdateComAndUsbPair 更新串口与USB设备映射关系
func (c *ComUsbPair) UpdateComAndUsbPair() error {
	c.Lock.Lock()
	defer c.Lock.Unlock()

	if IsWindows() {
		c.Data = make(map[string]string)
		return nil
	}

	result, err := exec.Command("sh", "-c", "ls -l /dev/").Output()
	if err != nil {
		return err
	}

	c.Data = parseComUsbPair(string(result))
	return nil
}

// GetUsbFromCom 根据COM号获取对应的USB路径
func (c *ComUsbPair) GetUsbFromCom(comName string) string {
	c.Lock.RLock()
	defer c.Lock.RUnlock()

	if usbPath, ok := c.Data[comName]; ok && usbPath != "" {
		return usbPath
	}
	return comName
}

// GetAllComNames 获取所有COM号列表
func (c *ComUsbPair) GetAllComNames() []string {
	c.Lock.RLock()
	defer c.Lock.RUnlock()

	comNames := make([]string, 0, len(c.Data))
	for k := range c.Data {
		comNames = append(comNames, k)
	}
	return comNames
}

// parseComUsbPair 解析 ls -l /dev/ 输出
// 示例: lrwxrwxrwx ... COM1 -> ttyUSB0
func parseComUsbPair(output string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			continue
		}

		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])

		// 提取左侧最后一个词作为设备名
		fields := strings.Fields(left)
		if len(fields) == 0 {
			continue
		}
		deviceName := fields[len(fields)-1]

		// 只处理 COM* 和 RS485_*
		if !strings.HasPrefix(deviceName, "COM") && !strings.HasPrefix(deviceName, "RS485_") {
			continue
		}

		usbPath := right
		if !strings.HasPrefix(usbPath, "/dev/") {
			usbPath = "/dev/" + usbPath
		}
		result[deviceName] = usbPath
	}
	return result
}

// GetPortName 获取用于打开串口的实际端口名
func GetPortName(comName string, useOrgPortName bool) string {
	if IsWindows() {
		return comName
	}

	// 如果已经是完整路径，直接返回
	if strings.HasPrefix(comName, "/dev/") {
		return comName
	}

	if useOrgPortName {
		return "/dev/" + comName
	}

	// 先尝试从映射获取
	usbPath := Default.GetUsbFromCom(comName)
	if usbPath != comName {
		return usbPath
	}

	// 否则加上 /dev/ 前缀
	return "/dev/" + comName
}

// ScanAvailablePorts 扫描可用串口
// Linux: 返回 COM*、RS485_* 符号链接（不带 /dev/） + 底层串口设备（带 /dev/）
// Windows: 返回 COM1-COM256
func ScanAvailablePorts() []string {
	var ports []string

	if IsWindows() {
		for i := 1; i <= 256; i++ {
			portName := fmt.Sprintf("COM%d", i)
			// Windows 串口需要用 serial.Open 打开
			c := &serial.Config{Name: portName, Baud: 9600}
			if s, err := serial.OpenPort(c); err == nil {
				s.Close()
				ports = append(ports, portName)
			}
		}
	} else {
		// 扫描 COM* 和 RS485_* 符号链接（去掉 /dev/ 前缀）
		if matches, err := filepath.Glob("/dev/COM*"); err == nil {
			for _, m := range matches {
				ports = append(ports, strings.TrimPrefix(m, "/dev/"))
			}
		}
		if matches, err := filepath.Glob("/dev/RS485_*"); err == nil {
			for _, m := range matches {
				ports = append(ports, strings.TrimPrefix(m, "/dev/"))
			}
		}

		// 扫描底层串口设备（保留 /dev/ 前缀）
		serialPatterns := []string{
			"/dev/ttyUSB*",
			"/dev/ttyACM*",
			"/dev/ttyS*",
			"/dev/ttyFIQ*",
		}
		for _, pattern := range serialPatterns {
			if matches, err := filepath.Glob(pattern); err == nil {
				ports = append(ports, matches...)
			}
		}
	}
	return ports
}
