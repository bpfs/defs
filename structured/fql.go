// FQL子包：用于实现文件查询语言（File Query Language）
package bpfsfql

import (
	"fmt"
	"strings"
)

// Exec 函数用于执行给定的 FQL 查询
func Exec(query string) (string, error) {
	// 1. 词法分析
	tokens, err := Lex(query)
	if err != nil {
		return "", fmt.Errorf("词法分析错误: %w", err)
	}

	// 打印Token列表
	printTokens(tokens)

	// 2. 语法分析
	parser := NewParser(tokens)
	statement := parser.ParseStatement() // 注意首字母大写，以适应外部调用

	// 根据语法分析的结果来判断查询的类型并执行相应操作
	switch stmt := statement.(type) {
	case *ActionStatement:
		return "", execActionStatement(stmt)
	default:
		return "", fmt.Errorf("unsupported statement type")
	}
}

// printTokens 函数用于打印词法分析的结果
func printTokens(tokens []Token) {
	fmt.Println("Tokens:")
	for _, t := range tokens {
		fmt.Printf("{Type:%s Literal:%s}\n", t.Type, t.Literal)
	}
	fmt.Println()
}

// execActionStatement 函数用于执行动作语句
func execActionStatement(stmt *ActionStatement) error {
	// 打印语句的主要信息
	fmt.Printf("Action: %s\n", stmt.Token.Literal)
	fmt.Printf("Type: %s\n", stmt.Token.Type)
	fmt.Printf("Target: %s\n", stmt.Target.TokenLiteral())

	// 打印参数信息
	for _, param := range stmt.Parameters {
		if p, ok := param.(*ParameterExpression); ok {
			fmt.Printf("Param Key: %s, Value: %s\n", p.Key, p.Value.TokenLiteral())
		}
	}

	// 根据动作语句的类型执行相应操作
	// 目前仅支持创建文件操作
	if stmt.Token.Literal == "CREATE" && stmt.Target.TokenLiteral() == "FILE" {
		return createFile(stmt.Parameters)
	}
	return fmt.Errorf("不支持的动作语句")
}

// createFile 函数用于处理文件创建操作
func createFile(params []Expression) error {
	var filepath string
	var owned string
	var customName string
	metadata := make(map[string]string)

	// 解析参数
	for _, expr := range params {
		if param, ok := expr.(*ParameterExpression); ok {
			switch param.Key {
			case "filename":
				filepath = param.Value.TokenLiteral()
			case "OWNED_BY":
				owned = strings.Trim(param.Value.TokenLiteral(), "\"")
			case "CUSTOM_NAME":
				customName = strings.Trim(param.Value.TokenLiteral(), "\"")
			case "METADATA":
				// 对元数据字符串进行解析
				metaStr := strings.Trim(param.Value.TokenLiteral(), "\"")
				pairs := strings.Split(metaStr, ",")
				for _, pair := range pairs {
					kv := strings.Split(pair, ":")
					if len(kv) == 2 {
						metadata[kv[0]] = kv[1]
					} else {
						return fmt.Errorf("元数据格式不正确: %s", pair)
					}
				}
			}
		}
	}

	// 打印获取到的信息
	fmt.Println("创建文件:")
	fmt.Printf("原文件路径: %s\n", filepath)
	fmt.Printf("拥有者: %s\n", owned)
	fmt.Printf("自定义名称: %s\n", customName)
	fmt.Printf("元数据: %+v\n", metadata)

	// TODO: 在此处添加实际的文件创建逻辑

	return nil
}
