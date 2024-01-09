package tempfile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// generateTempFilename 生成唯一的临时文件名
func generateTempFilename() string {
	timestamp := time.Now().UnixNano()
	return filepath.Join(os.TempDir(), "tempfile_"+fmt.Sprint(timestamp))
}
