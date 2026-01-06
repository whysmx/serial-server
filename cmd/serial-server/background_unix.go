//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	// 使用 nohup 启动，设置环境变量告诉子进程直接启动
	// 不显示菜单，直接进入运行模式
	cmd := exec.Command("nohup", execPath, "-c", absConfigPath)
	cmd.Dir = wd

	// 设置环境变量，跳过菜单直接启动
	cmd.Env = append(os.Environ(), "SERIAL_SERVER_DAEMON=true")

	// 重定向标准输出和错误到 /dev/null
	devNull, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开 /dev/null 失败: %v\n", err)
		os.Exit(1)
	}
	defer devNull.Close()
	cmd.Stdout = devNull
	cmd.Stderr = devNull

	// 启动进程
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "启动后台进程失败: %v\n", err)
		os.Exit(1)
	}

	pid := cmd.Process.Pid

	fmt.Fprintf(os.Stderr, "✓ 程序已在后台运行 (PID: %d)\n", pid)
	fmt.Fprintln(os.Stderr, "  使用 'ps aux | grep serial-server' 查看进程")
	fmt.Fprintf(os.Stderr, "  使用 'kill %d' 或 'pkill serial-server' 停止程序\n", pid)
	fmt.Fprintln(os.Stderr, "  使用 'tail -f serial-server.log' 查看日志")
	os.Exit(0)
}
