package defs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCountLinesOfCode(t *testing.T) {
	totalLines := 0

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			lines := strings.Count(string(content), "\n") + 1
			totalLines += lines
		}

		return nil
	})

	if err != nil {
		t.Fatalf("遍历目录时出错: %v", err)
	}

	t.Logf("当前项目总共有 %d 行 Go 代码", totalLines)
}
