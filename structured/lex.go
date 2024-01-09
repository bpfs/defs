// Lex: 词法分析

package bpfsfql

import (
	"fmt"
	"regexp"
	"strings"
)

// TokenType 用于标记Token的类型
type TokenType string

const (
	// ACTION 表示操作类型（如CREATE、DROP、UPDATE等）
	ACTION TokenType = "ACTION"
	// FILENAME 表示文件名
	FILENAME = "FILENAME"
	// ALIAS 表示别名或者自定义名称
	ALIAS = "ALIAS"
	// DID 表示分布式身份标识（Decentralized Identifier）
	DID = "DID"
	// PARAM 表示参数（如 METADATA, SIZE 等）
	PARAM = "PARAM"
	// OPTION 表示选项（用于扩展功能）
	OPTION = "OPTION"
	// OPERATOR 表示操作符（如 <, >, =, ... ）
	OPERATOR = "OPERATOR"
	// NUMBER 表示数字
	NUMBER = "NUMBER"
	// ASYNC 表示异步操作
	ASYNC = "ASYNC"
	// KEYVALUE 表示键值对（如 "key1:value1" ）
	KEYVALUE = "KEYVALUE"
	// LIST 表示列表格式
	LIST = "LIST"
	// DATE 表示日期格式
	DATE = "DATE"
	// CONTENT 表示文件的内容
	CONTENT = "CONTENT"
	// WHERE 表示查询条件
	WHERE = "WHERE"
	// TAGS 表示文件的标签
	TAGS = "TAGS"
	// TRANSFER 表示文件的转移操作
	TRANSFER = "TRANSFER"
	// CO_OWNERS 表示文件的共有者
	CO_OWNERS = "CO_OWNERS"
	// VERSION 表示文件的版本
	VERSION = "VERSION"
	// EXTENSION 表示扩展
	EXTENSION = "EXTENSION"
	// FILES 表示多个文件
	FILES = "FILES"
	// ATTRIBUTE 表示文件属性，如 SIZE, LAST_MODIFIED 等
	ATTRIBUTE = "ATTRIBUTE"
)

// 用于匹配关键字的映射
var keywordMap = map[string]TokenType{
	"CREATE":        ACTION,
	"DROP":          ACTION,
	"UPDATE":        ACTION,
	"SELECT":        ACTION,
	"BULK":          ACTION,
	"ADD":           ACTION,
	"SHOW":          ACTION,
	"SET":           ACTION,
	"SHARE":         ACTION,
	"CHECKOUT":      ACTION,
	"USE":           ACTION,
	"ASYNC":         ASYNC,
	"FILE":          FILENAME,
	"AUTHORIZED":    PARAM,
	"BY":            PARAM,
	"OWNER":         PARAM,
	"CONTENT":       CONTENT,
	"WHERE":         WHERE,
	"TAGS":          TAGS,
	"TRANSFER":      TRANSFER,
	"CO_OWNERS":     CO_OWNERS,
	"VERSION":       VERSION,
	"EXTENSION":     EXTENSION,
	"FILES":         FILES,
	"SIZE":          ATTRIBUTE,
	"LAST_MODIFIED": ATTRIBUTE,
	// ... 根据需求增加其他关键词
}

// 用于匹配操作符的映射
var operatorMap = map[string]TokenType{
	"<":       OPERATOR,
	">":       OPERATOR,
	"=":       OPERATOR,
	"AND":     OPERATOR,
	"TO":      OPERATOR,
	"FOR":     OPERATOR,
	"FROM":    OPERATOR,
	"INCLUDE": OPERATOR,
	"SET":     OPERATOR,
	"WITH":    OPERATOR,
	// ... 根据需求增加其他操作符
}

// Token 表示一个词法单元
type Token struct {
	Type    TokenType // 词法单元类型：如 ACTION, FILENAME 等
	Literal string    // 词法单元的字面值
}

// parseKeyValue: 识别键值对格式的Token。
// 支持的格式有:
// 1. "key:value"
// 2. 'key:value'
// 3. key: value (无外部引号, 内部可能有多个空格)
func parseKeyValue(input string) (Token, bool) {
	pattern := regexp.MustCompile(`^["']?([^:]+)["']?\s*:\s*["']?([^"']+)["']?$`)
	if matches := pattern.FindStringSubmatch(input); matches != nil {
		// 避免处理特定的前缀内容
		prefixes := []string{"filename", "did", "custom_name", "extension_name", "version_number"}
		for _, prefix := range prefixes {
			if strings.HasPrefix(matches[1], prefix) {
				return Token{}, false
			}
		}
		return Token{Type: KEYVALUE, Literal: input}, true
	}
	return Token{}, false
}

