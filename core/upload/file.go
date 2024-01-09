package upload

import (
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/util"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/ristretto"
	"github.com/bpfs/defs/script"
)

// readFile 读取文件信息
func readFile(opt *opts.Options, cache *ristretto.Cache, file afero.File, pubKeyHash []byte) (*core.FileInfo, []byte, error) {
	fileInfo := new(core.FileInfo)

	if f, err := file.Stat(); err == nil {
		size := f.Size()
		if size > opt.GetMaxBufferSize() {
			return nil, nil, fmt.Errorf("文件的大小 %d 不可大于 %d", size, opt.GetMaxBufferSize())
		}

		fileInfo.BuildName(f.Name())                                            // 文件的基本名称
		fileInfo.BuildSize(size)                                                // 文件的大小（以字节为单位）
		fileInfo.BuildModTime(time.Time{})                                      // 修改时间(空)
		fileInfo.BuildUploadTime(time.Now())                                    // 上传时间(当前时间)
		fileInfo.BuildFileType(strings.TrimPrefix(filepath.Ext(f.Name()), ".")) // 文件类型或格式
	} else {
		return nil, nil, err
	}

	fileHash, err := util.CalculateFileHash(file)
	if err != nil {
		return nil, nil, err
	}
	id := append(fileHash, pubKeyHash...)
	fileInfo.BuildFileID(hex.EncodeToString((util.CalculateHash(id)))) // 设置文件的唯一标识

	fileKey := opt.GetDefaultFileKey()
	if fileKey == "" {
		fileKey = hex.EncodeToString(fileHash)
	}
	fileInfo.BuildFileKey(fileKey) // 设置文件的哈希值

	scriptBuilder, err := script.NewScriptBuilder().
		AddOp(script.OP_DUP).AddOp(script.OP_HASH160).
		AddData(pubKeyHash).
		AddOp(script.OP_EQUALVERIFY).AddOp(script.OP_CHECKSIG).
		Script()
	if err != nil {
		return nil, nil, err
	}

	fileInfo.BuildP2pkhScript(scriptBuilder) // 设置文件的 P2PKH 脚本(所有者)

	fileInfo.BuildSliceTable() // 设置哈希表

	_, _ = file.Seek(0, io.SeekStart)

	// TODO：测试
	// 将反汇编脚本格式化为一行打印
	disasm, err := script.DisasmString(scriptBuilder)
	if err != nil {
		return nil, nil, err
	}
	fmt.Printf("脚本反汇编:\t%s\n", disasm)

	return fileInfo, fileHash, nil
}
