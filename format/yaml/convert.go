package yaml

import (
	"fmt"
	"math"
	"strconv"

	yamlv4 "go.yaml.in/yaml/v4"

	"github.com/taodev/goyj/format"
)

// maxAliasNodes 限制通过别名展开产生的节点总数,防止 "十亿笑声" 式的别名炸弹耗尽内存。
const maxAliasNodes = 1_000_000

// converter 把 yaml 节点树转换为由 map[string]any、[]any 和标量组成的中间表示。
// 每个文档使用独立的 converter。
type converter struct {
	strict bool
	// expanding 记录正在展开的别名目标,用于检测递归别名。
	expanding map[*yamlv4.Node]bool
	// aliasDepth 大于 0 表示当前处于别名展开过程中。
	aliasDepth int
	// aliasNodes 统计别名展开期间访问的节点数。
	aliasNodes int
}

func (c *converter) convertDocument(n *yamlv4.Node) (any, error) {
	if n.Kind == yamlv4.DocumentNode {
		if len(n.Content) == 0 {
			return nil, nil
		}
		n = n.Content[0]
	}
	return c.convert(n, "")
}

// strictErr 构造指向节点 n 的严格模式错误。
func strictErr(n *yamlv4.Node, path, msgFmt string, args ...any) error {
	return &format.StrictError{Line: n.Line, Column: n.Column, Path: path, Reason: fmt.Sprintf(msgFmt, args...)}
}

// plainErr 构造带位置的普通转换错误(宽松模式下同样无法继续的情形)。
func plainErr(n *yamlv4.Node, path, msgFmt string, args ...any) error {
	msg := fmt.Sprintf(msgFmt, args...)
	if path != "" {
		msg = fmt.Sprintf("(路径 %s)%s", path, msg)
	}
	return fmt.Errorf("第 %d 行第 %d 列:%s", n.Line, n.Column, msg)
}

func (c *converter) convert(n *yamlv4.Node, path string) (any, error) {
	if c.aliasDepth > 0 {
		c.aliasNodes++
		if c.aliasNodes > maxAliasNodes {
			return nil, plainErr(n, path, "别名展开产生的节点数超过上限 %d,疑似别名炸弹", maxAliasNodes)
		}
	}
	if c.strict && n.Anchor != "" && n.Kind != yamlv4.AliasNode {
		return nil, strictErr(n, path, "锚点 &%s 在 JSON 中没有对应表示,展开后会丢失引用关系", n.Anchor)
	}
	switch n.Kind {
	case yamlv4.AliasNode:
		return c.convertAlias(n, path)
	case yamlv4.ScalarNode:
		return c.convertScalar(n, path)
	case yamlv4.SequenceNode:
		if err := c.checkCustomTag(n, path); err != nil {
			return nil, err
		}
		s := make([]any, 0, len(n.Content))
		for i, item := range n.Content {
			v, err := c.convert(item, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			s = append(s, v)
		}
		return s, nil
	case yamlv4.MappingNode:
		if err := c.checkCustomTag(n, path); err != nil {
			return nil, err
		}
		return c.convertMapping(n, path)
	case yamlv4.DocumentNode:
		return c.convertDocument(n)
	}
	return nil, plainErr(n, path, "不支持的 YAML 节点类型 %d", n.Kind)
}

func (c *converter) convertAlias(n *yamlv4.Node, path string) (any, error) {
	if c.strict {
		return nil, strictErr(n, path, "别名 *%s 在 JSON 中没有对应表示,展开后会丢失引用关系", n.Value)
	}
	target := n.Alias
	if target == nil {
		return nil, plainErr(n, path, "别名 *%s 指向未定义的锚点", n.Value)
	}
	if c.expanding[target] {
		return nil, plainErr(n, path, "别名 *%s 形成递归引用,无法展开", n.Value)
	}
	if c.expanding == nil {
		c.expanding = map[*yamlv4.Node]bool{}
	}
	c.expanding[target] = true
	c.aliasDepth++
	defer func() {
		delete(c.expanding, target)
		c.aliasDepth--
	}()
	return c.convert(target, path)
}

// checkCustomTag 在严格模式下拒绝序列和映射上的非标准标签。
func (c *converter) checkCustomTag(n *yamlv4.Node, path string) error {
	if !c.strict {
		return nil
	}
	switch n.Tag {
	case "", "!!seq", "!!map":
		return nil
	}
	return strictErr(n, path, "标签 %s 在 JSON 中没有对应表示", n.Tag)
}

func (c *converter) convertScalar(n *yamlv4.Node, path string) (any, error) {
	switch n.Tag {
	case "!!null":
		return nil, nil
	case "!!str":
		return n.Value, nil
	case "!!bool", "!!int":
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, plainErr(n, path, "无法解析标量 %q:%v", n.Value, err)
		}
		return v, nil
	case "!!float":
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, plainErr(n, path, "无法解析标量 %q:%v", n.Value, err)
		}
		if f, ok := v.(float64); ok && (math.IsInf(f, 0) || math.IsNaN(f)) {
			if c.strict {
				return nil, strictErr(n, path, "浮点数 %s 无法用 JSON 数字表示", n.Value)
			}
			return n.Value, nil // 宽松模式:保留原始文本为字符串
		}
		return v, nil
	case "!!timestamp":
		if c.strict {
			return nil, strictErr(n, path, "时间戳 %s 在 JSON 中没有对应类型,转换后会变成字符串", n.Value)
		}
		var v any
		if err := n.Decode(&v); err != nil {
			return n.Value, nil
		}
		return v, nil
	case "!!binary":
		if c.strict {
			return nil, strictErr(n, path, "二进制标量(!!binary)在 JSON 中没有对应类型")
		}
		var v any
		if err := n.Decode(&v); err != nil {
			return n.Value, nil
		}
		return v, nil
	}
	// 其他(自定义)标签:宽松模式忽略标签,保留文本。
	if c.strict {
		return nil, strictErr(n, path, "标签 %s 在 JSON 中没有对应表示", n.Tag)
	}
	return n.Value, nil
}

