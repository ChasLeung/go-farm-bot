package tools

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"gofarm/proto/gatepb"
)

// DecodeOptions 解码选项
type DecodeOptions struct {
	Data         string // 输入数据
	IsHex        bool   // 是否为hex编码
	IsGateWrapped bool  // 是否为gate包装
	TypeName     string // 指定消息类型
}

// DecodeResult 解码结果
type DecodeResult struct {
	Success bool
	Type    string
	Data    interface{}
	Error   string
}

// DecodePB 解码PB数据
func DecodePB(opts DecodeOptions) *DecodeResult {
	// 解码输入数据
	var buf []byte
	var err error

	if opts.IsHex {
		buf, err = hex.DecodeString(opts.Data)
	} else {
		buf, err = base64.StdEncoding.DecodeString(opts.Data)
	}

	if err != nil {
		return &DecodeResult{
			Success: false,
			Error:   fmt.Sprintf("输入数据解码失败: %v", err),
		}
	}

	fmt.Printf("数据长度: %d 字节\n\n", len(buf))

	// --gate: 先解析外层 gatepb.Message
	if opts.IsGateWrapped {
		return decodeGateWrapped(buf, opts.TypeName)
	}

	// --type: 指定类型解码
	if opts.TypeName != "" {
		return decodeWithType(buf, opts.TypeName)
	}

	// 未指定类型，自动尝试
	fmt.Println("未指定类型，自动尝试...\n")
	
	// 尝试解析为 gatepb.Message
	var msg gatepb.Message
	if err := proto.Unmarshal(buf, &msg); err == nil {
		if msg.Meta != nil && (msg.Meta.ServiceName != "" || msg.Meta.MethodName != "") {
			fmt.Println("=== 检测为 gatepb.Message ===")
			return &DecodeResult{
				Success: true,
				Type:    "gatepb.Message",
				Data:    msgToMap(&msg),
			}
		}
	}

	// 通用解码
	return tryGenericDecode(buf)
}

// decodeGateWrapped 解码Gate包装的消息
func decodeGateWrapped(buf []byte, typeName string) *DecodeResult {
	var msg gatepb.Message
	if err := proto.Unmarshal(buf, &msg); err != nil {
		return &DecodeResult{
			Success: false,
			Error:   fmt.Sprintf("gatepb.Message 解码失败: %v", err),
		}
	}

	meta := msg.Meta
	fmt.Println("=== gatepb.Message (外层) ===")
	fmt.Printf("  service:     %s\n", meta.ServiceName)
	fmt.Printf("  method:      %s\n", meta.MethodName)
	fmt.Printf("  type:        %d (%s)\n", meta.MessageType, messageTypeName(meta.MessageType))
	fmt.Printf("  client_seq:  %d\n", meta.ClientSeq)
	fmt.Printf("  server_seq:  %d\n", meta.ServerSeq)
	if meta.ErrorCode != 0 {
		fmt.Printf("  error_code:  %d\n", meta.ErrorCode)
		fmt.Printf("  error_msg:   %s\n", meta.ErrorMessage)
	}
	fmt.Println()

	if len(msg.Body) > 0 {
		// 尝试自动推断body类型
		if typeName == "" {
			typeName = inferBodyType(meta)
		}

		if typeName != "" {
			fmt.Printf("=== %s (body) ===\n", typeName)
			// 由于Go是静态类型，这里我们只能显示hex和base64
			fmt.Printf("  hex:    %s\n", hex.EncodeToString(msg.Body))
			fmt.Printf("  base64: %s\n", base64.StdEncoding.EncodeToString(msg.Body))
			fmt.Println("  (Go版本暂不支持动态类型解码，请使用 --type 指定具体类型)")
		} else {
			fmt.Println("=== body (未能自动推断类型, 用 --type 手动指定 body 类型) ===")
			fmt.Printf("  hex:    %s\n", hex.EncodeToString(msg.Body))
			fmt.Printf("  base64: %s\n", base64.StdEncoding.EncodeToString(msg.Body))
			tryGenericDecode(msg.Body)
		}
	}

	return &DecodeResult{
		Success: true,
		Type:    "gatepb.Message",
		Data:    msgToMap(&msg),
	}
}

