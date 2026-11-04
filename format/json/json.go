// Package json 提供基于标准库 encoding/json/v2 的 JSON 格式实现,导入时自动注册到 format 注册表。
package json

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/taodev/goyj/format"
)

func init() {
	format.Register(Format{})
}

// Format 是 JSON 格式的 format.Format 实现。
type Format struct{}

// Name 返回 "json"。
func (Format) Name() string { return "json" }

// Extensions 返回 JSON 的文件后缀。
func (Format) Extensions() []string { return []string{".json"} }

// Decode 读取 r 中的全部 JSON 值。
// 输入可以包含多个以空白分隔的顶层值(例如 JSON Lines),每个值作为一个文档。
// 整数保持为 int64/uint64,避免经过 float64 造成精度丢失;重复的对象键会报错。
// JSON 本身可以无损映射到中间表示,因此严格模式没有额外检查。
func (Format) Decode(r io.Reader, opts *format.Options) ([]any, error) {
	dec := jsontext.NewDecoder(r)
	docs := []any{}
	for {
		if dec.PeekKind() == 0 {
			// 没有更多的值:区分正常结束与语法错误。
			if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
				return nil, wrap(err)
			}
			return docs, nil
		}
		v, err := readValue(dec)
		if err != nil {
			return nil, wrap(err)
		}
		docs = append(docs, v)
	}
}

func wrap(err error) error {
	return fmt.Errorf("JSON 解析失败:%w", err)
}

// readValue 从解码器中读取一个完整的值并构造中间表示。
func readValue(d *jsontext.Decoder) (any, error) {
	tok, err := d.ReadToken()
	if err != nil {
		return nil, err
	}
	switch tok.Kind() {
	case 'n':
		return nil, nil
	case 't', 'f':
		return tok.Bool(), nil
	case '"':
		return tok.String(), nil
	case '0':
		return parseNumber(tok.String())
	case '{':
		m := map[string]any{}
		for d.PeekKind() != '}' {
			k, err := d.ReadToken()
			if err != nil {
				return nil, err
			}
			key := k.String() // Token 在下一次读取后失效,必须先取出字符串
			v, err := readValue(d)
			if err != nil {
				return nil, err
			}
			m[key] = v
		}
		if _, err := d.ReadToken(); err != nil { // 消费 '}'
			return nil, err
		}
		return m, nil
	case '[':
		s := []any{}
		for d.PeekKind() != ']' {
			v, err := readValue(d)
			if err != nil {
				return nil, err
			}
			s = append(s, v)
		}
		if _, err := d.ReadToken(); err != nil { // 消费 ']'
			return nil, err
		}
		return s, nil
	}
	return nil, fmt.Errorf("意外的 JSON 标记 %q", tok.Kind())
}

// parseNumber 按 int64、uint64、float64 的顺序尝试解析数字字面量。
func parseNumber(s string) (any, error) {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, nil
	}
	if u, err := strconv.ParseUint(s, 10, 64); err == nil {
		return u, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("数字 %s 超出 float64 的表示范围", s)
	}
	return f, nil
}

// Encode 把 doc 序列化为 JSON 并写入 w,输出以换行结尾。
// 默认使用 opts.Indent 个空格缩进,opts.Compact 为 true 时输出紧凑单行。
// 对象键按字典序输出,以保证结果可复现(需求不要求顺序,确定性输出便于测试和比对)。
func (Format) Encode(w io.Writer, doc any, opts *format.Options) error {
	o := format.Resolve(opts)
	if err := o.Validate(); err != nil {
		return err
	}
	mopts := []jsonv2.Options{jsonv2.Deterministic(true)}
	if !o.Compact {
		mopts = append(mopts, jsontext.WithIndent(strings.Repeat(" ", o.Indent)))
	}
	b, err := jsonv2.Marshal(doc, mopts...)
	if err != nil {
		return fmt.Errorf("JSON 编码失败:%w", err)
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// Join 把多个已编码的文档合并为 JSON Lines:每个文档压缩为单行,以换行分隔。
func (Format) Join(docs [][]byte) ([]byte, error) {
	var out bytes.Buffer
	for _, d := range docs {
		v := jsontext.Value(bytes.TrimSpace(d))
		if err := v.Compact(); err != nil {
			return nil, fmt.Errorf("JSON Lines 合并失败:%w", err)
		}
		out.Write(v)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}
