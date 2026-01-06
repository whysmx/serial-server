// Package main - serial-server
package frp

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// FRP Dashboard 配置
const (
	FRPCAdminURL      = "http://localhost:7400"
	FRPCAdminUser     = "admin"
	FRPCAdminPassword = "admin"
)

// 日志符号 - 使用 ASCII 兼容字符
const (
	symOK   = "[OK]"   // 成功
	symERR  = "[ERR]"  // 错误
	symWARN = "[WARN]" // 警告
)

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
		// 检测是否在支持 ANSI 的终端中运行
		if os.Getenv("WT_SESSION") != "" || // Windows Terminal
			os.Getenv("ConEmuPID") != "" {   // ConEmu
			return true
		}
		// Windows CMD/PowerShell 默认不支持
		return false
	}

	// Linux/macOS 默认支持
	return true
}

var (
	useColor = shouldUseColor()
	colorOK  string
	colorERR string
	colorRST string
)

func init() {
	if useColor {
		colorOK = "\x1b[32m" // 绿色
		colorERR = "\x1b[31m" // 红色
		colorRST = "\x1b[0m"  // 重置
	}
}

// findFRPCConfigPath 通过查找 frpc 进程获取配置文件路径
func findFRPCConfigPath() (string, error) {
	// 使用 ps 命令查找 frpc 进程
	cmd := exec.Command("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("执行 ps 命令失败: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "frpc") && strings.Contains(line, "-c") {
			// 找到 frpc 进程，提取配置文件路径
			// 例如: frpc -c /home/forlinx/frp/frpc.ini
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "-c" && i+1 < len(parts) {
					configPath := parts[i+1]
					log.Printf("[FRP] 检测到 FRP 配置文件: %s", configPath)
					return configPath, nil
				}
			}
		}
	}

	return "", fmt.Errorf("未找到 frpc 进程")
}

// fixConfigPermissions 修复配置文件和目录权限
func fixConfigPermissions(configPath string) error {
	log.Printf("[FRP] ===== 开始自动修复权限 =====")
	log.Printf("[FRP]   配置文件: %s", configPath)

	// 获取配置文件所在目录
	configDir := configPath
	for strings.Count(configDir, "/") > strings.Count(strings.TrimRight(configPath, "/"), "/") {
		configDir = strings.TrimSuffix(configDir, "/")
	}
	if lastSlash := strings.LastIndex(configDir, "/"); lastSlash > 0 {
		configDir = configDir[:lastSlash]
	}
	log.Printf("[FRP]   配置目录: %s", configDir)

	// 修复目录权限为 755 (允许所有人进入)
	log.Printf("[FRP]   修复目录权限: chmod 755 %s", configDir)
	if err := os.Chmod(configDir, 0755); err != nil {
		log.Printf("[FRP] " + colorERR + "无法修改目录权限: %v", err)
		log.Printf("[FRP]   请手动执行: sudo chmod 755 %s", configDir)
	} else {
		log.Printf("[FRP]   " + colorOK + "目录权限已修复")
	}

	// 修复配置文件权限为 666 (允许所有人读写)
	log.Printf("[FRP]   修复文件权限: chmod 666 %s", configPath)
	if err := os.Chmod(configPath, 0666); err != nil {
		log.Printf("[FRP] " + colorERR + "无法修改文件权限: %v", err)
		log.Printf("[FRP]   请手动执行: sudo chmod 666 %s", configPath)
	} else {
		log.Printf("[FRP]   " + colorOK + "文件权限已修复")
	}

	log.Printf("[FRP] ===== 权限修复完成 =====")
	return nil
}

// isPermissionError 检查错误是否是权限错误
func isPermissionError(errMsg string) bool {
	return strings.Contains(errMsg, "permission denied") ||
		strings.Contains(errMsg, "open ") && strings.Contains(errMsg, ": permission denied")
}

