package bpfsfql

import (
	"fmt"
	"testing"
)

func TestLex(t *testing.T) {
	// 示例代码
	inputs := []string{
		`CREATE FILE 'filepath' OWNER 'did' CUSTOM_NAME 'custom_name' METADATA [key1:value1, key2:value2]`,
		`DROP FILE 'filehash' AUTHORIZED BY 'did'`,
		`UPDATE FILE 'filehash' SET CONTENT 'new content' WHERE SIZE < 5000 AND LAST_MODIFIED > '2023-01-01' AUTHORIZED BY 'did'`,
		`SELECT CONTENT FROM FILE 'filehash' AUTHORIZED BY 'did'`,
		`BULK TRANSFER FILES [filehash1, filehash2, ...] FROM 'did_from' TO 'did_to'`,
		`ALTER FILE 'filehash' ADD TAGS [tag1, tag2, ...] AUTHORIZED BY 'did'`,
		`SELECT FILES WHERE TAGS INCLUDE [tag1, tag2, ...] AND OWNER 'did'`,
		`TRANSFER FILE 'filehash' TO 'did' AUTHORIZED BY 'did'`,
		`ALTER FILE 'filehash' SET CO_OWNERS [did1, did2, ...] AUTHORIZED BY 'did'`,
		`SHARE FILE 'filehash' WITH 'did' AUTHORIZED BY 'did'`,
		`CHECKOUT FILE 'filehash' VERSION 'version_number' AUTHORIZED BY 'did'`,
		`USE EXTENSION 'extension_name' PARAMETERS [key1:value1, key2:value2]`,
		`ASYNC CREATE FILE 'filepath' OWNER 'did'`,

		// `CREATE FILE filename OWNED_BY=did CUSTOM_NAME=custom_name METADATA=\"key1:value1,key2:value2\"`,
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

		// `CREATE FILE 'filename' OWNED_BY='did' CUSTOM_NAME='custom_name' METADATA='key1:value1, key2:value2'`,
		// `DROP FILE 'filename' AUTHORIZED BY 'did'`,
		// `UPDATE FILE 'filename' SET CONTENT='new content' WHERE SIZE < 5000 AND LAST_MODIFIED > '2023-01-01' AUTHORIZED BY 'did'`,
		// `SELECT CONTENT FROM FILE 'filename' AUTHORIZED BY 'did'`,
		// `BULK TRANSFER FILES ['file1', 'file2'] FROM 'did_from' TO 'did_to'`,
		// `ADD TAGS TO FILE 'filename' TAGS ['tag1', 'tag2'] AUTHORIZED BY 'did'`,
		// `SHOW FILES WHERE TAGS INCLUDE ['tag1', 'tag2'] AND OWNED_BY='did'`,
		// `TRANSFER FILE 'filename' TO 'did' AUTHORIZED BY 'did'`,
		// `SET CO_OWNERS FOR FILE 'filename' CO_OWNERS=['did1', 'did2'] AUTHORIZED BY 'did'`,
		// `SHARE FILE 'filename' WITH 'did' AUTHORIZED BY 'did'`,
		// `CHECKOUT FILE 'filename' VERSION 'version_number' AUTHORIZED BY 'did'`,
		// `USE EXTENSION 'extension_name' PARAMETERS='key1:value1, key2:value2'`,
		// `ASYNC CREATE FILE 'filename' OWNED_BY='did'`,

		// ... 添加其他测试语句
	}

	for _, input := range inputs {
		fmt.Println("Parsing:", input)
		tokens, err := Lex(input)
		if err != nil {
			fmt.Printf("错误:%v\n\n", err)
			continue
		}
		for _, token := range tokens {
			fmt.Printf("%+v\n", token)
		}

		fmt.Printf("\n\n")
	}
}
