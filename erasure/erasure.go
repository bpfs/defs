package erasure

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"time"

	"github.com/bpfs/defs"

	"github.com/klauspost/reedsolomon"
	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
)

type mode int

const (
	modeSlice   mode = iota + 1 // 切片模式
	modeErasure                 // 纠删模式
)

// FileInfo 描述一个文件
type FileInfo struct {
	name        string    // 文件的基本名称
	size        int64     // 常规文件的长度（以字节为单位）
	modTime     time.Time // 修改时间
	data        []byte    // 文件的内容
	hash        string    // 文件内容的Hash
	SubFileInfo []SubFileInfo

	// Mode    fs.FileMode // 文件模式位
	// IsDir   bool        // Mode().IsDir() 的缩写
	// Sys     any         // 底层数据源（可以返回nil），不是跨平台的
}
type SubFileInfo struct {
	size int64  // 常规文件的长度（以字节为单位）
	data []byte // 文件的内容
	hash string // 文件内容的Hash
	mod  mode
}

func ReadFile(filename string) (*FileInfo, error) {
	fs := afero.NewOsFs()

	// Open 打开一个文件，返回该文件或错误（如果发生）。
	f, err := fs.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// FileInfo 会准确地告诉我们要读多少内容，这是一个很好但不确定的赌注，所以让我们尝试一下，但要做好答案错误的准备。
	var n int64

	fileInfo := new(FileInfo)

	if fi, err := f.Stat(); err == nil {
		fileInfo.name = fi.Name()       // 文件的基本名称
		fileInfo.size = fi.Size()       // 常规文件的长度（以字节为单位）； 系统依赖于其他人
		fileInfo.modTime = fi.ModTime() // 修改时间

		// 不要预先分配巨大的缓冲区，以防万一。
		if size := fi.Size(); size < 1e9 {
			n = size
		}
	}

	data, err := readAll(f, n+bytes.MinRead)
	if err != nil {
		return nil, err
	}
	fileInfo.data = data

	// 重置文件阅读器
	// _, _ = f.Seek(0, io.SeekStart)

	hasher := sha256.New()
	hasher.Write(data)
	// _, err = io.Copy(hasher, f)
	// if err != nil {
	// 	return nil, err
	// }
	fileInfo.hash = hex.EncodeToString(hasher.Sum(nil))

	subFileInfo, err := readSplit(fileInfo.hash, data, 10, 3)
	if err != nil {
		return nil, err
	}
	fileInfo.SubFileInfo = subFileInfo

	return fileInfo, nil
}

// readAll 从 r 读取直到出现错误或 EOF，并返回从分配有指定容量的内部缓冲区读取的数据。
func readAll(r io.Reader, capacity int64) (b []byte, err error) {
	// NewBuffer 使用 buf 作为初始内容创建并初始化一个新的 Buffer。
	buf := bytes.NewBuffer(make([]byte, 0, capacity))
	// 如果缓冲区溢出，我们将得到 bytes.ErrTooLarge。
	// 将其作为错误返回。 任何其他恐慌仍然存在。
	defer func() {
		e := recover()
		if e == nil {
			return
		}
		if panicErr, ok := e.(error); ok && panicErr == bytes.ErrTooLarge {
			err = panicErr
		} else {
			panic(e)
		}
	}()

	// ReadFrom 从 r 读取数据直到 EOF 并将其附加到缓冲区，根据需要增加缓冲区。
	_, err = buf.ReadFrom(r)
	return buf.Bytes(), err
}

// readSplit 对数据切片进行分割，并根据需要创建纠删码
func readSplit(fileHash string, data []byte, dataShards, parityShards int) ([]SubFileInfo, error) {
	enc, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil, err
	}

	split, err := enc.Split(data)
	if err != nil {
		log.Panic(err.Error())
	}

	if err := enc.Encode(split); err != nil {
		log.Panic(err.Error())
	}

	var subFileInfo []SubFileInfo
	hasher := sha256.New()

	for k, v := range split {
		sfi := new(SubFileInfo)

		hasher.Write(v)
		sfi.hash = hex.EncodeToString(hasher.Sum(nil)) // 切片内容的Hash
		sfi.size = int64(len(v))                       // 内容的大小
		// sfi.data = v                                   // 切片的内容

		fileHashMd5 := md5.Sum([]byte(fileHash))
		// 使用指定的密钥和明文对数据进行加密
		sliceBytePwd, err := defs.Encrypt([]byte(hex.EncodeToString(fileHashMd5[:])), v)
		if err != nil {
			return nil, err
		}

		log.Printf("加密后==>\t\tdata\t%v", len(sliceBytePwd))
		var buff bytes.Buffer
		buff.Write(sliceBytePwd)
		log.Printf("第一次追加==>\tdata\t%v", len(buff.Bytes()))

		buff.Write([]byte(sfi.hash))
		log.Printf("第二次追加==>\tdata\t%v", len(buff.Bytes()))

		sfi.data = buff.Bytes() // 切片的内容(加密)

		if k < dataShards {
			sfi.mod = modeSlice // 切片模式
		} else {
			sfi.mod = modeErasure // 纠删模式

		}

		subFileInfo = append(subFileInfo, *sfi)

		log.Printf("--->\t%d\t%d\t\t%x", k+1, len(v), md5.Sum(v))
	}

	return subFileInfo, nil
}

