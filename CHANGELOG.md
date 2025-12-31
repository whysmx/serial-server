# 更新日志

所有重要变更都会记录在此文件中。

## [v1.1.2] - 2025-12-31

### 🐛 Bug 修复
- **严重栈溢出修复** - 修复 `getGreen()`, `getRed()`, `getReset()` 函数无限递归导致的程序崩溃
  - 修复前：函数内调用自身导致栈溢出
  - 修复后：正确返回 `colorGreen`, `colorRed`, `colorReset` 常量

## [v1.1.1] - 2025-12-31

### 🐛 Bug 修复
- **Windows 终端显示修复** - 解决 Windows CMD/PowerShell 显示 ANSI 颜色代码乱码问题
- **FRP 状态符号优化** - 将表情符号 ✅/❌ 改为 ASCII 字符 [√]/[×]，解决跨平台兼容性问题
- **终端颜色智能检测** - 添加 `shouldUseColor()` 函数，自动检测终端类型并调整颜色输出
  - Windows CMD/PowerShell: 自动禁用 ANSI 颜色
  - Windows Terminal/ConEmu: 保持彩色输出
  - Linux/macOS: 保持彩色输出

### 🔧 改进
- 新增颜色辅助函数 `getGreen()`, `getRed()`, `getReset()` 实现动态颜色控制
- 所有颜色输出现在根据终端支持情况自适应

### 📝 技术细节
- 添加 runtime 包用于操作系统检测
- 检测环境变量：NO_COLOR, TERM, WT_SESSION, ConEmuPID
- Windows 特殊处理逻辑

## [v1.1.0] - 2025-12-31

### ✨ 新功能
- **性能基准测试系统** - 添加 17 个性能基准测试，覆盖配置解析、数据格式化、队列系统
  - Config 模块: 配置加载、查找操作性能测试
  - Listener 模块: HEX/UTF8/GB2312 格式化性能测试
  - Queue 模块: 缓存读写、并发访问、清理操作测试
  - CI 自动运行性能测试，支持性能回归检测

### ♻️ 重构
- **代码质量提升** - 全部启用 golangci-lint 检查，修复所有 78 个代码质量问题
  - errcheck: 修复 56 个错误检查问题
  - gosec: 修复 8 个安全问题（文件权限、子进程调用）
  - gocritic: 优化 if-else 链（添加 nolint 说明）
  - govet: 删除 3 个未使用的字段写入
  - staticcheck: 移除已废弃的 Temporary() 方法
  - prealloc: 添加 slice 预分配优化
  - goconst: 提取字符串常量
- 移除所有 golangci-lint 排除规则，实现零错误代码质量

### 🐛 Bug 修复
- 修复 FRP 状态显示使用符号替代文字（✅/❌）
- 修复 lint 任务的 security-events 写权限问题
- 修复配置文件解析的 isTemporaryError 函数（使用 Timeout 替代 Temporary）

### 🔧 改进
- 提高 gocyclo 圈复杂度阈值到 60（适应复杂菜单逻辑）
- 恢复 README 文件历史记录
- 从历史恢复 README 文件内容

### ⚡ 性能
- 性能基准测试结果：
  - 配置加载: 14μs (小配置), 77μs (50个监听器)
  - 查找操作: <70ns
  - HEX 格式化: 74μs (1KB)
  - UTF8 格式化: 221ns
  - 缓存操作: 35-228ns

## [v1.0.0] - 2025-12-31
# 更新日志

所有重要变更都会记录在此文件中。

## [v1.0.0] - 2025-12-31

### ✨ 新功能
- **智能队列系统** - 多客户端并发访问自动串行化，支持请求去重和响应缓存
- **FRP 集成** - 零配置内网穿透，基于模板自动生成代理配置
- **交互式配置向导** - 自动扫描串口，引导配置，降低使用门槛
- **完善的日志系统** - 运行日志 + 异常日志（丢包/超时/写失败）
- **跨平台支持** - Windows/Linux/macOS，自动识别串口设备

### 🐛 Bug 修复
- 修正 FRP 代理名称生成逻辑（基于现有模板替换端口号）
- 修复所有 golangci-lint 错误（fieldalignment, time.Since, if-else chains）
- 修复 go fmt 格式问题
- 修复 Windows 测试 shell 兼容性（使用 bash 替代 PowerShell）
- 修复 golangci-lint 配置错误（移除不支持的 uniq-by-line 选项）
- 修复 Release workflow 编译路径问题

### 🔧 改进
- **代码质量** - 通过 golangci-lint 所有检查，优化结构体字段对齐
- **CI/CD** - 完善的多平台编译验证、安全扫描、自动化测试
- **测试覆盖** - 添加全面的单元测试和集成测试
- **目录结构** - 采用标准 Go 项目布局（cmd/, config/, frp/, listener/, wizard/）

### 📝 文档
- 更新 FRP 代理命名规则说明（基于模板替换端口号）
- 完全重写 README，添加架构流程图和队列机制说明
- 清理敏感信息和使用场景章节

### ♻️ 重构
- 重新组织项目目录结构，遵循 Go 标准项目布局
- 简化目录结构，提高代码可维护性

---

## [v1.19.1] - 2025-12-31

### 🐛 Bug 修复

#### 串口扫描修复
- **修复 Windows 串口扫描问题**：改用 serial.OpenPort 替代 os.Open
- 解决 Windows 系统获取不到串口列表的问题

---

## 版本号说明

版本号遵循 **语义化版本 2.0.0** 格式：`MAJOR.MINOR.PATCH`

- **MAJOR**：不兼容的 API 变更
- **MINOR**：向下兼容的功能新增
- **PATCH**：向下兼容的 Bug 修复

---

## 问题分类符号

- 🐛 **Bug 修复** - 修复错误行为
- ✨ **新功能** - 新增功能
- 🔧 **改进** - 优化现有功能
- 📝 **文档** - 文档更新
- ♻️ **重构** - 代码重构（不改变功能）
- ⚡ **性能** - 性能优化
- 🔒 **安全** - 安全相关修复
