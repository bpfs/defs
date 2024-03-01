package util

import (
	"fmt"
	"testing"
)

func TestCalculateHash(t *testing.T) {
	hash := CalculateHash([]byte("1234234"))
	fmt.Printf("%d", len(hash))
}
