package download

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/network"
	"github.com/bpfs/defs/core/pool"
	"github.com/bpfs/defs/core/sqlite"
	"github.com/bpfs/defs/core/util"
	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/segment"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/defs/util/crypto/gcm"
	"github.com/bpfs/defs/util/sign/rsa"
	"github.com/bpfs/defs/util/zip/gzip"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/bpfs/dep2p/streams"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/sirupsen/logrus"
)

// HandleFileDownloadRequestPubSub 处理文件下载请求的订阅消息
func HandleFileDownloadRequestPubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, res *streams.RequestMessage) {
	// 新建文件存储
	fs, err := afero.NewFileStore(filepath.Join(paths.SlicePath, p2p.Host().ID().String()))
	if err != nil {
		return
	}

	receiver, err := peer.Decode(res.Message.Sender)
	if err != nil {
		return
	}

	switch res.Message.Type {
	// 清单
	case "checklist":
		// 文件下载请求(清单)
		payload := new(FileDownloadRequestChecklistPayload)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			return
		}

		// 文件下载响应(清单)
		responseChecklistPayload := FileDownloadResponseChecklistPayload{
			FileID:          payload.FileID,           // 文件的唯一标识
			FileKey:         "",                       // 文件的密钥
			Name:            "",                       // 文件的名称
			Size:            0,                        // 文件的长度(以字节为单位)
			SliceTable:      map[int]core.HashTable{}, // 文件片段的哈希表
			AvailableSlices: map[int]string{},         // 本地存储的文件片段信息
		}

		// 列出指定子目录中的所有文件
		slices, err := fs.ListFiles(payload.FileID)
		if err != nil {
			return
		}

	slicesLoop:
		for _, sliceHash := range slices {
			sliceFile, err := fs.OpenFile(payload.FileID, sliceHash)
			if err != nil {
				logrus.Errorf("%s 打开文件失败:\t%v", sliceHash, err)
				continue
			}

			// 定义需要读取的段类型
			segmentTypes := []string{"FILEID", "NAME", "SIZE", "SLICEHASH", "INDEX", "SHARED", "P2PKHSCRIPT"}
			if len(responseChecklistPayload.SliceTable) == 0 {
				segmentTypes = append(segmentTypes, "SLICETABLE")
			}
			segmentResults, xref, err := segment.ReadFileSegments(sliceFile, segmentTypes)
			if err != nil {
				continue
			}

			// 处理每个段的结果
			for segmentType, result := range segmentResults {
				if result.Error != nil {
					// 出现任何错误，立即继续下一个 sliceHash
					continue slicesLoop
				}

				switch segmentType {
				case "FILEID":
					if payload.FileID != string(result.Data) {
						continue slicesLoop
					}
				case "SLICETABLE":
					var sliceTable map[int]core.HashTable
					if err := util.DecodeFromBytes(result.Data, &sliceTable); err != nil {
						continue slicesLoop
					}
					responseChecklistPayload.SliceTable = sliceTable
				case "NAME":
					responseChecklistPayload.Name = string(result.Data)
				case "SIZE":
					if responseChecklistPayload.Size, err = util.FromBytes[int64](result.Data); err != nil {
						continue slicesLoop
					}
				case "SLICEHASH":
					if sliceHash != string(result.Data) {
						continue slicesLoop
					}
				case "INDEX":
					index, err := util.FromBytes[int32](result.Data)
					if err != nil {
						continue slicesLoop
					}
					responseChecklistPayload.AvailableSlices[int(index)] = sliceHash
				case "SHARED":
					shared, err := util.FromBytes[bool](result.Data)
					if err != nil {
						continue slicesLoop
					}

					if shared {
						// 计算 UserPubHash 的 MD5 哈希值
						hash := md5.New()
						hash.Write(payload.UserPubHash)
						md5Hash := strings.ToUpper(hex.EncodeToString(hash.Sum(nil))) // 字母都大写
						// 从文件读取文件内容的密钥和用户的公钥哈希
						sharedTypes := []string{"FILEKEY", md5Hash}
						sharedResults, _, err := segment.ReadFileSegments(sliceFile, sharedTypes, xref)
						if err != nil {
							continue slicesLoop
						}

						// 处理每个段的结果
						for s, r := range sharedResults {
							if r.Error != nil {
								// 出现任何错误，立即继续下一个 sliceHash
								continue slicesLoop
							}
							switch s {
							case "FILEKEY":
								responseChecklistPayload.FileKey = string(r.Data)
							case md5Hash:
								// 验证用户的公钥哈希
								expiryUnix, err := util.FromBytes[int64](r.Data)
								if err != nil {
									continue slicesLoop
								}
								expiry := time.Unix(expiryUnix, 0) // 从 Unix 时间戳还原为 time.Time
								// 判断有效期是否小于当前时间
								if time.Now().After(expiry) {
									continue slicesLoop
								}
							}
						}
					} else {
						// 验证脚本中所有者的公钥哈希
						if !script.VerifyScriptPubKeyHash(segmentResults["P2PKHSCRIPT"].Data, payload.UserPubHash) {
							continue slicesLoop
						}
					}
				}
			}
		}

		// 发送响应清单
		_ = network.SendDataToPeer(p2p, pubsub, config.StreamFileDownloadResponseProtocol, config.PubsubFileDownloadResponseTopic, "checklist", receiver, responseChecklistPayload)

	// 内容
	case "content":
		// 文件下载请求(内容)
		payload := new(FileDownloadRequestContentPayload)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			logrus.Errorf("[HandleBlock] 解码失败:\t%v", err)
			return
		}

		// 读取文件
		sliceContent, err := fs.Read(payload.FileID, payload.SliceHash)
		if err != nil {
			return
		}

		// 文件下载响应(内容)
		responseContentPayload := FileDownloadResponseContentPayload{
			FileID:       payload.FileID,
			SliceHash:    payload.SliceHash,
			Index:        payload.Index,
			SliceContent: sliceContent,
		}

		// 尝试先通过流发送数据，失败后通过订阅发送
		_ = network.SendDataToPeer(p2p, pubsub, config.StreamFileDownloadResponseProtocol, config.PubsubFileDownloadResponseTopic, "content", receiver, responseContentPayload)
	}
}

