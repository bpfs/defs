package defs

import (
	"testing"
)

func TestReadFileWithShards(t *testing.T) {
	// 无效的文件路径
	// file, err := ReadFileWithShards("/Users/qinglong/go/src/chaincodes/传输和存储/bpfs1.0.1/DeP2P 白皮书.pdf", 2, 2)
	// if err == nil {
	// 	fmt.Println("=========", file.fileHash)
	// 	for _, v := range file.sliceList {
	// 		fmt.Println("---------", v.sliceHash)
	// 	}
	// } else {
	// 	t.Errorf("预期错误为不存在的文件，得到 %v", err)
	// }

	// 无效的参数
	// _, err = ReadFileWithShards("your_actual_file.txt", -1, 2)
	// if err == nil {
	// 	t.Errorf("Expected error for invalid dataShards, got nil")
	// }

	// // 小文件
	// // 创建一个临时小文件
	// fs := afero.NewMemMapFs()
	// afero.WriteFile(fs, "smallfile.txt", []byte("this is small"), os.ModePerm)
	// _, err = ReadFileWithShards("smallfile.txt", 20, 10)
	// if err == nil {
	// 	t.Errorf("Expected error for small file, got nil")
	// }

	// // 大文件
	// // 创建一个临时大文件
	// bigContent := make([]byte, MaxBufferSize+1)
	// rand.Read(bigContent)
	// afero.WriteFile(fs, "bigfile.txt", bigContent, os.ModePerm)
	// _, err = ReadFileWithShards("bigfile.txt", 2, 2)
	// if err == nil {
	// 	t.Errorf("Expected error for large file, got nil")
	// }

	// Hash值检查和时间信息
	// 在这里添加针对您实际文件的hash和时间信息的检查

	// 并发测试
	// 尝试在多线程环境中执行读取操作
}

// func TestReadFileWithSizeAndRatio(t *testing.T) {
// 	// 无效的文件路径
// 	_, err := ReadFileWithSizeAndRatio("nonexistentfile.txt", 1024, 0.5)
// 	if err == nil {
// 		t.Errorf("Expected error for nonexistent file, got nil")
// 	}

// 	// 无效的参数
// 	_, err = ReadFileWithSizeAndRatio("your_actual_file.txt", -1024, 0.5)
// 	if err == nil {
// 		t.Errorf("Expected error for invalid shardSize, got nil")
// 	}

// 	// 小文件
// 	fs := afero.NewMemMapFs()
// 	afero.WriteFile(fs, "smallfile.txt", []byte("this is small"), os.ModePerm)
// 	_, err = ReadFileWithSizeAndRatio("smallfile.txt", 10240, 0.5)
// 	if err == nil {
// 		t.Errorf("Expected error for small file, got nil")
// 	}

// 	// 大文件
// 	bigContent := make([]byte, MaxBufferSize+1)
// 	rand.Read(bigContent)
// 	afero.WriteFile(fs, "bigfile.txt", bigContent, os.ModePerm)
// 	_, err = ReadFileWithSizeAndRatio("bigfile.txt", 1024, 0.5)
// 	if err == nil {
// 		t.Errorf("Expected error for large file, got nil")
// 	}

// 	// Hash值检查和时间信息
// 	// 在这里添加针对您实际文件的hash和时间信息的检查

// 	// 并发测试
// 	// 尝试在多线程环境中执行读取操作
// }

// func TestYyy(t *testing.T) {
// 	fi, err := ReadFileWithShards("合约 - 比特币.pdf", 10, 3)
// 	if err != nil {
// 		logrus.Printf("%v", err.Error())
// 		return
// 	}

// 	logrus.Printf("name\t%v", fi.name)
// 	logrus.Printf("size\t%v", fi.size)
// 	logrus.Printf("modTime\t%v", fi.modTime)
// 	logrus.Printf("hash\t%v", fi.hash)
// 	logrus.Printf("dataShards\t%v", fi.dataShards)
// 	logrus.Printf("parityShards\t%v", fi.parityShards)

// 	for k, singleData := range fi.slice {
// 		logrus.Printf("【%d】\n", k)
// 		logrus.Printf("index\t==>\t%v", singleData.index)
// 		logrus.Printf("hash\t==>\t%v", singleData.hash)
// 		logrus.Printf("mode\t==>\t%v", singleData.mode)
// 		logrus.Printf("content\t==>\t%v", len(singleData.content))

// 		// file, err := os.Create(fmt.Sprint(k) + ".pnga")
// 		file, err := os.Create(singleData.hash + ".bpdf")
// 		if err != nil {
// 			logrus.Printf("///\t%s", err.Error())
// 			return
// 		}
// 		defer file.Close()

// 		file.Write(singleData.content)
// 	}

// 	// AES加密的密钥，长度需要是16、24或32字节
// 	key := md5.Sum([]byte(fi.hash))
// 	op(fi.slice[0].hash, key[:])
// }

// func op(hash string, key []byte) {
// 	// 重新打开文件进行读取
// 	file, err := os.Open(hash + ".pnga")
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer file.Close()

// 	// 读取并验证文件头
// 	header := make([]byte, len(FileHeader))
// 	if _, err := io.ReadFull(file, header); err != nil {
// 		panic(err)
// 	}
// 	if string(header) != FileHeader {
// 		fmt.Println("这不是一个有效的PNGA文件")
// 		return
// 	}

// 	// var version uint32
// 	// if err := binary.Read(file, binary.BigEndian, &version); err != nil {
// 	// 	panic(err)
// 	// }

// 	// // 验证版本信息
// 	// if version != Version {
// 	// 	fmt.Printf("不支持的文件版本：%d\n", version)
// 	// 	return
// 	// }

// 	// 读取IHDR块
// 	chunkType, data, err := readChunk(file, "IHDR", key)
// 	if err != nil {
// 		logrus.Printf("【报错】\t%s", err)
// 		return
// 	}
// 	fmt.Printf("块类型：%s，数据：%s\n", chunkType, string(data))

// 	// 读取META块
// 	chunkType, data, err = readChunk(file, "META", key)
// 	if err != nil {
// 		logrus.Printf("【报错】\t%s", err)
// 		return
// 	}
// 	fmt.Printf("块类型：%s，数据：%s\n", chunkType, string(data))
// }
