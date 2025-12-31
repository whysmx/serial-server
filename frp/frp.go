// Package main - serial-server
package frp

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// FRP Dashboard 配置
const (
	FRPCAdminURL      = "http://localhost:7400"
	FRPCAdminUser     = "admin"
	FRPCAdminPassword = "admin"
)

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
	baseURL      string
	adminUser    string
	adminPassword string
	httpClient   *http.Client
}

// NewClient creates a new FRP client.
func NewClient() *Client {
	return &Client{
		baseURL:      FRPCAdminURL,
		adminUser:    FRPCAdminUser,
		adminPassword: FRPCAdminPassword,
		httpClient:   &http.Client{},
	}
}

// NewClientWithConfig creates a new FRP client with custom settings.
func NewClientWithConfig(baseURL, adminUser, adminPassword string) *Client {
	return &Client{
		baseURL:      baseURL,
		adminUser:    adminUser,
		adminPassword: adminPassword,
		httpClient:   &http.Client{},
	}
}

// getConfig retrieves the current FRPC configuration.
func (c *Client) GetConfig() (string, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/config", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(c.adminUser, c.adminPassword)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(body), nil
}

// putConfig uploads new FRPC configuration.
func (c *Client) PutConfig(config string) error {
	req, err := http.NewRequest("PUT", c.baseURL+"/api/config", strings.NewReader(config))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(c.adminUser, c.adminPassword)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to put config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to put config: %s", string(body))
	}

	return nil
}

// reload triggers FRPC to reload the configuration.
func (c *Client) Reload() error {
	req, err := http.NewRequest("GET", c.baseURL+"/api/reload", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(c.adminUser, c.adminPassword)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to reload: %s", string(body))
	}

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
				for _, l := range lines[i+1:] {
					l = strings.TrimSpace(l)
					if l == "" || strings.HasPrefix(l, "[") {
						break
					}
					if strings.HasPrefix(l, "local_ip = ") {
						localIP = strings.TrimPrefix(l, "local_ip = ")
					} else if strings.HasPrefix(l, "local_port = ") {
						fmt.Sscanf(l, "local_port = %d", &localPort)
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
func (c *Client) AddSTCPProxy(serialPort string, newLocalPort int) error {
	_, localIP, _, sk, useEncryption, useCompression, err := c.FindFirstSTCPProxy()
	if err != nil {
		return fmt.Errorf("failed to find STCP template: %w", err)
	}

	// 获取当前配置
	config, err := c.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// 检查是否已存在 local_port = newLocalPort 的代理
	if hasSerialServerProxy(config, newLocalPort) {
		return fmt.Errorf("端口 %d 的串口代理已存在", newLocalPort)
	}

	// 解析模板名称，获取前缀部分（最后一个-之前的所有内容）
	// 用于生成新的代理名称，保持原有命名规则
	lastDash := strings.LastIndex(serialPort, "-")
	var prefix string
	if lastDash == -1 {
		// 串口名称没有 -，用模板名称的前缀
		templateName, _, _, _, _, _, _ := c.FindFirstSTCPProxy()
		templateLastDash := strings.LastIndex(templateName, "-")
		if templateLastDash == -1 {
			prefix = "stcp"
		} else {
			prefix = templateName[:templateLastDash]
		}
	} else {
		// 从串口名称提取（如果有端口后缀）
		prefix = serialPort[:lastDash]
	}

	// 生成新的名称（保持原有规则）
	newName := fmt.Sprintf("%s-%d", prefix, newLocalPort)

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

	// 追加到配置末尾
	newConfig := config + newProxySection

	// 上传新配置
	if err := c.PutConfig(newConfig); err != nil {
		return fmt.Errorf("failed to put config: %w", err)
	}

	// 重新加载配置
	if err := c.Reload(); err != nil {
		return fmt.Errorf("failed to reload: %w", err)
	}

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
	var proxyNames []string
	proxyPorts := make(map[string]int)
	inSerialServerSection := false
	currentName := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		// 检查是否进入新 section
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSerialServerSection = false
			currentName = strings.Trim(line, "[]")
			continue
		}

		// 检查是否是我们添加的代理配置（只要有 my_serial_server = xxx 就认为是）
		if strings.HasPrefix(line, "my_serial_server = ") {
			inSerialServerSection = true
			continue
		}

		// 解析端口号
		if inSerialServerSection && strings.HasPrefix(line, "local_port = ") {
			var port int
			fmt.Sscanf(line, "local_port = %d", &port)
			if currentName != "" && port > 0 {
				proxyNames = append(proxyNames, currentName)
				proxyPorts[currentName] = port
			}
		}
	}

	return proxyNames, proxyPorts, nil
}

// RemoveSerialServerProxy 从配置中移除指定的串口服务器代理
func (c *Client) RemoveSerialServerProxy(proxyName string) error {
	config, err := c.GetConfig()
	if err != nil {
		return err
	}

	lines := strings.Split(config, "\n")
	var newLines []string
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