// func FileSplit(filename string, data []byte) ([][]byte, error) {
// 	bigfile, _ := afero.ReadDir(afero.NewOsFs(), filename)

// 	return nil, nil
// }

func New(path string, parityShards int) error {

	bigfile, err := afero.ReadFile(afero.NewOsFs(), path)
	if err != nil {
		logrus.Panic(err.Error())
	}

	// 	New 创建一个新编码器并将其初始化为您要使用的数据分片和奇偶校验分片的数量。 您可以重复使用该编码器。 请注意，总分片的最大数量为 65536，对于总分片数量超过 256 有一些限制：
	// · 分片大小必须是 64 的倍数
	// · 不支持 Join/Split/Update/EncodeIdx 方法
	// 如果未提供任何选项，则使用默认选项。
	//
	// WithAutoGoroutines 将调整 goroutine 的数量，以达到特定分片大小的最佳速度。
	// 发送您期望发送的分片大小。 其他分片大小也可以，但可能无法以最佳速度运行。
	// 覆盖 WithMaxGoroutines。
	// 如果 shardSize <= 0，则会被忽略。
	// enc, err := reedsolomon.New(10, 3, reedsolomon.WithAutoGoroutines(100))
	enc, err := reedsolomon.New(10, parityShards)
	if err != nil {
		return err
	}

	// Split the file
	// 将数据切片分割为提供给编码器的分片数量，并在必要时创建空奇偶校验分片。
	// 数据将被分割成大小相等的分片。 如果数据大小不能被分片数量整除，则最后一个分片将包含额外的零。
	// 如果提供的数据片上有额外的容量，则将使用它而不是分配奇偶校验分片。 它将被归零。
	// 必须至少有 1 个字节，否则将返回 ErrShortData。
	// 除最后一个分片外，数据不会被复制，因此您不应在之后修改输入分片的数据。
	split, err := enc.Split(bigfile)
	if err != nil {
		log.Panic(err.Error())
	}

	for k, v := range split {
		log.Printf("--->\t%d\t%d\t\t%x", k+1, len(v), md5.Sum(v))
	}

	// Encode 一组数据分片的奇偶校验。
	// 输入是“分片”，其中包含数据分片，后跟奇偶校验分片。
	// 分片的数量必须与 New() 指定的数量相匹配。
	// 每个分片都是一个字节数组，并且它们的大小必须相同。
	// 奇偶校验分片将始终被覆盖，并且数据分片将保持不变，因此您在运行时从数据分片中读取数据是安全的。
	if err := enc.Encode(split); err != nil {
		log.Panic(err.Error())
	}
	log.Printf("\n")
	for k, v := range split {
		log.Printf("--->\t%d\t%d\t\t%x", k+1, len(v), md5.Sum(v))
	}

	// 检查它是否验证
	ok, err := enc.Verify(split)
	if !ok || err != nil {
		log.Panic("not ok:", ok, "err:", err)
	}

	// 删除分片
	split[0] = nil

	log.Printf("\n")
	for k, v := range split {
		log.Printf("--->\t%d\t%d\t\t%x", k+1, len(v), md5.Sum(v))
	}

	// 应该重建
	err = enc.Reconstruct(split)
	if err != nil {
		log.Panic(err.Error())
	}
	log.Printf("\n")
	for k, v := range split {
		log.Printf("--->\t%d\t%d\t\t%x", k+1, len(v), md5.Sum(v))
	}

	// 检查它是否验证
	ok, err = enc.Verify(split)
	if err != nil {
		log.Print("err:", err.Error())
		return nil
	}
	if !ok {
		log.Print("not ok:", ok)
		// return nil
	}

	// 恢复原始字节
	buf := new(bytes.Buffer)
	// Join a data set and write it to io.Discard.
	// 	Join 分片并将数据段写入 dst。
	// 仅考虑数据碎片。 您必须提供所需的准确输出尺寸。
	// 如果给出的分片太少，将返回 ErrTooFewShards。
	// 如果总数据大小小于outSize，将返回ErrShortData。
	// err = enc.Join(io.Discard, split, len(bigfile))
	err = enc.Join(buf, split, len(bigfile))
	if err != nil {
		log.Panic(err.Error())
	}

	// 破坏碎片
	split[0] = nil
	split[1][0], split[1][500] = 75, 75

	// 应该重建（但数据损坏）
	err = enc.Reconstruct(split)
	if err != nil {
		log.Panic(err.Error())
	}

	// 检查它是否验证
	ok, err = enc.Verify(split)
	if err != nil {
		log.Print("err:", err.Error())
		return nil
	}
	if !ok {
		log.Print("not ok:", ok)
		// return nil
	}

	// 恢复的数据不应与原始数据匹配
	buf.Reset()
	err = enc.Join(buf, split, len(bigfile))
	if err != nil {
		log.Panic(err.Error())
	}

	return nil
}
