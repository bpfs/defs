package search

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestXxx(t *testing.T) {
	u := uuid.New()
	fmt.Println(u.String()) // 输出 UUID 的字符串表示

	uuidBytes, _ := u.MarshalBinary()
	base64String := base64.RawURLEncoding.EncodeToString(uuidBytes)
	fmt.Println(base64String) // 输出 Base64 编码的 UUID

	uuidWithHyphens := u.String()                                       // 标准 UUID 字符串
	uuidWithoutHyphens := strings.Replace(uuidWithHyphens, "-", "", -1) // 移除连字符

	fmt.Println("UUID with hyphens:", uuidWithHyphens)
	fmt.Println("UUID without hyphens:", uuidWithoutHyphens)

	namespace := uuid.New()
	name := "example.com"

	uuidV3 := uuid.NewMD5(namespace, []byte(name))
	fmt.Printf("Version 3 UUID: %s\n", uuidV3)

	////////////////////

	namespace1 := uuid.New()
	name1 := "example.com"

	uuidV33 := uuid.NewMD5(namespace1, []byte(name1))
	fmt.Printf("Version 3 UUID: %s\n", uuidV33)
}
