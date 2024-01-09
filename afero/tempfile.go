package afero

// type TempFile struct {
// 	*FileStore
// 	ExpireAfter time.Duration
// }

// // 新建临时文件
// func NewTempFile(basePath string, expireAfter time.Duration) (*TempFile, error) {
// 	fs, err := NewFileStore(basePath)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &TempFile{
// 		FileStore:   fs,
// 		ExpireAfter: expireAfter,
// 	}, nil
// }

// func (tf *TempFile) Create(pattern string, data []byte) (string, error) {
// 	tempFile, err := afero.TempFile(tf.Fs, tf.BasePath, pattern)
// 	if err != nil {
// 		return "", err
// 	}
// 	defer tempFile.Close()

// 	if _, err := tempFile.Write(data); err != nil {
// 		return "", err
// 	}

// 	fileName := tempFile.Name()

// 	time.AfterFunc(tf.ExpireAfter, func() {
// 		go tf.Delete("", filepath.Base(fileName))
// 	})

// 	return filepath.Base(fileName), nil
// }
