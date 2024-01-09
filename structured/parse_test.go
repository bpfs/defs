package bpfsfql

import (
	"fmt"
	"testing"
)

func TestParse(t *testing.T) {
	// 示例代码
	inputs := []string{
		`CREATE FILE filename OWNED_BY=did CUSTOM_NAME=custom_name METADATA=\"key1:value1,key2:value2\"`,
		// `DROP FILE filename AUTHORIZED BY did`,
		// `CREATE FILE filename OWNED_BY=did CUSTOM_NAME=custom_name METADATA="key1:value1,key2:value2"`,
		// `DROP FILE filename AUTHORIZED BY did`,
		// `UPDATE FILE filename SET CONTENT='new content' WHERE SIZE < 5000 AND LAST_MODIFIED > '2023-01-01' AUTHORIZED BY did`,
		// `SELECT CONTENT FROM FILE filename AUTHORIZED BY did`,
		// `BULK TRANSFER FILES [file1, file2] TO did_from did_to`,
		// `ADD TAGS TO FILE filename TAGS [tag1, tag2] AUTHORIZED BY did`,
		// `SHOW FILES WHERE TAGS INCLUDE [tag1, tag2] AND OWNED_BY=did`,
		// `TRANSFER FILE filename TO did AUTHORIZED BY did`,
		// `SET CO_OWNERS FOR FILE filename CO_OWNERS [did1, did2] AUTHORIZED BY did`,
		// `SHARE FILE filename WITH did AUTHORIZED BY did`,
		// `CHECKOUT FILE filename VERSION version_number AUTHORIZED BY did`,
		// `USE EXTENSION extension_name PARAMETERS="key1:value1,key2:value2"`,
		// `ASYNC CREATE FILE filename OWNED_BY=did`,
		// ... 添加其他测试语句
	}

	for _, input := range inputs {
		fmt.Println("Parsing:\t", input)
		tokens, err := Lex(input)
		if err != nil {
			fmt.Printf("词法分析错误:\t%v\n\n", err)
			continue
		}

		// 2. 语法分析
		parser := NewParser(tokens)
		statement := parser.ParseStatement() // 注意首字母大写，以适应外部调用
		fmt.Printf("%v\n\n", statement)
	}
}

// for _, c := range cmd {
// fmt.Printf("Action:\t\t%+v\n", cmd.Action)
// fmt.Printf("FileType:\t%+v\n", cmd.FileType)
// fmt.Printf("Filename:\t%+v\n", cmd.Filename)
// fmt.Printf("Params:\t\t%+v\n", cmd.Params)
// fmt.Printf("Condition:\t%+v\n", cmd.Condition)
// fmt.Printf("Tags:\t\t%+v\n", cmd.Tags)
// fmt.Printf("CoOwners:\t%+v\n", cmd.CoOwners)
// fmt.Printf("Content:\t%+v\n", cmd.Content)
// fmt.Printf("Transfer:\t\t%+v\n", cmd.Transfer)
// fmt.Printf("Extension:\t%+v\n", cmd.Extension)
// fmt.Printf("Version:\t\t%+v\n", cmd.Version)
// fmt.Printf("AuthorizedBy:\t%+v\n", cmd.AuthorizedBy)
// }

// func TestParse(t *testing.T) {
// 	tests := []struct {
// 		input string
// 		// 预期的解析输出
// 		expectedOutput *Command // 假设Command是您解析的输出结构
// 		expectError    bool
// 	}{
// 		{
// 			input: `CREATE FILE filename OWNED_BY=did CUSTOM_NAME=custom_name METADATA="key1:value1,key2:value2"`,
// 			expectedOutput: &Command{
// 				Action:   "CREATE",
// 				FileType: "FILE",
// 				Params: map[TokenType]string{
// 					FILENAME: "filename",
// 					DID:      "did",
// 					ALIAS:    "custom_name",
// 					METADATA: "key1:value1,key2:value2",
// 				},
// 				// ... 其他预期字段
// 			},
// 		},
// 		// ... 添加其他测试用例
// 	}

// 	for _, test := range tests {
// 		tokens, err := Lex(test.input)
// 		if err != nil {
// 			if test.expectError {
// 				return
// 			}
// 			t.Errorf("unexpected error during Lex: %v", err)
// 			continue
// 		}

// 		actualOutput, err := Parse(tokens)
// 		if err != nil {
// 			if test.expectError {
// 				return
// 			}
// 			t.Errorf("unexpected error during Parse: %v", err)
// 			continue
// 		}

// 		// 检查实际的输出和期望的输出是否一致
// 		if !reflect.DeepEqual(actualOutput, test.expectedOutput) {
// 			t.Errorf("for input %q, expected %+v, got %+v", test.input, test.expectedOutput, actualOutput)
// 		}
// 	}
// }
