//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// startInBackground 后台运行程序（Windows 版本）
func startInBackground() {
	fmt.Fprintln(os.Stderr, "正在启动后台运行...")

	// 获取当前可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取可执行文件路径失败: %v\n", err)
		os.Exit(1)
	}

	// 获取配置文件路径
	configPath := findConfigFile(configFile)

	// 创建命令
	cmd := exec.Command(execPath, "-c", configPath)
	cmd.Dir = "."

	// 启动进程
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "启动后台进程失败: %v\n", err)
		os.Exit(1)
	}

	pid := cmd.Process.Pid

	// 等待一小段时间，确保进程成功启动
	time.Sleep(200 * time.Millisecond)

	// 检查进程是否还在运行
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		fmt.Fprintf(os.Stderr, "后台进程启动后立即退出\n")
		fmt.Fprintf(os.Stderr, "请检查日志文件\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "✓ 程序已在后台运行 (PID: %d)\n", pid)
	fmt.Fprintln(os.Stderr, "  使用任务管理器查看进程")
	fmt.Fprintf(os.Stderr, "  使用 'taskkill /F /PID %d' 停止程序\n", pid)
	os.Exit(0)
}
