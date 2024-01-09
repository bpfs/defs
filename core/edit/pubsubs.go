package edit

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/core/sqlite"
	"github.com/bpfs/defs/core/util"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/segment"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/sirupsen/logrus"
)

// HandleFileEditRequestPubSub 处理文件修改响应的订阅消息
func HandleFileEditRequestPubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, db *sqlites.SqliteDB, res *streams.RequestMessage) {
	// 新建文件存储
	fs, err := afero.NewFileStore(filepath.Join(paths.SlicePath, p2p.Host().ID().String()))
	if err != nil {
		return
	}

	switch res.Message.Type {
	// 名称
	case "name":
		// 文件名称修改请求
		payload := new(FileEditNameRequestPayload)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			return
		}

		// 列出指定子目录中的所有文件
		slices, err := fs.ListFiles(payload.FileID)
		if err != nil {
			return
		}

		for _, sliceHash := range slices {
			sliceFile, err := fs.OpenFile(payload.FileID, sliceHash)
			if err != nil {
				continue
			}

			// 读取文件的 P2PKH 脚本
			p2pkhScriptData, xref, err := segment.ReadFileSegment(sliceFile, "P2PKHSCRIPT")
			if err != nil {
				continue
			}

			// 验证脚本中的公钥哈希
			if script.VerifyScriptPubKeyHash(p2pkhScriptData, payload.PubKeyHash) {
				// 将 modTime、uploadTime 转换为 []byte
				modTimeByte, err := util.ToBytes[int64](time.Now().Unix())
				if err != nil {
					continue
				}

				// 创建多个段
				segments := map[string][]byte{
					"NAME":    []byte(payload.NewName), // 文件的基本名称
					"MODTIME": modTimeByte,             // 文件的修改时间
				}
				if err := segment.AppendSegmentsToFile(sliceFile, segments, xref); err != nil {
					continue
				}
			}
		}

	// 共享
	case "shared":
		// 文件共享修改请求
		payload := new(FileEditSharedRequestPayload)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			return
		}

		// 列出指定子目录中的所有文件
		slices, err := fs.ListFiles(payload.FileID)
		if err != nil {
			return
		}

		for _, sliceHash := range slices {
			sliceFile, err := fs.OpenFile(payload.FileID, sliceHash)
			if err != nil {
				continue
			}

			// 定义需要读取的段类型
			segmentTypes := []string{
				"FILEID",
				"SHARED",
				"P2PKHSCRIPT",
				"NAME",
				"SIZE",
				"MODTIME",
				"UPLOADTIME",
			}
			segmentResults, xref, err := segment.ReadFileSegments(sliceFile, segmentTypes)
			if err != nil {
				continue
			}
			// 处理每个段的结果
			for segmentType, result := range segmentResults {
				if result.Error != nil {
					// 出现任何错误，立即退出
					continue
				}
				switch segmentType {
				case "FILEID":
					if payload.FileID != string(result.Data) {
						continue
					}
				case "P2PKHSCRIPT":
					// 验证脚本中的公钥哈希
					if !script.VerifyScriptPubKeyHash(result.Data, payload.PubKeyHash) {
						continue
					}
				case "SHARED":
					shared, err := util.FromBytes[bool](result.Data)
					if err != nil || shared == payload.Shared { // 共享状态相同则不用修改
						continue
					}
				}
			}

			// 将 modTime、uploadTime 转换为 []byte
			modTimeByte, err := util.ToBytes[int64](time.Now().Unix())
			if err != nil {
				continue
			}
			// 将 shared 转换为 []byte
			sharedByte, err := util.ToBytes[bool](payload.Shared)
			if err != nil {
				continue
			}

			// 创建多个段
			segments := map[string][]byte{
				"SHARED":  sharedByte,              // 写入文件共享状态
				"FILEKEY": []byte(payload.FileKey), // 写入文件的密钥
				"MODTIME": modTimeByte,             // 文件的修改时间
			}
			if err := segment.AppendSegmentsToFile(sliceFile, segments, xref); err != nil {
				logrus.Errorf("写入段错误: %v", err)
				continue
			}

			size32, err := util.FromBytes[int32](segmentResults["SIZE"].Data)
			if err != nil {
				continue
			}
			size := int(size32)

			modTimeUnix, err := util.FromBytes[int64](segmentResults["MODTIME"].Data)
			if err != nil {
				continue
			}
			modTime := time.Unix(modTimeUnix, 0) // 从 Unix 时间戳还原为 time.Time

			uploadTimeUnix, err := util.FromBytes[int64](segmentResults["MODTIME"].Data)
			if err != nil {
				continue
			}
			uploadTime := time.Unix(uploadTimeUnix, 0) // 从 Unix 时间戳还原为 time.Time

			// 如果取消应该把共享的数据删除
			if !payload.Shared {
				// 删除
				if err := sqlite.DeleteSharedDatabase(db, payload.FileID); err != nil {
					continue
				}
			} else {
				// 更新文件共享数据
				if err := sqlite.UpdateSharedDatabase(db,
					payload.FileID,                      // 文件的唯一标识
					string(segmentResults["NAME"].Data), // 文件的名称
					size,                                // 文件的长度
					len(xref.XrefTable),                 // Xref表中段的数量
					modTime,                             // 上传时间
					uploadTime,                          // 修改时间
				); err != nil {
					continue
				}
			}

		}
	}
}

