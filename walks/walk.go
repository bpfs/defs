package walks

import (
	"regexp"
)

// 获取文件夹下所有文件名称列表
// func WalkFilesNameList(directory string) ([]string, error) {
// 	appfs := afero.NewOsFs()
// 	fileNames := []string{}

// 	if err := afero.Walk(appfs, directory, func(path string, info os.FileInfo, err error) error {
// 		if err != nil {
// 			return err
// 		}
// 		if info != nil && !info.IsDir() {
// 			// Split 在最后一个分隔符之后立即拆分路径，将其分成目录和文件名组件。 如果路径中没有分隔符，则 Split 返回一个空目录并将文件设置为路径。 返回值具有 path = dir+file 的属性。
// 			_, base := filepath.Split(path)
// 			// Contains 报告 substr 是否在 s 内。
// 			// if strings.Contains(base, " ") && isNum(base) {
// 			if isNum(base) {
// 				// 文件的基本名称
// 				fileNames = append(fileNames, info.Name())
// 			}

// 		}

// 		return nil
// 	}); err != nil {
// 		return nil, err
// 	}

// 	return fileNames, nil
// }

////////////////////

var isNum = regexp.MustCompile(`[0-9]+`).MatchString

// 根据文件Hash获取文件内容
// func WalkFileName(root, fileHash string) (map[string][]byte, error) {
// 	appfs := afero.NewOsFs()
// 	slices := make(map[string][]byte)

// 	if err := afero.Walk(appfs, root, func(path string, info os.FileInfo, err error) error {
// 		if err != nil {
// 			return err
// 		}

// 		if info != nil && !info.IsDir() {
// 			// Split 在最后一个分隔符之后立即拆分路径，将其分成目录和文件名组件。 如果路径中没有分隔符，则 Split 返回一个空目录并将文件设置为路径。 返回值具有 path = dir+file 的属性。
// 			_, base := filepath.Split(path)

// 			// Contains 报告 substr 是否在 s 内。
// 			if strings.Contains(base, " ") && isNum(base) {
// 				sliceNames := info.Name()
// 				parts := strings.Split(sliceNames, " ")

// 				if parts[0] == fileHash {
// 					content, err := os.ReadFile(path)
// 					if err != nil {
// 						return err
// 					}
// 					slices[sliceNames] = content
// 				}
// 			}
// 		}

// 		return nil
// 	}); err != nil {
// 		return nil, err
// 	}

// 	return slices, nil
// }

// 根据文件Hash获取文件路径
// func WalkFilePath(root, fileHash string) (map[string]string, error) {
// 	appfs := afero.NewOsFs()
// 	slices := make(map[string]string)

// 	if err := afero.Walk(appfs, root, func(path string, info os.FileInfo, err error) error {
// 		if err != nil {
// 			return err
// 		}

// 		if info != nil && !info.IsDir() {
// 			_, base := filepath.Split(path)

// 			if strings.Contains(base, " ") && isNum(base) {
// 				sliceNames := info.Name()
// 				parts := strings.Split(sliceNames, " ")

// 				if parts[0] == fileHash {

// 					slices[sliceNames] = path
// 				}
// 			}
// 		}

// 		return nil
// 	}); err != nil {
// 		return nil, err
// 	}

// 	return slices, nil
// }

// 下载循环
// func WalkFileNameDownload(root, fileHash string) (map[string][]byte, error) {
// 	appfs := afero.NewOsFs()
// 	// 初始化一个映射，存储切片哈希到切片内容的映射
// 	slices := make(map[string][]byte)

// 	if err := afero.Walk(appfs, root, func(path string, info os.FileInfo, err error) error {
// 		if err != nil {
// 			return err
// 		}

// 		// 如果遇到文件，则检查文件名是否符合要求
// 		if info != nil && !info.IsDir() {

// 			_, base := filepath.Split(path)
// 			// 判断字符串是否包含“&”和数字
// 			if strings.Contains(base, " ") && isNum(base) {
// 				// 文件的基本名称
// 				sliceNames := info.Name()
// 				parts := strings.Split(sliceNames, " ")
// 				// 如果文件名包含文件哈希，将切片哈希添加到映射中

// 				if parts[0]+" "+parts[1] == fileHash {
// 					// 读取文件名命名的文件并返回内容。
// 					content, err := os.ReadFile(path)
// 					if err != nil {
// 						return err
// 					}
// 					slices[sliceNames] = content
// 				}
// 			}

// 		}

// 		return nil
// 	}); err != nil {
// 		return nil, err
// 	}

// 	return slices, nil
// }

// func WalkSliceName(root, fileHash string) ([]string, error) {
// 	appfs := afero.NewOsFs()
// 	// 初始化一个映射，存储切片哈希到切片内容的映射
// 	var slices []string

// 	if err := afero.Walk(appfs, root, func(path string, info os.FileInfo, err error) error {
// 		if err != nil {
// 			return err
// 		}

// 		// 如果遇到文件，则检查文件名是否符合要求
// 		if info != nil && !info.IsDir() {
// 			// 文件的基本名称
// 			sliceNames := info.Name()
// 			parts := strings.Split(sliceNames, " ")
// 			if fileHash != "" {
// 				// 如果文件名包含文件哈希，将切片哈希添加到映射中
// 				if parts[0] == fileHash {
// 					slices = append(slices, sliceNames)
// 				}
// 			} else {
// 				slices = append(slices, sliceNames)
// 			}

// 		}

// 		return nil
// 	}); err != nil {
// 		return nil, err
// 	}

// 	return slices, nil
// }