// decodeWithType 使用指定类型解码
func decodeWithType(buf []byte, typeName string) *DecodeResult {
	// Go是静态类型语言，无法像JavaScript那样动态查找类型
	// 这里我们支持一些常见类型的硬编码解码
	
	switch typeName {
	case "gatepb.Message":
		var msg gatepb.Message
		if err := proto.Unmarshal(buf, &msg); err != nil {
			return &DecodeResult{Success: false, Error: err.Error()}
		}
		return &DecodeResult{
			Success: true,
			Type:    typeName,
			Data:    msgToMap(&msg),
		}
		
	case "gatepb.Meta":
		var meta gatepb.Meta
		if err := proto.Unmarshal(buf, &meta); err != nil {
			return &DecodeResult{Success: false, Error: err.Error()}
		}
		return &DecodeResult{
			Success: true,
			Type:    typeName,
			Data:    metaToMap(&meta),
		}
		
	default:
		return &DecodeResult{
			Success: false,
			Error:   fmt.Sprintf("不支持的类型: %s (Go版本仅支持 gatepb.Message 和 gatepb.Meta)", typeName),
		}
	}
}

// tryGenericDecode 通用protobuf解码 (无schema)
func tryGenericDecode(buf []byte) *DecodeResult {
	fmt.Println("=== 通用 protobuf 解码 (无schema) ===")
	
	result := make(map[string]interface{})
	var fields []map[string]interface{}
	
	for len(buf) > 0 {
		if len(buf) < 1 {
			break
		}
		
		// 读取tag
		tag, n := protowire.ConsumeVarint(buf)
		if n < 0 {
			fmt.Printf("  解码中断: 无法读取tag\n")
			break
		}
		buf = buf[n:]
		
		fieldNum := int(tag >> 3)
		wireType := tag & 7
		
		field := map[string]interface{}{
			"field": fieldNum,
			"wire":  wireType,
		}
		
		switch wireType {
		case 0: // varint
			val, n := protowire.ConsumeVarint(buf)
			if n < 0 {
				fmt.Printf("  field %d (varint): <error>\n", fieldNum)
				break
			}
			buf = buf[n:]
			field["type"] = "varint"
			field["value"] = strconv.FormatInt(int64(val), 10)
			fmt.Printf("  field %d (varint): %d\n", fieldNum, val)
			
		case 1: // fixed64
			if len(buf) < 8 {
				fmt.Printf("  field %d (fixed64): <truncated>\n", fieldNum)
				break
			}
			val := buf[:8]
			buf = buf[8:]
			field["type"] = "fixed64"
			field["value"] = hex.EncodeToString(val)
			fmt.Printf("  field %d (fixed64): %s\n", fieldNum, hex.EncodeToString(val))
			
		case 2: // length-delimited (bytes/string)
			length, n := protowire.ConsumeVarint(buf)
			if n < 0 || len(buf) < n+int(length) {
				fmt.Printf("  field %d (bytes): <truncated>\n", fieldNum)
				break
			}
			buf = buf[n:]
			data := buf[:length]
			buf = buf[length:]
			
			// 尝试解码为字符串
			if str := tryDecodeString(data); str != "" {
				field["type"] = "string"
				field["value"] = str
				fmt.Printf("  field %d (string): \"%s\"\n", fieldNum, str)
			} else {
				field["type"] = "bytes"
				field["value"] = hex.EncodeToString(data)
				fmt.Printf("  field %d (bytes/%d): %s\n", fieldNum, length, hex.EncodeToString(data))
			}
			
		case 5: // fixed32
			if len(buf) < 4 {
				fmt.Printf("  field %d (fixed32): <truncated>\n", fieldNum)
				break
			}
			val := buf[:4]
			buf = buf[4:]
			field["type"] = "fixed32"
			field["value"] = hex.EncodeToString(val)
			fmt.Printf("  field %d (fixed32): %s\n", fieldNum, hex.EncodeToString(val))
			
		default:
			fmt.Printf("  field %d (wire %d): <skip>\n", fieldNum, wireType)
			field["type"] = fmt.Sprintf("unknown(%d)", wireType)
			field["value"] = "<skip>"
			// 跳过未知类型
			break
		}
		
		fields = append(fields, field)
	}
	
	result["fields"] = fields
	
	return &DecodeResult{
		Success: true,
		Type:    "generic",
		Data:    result,
	}
}

// tryDecodeString 尝试将bytes解码为UTF-8字符串
func tryDecodeString(data []byte) string {
	// 检查是否为可打印字符
	printable := 0
	for _, b := range data {
		if b >= 32 || b == '\n' || b == '\r' || b == '\t' {
			printable++
		}
	}
	
	// 如果可打印字符比例大于80%，认为是字符串
	if len(data) > 0 && float64(printable)/float64(len(data)) > 0.8 {
		return string(data)
	}
	return ""
}

