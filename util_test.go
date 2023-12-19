package defs

import (
	"fmt"
	"testing"
)

func TestGetDefaultDownloadPath(t *testing.T) {
	path := GetDefaultDownloadPath()
	fmt.Printf("默认路径：\t%s", path)
}
