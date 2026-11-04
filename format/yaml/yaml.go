// Package yaml 提供基于 go.yaml.in/yaml/v4 的 YAML 格式实现,导入时自动注册到 format 注册表。
//
// 解析使用 v4 的 NewLoader(而非顶层 Unmarshal),先得到带位置信息的节点树,
// 再自行转换为中间表示,这样可以精确控制锚点、别名、非字符串 key 的处理方式,
// 并在严格模式下给出带行列号的错误。输出使用 NewDumper。
package yaml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	yamlv4 "go.yaml.in/yaml/v4"

	"github.com/taodev/goyj/format"
)

func init() {
	format.Register(Format{})
}

// Format 是 YAML 格式的 format.Format 实现。
type Format struct{}

// Name 返回 "yaml"。
func (Format) Name() string { return "yaml" }

// Extensions 返回 YAML 的文件后缀。
func (Format) Extensions() []string { return []string{".yaml", ".yml"} }

// Decode 读取 r 中以 "---" 分隔的全部 YAML 文档。
//
// 宽松模式(默认):锚点与别名自动展开,合并键 "<<" 被展开,注释丢弃,
// 非字符串 key 转为字符串,重复 key 以后出现者为准。
// 严格模式:遇到上述任何无法无损转换为 JSON 的特性时返回 *format.StrictError。
// 只有注释的输入不含任何文档,返回空切片。
func (Format) Decode(r io.Reader, opts *format.Options) ([]any, error) {
	o := format.Resolve(opts)
	loader, err := yamlv4.NewLoader(r, yamlv4.WithV4Defaults())
	if err != nil {
		return nil, fmt.Errorf("YAML 解析器初始化失败:%w", err)
	}
	docs := []any{}
	for {
		var n yamlv4.Node
		if err := loader.Load(&n); err != nil {
			if errors.Is(err, io.EOF) {
				return docs, nil
			}
			return nil, fmt.Errorf("YAML 解析失败:%w", err)
		}
		c := converter{strict: o.Strict}
		doc, err := c.convertDocument(&n)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
}

// Encode 把 doc 序列化为 YAML 并写入 w。缩进宽度取自 opts.Indent;opts.Compact 对 YAML 无效。
func (Format) Encode(w io.Writer, doc any, opts *format.Options) error {
	o := format.Resolve(opts)
	if err := o.Validate(); err != nil {
		return err
	}
	d, err := yamlv4.NewDumper(w, yamlv4.WithV4Defaults(), yamlv4.WithIndent(o.Indent),
		// 序列项相对父键缩进,与 kubectl、yq 等常见工具的输出风格一致。
		yamlv4.WithCompactSeqIndent(false))
	if err != nil {
		return fmt.Errorf("YAML 编码器初始化失败:%w", err)
	}
	if err := d.Dump(prepare(doc)); err != nil {
		return fmt.Errorf("YAML 编码失败:%w", err)
	}
	if err := d.Close(); err != nil {
		return fmt.Errorf("YAML 编码失败:%w", err)
	}
	return nil
}

// Join 把多个已编码的文档合并为以 "---" 分隔的多文档流。
func (Format) Join(docs [][]byte) ([]byte, error) {
	var out bytes.Buffer
	for i, d := range docs {
		if i > 0 {
			out.WriteString("---\n")
		}
		out.Write(d)
		if len(d) > 0 && d[len(d)-1] != '\n' {
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), nil
}

// quotedString 让 Dumper 把字符串强制输出为双引号风格。
type quotedString string

// MarshalYAML 实现 yaml.Marshaler。
func (q quotedString) MarshalYAML() (any, error) {
	return &yamlv4.Node{
		Kind:  yamlv4.ScalarNode,
		Tag:   "!!str",
		Value: string(q),
		Style: yamlv4.DoubleQuotedStyle,
	}, nil
}

// prepare 复制文档树,并把 Dumper 无法正确往返的字符串替换为强制双引号的 quotedString。
//
// 已知问题:go.yaml.in/yaml/v4 v4.0.0-rc.6 对多行字符串默认使用 "|" 字面块风格,
// 但当字符串以换行、空格或制表符开头时,输出缺少缩进指示符,读回来会静默丢失
// 空白字符甚至解析失败(例如 "\n 0" 读回为 "\n0")。这类字符串改用双引号即可无损往返。
func prepare(v any) any {
	switch x := v.(type) {
	case string:
		if needsQuoting(x) {
			return quotedString(x)
		}
		return x
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = prepare(e)
		}
		return out
	case map[string]any:
		unsafeKey := false
		for k := range x {
			if needsQuoting(k) {
				unsafeKey = true
				break
			}
		}
		if unsafeKey {
			// key 是 string 类型,无法直接包装;改用 map[any]any,让不安全的 key 变成 quotedString。
			out := make(map[any]any, len(x))
			for k, e := range x {
				if needsQuoting(k) {
					out[quotedString(k)] = prepare(e)
				} else {
					out[k] = prepare(e)
				}
			}
			return out
		}
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = prepare(e)
		}
		return out
	}
	return v
}

// needsQuoting 判断字符串是否会被 Dumper 输出为不能无损往返的字面块:
// 含有换行,且以空白字符(空格、制表符、换行、\u2028 等)开头。
func needsQuoting(s string) bool {
	if !strings.Contains(s, "\n") {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsSpace(r)
}