// inferBodyType 根据meta自动推断body类型
func inferBodyType(meta *gatepb.Meta) string {
	if meta == nil {
		return ""
	}
	
	svc := meta.ServiceName
	mtd := meta.MethodName
	isReq := meta.MessageType == 1
	
	// 移除Service后缀
	svc = strings.TrimSuffix(svc, "Service")
	
	suffix := "Reply"
	if isReq {
		suffix = "Request"
	}
	
	return fmt.Sprintf("%s.%s%s", svc, mtd, suffix)
}

// messageTypeName 获取消息类型名称
func messageTypeName(t int32) string {
	switch t {
	case 1:
		return "Request"
	case 2:
		return "Response"
	case 3:
		return "Notify"
	default:
		return "Unknown"
	}
}

// msgToMap 将Message转换为map
func msgToMap(msg *gatepb.Message) map[string]interface{} {
	if msg == nil {
		return nil
	}
	
	return map[string]interface{}{
		"meta": metaToMap(msg.Meta),
		"body": fmt.Sprintf("<%d bytes>", len(msg.Body)),
	}
}

// metaToMap 将Meta转换为map
func metaToMap(meta *gatepb.Meta) map[string]interface{} {
	if meta == nil {
		return nil
	}
	
	return map[string]interface{}{
		"service_name":  meta.ServiceName,
		"method_name":   meta.MethodName,
		"message_type":  meta.MessageType,
		"client_seq":    meta.ClientSeq,
		"server_seq":    meta.ServerSeq,
		"error_code":    meta.ErrorCode,
		"error_message": meta.ErrorMessage,
		"metadata":      meta.Metadata,
	}
}

// PrintDecodeHelp 打印解码工具帮助信息
func PrintDecodeHelp() {
	help := `
PB数据解码工具
==============

用法:
  gofarm.exe --decode <base64数据>
  gofarm.exe --decode <hex数据> --hex
  gofarm.exe --decode <base64数据> --type <消息类型>
  gofarm.exe --decode <base64数据> --gate

参数:
  <数据>       base64编码的pb数据 (默认), 或hex编码 (配合 --hex)
  --hex       输入数据为hex编码
  --gate      外层是 gatepb.Message 包装, 自动解析 meta + body
  --type      指定消息类型 (目前仅支持: gatepb.Message, gatepb.Meta)

示例:
  gofarm.exe --decode CigKGWdhbWVwYi51c2VycGIuVXNlclNlcnZpY2USBUxvZ2luGAEgASgAEmEYACIAKjwKEDEuNi4wLjhfMjAyNTEyMjQSE1dpbmRvd3MgVW5rbm93biB4NjQqBHdpZmlQzL0BagltaWNyb3NvZnQwADoEMTI1NkIVCgASABoAIgAqBW90aGVyMAI6AEIA --gate
  gofarm.exe --decode 0a1c0a19... --hex --type gatepb.Message

注意:
  Go版本暂不支持动态类型查找，--type 参数仅支持 gatepb.Message 和 gatepb.Meta。
  对于其他类型，请使用 --gate 参数解析外层，然后手动解析body。
`
	fmt.Println(help)
}

// VerifyMode 验证模式 - 测试一些预定义的PB数据
func VerifyMode() {
	fmt.Println("\n====== 验证模式 ======\n")

	// Login Request
	loginB64 := "CigKGWdhbWVwYi51c2VycGIuVXNlclNlcnZpY2USBUxvZ2luGAEgASgAEmEYACIAKjwKEDEuNi4wLjhfMjAyNTEyMjQSE1dpbmRvd3MgVW5rbm93biB4NjQqBHdpZmlQzL0BagltaWNyb3NvZnQwADoEMTI1NkIVCgASABoAIgAqBW90aGVyMAI6AEIA"
	opts := DecodeOptions{Data: loginB64, IsGateWrapped: true}
	result := DecodePB(opts)
	if result.Success {
		fmt.Println("[OK] Login Request 解码成功")
		if data, ok := result.Data.(map[string]interface{}); ok {
			if meta, ok := data["meta"].(map[string]interface{}); ok {
				fmt.Printf("     service=%s, method=%s\n", meta["service_name"], meta["method_name"])
			}
		}
	} else {
		fmt.Printf("[FAIL] Login Request: %s\n", result.Error)
	}
	fmt.Println()

	fmt.Println("====== 验证完成 ======\n")
}

// FormatJSON 格式化输出JSON
func FormatJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("<error: %v>", err)
	}
	return string(b)
}
