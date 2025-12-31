package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndSave(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.ini")

	// 创建测试配置
	cfg := &Config{
		Listeners: []*ListenerConfig{
			{
				Name:          "test_listener",
				SerialPort:    "/dev/ttyUSB0",
				ListenPort:    8000,
				BaudRate:      115200,
				DataBits:      8,
				StopBits:      1,
				Parity:        "N",
				DisplayFormat: "HEX",
			},
		},
	}

	// 测试保存
	err := Save(configPath, cfg)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// 测试加载
	loadedCfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 验证加载的数据
	if len(loadedCfg.Listeners) != 1 {
		t.Errorf("Expected 1 listener, got %d", len(loadedCfg.Listeners))
	}

	l := loadedCfg.Listeners[0]
	if l.Name != "test_listener" {
		t.Errorf("Expected name 'test_listener', got '%s'", l.Name)
	}
	if l.SerialPort != "/dev/ttyUSB0" {
		t.Errorf("Expected serial port '/dev/ttyUSB0', got '%s'", l.SerialPort)
	}
	if l.ListenPort != 8000 {
		t.Errorf("Expected listen port 8000, got %d", l.ListenPort)
	}
	if l.BaudRate != 115200 {
		t.Errorf("Expected baud rate 115200, got %d", l.BaudRate)
	}
	if l.DataBits != 8 {
		t.Errorf("Expected data bits 8, got %d", l.DataBits)
	}
	if l.StopBits != 1 {
		t.Errorf("Expected stop bits 1, got %d", l.StopBits)
	}
	if l.Parity != "N" {
		t.Errorf("Expected parity 'N', got '%s'", l.Parity)
	}
	if l.DisplayFormat != "HEX" {
		t.Errorf("Expected display format 'HEX', got '%s'", l.DisplayFormat)
	}
}

func TestLoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.ini")

	cfg, err := Load(configPath)
	// Load returns empty config for nonexistent files (design decision)
	if err != nil {
		t.Errorf("Expected no error for nonexistent file, got: %v", err)
	}
	if cfg == nil {
		t.Error("Expected empty config, got nil")
	}
	if len(cfg.Listeners) != 0 {
		t.Errorf("Expected empty listeners list, got %d", len(cfg.Listeners))
	}
}

func TestFindListenerByPort(t *testing.T) {
	cfg := &Config{
		Listeners: []*ListenerConfig{
			{Name: "listener1", ListenPort: 8000},
			{Name: "listener2", ListenPort: 8001},
		},
	}

	// 测试找到的监听器
	l := cfg.FindListenerByPort(8000)
	if l == nil {
		t.Error("Expected to find listener on port 8000, got nil")
	} else if l.Name != "listener1" {
		t.Errorf("Expected 'listener1', got '%s'", l.Name)
	}

	// 测试找不到的情况
	l = cfg.FindListenerByPort(9999)
	if l != nil {
		t.Error("Expected nil for nonexistent port, got listener")
	}
}

func TestFindListenerByName(t *testing.T) {
	cfg := &Config{
		Listeners: []*ListenerConfig{
			{Name: "listener1", ListenPort: 8000},
			{Name: "listener2", ListenPort: 8001},
		},
	}

	// 测试找到的监听器
	l := cfg.FindListenerByName("listener1")
	if l == nil {
		t.Error("Expected to find listener 'listener1', got nil")
	} else if l.ListenPort != 8000 {
		t.Errorf("Expected port 8000, got %d", l.ListenPort)
	}

	// 测试找不到的情况
	l = cfg.FindListenerByName("nonexistent")
	if l != nil {
		t.Error("Expected nil for nonexistent name, got listener")
	}
}

func TestAddListener(t *testing.T) {
	cfg := &Config{
		Listeners: []*ListenerConfig{
			{Name: "listener1", ListenPort: 8000},
		},
	}

	newListener := &ListenerConfig{
		Name:       "listener2",
		ListenPort: 8001,
	}

	cfg.AddListener(newListener)

	if len(cfg.Listeners) != 2 {
		t.Errorf("Expected 2 listeners, got %d", len(cfg.Listeners))
	}

	found := false
	for _, l := range cfg.Listeners {
		if l.Name == "listener2" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Listener was not added")
	}
}

func TestRemoveListener(t *testing.T) {
	cfg := &Config{
		Listeners: []*ListenerConfig{
			{Name: "listener1", ListenPort: 8000},
			{Name: "listener2", ListenPort: 8001},
			{Name: "listener3", ListenPort: 8002},
		},
	}

	cfg.RemoveListener("listener2")

	if len(cfg.Listeners) != 2 {
		t.Errorf("Expected 2 listeners after removal, got %d", len(cfg.Listeners))
	}

	// 验证正确的监听器被删除
	if cfg.FindListenerByName("listener2") != nil {
		t.Error("Listener 'listener2' was not removed")
	}

	// 验证其他监听器还在
	if cfg.FindListenerByName("listener1") == nil {
		t.Error("Listener 'listener1' was incorrectly removed")
	}
	if cfg.FindListenerByName("listener3") == nil {
		t.Error("Listener 'listener3' was incorrectly removed")
	}
}

func TestSaveMultipleListeners(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "multi_listeners.ini")

	cfg := &Config{
		Listeners: []*ListenerConfig{
			{
				Name:          "device1",
				SerialPort:    "/dev/ttyUSB0",
				ListenPort:    8000,
				BaudRate:      9600,
				DataBits:      8,
				StopBits:      1,
				Parity:        "N",
				DisplayFormat: "HEX",
			},
			{
				Name:          "device2",
				SerialPort:    "/dev/ttyUSB1",
				ListenPort:    8001,
				BaudRate:      115200,
				DataBits:      8,
				StopBits:      1,
				Parity:        "E",
				DisplayFormat: "UTF8",
			},
		},
	}

	err := Save(configPath, cfg)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 加载并验证
	loadedCfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loadedCfg.Listeners) != 2 {
		t.Fatalf("Expected 2 listeners, got %d", len(loadedCfg.Listeners))
	}

	// 验证第一个监听器
	l1 := loadedCfg.Listeners[0]
	if l1.Name != "device1" {
		t.Errorf("Expected 'device1', got '%s'", l1.Name)
	}
	if l1.BaudRate != 9600 {
		t.Errorf("Expected baud rate 9600, got %d", l1.BaudRate)
	}

	// 验证第二个监听器
	l2 := loadedCfg.Listeners[1]
	if l2.Name != "device2" {
		t.Errorf("Expected 'device2', got '%s'", l2.Name)
	}
	if l2.BaudRate != 115200 {
		t.Errorf("Expected baud rate 115200, got %d", l2.BaudRate)
	}
	if l2.Parity != "E" {
		t.Errorf("Expected parity 'E', got '%s'", l2.Parity)
	}
}
