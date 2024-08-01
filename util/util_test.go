package util

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestCalculateHash(t *testing.T) {
	hash := CalculateHash([]byte("1234234"))
	logrus.Printf("%d", len(hash))
}
