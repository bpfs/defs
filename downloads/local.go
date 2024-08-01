package downloads

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"path/filepath"

	"github.com/bpfs/defs/afero"
	"github.com/bpfs/defs/crypto/gcm"
	"github.com/bpfs/defs/debug"
	"github.com/bpfs/defs/opts"
	"github.com/bpfs/defs/paths"
	"github.com/bpfs/defs/script"
	"github.com/bpfs/defs/segment"
	"github.com/bpfs/defs/sign/ecdsa"
	"github.com/bpfs/defs/util"
	"github.com/bpfs/defs/zip/gzip"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/bpfs/dep2p"
	"github.com/sirupsen/logrus"
)

// sendDownloadRequest 向指定的节点发送下载请求
// 参数：
//   - opt: *opts.Options 文件存储选项配置
//   - afe: afero.Afero 文件系统接口
//   - p2p: *dep2p.DeP2P 网络主机
//   - downloadChan: chan *DownloadChan 下载状态更新通道
//   - receiver: peer.ID 目标节点的 ID
//   - downloadMaximumSize: int64 下载最大回复大小
//   - prioritySegment: int 优先下载的文件片段索引
//   - segmentInfo: map[int]string 文件片段的索引和唯一标识的映射
//
// 返回值：
//   - bool: 请求是否成功
func (task *DownloadTask) sendDownloadRequest(
	opt *opts.Options,
	afe afero.Afero,
	p2p *dep2p.DeP2P,
	downloadChan chan *DownloadChan,
	receiver peer.ID,
	downloadMaximumSize int64,
	prioritySegment int,
	segmentInfo map[int]string,
) bool {
	// 向指定的节点发送请求以下载文件片段
	reply, err := RequestStreamGetSliceToLocal(p2p, receiver, downloadMaximumSize, task.UserPubHash, task.TaskID, task.File.FileID, prioritySegment, segmentInfo)
	if err != nil {
		logrus.Errorf("[%s]向指定的节点发送请求以下载文件片段失败: %v", debug.WhereAmI(), err)
		return false
	}

	if reply == nil || len(reply.SegmentInfo) == 0 {
		return false
	}

	// 处理下载文件片段的回复信息
	if _, err := processSegmentInfo(opt, afe, p2p, task, receiver, reply.SegmentInfo, downloadChan); err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return false
	}

	return true
}

// updateDownloadProgress 更新下载进度，并检查是否需要合并文件
// 参数：
//   - task: *DownloadTask 下载任务
//   - index: int 文件片段的索引
//
// 返回值：
//   - bool: 是否触发了合并操作
func updateDownloadProgress(task *DownloadTask, index int) {
	// func updateDownloadProgress(task *DownloadTask, index int) bool {
	// 设置文件片段的下载状态: 下载完成
	task.File.SetSegmentStatus(index, SegmentStatusCompleted)
	task.Progress.Set(index)

	// 检查已完成的片段数量并触发合并操作
	// return task.CheckAndTriggerMerge()
}

// requestErasureCodeDownload 请求下载纠删码片段
// 参数：
//   - task: *DownloadTask 下载任务
//   - index: int 文件片段的索引
func requestErasureCodeDownload(task *DownloadTask, index int) {
	// 检查指定索引的片段状态是否为下载失败
	if segment, ok := task.File.GetSegment(index); ok && segment.IsStatus(SegmentStatusFailed) {
		// 遍历所有文件片段，查找纠删码片段
		task.File.Segments.Range(func(key, value interface{}) bool {
			segment := value.(*FileSegment)
			// 如果片段是纠删码片段并且状态为待下载，触发下载事件
			if segment.IsRsCodes && segment.IsStatus(SegmentStatusPending) {
				task.EventDownSnippetChan(segment.Index) // 传递需要下载的片段索引
				return false                             // 终止遍历
			}
			return true // 继续遍历
		})
	}
}

// requestErasureCodeDownload 请求下载纠删码片段
// 参数：
//   - task: *DownloadTask 下载任务
//   - index: int 文件片段的索引
// func requestErasureCodeDownload(task *DownloadTask, index int) {
// 	if task.File.Segments[index].Status == SegmentStatusFailed {
// 		for _, segment := range task.File.Segments {
// 			if segment.IsRsCodes && segment.Status == SegmentStatusPending {
// 				task.EventDownSnippetChan(segment.Index) // 传递需要下载的片段索引
// 				break
// 			}
// 		}
// 	}
// }

