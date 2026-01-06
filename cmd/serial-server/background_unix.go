//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// startInBackground 后台运行程序（Unix 版本）
func startInBackground() {
	fmt.Fprintln(os.Stderr, "正在启动后台运行...")

	// 获取当前可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取可执行文件路径失败: %v\n", err)
		os.Exit(1)
	}

	// 获取配置文件路径（绝对路径）
	configPath := findConfigFile(configFile)
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取配置文件绝对路径失败: %v\n", err)
		os.Exit(1)
	}

	// 获取当前工作目录
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取当前工作目录失败: %v\n", err)
		os.Exit(1)
	}

	// 创建命令
	cmd := exec.Command(execPath, "-c", absConfigPath)
	cmd.Dir = wd // 设置工作目录为当前目录

	// 设置进程属性，创建新的会话，完全脱离终端
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: false,
	}

	// 启动进程
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "启动后台进程失败: %v\n", err)
		os.Exit(1)
	}

	pid := cmd.Process.Pid

	// 等待一小段时间，确保进程成功启动
	time.Sleep(300 * time.Millisecond)

	// 检查进程是否还在运行
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		fmt.Fprintf(os.Stderr, "后台进程启动后立即退出\n")
		fmt.Fprintf(os.Stderr, "请检查日志: tail -n 20 serial-server.log\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "✓ 程序已在后台运行 (PID: %d)\n", pid)
	fmt.Fprintln(os.Stderr, "  使用 'ps aux | grep serial-server' 查看进程")
	fmt.Fprintf(os.Stderr, "  使用 'kill %d' 或 'pkill serial-server' 停止程序\n", pid)
	fmt.Fprintln(os.Stderr, "  使用 'tail -f serial-server.log' 查看日志")
	os.Exit(0)
}
