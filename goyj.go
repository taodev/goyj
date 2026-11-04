// Package goyj 在 YAML 与 JSON 之间互相转换,同时提供可复用的 Go 库和命令行工具。
//
// 最常见的用法是两个字节切片函数:
//
//	out, err := goyj.YAMLToJSON(yamlData)
//	out, err := goyj.JSONToYAML(jsonData, goyj.WithIndent(4))
//
// 默认采用宽松模式:锚点与别名自动展开,注释丢弃,非字符串 key 转为字符串 key。
// 使用 WithStrict(true) 可在遇到无法无损转换的特性时报错。
//
// 多文档:YAML 中以 "---" 分隔的多个文档会按 JSON Lines 输出(每个文档一行);
// 反方向上,输入中多个顶层 JSON 值会输出为以 "---" 分隔的 YAML 多文档。
// 需要逐个文档处理时使用 ConvertDocs。
//
// 格式通过 Register 扩展,新增格式不需要修改本包。
package goyj

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/taodev/goyj/format"
)

// YAMLToJSON 把 YAML 数据转换为 JSON。
//
// 输入为空或只有注释时返回空结果。输入包含多个文档时,输出为 JSON Lines:
// 每个文档压缩为单行,以换行分隔。JSON 对象的键按字典序输出。
func YAMLToJSON(data []byte, opts ...Option) ([]byte, error) {
	return Convert(data, "yaml", "json", opts...)
}

// JSONToYAML 把 JSON 数据转换为 YAML。
//
// 输入为空时返回空结果。输入包含多个顶层 JSON 值(例如 JSON Lines)时,
// 输出为以 "---" 分隔的 YAML 多文档。
func JSONToYAML(data []byte, opts ...Option) ([]byte, error) {
	return Convert(data, "json", "yaml", opts...)
}

// Convert 把 data 从格式 from 转换为格式 to,多个文档合并为单个输出流。
// from 与 to 是已注册格式的名称,例如 "yaml"、"json"。
func Convert(data []byte, from, to string, opts ...Option) ([]byte, error) {
	docs, err := ConvertDocs(data, from, to, opts...)
	if err != nil {
		return nil, err
	}
	dst, err := lookup(to)
	if err != nil {
		return nil, err
	}
	return format.JoinDocs(dst, docs)
}

// ConvertDocs 与 Convert 类似,但每个文档单独返回一份编码结果,适合需要分别写入多个文件的场景。
// 输入中没有任何文档时返回空切片。
func ConvertDocs(data []byte, from, to string, opts ...Option) ([][]byte, error) {
	o, err := resolveOptions(opts)
	if err != nil {
		return nil, err
	}
	src, err := lookup(from)
	if err != nil {
		return nil, err
	}
	dst, err := lookup(to)
	if err != nil {
		return nil, err
	}
	docs, err := src.Decode(bytes.NewReader(data), &o)
	if err != nil {
		return nil, err
	}
	out := make([][]byte, 0, len(docs))
	for _, doc := range docs {
		var buf bytes.Buffer
		if err := dst.Encode(&buf, doc, &o); err != nil {
			return nil, err
		}
		out = append(out, buf.Bytes())
	}
	return out, nil
}

// JoinDocs 把 ConvertDocs 返回的多个文档合并为格式 to 的单个输出流
// (JSON 合并为 JSON Lines,YAML 合并为以 "---" 分隔的多文档)。
func JoinDocs(to string, docs [][]byte) ([]byte, error) {
	dst, err := lookup(to)
	if err != nil {
		return nil, err
	}
	return format.JoinDocs(dst, docs)
}

func lookup(name string) (Format, error) {
	f, ok := format.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("未知格式 %q,可用格式:%s", name, strings.Join(format.Names(), "、"))
	}
	return f, nil
}