// HandleAddSharedRequestPubSub 处理文件新增共享响应的订阅消息
func HandleAddSharedRequestPubSub(opt *opts.Options, p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, db *sqlites.SqliteDB, res *streams.RequestMessage) {
	// 新建文件存储
	fs, err := afero.NewFileStore(filepath.Join(paths.SlicePath, p2p.Host().ID().String()))
	if err != nil {
		return
	}

	// 文件新增共享请求
	payload := new(FileAddSharedRequestPayload)
	if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
		return
	}

	// 判断有效期是否小于当前时间
	if time.Now().After(payload.Expiry) {
		return // 有效期已过，不处理
	}

	// 列出指定子目录中的所有文件
	slices, err := fs.ListFiles(payload.FileID)
	if err != nil {
		return
	}

	for _, sliceHash := range slices {
		sliceFile, err := fs.OpenFile(payload.FileID, sliceHash)
		if err != nil {
			continue
		}

		// 定义需要读取的段类型
		segmentTypes := []string{
			"FILEID",
			"SHARED",
			"FILEKEY",
		}
		segmentResults, xref, err := segment.ReadFileSegments(sliceFile, segmentTypes)
		if err != nil {
			// TODO: 需要与请求方进行交互
			continue
		}
		// 当xref表中的字段大于Xref表中段的最大数量时直接退出
		if len(xref.XrefTable) > int(opt.GetMaxXrefTable()) {
			continue
		}
		// 处理每个段的结果
		for segmentType, result := range segmentResults {
			if result.Error != nil {
				// 出现任何错误，立即退出
				continue
			}
			switch segmentType {
			case "FILEID":
				if payload.FileID != string(result.Data) {
					continue
				}
			case "SHARED":
				shared, err := util.FromBytes[bool](result.Data)
				if err != nil || !shared { // 共享状态不对直接退出
					continue
				}
			case "FILEKEY":
				if bytes.Equal(result.Data, []byte(payload.FileKey)) {
					continue // 文件的密钥不对
				}
			}
		}

		// 将 modTime、uploadTime 转换为 []byte
		modTimeByte, err := util.ToBytes[int64](time.Now().Unix())
		if err != nil {
			continue
		}

		// 计算 UserPubHash 的 MD5 哈希值
		hash := md5.New()
		hash.Write(payload.UserPubHash)
		md5Hash := strings.ToUpper(hex.EncodeToString(hash.Sum(nil))) // 字母都大写

		// 将有效期转换为 []byte
		expiryByte, err := util.ToBytes[int64](payload.Expiry.Unix())
		if err != nil {
			return
		}

		// 创建多个段
		segments := map[string][]byte{
			"MODTIME": modTimeByte, // 文件的修改时间
			md5Hash:   expiryByte,  // 有效期的段
		}
		if err := segment.AppendSegmentsToFile(sliceFile, segments, xref); err != nil {
			logrus.Errorf("写入段错误: %v", err)
			continue
		}
	}
}