// parseList: 识别列表格式的Token。
// 支持的格式有:
// 1. [item1, item2, item3]
// 2. ['item1', 'item2', 'item3']
// 3. ["item1", "item2", "item3"]
// 列表之间的项可以有多个空格，也可以被单/双引号包围。
func parseList(input string) ([]Token, bool) {
	// 正则表达式匹配 参数名=列表 这种形式
	paramListPattern := regexp.MustCompile(`^(\w+)=\[.*\]$`)

	if matches := paramListPattern.FindStringSubmatch(input); matches != nil {
		// 取出参数名和参数值
		paramName := matches[1]
		listContent := strings.Trim(strings.Trim(input, paramName+"="), "[]")

		itemsSlice := strings.Split(listContent, ",")
		cleanItemsSlice := []string{}
		for _, item := range itemsSlice {
			item = strings.TrimSpace(item)
			item = strings.Trim(item, "\"'")
			if item == "" {
				return nil, false
			}
			cleanItemsSlice = append(cleanItemsSlice, item)
		}
		cleanItems := "[" + strings.Join(cleanItemsSlice, ", ") + "]"

		return []Token{
			{Type: PARAM, Literal: paramName},
			{Type: LIST, Literal: cleanItems},
		}, true
	}
	return nil, false
}

// parseParam: 识别参数格式的Token。
// 支持的格式有:
// 1. param=value
// 2. param="value"
// 3. param='value'
// 参数和它的值之间可能有多个空格，值也可能被单/双引号包围。
func parseParam(input string) (Token, bool) {
	// 不再匹配列表，因为这个已经在 parseList 中处理了
	paramPattern := regexp.MustCompile(`([^=]+)\s*=\s*(["']?[^ ]+["']?)`)
	if paramPattern.MatchString(input) {
		return Token{Type: PARAM, Literal: input}, true
	}
	return Token{}, false
}

// parseDate: 识别日期格式的Token。
// 支持的格式有:
// 1. '2023-09-19'
// 2. 2023-09-19 (无引号)
// 这个方法还会识别被单引号包围的任意内容作为别名Token。
func parseDate(input string) (Token, bool) {
	datePattern := regexp.MustCompile(`^'?\d{4}-\d{2}-\d{2}'?$`)
	if datePattern.MatchString(input) {
		return Token{Type: DATE, Literal: input}, true
	} else if strings.HasPrefix(input, "'") && strings.HasSuffix(input, "'") {
		return Token{Type: ALIAS, Literal: input}, true
	}
	return Token{}, false
}

// parseNumber: 识别数字格式的Token。
// 支持的格式有:
// 1. 整数，如 123
// 2. 浮点数，如 123.45
func parseNumber(input string) (Token, bool) {
	numberPattern := regexp.MustCompile(`^\d+(\.\d+)?$`)
	if numberPattern.MatchString(input) {
		return Token{Type: NUMBER, Literal: input}, true
	}
	return Token{}, false
}

// encodeSpaces 用于编码字符串中的空格
// 找到所有的单引号和双引号之间的字符串，并将这些字符串中的空格替换为 "_SPACE_"。
// 在分割字符串后，这些空格将会被还原。
// 这样就可以保持字符串中的空格，而不影响其它部分的分割。
func encodeSpaces(input string) (string, map[string]string) {
	re := regexp.MustCompile(`'([^']*)'|"([^"]*)"`)

	matches := re.FindAllStringSubmatch(input, -1)
	replacements := make(map[string]string)

	for _, match := range matches {
		var original string
		if match[1] != "" {
			original = "'" + match[1] + "'"
		} else {
			original = "\"" + match[2] + "\""
		}

		replaced := strings.ReplaceAll(original, " ", "_SPACE_")
		replacements[replaced] = original
		input = strings.Replace(input, original, replaced, 1)
	}

	return input, replacements
}

// decodeSpaces 解码字符串中的空格
func decodeSpaces(input string, replacements map[string]string) string {
	for replaced, original := range replacements {
		input = strings.Replace(input, replaced, original, 1)
	}

	return input
}

