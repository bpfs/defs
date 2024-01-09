package tempfile

var fileMap = make(map[string]string)

func addKeyToFileMapping(key, filename string) {
	fileMap[key] = filename
}

func getKeyToFileMapping(key string) (string, bool) {
	filename, ok := fileMap[key]
	return filename, ok
}

func deleteKeyToFileMapping(key string) {
	delete(fileMap, key)
}