// HandleFileDownloadResponsePubSub 处理文件下载响应的订阅消息
func HandleFileDownloadResponsePubSub(p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, db *sqlites.SqliteDB, downloadChan chan *core.DownloadChan, registry *eventbus.EventRegistry, pool *pool.MemoryPool, res *streams.RequestMessage) {
	receiver, err := peer.Decode(res.Message.Sender)
	if err != nil {
		logrus.Errorf("解析peerid失败:\t%v", err)
		return
	}

	switch res.Message.Type {
	case "checklist":
		payload := new(FileDownloadResponseChecklistPayload)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			logrus.Errorf("[HandleFileDownloadResponsePubSub] 解码失败:\t%v", err)
			return
		}

		// 处理文件下载响应清单
		ProcessDownloadResponseChecklist(pool, db, p2p, pubsub, payload, receiver)

	case "content":
		payload := new(FileDownloadResponseContentPayload)
		if err := util.DecodeFromBytes(res.Payload, payload); err != nil {
			logrus.Errorf("[HandleFileDownloadResponsePubSub] 解码失败:\t%v", err)
			return
		}

		// 处理文件下载响应内容
		ProcessDownloadResponseContent(p2p, db, downloadChan, registry, pool, payload)
	}
}

// ProcessDownloadResponseChecklist 处理文件下载响应清单
func ProcessDownloadResponseChecklist(pool *pool.MemoryPool, db *sqlites.SqliteDB, p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, payload *FileDownloadResponseChecklistPayload, receiver peer.ID) {
	// 更新下载任务中特定片段的信息
	if len(payload.FileKey) > 0 {
		pool.UpdateDownloadPieceInfo(receiver.String(), payload.FileID, payload.Name, payload.Size, payload.SliceTable, payload.AvailableSlices, payload.FileKey)
	} else {
		pool.UpdateDownloadPieceInfo(receiver.String(), payload.FileID, payload.Name, payload.Size, payload.SliceTable, payload.AvailableSlices)
	}

	// 更新文件下载数据对象的状态
	if err := sqlite.UpdateFileDatabaseStatus(db, payload.FileID, 0, 3); err != nil { // 状态(3:进行中)
		logrus.Errorf("更新数据库状态失败:\t%v", err)
		return
	}
	// 发送文件下载请求（内容）
	SendDownloadRequestContents(pool, p2p, pubsub, payload, receiver)
}