// writeToLocalFile 写入本地文件
// 参数：
//   - fs: *afero.FileStore 文件存储对象，用于文件的读写操作
//   - secret: []byte 文件加密密钥
//   - fileID: string 文件唯一标识
//   - segmentID: string 文件片段的唯一标识
//   - data: []byte 文件片段数据
//
// 返回值：
//   - error: 如果发生错误，返回错误信息
func writeToLocalFile(opt *opts.Options, afe afero.Afero, p2p *dep2p.DeP2P, secret []byte, fileID, segmentID string, data []byte) error {
	// 创建一个字节读取器
	bytesReader := bytes.NewReader(data)

	// 从字节缓冲区中加载交叉引用表（xref）
	xref, err := segment.LoadXrefFromBuffer(bytesReader)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 定义文件片段的数据类型
	segmentTypes := []string{
		"FILEID",          // 文件唯一标识
		"CONTENTTYPE",     // MIME类型
		"CHECKSUM",        // 文件的校验和
		"P2PKSCRIPT",      // 写入文件的 P2PK 脚本
		"SLICETABLE",      // 文件片段的哈希表
		"SEGMENTID",       // 文件片段的唯一标识
		"INDEX",           // 文件片段的索引
		"SEGMENTCHECKSUM", // 分片的校验和
		"CONTENT",         // 文件片段的内容(加密)
		"SIGNATURE",       // 文件和文件片段的数据签名
	}

	// 从字节数据中读取字段
	segmentResults, err := segment.ReadFieldsFromBytes(data, segmentTypes, xref)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return fmt.Errorf("非法文件片段")
	}

	// 检查并提取每个段的数据
	for _, result := range segmentResults {
		if result.Error != nil {
			logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
			return err
		}
	}

	// 提取具体数据
	fileIDData := segmentResults["FILEID"].Data         // 读取文件的唯一标识
	segmentIDData := segmentResults["SEGMENTID"].Data   // 读取文件的唯一标识
	contentData := segmentResults["CONTENT"].Data       // 文件片段的内容(加密)
	signatureData := segmentResults["SIGNATURE"].Data   // 文件和文件片段的数据签名
	p2pkScriptData := segmentResults["P2PKSCRIPT"].Data // 读取文件的 P2PK 脚本

	// 检查文件ID和片段ID是否匹配
	if fileID != string(fileIDData) || segmentID != string(segmentIDData) {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 从脚本中提取公钥
	pubKey, err := script.ExtractPubKeyFromP2PKScriptToECDSA(p2pkScriptData)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 组装签名数据
	merged, err := util.MergeFieldsForSigning(
		segmentResults["FILEID"].Data,
		segmentResults["CONTENTTYPE"].Data,
		segmentResults["CHECKSUM"].Data,
		segmentResults["SLICETABLE"].Data,
		segmentResults["SEGMENTID"].Data,
		segmentResults["INDEX"].Data,
		segmentResults["SEGMENTCHECKSUM"].Data,
		segmentResults["CONTENT"].Data,
	)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 使用ECDSA公钥验证数据的签名
	valid, err := ecdsa.VerifySignature(pubKey, merged, signatureData)
	if err != nil || !valid {
		logrus.Errorf("[%s]使用ECDSA公钥验证数据的签名时失败: %v", debug.WhereAmI(), err)
		return fmt.Errorf("验证数据的签名时失败") // 签名验证失败
	}

	// 解压数据
	decompressData, err := gzip.DecompressData(contentData)
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 使用密钥对数据进行解密
	key := md5.Sum(secret)
	content, err := gcm.DecryptData(decompressData, key[:])
	if err != nil {
		logrus.Errorf("[%s]: %v", debug.WhereAmI(), err)
		return err
	}

	// 写入本地文件
	subDir := filepath.Join(paths.GetDownloadPath(), p2p.Host().ID().String(), fileID)
	if err := util.Write(opt, afe, subDir, segmentID, content); err != nil {
		logrus.Errorf("[%s]写入本地文件失败: %v", debug.WhereAmI(), err)
		return err
	}

	return nil
}
