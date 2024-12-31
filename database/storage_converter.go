package database

import (
	"fmt"

	"github.com/bpfs/defs/pb"
	"github.com/gogo/protobuf/proto"
)

// MapEntry 用于序列化单个 map 条目
type MapEntry struct {
	Key   int64        `protobuf:"varint,1,opt,name=key,proto3" json:"key,omitempty"`
	Value pb.HashTable `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
}

// SliceTableWrapper 用于包装 SliceTable map 以便序列化
type SliceTableWrapper struct {
	Entries []MapEntry `protobuf:"bytes,1,rep,name=entries,proto3" json:"entries,omitempty"`
}

func (m *SliceTableWrapper) Reset()         { *m = SliceTableWrapper{} }
func (m *SliceTableWrapper) String() string { return proto.CompactTextString(m) }
func (*SliceTableWrapper) ProtoMessage()    {}

// ToFileSegmentStorageSql 将 FileSegmentStorage 转换为 FileSegmentStorageSql
func ToFileSegmentStorageSql(m *pb.FileSegmentStorage) (*pb.FileSegmentStorageSql, error) {
	if m == nil {
		return nil, nil
	}

	// 将 map 转换为 entries 数组
	var entries []MapEntry
	for k, v := range m.SliceTable {
		if v != nil {
			entries = append(entries, MapEntry{
				Key:   k,
				Value: *v,
			})
		}
	}

	// 序列化 entries
	wrapper := &SliceTableWrapper{Entries: entries}
	sliceTableBytes, err := proto.Marshal(wrapper)
	if err != nil {
		return nil, fmt.Errorf("序列化 SliceTable 失败: %v", err)
	}

	return &pb.FileSegmentStorageSql{
		FileId:         m.FileId,
		Name:           m.Name,
		Extension:      m.Extension,
		Size_:          m.Size_,
		ContentType:    m.ContentType,
		Sha256Hash:     m.Sha256Hash,
		UploadTime:     m.UploadTime,
		P2PkhScript:    m.P2PkhScript,
		P2PkScript:     m.P2PkScript,
		SliceTable:     sliceTableBytes,
		SegmentId:      m.SegmentId,
		SegmentIndex:   m.SegmentIndex,
		Crc32Checksum:  m.Crc32Checksum,
		SegmentContent: m.SegmentContent,
		EncryptionKey:  m.EncryptionKey,
		Signature:      m.Signature,
		Shared:         m.Shared,
		Version:        m.Version,
	}, nil
}

// ToFileSegmentStorage 将 FileSegmentStorageSql 转换为 FileSegmentStorage
func ToFileSegmentStorage(m *pb.FileSegmentStorageSql) (*pb.FileSegmentStorage, error) {
	if m == nil {
		return nil, nil
	}

	var wrapper SliceTableWrapper
	if err := proto.Unmarshal(m.SliceTable, &wrapper); err != nil {
		return nil, fmt.Errorf("反序列化 SliceTable 失败: %v", err)
	}

	// 将 entries 转换回 map
	sliceTable := make(map[int64]*pb.HashTable)
	for _, entry := range wrapper.Entries {
		value := entry.Value
		sliceTable[entry.Key] = &value
	}

	return &pb.FileSegmentStorage{
		FileId:         m.FileId,
		Name:           m.Name,
		Extension:      m.Extension,
		Size_:          m.Size_,
		ContentType:    m.ContentType,
		Sha256Hash:     m.Sha256Hash,
		UploadTime:     m.UploadTime,
		P2PkhScript:    m.P2PkhScript,
		P2PkScript:     m.P2PkScript,
		SliceTable:     sliceTable,
		SegmentId:      m.SegmentId,
		SegmentIndex:   m.SegmentIndex,
		Crc32Checksum:  m.Crc32Checksum,
		SegmentContent: m.SegmentContent,
		EncryptionKey:  m.EncryptionKey,
		Signature:      m.Signature,
		Shared:         m.Shared,
		Version:        m.Version,
	}, nil
}
