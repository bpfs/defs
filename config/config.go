package config

// 事件协议
const (
	// 文件上传检查事件
	EventFileUploadCheck = "defs@event:file/upload/check/1.0.0"
	// 文件片段上传事件
	EventFileSliceUpload = "defs@event:file/slice/upload/1.0.0"
	// 文件下载开始事件
	EventFileDownloadStart = "defs@event:file/download/start/1.0.0"
	// 文件下载检查事件
	EventFileDownloadCheck = "defs@event:file/download/check/1.0.0"
)

// 订阅主题
const (
	// 文件上传请求主题
	PubsubFileUploadRequestTopic = "defs@pubsub:file/upload/request/1.0.0"
	// 文件上传响应主题
	PubsubFileUploadResponseTopic = "defs@pubsub:file/upload/response/1.0.0"
	// 文件下载请求主题
	PubsubFileDownloadRequestTopic = "defs@pubsub:file/download/request/1.0.0"
	// 文件下载响应主题
	PubsubFileDownloadResponseTopic = "defs@pubsub:file/download/response/1.0.0"
	// 文件删除请求主题
	PubsubFileDeleteRequestTopic = "defs@pubsub:file/delete/request/1.0.0"
	// 文件修改请求主题
	PubsubFileEditRequestTopic = "defs@pubsub:file/edit/request/1.0.0"
	// 新增共享主题
	PubsubAddSharedRequestTopic = "defs@pubsub:add/shared/request/1.0.0"
	// 新增搜索请求主题
	PubsubAddSearchRequestTopic = "defs@pubsub:add/search/request/1.0.0"
	// 新增搜索响应主题
	PubsubAddSearchResponseTopic = "defs@pubsub:add/search/response/1.0.0"
)

// 流协议
const (
	// 文件片段上传协议
	StreamFileSliceUploadProtocol = "defs@stream:file/slice/upload/1.0.0"
	// 文件下载响应协议
	StreamFileDownloadResponseProtocol = "defs@stream:file/download/response/1.0.0"
)