// Lex 是词法分析器，将输入字符串转换为Token序列
func Lex(input string) ([]Token, error) {
	// 首先对输入字符串中带引号的部分进行空格的编码
	// 为了在分割字符串时能保留带引号的部分中的空格
	input, replacements := encodeSpaces(input)

	// 使用 strings.FieldsFunc 来分割字符串，按空格分割，但保留带引号的部分
	f := func(c rune) bool {
		return c == ' ' && !strings.ContainsRune("\"'", c)
	}
	parts := strings.FieldsFunc(input, f)
	tokens := []Token{}

	// 用于处理跨多个part的Token，例如处理 "[tag1, tag2]"
	var isProcessingList bool
	var tempListContent string

	// 遍历所有的部分并将它们转换为Token
	for index := 0; index < len(parts); index++ {
		part := parts[index]

		// 当词法分析器遇到 "FILE" 时，它需要检查下一个 token，捕获文件名并返回正确的 Literal。
		if part == "FILE" {
			if index+1 < len(parts) {
				filenameToken := strings.Trim(parts[index+1], "'")
				tokens = append(tokens, Token{Type: FILENAME, Literal: filenameToken})
				index++ // skip the filename token
				continue
			}
		}

		if part == "AUTHORIZED" {
			if index+2 < len(parts) && parts[index+1] == "BY" {
				didToken := strings.Trim(parts[index+2], "'")
				tokens = append(tokens, Token{Type: DID, Literal: didToken})
				index += 2 // skip the BY and did token
				continue
			}
		}

		// 检查是否在处理列表或带空格的字符串
		if isProcessingList || strings.HasPrefix(part, "'") {
			tempListContent += " " + part
			if strings.HasSuffix(part, "]") || strings.HasSuffix(part, "'") {
				isProcessingList = false
				// 尝试将组合的内容处理为列表或日期Token
				if token, handled := parseList(tempListContent); handled {
					tokens = append(tokens, token...)
					tempListContent = ""
				} else if token, handled := parseDate(tempListContent); handled {
					tokens = append(tokens, token)
					tempListContent = ""
				}
			}
			continue
		}

		// 检查当前部分是否是已定义的关键词
		if tokenType, exists := keywordMap[part]; exists {
			// 如果关键字是一个文件属性并且后续还有两个token（操作符和数字）
			if tokenType == ATTRIBUTE && index+2 < len(parts) && operatorMap[parts[index+1]] == OPERATOR {
				combinedToken := part + " " + parts[index+1] + " " + parts[index+2]
				tokens = append(tokens, Token{Type: ATTRIBUTE, Literal: combinedToken})
				index += 2 // skip the next two tokens
				continue
			} else {
				tokens = append(tokens, Token{Type: tokenType, Literal: part})
				continue
			}
		} else if tokenType, exists := operatorMap[part]; exists {
			// 检查当前部分是否是已定义的操作符
			tokens = append(tokens, Token{Type: tokenType, Literal: part})
		} else if token, handled := parseKeyValue(part); handled {
			// 尝试将当前部分处理为键值对Token
			tokens = append(tokens, token)
		} else if token, handled := parseParam(part); handled {
			// 尝试将当前部分处理为KEY=VALUE格式的参数Token
			tokens = append(tokens, token)
		} else if token, handled := parseDate(part); handled {
			// 尝试将当前部分处理为日期Token
			tokens = append(tokens, token)
		} else if token, handled := parseNumber(part); handled {
			// 尝试将当前部分处理为数字Token
			tokens = append(tokens, token)
		} else if strings.HasPrefix(part, "[") {
			// 开始处理列表，例如 "[tag1"
			isProcessingList = true
			tempListContent = part
		} else if strings.HasPrefix(part, "filename") || strings.HasPrefix(part, "did") ||
			strings.HasPrefix(part, "custom_name") || strings.HasPrefix(part, "extension_name") ||
			strings.HasPrefix(part, "version_number") {
			// 如果当前部分是某些已知的前缀，则将其处理为别名Token
			tokens = append(tokens, Token{Type: ALIAS, Literal: part})
		} else {
			// 如果当前部分不能被任何规则处理，那么它是一个未知的Token
			return nil, fmt.Errorf("未知的Token: " + part)
		}
	}

	// 对tokens中的字面值进行空格的解码，以还原被编码过的空格
	for i := range tokens {
		tokens[i].Literal = decodeSpaces(tokens[i].Literal, replacements)
	}

	return tokens, nil
}