// SafeProxyName 生成安全的 FRP 代理名称
// COM1_8001 -> SERIALSERVER_COM1_8001
// /dev/ttyUSB0_8002 -> SERIALSERVER_ttyUSB0_8002
func SafeProxyName(serialPort string, localPort int) string {
	// 清理串口名称：移除 /dev/ 前缀，移除所有非字母数字字符
	cleaned := strings.TrimPrefix(serialPort, "/dev/")
	cleaned = regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(cleaned, "_")

	// 生成格式：SERIALSERVER_<串口名>_<端口>
	return fmt.Sprintf("SERIALSERVER_%s_%d", cleaned, localPort)
}

// Client provides methods to interact with local FRP Dashboard API.
type Client struct {
	baseURL       string
	adminUser     string
	adminPassword string
	httpClient    *http.Client
}

// NewClient creates a new FRP client.
func NewClient() *Client {
	return &Client{
		baseURL:       FRPCAdminURL,
		adminUser:     FRPCAdminUser,
		adminPassword: FRPCAdminPassword,
		httpClient:    &http.Client{},
	}
}

// NewClientWithConfig creates a new FRP client with custom settings.
func NewClientWithConfig(baseURL, adminUser, adminPassword string) *Client {
	return &Client{
		baseURL:       baseURL,
		adminUser:     adminUser,
		adminPassword: adminPassword,
		httpClient:    &http.Client{},
	}
}