// SendDownloadRequestContents 发送文件下载请求（内容）
func SendDownloadRequestContents(pool *pool.MemoryPool, p2p *dep2p.DeP2P, pubsub *pubsub.DeP2PPubSub, payload *FileDownloadResponseChecklistPayload, receiver peer.ID) {
	// 获取未完成的下载片段的哈希值
	pieceHashes := pool.GetIncompleteDownloadPieces(payload.FileID)
	for _, hash := range pieceHashes {
		for availableIndex, availableHash := range payload.AvailableSlices { // 本地存储的文件片段信息
			if hash == availableHash {
				responseContentPayload := FileDownloadRequestContentPayload{
					FileID:    payload.FileID,
					SliceHash: availableHash,
					Index:     availableIndex,
				}
				// 尝试先通过流发送数据，失败后通过订阅发送
				//_ = SendDataToPeer(p2p, pubsub, StreamFileDownloadResponseProtocol, PubsubFileDownloadResponseTopic, "content", receiver, responseContentPayload)
				if err := network.SendPubSub(p2p, pubsub, config.PubsubFileDownloadRequestTopic, "content", receiver, responseContentPayload); err != nil {
					logrus.Errorf("[PubsubFileDownloadRequestTopic]发送订阅失败%v", err)
					return
				}

				time.Sleep(1 * time.Second) // 延时以释放网络压力
			}
		}
	}
}

// ProcessDownloadResponseContent 处理文件下载响应内容
func ProcessDownloadResponseContent(p2p *dep2p.DeP2P, db *sqlites.SqliteDB, downloadChan chan *core.DownloadChan, registry *eventbus.EventRegistry, pool *pool.MemoryPool, payload *FileDownloadResponseContentPayload) {
	fs, err := afero.NewFileStore(paths.DownloadPath)
	if err != nil {
		logrus.Errorf("创建文件存储失败:\t%v", err)
		return
	}
	// 子目录当前主机+文件hash
	fs.BasePath = filepath.Join(fs.BasePath, p2p.Host().ID().String()) // 设置子目录

	// 写入本地文件
	if err := writeToLocalFile(pool, fs, payload.FileID, payload.SliceHash, payload.SliceContent); err != nil {
		return
	}

	// 根据文件片段的哈希值标记下载任务中的一个片段为完成
	if pool.MarkDownloadPieceCompleteByHash(payload.FileID, payload.SliceHash) {
		// 触发文件下载检查事件
		bus := registry.GetEventBus(config.EventFileDownloadCheck)
		if bus != nil {
			bus.Publish(config.EventFileDownloadCheck, payload.FileID)
		}
	}

	go func() {
		// 获取下载任务
		task, exists := pool.DownloadTasks[string(payload.FileID)]
		if !exists {
			logrus.Errorf("下载任务不存在: %s", string(payload.FileID))
		}
		// 向下载通道发送信息
		SendDownloadInfo(downloadChan, payload.FileID, payload.SliceHash, task.TotalPieces, payload.Index)
	}()

}

