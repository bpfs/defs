package upload

import (
	"fmt"
	"log"
	"time"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/core"
	"github.com/bpfs/defs/core/config"
	"github.com/bpfs/defs/core/network"
	"github.com/bpfs/defs/core/pool"
	"github.com/bpfs/defs/opts"

	"github.com/bpfs/defs/core/sqlite"
	"github.com/bpfs/defs/eventbus"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/ristretto"
	"github.com/bpfs/defs/sqlites"
	"github.com/bpfs/dep2p"
	"github.com/bpfs/dep2p/pubsub"
	"github.com/sirupsen/logrus"
)

// registerFileUploadCheckEvent 注册文件上传检查事件
func RegisterFileUploadCheckEvent(
	p2p *dep2p.DeP2P,
	pubsub *pubsub.DeP2PPubSub,
	db *sqlites.SqliteDB,
	registry *eventbus.EventRegistry,
	pool *pool.MemoryPool,
) error {

	// 获取事件总线
	bus := registry.GetEventBus(config.EventFileUploadCheck)
	if bus == nil {
		return fmt.Errorf("无法获取切片上传事件总线")
	}

	// 注册
	return bus.Subscribe(config.EventFileUploadCheck, func(
		fileID string, // 文件的唯一标识
		dataShards int64, // 数据分片
		parityShards int64, // 奇偶分片
		uploadTime time.Time, // 上传时间
	) error {
		// 插入文件上传数据
		if err := sqlite.InsertFilesDatabase(db,
			fileID,                         // 文件的唯一标识
			int64(dataShards+parityShards), // 文件片段的总量
			1,                              // 操作(1:上传)
			2,                              // 状态(2:待开始)
			time.Now(),                     // 时间(当前时间)
		); err != nil {
			return err
		}

		log.Printf("[文件网络检查] fileID:\t%s\n", fileID)
		// 本地数据存储
		// 发送本地上传的切片
		// go SendLocalUploadSlice(db, registry, pool, fileID, sliceHash, total, current)

		// 文件上传请求(检查)
		requestCheckPayload := &FileUploadRequestCheckPayload{
			FileID:     fileID,
			UploadTime: uploadTime,
		}

		// 向指定的全网节点发送文件上传请求的订阅消息
		if err := network.SendPubSub(p2p, pubsub, config.PubsubFileUploadRequestTopic, "check", "", requestCheckPayload); err != nil {
			return err
		}

		return nil
	})
}

// registerFileSliceUploadEvent 注册文件片段上传事件
func RegisterFileSliceUploadEvent(
	opt *opts.Options,
	p2p *dep2p.DeP2P,
	pubsub *pubsub.DeP2PPubSub,
	db *sqlites.SqliteDB,
	uploadChan chan *core.UploadChan,
	registry *eventbus.EventRegistry,
	cache *ristretto.Cache,
	pool *pool.MemoryPool,
) error {
	// 获取事件总线
	bus := registry.GetEventBus(config.EventFileSliceUpload)
	if bus == nil {
		return fmt.Errorf("无法获取文件片段上传事件总线")
	}

	// 注册
	return bus.Subscribe(config.EventFileSliceUpload, func(
		fileID string, // 文件的唯一标识(外部标识)
		sliceHash string, // 文件片段的哈希值(外部标识)
		totalPieces int, // 文件片段总量
		current int, // 当前序列
	) error {
		// 插入文件片段数据，状态(0:失败)
		if err := sqlite.InsertSlicesDatabase(db, fileID, sliceHash, current, 0); err != nil {
			return err
		}

	undone:

		// 发送本地上传的切片
		if err := SendFileSliceToNetwork(opt, p2p, uploadChan, registry, cache, pool, fileID, sliceHash, totalPieces, current); err != nil {
			// 更新文件片段数据对象的状态
			if err := sqlite.UpdateSlicesDatabaseStatus(db, fileID, sliceHash, 0); err != nil { // 状态(0:失败)
				return err
			}
			// 更新文件上传数据对象的状态
			if err := sqlite.UpdateFileDatabaseStatus(db, fileID, 1, 0); err != nil { // 状态(0:失败)
				return err
			}

			return err
		}

		// 标记上传任务中的一个片段为完成
		pool.MarkUploadPieceComplete(fileID, current)

		// 更新文件片段数据对象的状态(1:成功)
		if err := sqlite.UpdateSlicesDatabaseStatus(db, fileID, sliceHash, 1); err != nil {
			return err
		}
		// 更新文件上传数据对象的状态
		if current == 1 { // 第一个数据片段
			if err := sqlite.UpdateFileDatabaseStatus(db, fileID, 1, 3); err != nil { // 状态(3:进行中)
				return err
			}
		} else if current == totalPieces { // 最后一个数据片段
			// 检查指定文件是否上传完成
			if !pool.IsUploadComplete(fileID) {
				// 获取未完成的上传片段
				undoneFileID := pool.GetIncompleteUploadPieces(fileID)
				if len(undoneFileID) != 0 {
					s, err := sqlite.SelectOneSlicesDatabaseStatus(db, undoneFileID[0])
					if err != nil {
						sliceHash = s.SliceHash // 文件片段的哈希值
						current = s.SliceIndex  // 文件片段的索引

						goto undone
					}
				}
			}
			if err := sqlite.UpdateFileDatabaseStatus(db, fileID, 1, 1); err != nil { // 状态(1:成功)
				return err
			}

			// 开启本地存储
			if !opt.GetLocalStorage() {
				// 新建文件存储
				fs, err := afero.NewFileStore(paths.UploadPath)
				if err == nil {
					// for _, v := range pool.GetAllPieceHashes(fileID) {
					// 	if err := fs.Delete(fileID, v); err != nil { // 删除文件
					// 		logrus.Errorf("删除文件失败: %v", err)
					// 	}
					// }
					if err := fs.DeleteAll(fileID); err != nil { // 删除所有文件
						logrus.Errorf("删除 %s 的所有文件失败: %v", fileID, err)
					}
				}
				pool.DeleteUploadTask(fileID) // 删除指定文件的上传任务信息
			}

		}

		return nil
	})
}
