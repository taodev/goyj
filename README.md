# README.md

# goyj

在 YAML 和 JSON 之间互相转换的 Go 工具，同时提供命令行（CLI）和可复用的 Go 库。

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

## 特性

- YAML ↔ JSON 双向转换
- 支持单文件、标准输入输出、目录批量转换
- 自动识别格式，也可以用 `--from` / `--to` 强制指定
- 多文档 YAML 支持（`---`）
- 默认宽松模式，`--strict` 切换为严格模式
- 可配置缩进宽度，JSON 支持紧凑输出
- 可作为 Go 库直接调用

## 安装

### 使用 Go 安装

需要 Go 1.27 或更高版本。

```bash
go install github.com/taodev/goyj/cmd/goyj@latest
```

### 使用 Scoop（Windows）

```powershell
scoop bucket add taodev https://github.com/taodev/scoop-bucket
scoop install goyj
```

### 下载二进制

前往 [Releases](https://github.com/taodev/goyj/releases) 下载对应平台的版本。

## 命令行用法

```bash
# YAML 转 JSON（根据后缀自动判断）
goyj config.yaml                 # 生成 config.json，原文件保留

# 指定输出目录
goyj -o out/ config.yaml         # 输出到 out/config.json

# 批量转换整个目录
goyj -o out/ configs/

# 标准输入（必须指定 --from）
cat config.yaml | goyj --from yaml --to json

# JSON 转 YAML，缩进 4 个空格
goyj --indent 4 data.json

# JSON 紧凑单行输出
goyj --compact --to json config.yaml

# 严格模式：遇到无法无损转换的特性时报错
goyj --strict config.yaml

# 覆盖已存在文件时跳过确认
goyj -y -o out/ configs/
```

### 参数

| 参数 | 说明 |
|------|------|
| `<input>...` | 输入文件或目录，可多个；目录会递归处理其中 `.yaml`、`.yml`、`.json` 文件 |
| `-o, --output <目录>` | 输出目录，保持相对目录结构；不指定则在原目录生成新后缀文件 |
| `--from <yaml\|json>` | 强制指定输入格式，从标准输入读取时必须指定 |
| `--to <yaml\|json>` | 强制指定输出格式；不指定则取与输入相反的格式 |
| `--strict` | 严格模式 |
| `--indent N` | 缩进宽度，范围 2~9，默认 2 |
| `--compact` | JSON 紧凑单行输出（对 YAML 输出无效） |
| `-y, --yes` | 覆盖已存在的文件时跳过确认 |
| `--version` | 显示版本号 |

### 输入与输出

- **文件输入**：输入格式由后缀推断（`.yaml`、`.yml` 为 YAML，`.json` 为 JSON），可用 `--from` 覆盖。
  无法识别的后缀且未指定 `--from` 时，该文件失败。
- **标准输入**：不带输入参数（或输入为 `-`）时从标准输入读取，结果写到标准输出。
  此时**必须**指定 `--from`，否则以退出码 2 报错；不能同时使用 `-o`。
- **输出位置**：文件输入总是写文件。未指定 `-o` 时在原目录生成同名新后缀文件（`a.yaml` → `a.json`），
  指定 `-o` 时输出到该目录，原文件始终不改动。
- **扫描目录**：指定 `--from` 时只处理该格式的文件；指定 `--to` 时跳过已经是目标格式的文件。
- **空输入**：没有任何文档的文件（空文件、只有注释）会被跳过，并在标准错误给出提示。

### 多文档 YAML

YAML 中用 `---` 分隔的多个文档会分别输出：

- 写入文件：生成 `a-1.json`、`a-2.json`……
- 输出到标准输出：使用 JSON Lines，每个文档一行。

反方向上，输入中包含多个顶层 JSON 值（例如 JSON Lines）时，会输出以 `---` 分隔的 YAML 多文档，
或写成 `a-1.yaml`、`a-2.yaml`……

### 覆盖确认

目标文件已存在时：

- 在终端中运行：逐个询问是否覆盖，可回答 `y`（是）、`n`（否，默认）、`a`（全部覆盖）、`q`（退出）。
- 指定 `-y` / `--yes`：直接覆盖，不询问。
- 非交互环境（标准输入不是终端，如 CI、管道）且未指定 `--yes`：**报错并以退出码 2 退出**，不会挂起等待输入。

覆盖确认在写出任何文件**之前**完成，因此遇到上述错误或中途退出时不会留下写了一半的结果。
goyj 也不会覆盖任何一个输入文件：输出路径与某个输入相同，或两个输入产生同一个输出时，对应文件会失败。

### 宽松模式与严格模式

默认是**宽松模式**，尽量完成转换：

- 锚点（`&`）与别名（`*`）自动展开，合并键（`<<`）被展开；
- 注释直接丢弃；
- 非字符串 key（如 `1: a`、`true: b`）转为字符串 key（`"1"`、`"true"`）；
- 重复 key 以最后出现的为准；
- `.inf`、`.nan` 这类 JSON 无法表示的浮点数保留为字符串；时间戳转为 RFC 3339 字符串。

`--strict` 切换为**严格模式**：遇到无法无损转换为 JSON 的特性时报错，并指出行列号、数据路径和原因：

```text
$ goyj --strict config.yaml

1 个输入处理失败:
  config.yaml: 严格模式:第 2 行第 1 列:非字符串 key "1"(类型 !!int)无法无损转换为 JSON 字符串 key
```

严格模式会拒绝：非字符串 key、锚点与别名、合并键、重复 key、时间戳、`.inf`/`.nan`、`!!binary`
以及自定义标签。注释虽然会丢失，但不属于数据，严格模式**不会**因此报错。

### 批量处理与失败汇总

单个文件失败不会中断整个批处理，其余文件继续转换，最后汇总所有失败项并以退出码 1 退出。

### 退出码

| 退出码 | 含义 |
|--------|------|
| `0` | 全部成功 |
| `1` | 转换或 I/O 错误（含批量处理中有文件失败） |
| `2` | 参数错误，例如标准输入未指定 `--from`、非交互环境下需要覆盖却未指定 `--yes` |

## 作为 Go 库使用

```bash
go get github.com/taodev/goyj
```

```go
package main

import (
    "fmt"
    "log"

    "github.com/taodev/goyj"
)

func main() {
    src := []byte("name: goyj\nversion: 1\n")

    out, err := goyj.YAMLToJSON(src, goyj.WithIndent(2))
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(string(out))
}
```

### API

```go
func YAMLToJSON(data []byte, opts ...Option) ([]byte, error)
func JSONToYAML(data []byte, opts ...Option) ([]byte, error)
```

| 选项 | 说明 |
|------|------|
| `WithStrict(bool)` | 严格模式 |
| `WithIndent(n)` | 缩进宽度，2~9，0 表示默认值 2；超出范围时转换函数返回错误 |
| `WithCompact(bool)` | JSON 紧凑单行输出 |

输入包含多个文档时，`YAMLToJSON` 输出 JSON Lines，`JSONToYAML` 输出以 `---` 分隔的多文档。
需要逐个文档处理时使用 `ConvertDocs`，它为每个文档返回一份独立的结果。

### 处理严格模式错误

严格模式的错误是 `*goyj.StrictError`，携带位置和原因，可以用 `errors.As` 取出：

```go
_, err := goyj.YAMLToJSON(data, goyj.WithStrict(true))

var se *goyj.StrictError
if errors.As(err, &se) {
    fmt.Printf("第 %d 行第 %d 列(%s):%s\n", se.Line, se.Column, se.Path, se.Reason)
}
```

### 扩展新格式

实现 `goyj.Format` 接口并调用 `goyj.Register` 即可，不需要修改已有代码：

```go
type Format interface {
    Name() string
    Extensions() []string
    Decode(r io.Reader, opts *Options) ([]any, error)
    Encode(w io.Writer, doc any, opts *Options) error
}
```

注册之后，可以通过 `goyj.Convert(data, "yaml", "toml")` 这样的方式按名称使用。
中间表示为 `map[string]any`、`[]any`、`string`、`bool`、`nil` 和各类数值。

## 与 sclevine/yj 的区别

[`sclevine/yj`](https://github.com/sclevine/yj) 是功能成熟的同类工具，支持 TOML、HCL，并保留 map 顺序。

`goyj` 的定位不同：

- 只专注 YAML 和 JSON，范围更单纯。
- JSON 输出使用 Go 1.27 的 `encoding/json/v2`。
- 提供严格模式，便于在 CI 中发现无法无损转换的内容。
- 不保留 JSON key 的原有顺序，输出时按字典序排列（结果稳定可复现）。如果需要保留顺序，请使用 `sclevine/yj`。
- 覆盖已有文件前会确认；非交互环境下不会挂起，而是以退出码 2 报错。

## 已知限制

- 依赖的 `go.yaml.in/yaml/v4` 目前是预发布版本（`v4.0.0-rc.6`），已固定版本号。
  该版本对以空白或换行开头的多行字符串会输出无法正确读回的 `|` 字面块，
  goyj 对这类字符串改用双引号输出以保证无损往返（见 `format/yaml` 中的 `prepare`）。
- 不保留注释、锚点名称和 YAML 中的格式风格。

## 开发

```bash
git clone https://github.com/taodev/goyj.git
cd goyj
go test ./...
go vet ./...

# 模糊测试（-fuzz 一次只能针对一个包；-fuzzminimizetime 避免最小化阶段长时间无输出）
go test -run '^$' -fuzz '^FuzzYAMLToJSON$' -fuzztime 30s -fuzzminimizetime 2s .
go test -run '^$' -fuzz '^FuzzJSONToYAML$' -fuzztime 30s -fuzzminimizetime 2s .

# 基准测试
go test -run '^$' -bench . -benchmem .
```

### 发布

推送形如 `v1.2.3` 的标签会触发 GitHub Actions 中的 goreleaser：构建 Linux、macOS、Windows
的 amd64 与 arm64 二进制，更新 [`taodev/scoop-bucket`](https://github.com/taodev/scoop-bucket)
中的 `goyj.json`。

发布前需要在仓库的 Secrets 中配置 `SCOOP_BUCKET_TOKEN`（对 `taodev/scoop-bucket` 有写权限的 token），
**不要**把它写入代码仓库。

## 许可证

本项目基于 [MIT 许可证](LICENSE) 发布。