// GetConfig retrieves the current FRPC configuration.
func (c *Client) GetConfig() (string, error) {
	log.Printf("[FRP] 正在获取配置...")
	log.Printf("[FRP]   Dashboard URL: %s", c.baseURL)
	log.Printf("[FRP]   认证用户: %s", c.adminUser)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/config", nil)
	if err != nil {
		log.Printf("[FRP] " + colorERR + "创建请求失败: %v", err)
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(c.adminUser, c.adminPassword)

	log.Printf("[FRP] 发送 GET 请求到: %s/api/config", c.baseURL)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[FRP] " + colorERR + "连接 FRP Dashboard 失败: %v", err)
		log.Printf("[FRP]   请检查 FRP 服务是否运行在: %s", c.baseURL)
		return "", fmt.Errorf("连接 FRP Dashboard 失败: %w\n\n请检查:\n  1. FRP Dashboard 是否运行 (默认: http://localhost:7400)\n  2. 地址配置是否正确", err)
	}
	defer resp.Body.Close()

	log.Printf("[FRP] 收到响应: Status=%d, ContentLength=%d", resp.StatusCode, resp.ContentLength)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errMsg := string(body)
		log.Printf("[FRP] " + colorERR + "HTTP 错误: %d", resp.StatusCode)
		log.Printf("[FRP]   响应内容: %s", errMsg)

		// 检查是否是权限错误
		if isPermissionError(errMsg) {
			log.Printf("[FRP] " + colorERR + "检测到权限错误，尝试自动修复...")

			configPath, err := findFRPCConfigPath()
			if err != nil {
				log.Printf("[FRP] " + colorERR + "无法自动修复: %v", err)
				return "", fmt.Errorf("权限错误且无法自动修复: %s\n\n请手动执行:\n  sudo chmod 755 <frpc配置目录>\n  sudo chmod 666 <frpc配置文件>", errMsg)
			}

			// 修复权限
			if err := fixConfigPermissions(configPath); err != nil {
				return "", fmt.Errorf("修复权限失败: %w", err)
			}

			log.Printf("[FRP] 权限已修复，重试获取配置...")
			// 重试一次
			return c.GetConfig()
		}

		return "", fmt.Errorf("FRP Dashboard 返回错误: %d\n\n可能原因:\n  1. 认证失败 (默认: admin/admin)\n  2. Dashboard 地址不正确\n  3. FRP 服务未正常运行", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[FRP] " + colorERR + "读取响应失败: %v", err)
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("[FRP] " + colorOK + "成功获取配置，大小: %d 字节", len(body))
	return string(body), nil
}

// PutConfig uploads new FRPC configuration.
func (c *Client) PutConfig(config string) error {
	log.Printf("[FRP] 正在上传配置...")
	log.Printf("[FRP]   配置大小: %d 字节", len(config))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+"/api/config", strings.NewReader(config))
	if err != nil {
		log.Printf("[FRP] " + colorERR + "创建请求失败: %v", err)
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(c.adminUser, c.adminPassword)
	req.Header.Set("Content-Type", "text/plain")

	log.Printf("[FRP] 发送 PUT 请求到: %s/api/config", c.baseURL)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[FRP] " + colorERR + "上传配置失败: %v", err)
		return fmt.Errorf("failed to put config: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[FRP] 上传响应: Status=%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errMsg := string(body)
		log.Printf("[FRP] " + colorERR + "上传失败: HTTP %d", resp.StatusCode)
		log.Printf("[FRP]   响应内容: %s", errMsg)

		// 检查是否是权限错误
		if isPermissionError(errMsg) {
			log.Printf("[FRP] " + colorERR + "检测到权限错误，尝试自动修复...")

			configPath, err := findFRPCConfigPath()
			if err != nil {
				log.Printf("[FRP] " + colorERR + "无法自动修复: %v", err)
				return fmt.Errorf("权限错误且无法自动修复: %s\n\n请手动执行:\n  sudo chmod 755 <frpc配置目录>\n  sudo chmod 666 <frpc配置文件>", errMsg)
			}

			// 修复权限
			if err := fixConfigPermissions(configPath); err != nil {
				return fmt.Errorf("修复权限失败: %w", err)
			}

			log.Printf("[FRP] 权限已修复，重试上传配置...")
			// 重试一次
			return c.PutConfig(config)
		}

		return fmt.Errorf("failed to put config: %s", errMsg)
	}

	log.Printf("[FRP] " + colorOK + "配置上传成功")
	return nil
}

// reload triggers FRPC to reload the configuration.
func (c *Client) Reload() error {
	log.Printf("[FRP] 正在重新加载配置...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/reload", nil)
	if err != nil {
		log.Printf("[FRP] " + colorERR + "创建重载请求失败: %v", err)
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(c.adminUser, c.adminPassword)

	log.Printf("[FRP] 发送 GET 请求到: %s/api/reload", c.baseURL)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[FRP] " + colorERR + "重载失败: %v", err)
		return fmt.Errorf("failed to reload: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[FRP] 重载响应: Status=%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[FRP] " + colorERR + "重载失败: HTTP %d", resp.StatusCode)
		log.Printf("[FRP]   响应内容: %s", string(body))
		return fmt.Errorf("failed to reload: %s", string(body))
	}

	log.Printf("[FRP] " + colorOK + "配置重载成功")
	return nil
}

// FindFirstSTCPProxy finds the first STCP proxy in the config to use as a template.
func (c *Client) FindFirstSTCPProxy() (proxyName string, localIP string, localPort int, sk string, useEncryption bool, useCompression bool, err error) {
	config, err := c.GetConfig()
	if err != nil {
		return "", "", 0, "", false, false, err
	}

	lines := strings.Split(config, "\n")
	inSection := false
	currentSection := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sectionName := strings.Trim(line, "[]")
			if sectionName != "common" {
				inSection = true
				currentSection = sectionName
			} else {
				inSection = false
			}
			continue
		}

		if inSection {
			if strings.HasPrefix(line, "type = stcp") {
				// 找到 STCP 代理，返回section名，后续解析其他字段
				proxyName = currentSection
				break
			}
		}
	}

	if proxyName == "" {
		return "", "", 0, "", false, false, fmt.Errorf("no STCP proxy found")
	}

	// 解析模板的详细信息
	lines = strings.Split(config, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sectionName := strings.Trim(line, "[]")
			if sectionName == proxyName {
				// 解析这个 section 下的内容
				//nolint:gocritic // Complex menu logic - if-else chain is appropriate
				for _, l := range lines[i+1:] {
					l = strings.TrimSpace(l)
					if l == "" || strings.HasPrefix(l, "[") {
						break
					}
					if strings.HasPrefix(l, "local_ip = ") {
						localIP = strings.TrimPrefix(l, "local_ip = ")
					} else if strings.HasPrefix(l, "local_port = ") {
						_, _ = fmt.Sscanf(l, "local_port = %d", &localPort)
					} else if strings.HasPrefix(l, "sk = ") {
						sk = strings.TrimPrefix(l, "sk = ")
					} else if strings.HasPrefix(l, "use_encryption = ") {
						useEncryption = strings.TrimPrefix(l, "use_encryption = ") == "true"
					} else if strings.HasPrefix(l, "use_compression = ") {
						useCompression = strings.TrimPrefix(l, "use_compression = ") == "true"
					}
				}
				break
			}
		}
	}

	return proxyName, localIP, localPort, sk, useEncryption, useCompression, nil
}

