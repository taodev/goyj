# REQUIREMENTS.md

## 项目概述

- **项目名**：goyj
- **Module 路径**：`github.com/taodev/goyj`
- **许可证**：MIT
- **语言**：Go，最低版本 Go 1.27
- **用途**：在 YAML 和 JSON 之间互相转换，同时提供 CLI 工具和可复用的 Go 库
- **目标用户**：所有 Go 开发者、运维和 DevOps 人员

## 依赖

| 用途 | 依赖 |
|------|------|
| YAML 解析与序列化 | `go.yaml.in/yaml/v4` `v4.0.0-rc.6`（预发布版本，需固定版本号） |
| JSON 解析与序列化 | 标准库 `encoding/json/v2`（Go 1.27 起正式可用，无需 `GOEXPERIMENT`） |
| CLI 参数解析 | 第三方库，由实现者选择（推荐 `github.com/spf13/cobra`） |

注意：YAML 解析应使用 v4 的 `NewLoader` / `NewDumper`，不要使用顶层的 `Marshal` / `Unmarshal`，因为后者使用 v3 默认值。

## 功能需求

### 1. 转换方向

- 支持 YAML → JSON、JSON → YAML。
- 默认根据文件后缀自动判断输入格式：`.yaml`、`.yml` 为 YAML，`.json` 为 JSON。
- 允许通过 `--from` 和 `--to` 强制指定格式，覆盖自动判断。

### 2. 输入输出方式

- 支持单个文件输入、标准输入（stdin）、标准输出（stdout）。
- 支持目录批量转换。
- 标准输入时**必须**通过 `--from` 指定输入格式，否则报错退出，不做内容推断。
- 标准输出时，输出目标格式由 `--to` 指定；未指定时，若输入来自文件，则由目标后缀推断。

### 3. 输出路径规则

- 指定 `-o <目录>` 时：输出到该目录，保持原目录结构，原文件不改动。
- 未指定 `-o` 时：在原目录生成同名、新后缀的文件（如 `a.yaml` 生成 `a.json`），原文件保留。
- 批量转换时，单个文件失败不应中断整个批处理，最后汇总报告失败项，并以非零状态码退出。

### 4. 覆盖策略

- 输出文件已存在时，默认进入交互式确认，询问是否覆盖。
- 使用 `--yes` 或 `-y` 跳过确认，直接覆盖。
- 非交互环境（如 CI、管道）下若未指定 `--yes`，应报错退出，而不是挂起等待输入。

### 5. 多文档 YAML

- YAML 中用 `---` 分隔的多个文档，每个文档单独输出。
- 写入文件时：生成多个文件，文件名带序号，如 `a-1.json`、`a-2.json`。
- 输出到标准输出时：使用 JSON Lines 格式，每个文档占一行。

### 6. 严格模式与宽松模式

- **默认宽松模式**：尽量完成转换。
  - YAML 锚点（`&`）和别名（`*`）自动展开。
  - 注释直接丢弃。
  - 非字符串 key（如 `1: a`、`true: b`）自动转成字符串 key（`"1"`、`"true"`）。
- **严格模式（`--strict`）**：遇到无法无损转换的特性时报错退出，并给出明确提示。
  - 非字符串 key 在严格模式下报错，提示该 key 无法无损转换为 JSON 字符串 key。
  - 其他无法无损转换的特性同样报错。

### 7. 输出格式选项

- JSON 和 YAML 输出都支持以下选项：
  - `--indent N`：缩进宽度，默认 2。
  - `--compact`：JSON 紧凑单行输出（仅 JSON 生效）。
- JSON 输出不保证 key 顺序，不做特殊排序，使用 Go 默认的无序 map。

### 8. 格式扩展

- 定义 `Format` 接口（包含 `Name`、`Extensions`、`Decode`、`Encode` 方法），通过全局注册表 `Register` 注册格式。
- 当前只实现 YAML 和 JSON。
- 新增格式（如 TOML）时，只需实现接口并注册，不修改已有代码。

### 9. 公开库 API

```go
func YAMLToJSON(data []byte, opts ...Option) ([]byte, error)
func JSONToYAML(data []byte, opts ...Option) ([]byte, error)
```

- `Option` 为函数式选项，至少包括：严格模式、缩进宽度、紧凑输出。
- 输入输出均为字节切片，不要求流式接口。

### 10. CLI 参数总表

| 参数 | 说明 |
|------|------|
| `<input>...` | 输入文件或目录，可多个 |
| `-o, --output <目录>` | 输出目录，不指定则在原目录生成新后缀文件 |
| `--from <yaml\|json>` | 强制指定输入格式，stdin 时必须指定 |
| `--to <yaml\|json>` | 强制指定输出格式 |
| `--strict` | 严格模式 |
| `--indent N` | 缩进宽度，默认 2 |
| `--compact` | JSON 紧凑单行输出 |
| `-y, --yes` | 覆盖时跳过确认 |
| `--version` | 显示版本号 |

## 非功能需求

- **测试**：
  - 单元测试与表驱动测试。
  - 覆盖 YAML 边界用例：锚点、别名、多文档、非字符串 key、注释、空文档。
  - 包含 Go 模糊测试（`go test -fuzz`）。
  - 包含基准测试（benchmark）。
- **CI**：GitHub Actions，至少运行构建、测试、lint。
- **发布**：使用 goreleaser 发布多平台二进制，平台为 Linux、macOS、Windows，架构为 amd64 和 arm64。
- **分发**：
  - 发布到 Scoop（Windows 包管理器），bucket 仓库为 `taodev/scoop-bucket`，由 goreleaser 在每次发版时自动更新 manifest。
- **文档**：README 包含安装、CLI 用法、库用法示例、与 `sclevine/yj` 的差异说明。
- **许可证**：MIT，仓库根目录包含 `LICENSE` 文件。

## 退出码

- `0`：全部成功。
- `1`：转换或 I/O 错误。
- `2`：参数错误，例如 stdin 未指定 `--from`，或非交互环境下未指定 `--yes`。