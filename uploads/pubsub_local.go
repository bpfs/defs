// Package uploads 实现文件上传相关功能
package uploads

import (
	"context"

	"github.com/bpfs/defs/pb"

	"github.com/dep2p/pubsub"
)

// RequestDeleteFileSegmentPubSub 发送删除文件片段请求
// 参数:
//   - ctx: 上下文,用于控制请求的生命周期
//   - nps: 发布订阅系统,用于节点之间的消息传递
//   - fileID: 文件唯一标识
//   - pubkeyHash: 所有者的公钥哈希
//
// 返回值:
//   - error: 如果请求过程中出现错误,返回相应的错误信息
func RequestDeleteFileSegmentPubSub(
	ctx context.Context,
	nps *pubsub.NodePubSub,
	fileID string,
	pubkeyHash []byte,
) error {
	// 构造删除文件片段请求数据
	requestData := &pb.UploadPubSubDeleteFileSegmentRequest{
		FileId:     fileID,     // 设置文件ID
		PubkeyHash: pubkeyHash, // 设置公钥哈希
	}

	// 序列化请求数据为二进制格式
	data, err := requestData.Marshal()
	if err != nil {
		logger.Errorf("序列化删除文件片段请求数据失败: fileID=%s, err=%v", fileID, err)
		return err
	}

	// 获取删除文件片段请求的发布主题
	topic, err := nps.GetTopic(PubSubDeleteFileSegmentRequestTopic.String())
	if err != nil {
		logger.Errorf("获取删除文件片段主题失败: topic=%s, err=%v", PubSubDeleteFileSegmentRequestTopic.String(), err)
		return err
	}

	// 发布删除文件片段请求消息
	if err = topic.Publish(ctx, data); err != nil {
		logger.Errorf("发布删除文件片段消息失败: fileID=%s, topic=%s, err=%v", fileID, PubSubDeleteFileSegmentRequestTopic.String(), err)
		return err
	}

	return nil
}