// AddSTCPProxy adds a new STCP proxy based on the first STCP proxy template.
// The proxy name is generated by replacing the port number in the template name.
// Example: Template "R-3E86A81DFEA9-22" with port 8001 -> "R-3E86A81DFEA9-8001"
func (c *Client) AddSTCPProxy(serialPort string, newLocalPort int) error {
	log.Printf("[FRP] ===== 开始添加 STCP 代理 =====")
	log.Printf("[FRP]   串口: %s", serialPort)
	log.Printf("[FRP]   端口: %d", newLocalPort)

	templateName, localIP, _, sk, useEncryption, useCompression, err := c.FindFirstSTCPProxy()
	if err != nil {
		log.Printf("[FRP] " + colorERR + "查找 STCP 模板失败: %v", err)
		log.Printf("[FRP]   请确保 FRP 配置文件中至少有一个 STCP 类型的代理作为模板")
		return fmt.Errorf("查找 STCP 模板失败: %w\n\n请确保 FRP 配置文件中至少有一个 STCP 类型的代理作为模板", err)
	}

	log.Printf("[FRP] " + colorOK + "找到 STCP 模板:")
	log.Printf("[FRP]   模板名称: %s", templateName)
	log.Printf("[FRP]   Local IP: %s", localIP)
	log.Printf("[FRP]   SK: %s", sk)
	log.Printf("[FRP]   加密: %v, 压缩: %v", useEncryption, useCompression)

	// 获取当前配置
	config, err := c.GetConfig()
	if err != nil {
		log.Printf("[FRP] " + colorERR + "获取配置失败: %v", err)
		return fmt.Errorf("获取配置失败: %w", err)
	}

	// 检查是否已存在 local_port = newLocalPort 的代理
	if hasSerialServerProxy(config, newLocalPort) {
		log.Printf("[FRP] " + colorERR + "端口 %d 的代理已存在", newLocalPort)
		return fmt.Errorf("端口 %d 的串口代理已存在", newLocalPort)
	}

	// 从模板名称提取前缀（最后一个 - 之前的部分），然后替换端口号
	// 例如: "R-3E86A81DFEA9-22" -> "R-3E86A81DFEA9-8001"
	lastDash := strings.LastIndex(templateName, "-")
	var newName string
	if lastDash == -1 {
		// 模板名称中没有 -，直接追加端口
		newName = fmt.Sprintf("%s-%d", templateName, newLocalPort)
	} else {
		// 替换最后一个 - 后面的端口部分
		prefix := templateName[:lastDash]
		newName = fmt.Sprintf("%s-%d", prefix, newLocalPort)
	}

	log.Printf("[FRP] 生成新代理名称: %s", newName)

	// 构建新的代理配置段
	newProxySection := fmt.Sprintf("\n[%s]\n", newName)
	newProxySection += "type = stcp\n"
	newProxySection += fmt.Sprintf("sk = %s\n", sk)
	newProxySection += fmt.Sprintf("local_ip = %s\n", localIP)
	newProxySection += fmt.Sprintf("local_port = %d\n", newLocalPort)
	if useEncryption {
		newProxySection += "use_encryption = true\n"
	}
	if useCompression {
		newProxySection += "use_compression = true\n"
	}
	newProxySection += "my_serial_server = true\n"

	log.Printf("[FRP] 新代理配置:\n%s", newProxySection)

	// 追加到配置末尾
	newConfig := config + newProxySection

	// 上传新配置
	if err := c.PutConfig(newConfig); err != nil {
		log.Printf("[FRP] " + colorERR + "上传配置失败: %v", err)
		return fmt.Errorf("上传配置失败: %w", err)
	}

	// 重新加载配置
	if err := c.Reload(); err != nil {
		log.Printf("[FRP] " + colorERR + "重新加载配置失败: %v", err)
		return fmt.Errorf("重新加载配置失败: %w", err)
	}

	log.Printf("[FRP] " + colorOK + "STCP 代理添加成功: %s", newName)
	log.Printf("[FRP] ===== 代理添加完成 =====")
	return nil
}