// writeToLocalFile 写入本地文件
func writeToLocalFile(pool *pool.MemoryPool, fs *afero.FileStore, fileID, sliceHash string, data []byte) error {
	// 获取下载任务
	task, exists := pool.DownloadTasks[fileID]
	if !exists {
		return fmt.Errorf("下载任务不存在: %s", fileID)
	}

	bytesReader := bytes.NewReader(data)
	xref, err := segment.LoadXrefFromBuffer(bytesReader)
	if err != nil {
		return err
	}
	//	fmt.Printf("xref:\t%v\n", xref)

	segmentTypes := []string{
		"FILEID",
		"P2PKSCRIPT",
		"SLICETABLE",
		"SLICEHASH",
		"INDEX",
		"CONTENT",
		"SIGNATURE",
	}
	segmentResults, err := segment.ReadFieldsFromBytes(data, segmentTypes, xref)
	if err != nil {
		return fmt.Errorf("非法文件片段")
	}

	// 检查并提取每个段的数据
	for _, result := range segmentResults {
		if result.Error != nil {
			return err
		}
	}

	// 提取具体数据
	fileIDData := segmentResults["FILEID"].Data         // 读取文件的唯一标识
	p2pkScriptData := segmentResults["P2PKSCRIPT"].Data // 读取文件的 P2PK 脚本
	sliceTableData := segmentResults["SLICETABLE"].Data // 读取文件片段的哈希表
	sliceHashData := segmentResults["SLICEHASH"].Data   // 读取文件片段的哈希值
	indexData := segmentResults["INDEX"].Data           // 读取文件片段的索引
	contentData := segmentResults["CONTENT"].Data       // 读取文件片段的内容(加密)
	signatureData := segmentResults["SIGNATURE"].Data   // 读取文件和文件片段的数据签名

	if fileID != string(fileIDData) || sliceHash != string(sliceHashData) {
		return err
	}

	pubKey, err := script.ExtractPubKeyFromP2PKScriptToRSA(p2pkScriptData) // 从脚本中提取公钥
	if err != nil {
		return err
	}

	var sliceTable map[int]core.HashTable
	if err := util.DecodeFromBytes(sliceTableData, &sliceTable); err != nil {
		return err
	}

	index32, err := util.FromBytes[int32](indexData)
	if err != nil {
		return err
	}
	index := int(index32)

	var rc bool
	hashTable, exists := sliceTable[index]
	if exists {
		rc = hashTable.IsRsCodes // 是否为纠删码
	} else {
		return err
	}

	st, err := json.Marshal(sliceTable)
	if err != nil {
		return err
	}
	merged, err := util.MergeFieldsForSigning( // 组装签名数据
		fileID,    // 文件的唯一标识
		st,        // 切片内容的哈希表
		index,     // 文件片段的索引
		sliceHash, // 文件片段的哈希值
		rc,        // 文件片段的存储模式
	)
	if err != nil {
		return err
	}

	// 验证签名
	if !rsa.VerifySignature(pubKey, merged, signatureData) {
		return err
	}

	// AES加密的密钥，长度需要是16、24或32字节
	key := md5.Sum([]byte(task.FileKey))

	// 数据解压
	decompressData, err := gzip.DecompressData(contentData)
	if err != nil {
		return err
	}

	// 解密验证
	decrypted, err := gcm.DecryptData(decompressData, key[:])
	if err != nil {
		return err
	}

	_, content, err := util.SeparateHashFromData(decrypted)
	if err != nil {
		return err
	}

	// // 对比切片hash是否一致
	// if hex.EncodeToString(hash) != sliceHash {
	// 	return fmt.Errorf("非法切片文件")
	// }
	// 写入本地文件
	if err := fs.Write(fileID, sliceHash, content); err != nil {
		logrus.Errorf("写入本地文件失败:\t%v", err)
		return err
	}

	return nil
}