func (c *converter) convertMapping(n *yamlv4.Node, path string) (any, error) {
	m := make(map[string]any, len(n.Content)/2)
	var merges []*yamlv4.Node // 合并键的值节点,稍后以较低优先级合并

	for i := 0; i+1 < len(n.Content); i += 2 {
		kn, vn := n.Content[i], n.Content[i+1]

		if kn.Kind == yamlv4.ScalarNode && kn.Tag == "!!merge" {
			if c.strict {
				return nil, strictErr(kn, path, "合并键 << 在 JSON 中没有对应表示")
			}
			merges = append(merges, vn)
			continue
		}

		key, err := c.keyString(kn, path)
		if err != nil {
			return nil, err
		}
		childPath := key
		if path != "" {
			childPath = path + "." + key
		}
		if _, dup := m[key]; dup && c.strict {
			return nil, strictErr(kn, childPath, "重复的 key %q,JSON 对象不允许重复键", key)
		}
		v, err := c.convert(vn, childPath)
		if err != nil {
			return nil, err
		}
		m[key] = v
	}

	if len(merges) > 0 {
		if err := c.applyMerges(m, merges, path); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// applyMerges 把合并键引用的映射并入 m:显式键优先于合并键,序列中靠前的映射优先于靠后的。
func (c *converter) applyMerges(m map[string]any, merges []*yamlv4.Node, path string) error {
	merged := map[string]any{}
	add := func(src *yamlv4.Node) error {
		v, err := c.convert(src, path)
		if err != nil {
			return err
		}
		mm, ok := v.(map[string]any)
		if !ok {
			return plainErr(src, path, "合并键 << 的值必须是映射或映射组成的序列")
		}
		for k, val := range mm {
			if _, exists := merged[k]; !exists {
				merged[k] = val
			}
		}
		return nil
	}
	for _, src := range merges {
		target := src
		if target.Kind == yamlv4.AliasNode && target.Alias != nil {
			target = target.Alias
		}
		if target.Kind == yamlv4.SequenceNode {
			for _, item := range target.Content {
				if err := add(item); err != nil {
					return err
				}
			}
			continue
		}
		if err := add(src); err != nil {
			return err
		}
	}
	for k, v := range merged {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	return nil
}

// keyString 把映射的键节点转换为字符串 key。
func (c *converter) keyString(kn *yamlv4.Node, path string) (string, error) {
	if kn.Kind == yamlv4.AliasNode {
		if c.strict {
			return "", strictErr(kn, path, "别名 *%s 作为 key 在 JSON 中没有对应表示", kn.Value)
		}
		if kn.Alias == nil {
			return "", plainErr(kn, path, "别名 *%s 指向未定义的锚点", kn.Value)
		}
		kn = kn.Alias
	}
	if kn.Kind != yamlv4.ScalarNode {
		return "", plainErr(kn, path, "复合类型(映射或序列)不能作为 key,无法转换为 JSON 字符串 key")
	}
	if c.strict && kn.Anchor != "" {
		return "", strictErr(kn, path, "锚点 &%s 在 JSON 中没有对应表示,展开后会丢失引用关系", kn.Anchor)
	}
	if kn.Tag == "!!str" {
		return kn.Value, nil
	}
	if c.strict {
		return "", strictErr(kn, path, "非字符串 key %q(类型 %s)无法无损转换为 JSON 字符串 key", kn.Value, kn.Tag)
	}
	switch kn.Tag {
	case "!!null":
		return "null", nil
	case "!!bool", "!!int":
		var v any
		if err := kn.Decode(&v); err == nil {
			return fmt.Sprint(v), nil
		}
	case "!!float":
		var v any
		if err := kn.Decode(&v); err == nil {
			if f, ok := v.(float64); ok && !math.IsInf(f, 0) && !math.IsNaN(f) {
				return strconv.FormatFloat(f, 'g', -1, 64), nil
			}
		}
	}
	return kn.Value, nil
}