// hasSerialServerProxy 检查配置中是否存在 my_serial_server 配置项且 local_port = localPort 的代理
func hasSerialServerProxy(config string, localPort int) bool {
	lines := strings.Split(config, "\n")
	inSerialServerSection := false
	localPortStr := fmt.Sprintf("local_port = %d", localPort)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		// 检查是否进入新 section
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSerialServerSection = false
			continue
		}

		// 检查是否在串口服务器 section 中且有 my_serial_server 标记
		if inSerialServerSection {
			if strings.HasPrefix(line, "local_port = ") {
				if strings.TrimSpace(line) == localPortStr {
					return true
				}
			}
		}

		// 检查是否是我们添加的代理配置（只要有 my_serial_server = xxx 就认为是）
		if strings.HasPrefix(line, "my_serial_server = ") {
			inSerialServerSection = true
		}
	}
	return false
}

// GetAllSerialServerProxies 获取所有串口服务器代理的名称和端口
func (c *Client) GetAllSerialServerProxies() ([]string, map[string]int, error) {
	config, err := c.GetConfig()
	if err != nil {
		return nil, nil, err
	}

	lines := strings.Split(config, "\n")
	type proxyInfo struct {
		name       string
		port       int
		hasMySerialServerMark bool
	}
	proxies := make(map[string]*proxyInfo)
	currentName := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		// 检查是否进入新 section
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentName = strings.Trim(line, "[]")
			if _, exists := proxies[currentName]; !exists {
				proxies[currentName] = &proxyInfo{name: currentName, port: 0, hasMySerialServerMark: false}
			}
			continue
		}

		// 检查是否是我们添加的代理配置
		if strings.HasPrefix(line, "my_serial_server = ") {
			if info, exists := proxies[currentName]; exists {
				info.hasMySerialServerMark = true
			}
			continue
		}

		// 解析端口号
		if strings.HasPrefix(line, "local_port = ") {
			var port int
			_, _ = fmt.Sscanf(line, "local_port = %d", &port)
			if info, exists := proxies[currentName]; exists && port > 0 {
				info.port = port
			}
		}
	}

	// 收集所有有 my_serial_server 标记的代理
	var proxyNames []string
	proxyPorts := make(map[string]int)
	for _, info := range proxies {
		if info.hasMySerialServerMark && info.port > 0 {
			proxyNames = append(proxyNames, info.name)
			proxyPorts[info.name] = info.port
			log.Printf("[DEBUG] 找到串口代理: %s -> 端口 %d", info.name, info.port)
		}
	}

	log.Printf("[DEBUG] 共找到 %d 个串口代理", len(proxyNames))
	return proxyNames, proxyPorts, nil
}

// RemoveSerialServerProxy 从配置中移除指定的串口服务器代理
func (c *Client) RemoveSerialServerProxy(proxyName string) error {
	config, err := c.GetConfig()
	if err != nil {
		return err
	}

	lines := strings.Split(config, "\n")
	newLines := make([]string, 0, len(lines))
	inSerialServerSection := false
	skipUntilNextSection := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 检查是否进入新 section
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sectionName := strings.Trim(line, "[]")
			if sectionName == proxyName && inSerialServerSection {
				skipUntilNextSection = true
				inSerialServerSection = false
				continue
			}
			skipUntilNextSection = false
			// 检查是否是串口服务器代理的 section
			inSerialServerSection = false
		}

		if skipUntilNextSection {
			continue
		}

		// 检查是否是我们添加的代理配置（只要有 my_serial_server = xxx 就认为是）
		if strings.HasPrefix(line, "my_serial_server = ") {
			inSerialServerSection = true
		}

		newLines = append(newLines, line)
	}

	newConfig := strings.Join(newLines, "\n")

	if err := c.PutConfig(newConfig); err != nil {
		return err
	}

	return c.Reload()
}
