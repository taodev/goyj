// Package format 定义 goyj 的格式抽象:Format 接口、转换选项、全局注册表以及错误类型。
//
// 该包是依赖图的叶子节点,具体格式实现(format/yaml、format/json)和顶层包 goyj
// 都依赖它,从而避免顶层包与格式子包之间的循环引用。
// 顶层包 goyj 通过类型别名重新导出本包的核心类型,使用者通常无需直接导入本包。
package format

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

const (
	// DefaultIndent 是未指定缩进时使用的缩进宽度。
	DefaultIndent = 2
	// MinIndent 是允许的最小缩进宽度(YAML 库要求至少为 2)。
	MinIndent = 2
	// MaxIndent 是允许的最大缩进宽度(YAML 库要求至多为 9)。
	MaxIndent = 9
)

// Options 是 Decode 与 Encode 共用的转换选项。
// 零值等价于默认选项:宽松模式、缩进 2、非紧凑输出。
type Options struct {
	// Strict 为 true 时启用严格模式:遇到无法无损转换的特性时返回 *StrictError。
	Strict bool
	// Indent 是输出缩进宽度,0 表示使用 DefaultIndent,合法范围为 MinIndent~MaxIndent。
	Indent int
	// Compact 为 true 时 JSON 输出为紧凑单行,对其他格式无效。
	Compact bool
}

// Resolve 返回填充了默认值的选项副本。opts 为 nil 时返回默认选项。
func Resolve(opts *Options) Options {
	var o Options
	if opts != nil {
		o = *opts
	}
	if o.Indent == 0 {
		o.Indent = DefaultIndent
	}
	return o
}

// Validate 检查选项是否合法,应在填充默认值之后调用。
func (o Options) Validate() error {
	if o.Indent < MinIndent || o.Indent > MaxIndent {
		return fmt.Errorf("缩进宽度必须在 %d 到 %d 之间,实际为 %d", MinIndent, MaxIndent, o.Indent)
	}
	return nil
}

// Format 描述一种可以与其他格式互转的数据格式。
//
// 转换通过中间表示进行:Decode 把输入解析为若干文档,每个文档是由
// map[string]any、[]any、string、bool、nil 以及数值类型组成的树;
// Encode 把这样的树序列化为目标格式。新增格式只需实现该接口并调用 Register。
type Format interface {
	// Name 返回格式的唯一名称,例如 "yaml"。
	Name() string
	// Extensions 返回该格式的文件后缀(含前导点,小写),例如 [".yaml", ".yml"]。
	Extensions() []string
	// Decode 从 r 读取全部内容并解析为文档列表。
	// 输入中没有任何文档时返回空切片;opts 为 nil 时使用默认选项。
	Decode(r io.Reader, opts *Options) ([]any, error)
	// Encode 把单个文档序列化后写入 w,输出以换行结尾;opts 为 nil 时使用默认选项。
	Encode(w io.Writer, doc any, opts *Options) error
}

// Joiner 是 Format 的可选扩展,用于把多个已编码的文档合并为单个输出流。
// 例如 JSON 合并为 JSON Lines,YAML 合并为以 "---" 分隔的多文档流。
// 未实现该接口的格式,文档按顺序直接拼接。
type Joiner interface {
	Join(docs [][]byte) ([]byte, error)
}

// JoinDocs 把多个已编码的文档合并为单个输出流。
// 只有一个文档时原样返回;没有文档时返回 nil。
func JoinDocs(f Format, docs [][]byte) ([]byte, error) {
	switch len(docs) {
	case 0:
		return nil, nil
	case 1:
		return docs[0], nil
	}
	if j, ok := f.(Joiner); ok {
		return j.Join(docs)
	}
	var out []byte
	for _, d := range docs {
		out = append(out, d...)
	}
	return out, nil
}

var (
	mu       sync.RWMutex
	byName   = map[string]Format{}
	byExt    = map[string]Format{}
	registry []string // 按注册顺序保存的格式名称
)

// Register 把格式加入全局注册表。
// 名称或后缀与已注册的格式冲突、或 f 为 nil 时 panic,与 database/sql.Register 的约定一致。
// 通常在格式实现包的 init 函数中调用。
func Register(f Format) {
	if f == nil {
		panic("goyj/format: Register 的参数为 nil")
	}
	name := strings.ToLower(f.Name())
	if name == "" {
		panic("goyj/format: 格式名称不能为空")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := byName[name]; dup {
		panic("goyj/format: 重复注册格式 " + name)
	}
	exts := make([]string, 0, len(f.Extensions()))
	for _, e := range f.Extensions() {
		e = normalizeExt(e)
		if other, dup := byExt[e]; dup {
			panic(fmt.Sprintf("goyj/format: 后缀 %s 已被格式 %s 注册", e, other.Name()))
		}
		exts = append(exts, e)
	}
	byName[name] = f
	for _, e := range exts {
		byExt[e] = f
	}
	registry = append(registry, name)
}

// Lookup 按名称(不区分大小写)查找已注册的格式。
func Lookup(name string) (Format, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := byName[strings.ToLower(name)]
	return f, ok
}

// ByExt 按文件后缀查找已注册的格式。后缀不区分大小写,前导点可省略。
func ByExt(ext string) (Format, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := byExt[normalizeExt(ext)]
	return f, ok
}

// Names 返回所有已注册格式的名称,按字典序排列。
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := append([]string(nil), registry...)
	sort.Strings(names)
	return names
}

func normalizeExt(ext string) string {
	ext = strings.ToLower(ext)
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}

// StrictError 表示严格模式下发现的无法无损转换的特性。
type StrictError struct {
	// Line 和 Column 是问题在输入中的位置(从 1 开始),未知时为 0。
	Line, Column int
	// Path 是问题所在的数据路径,例如 "a.b[2]";根节点为空。
	Path string
	// Reason 说明为什么无法无损转换。
	Reason string
}

// Error 实现 error 接口。
func (e *StrictError) Error() string {
	var b strings.Builder
	b.WriteString("严格模式")
	if e.Line > 0 {
		fmt.Fprintf(&b, ":第 %d 行第 %d 列", e.Line, e.Column)
	}
	if e.Path != "" {
		fmt.Fprintf(&b, "(路径 %s)", e.Path)
	}
	b.WriteString(":")
	b.WriteString(e.Reason)
	return b.String()
}
