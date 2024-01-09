package script

import (
	"encoding/hex"
	"fmt"
	"testing"
)

func TestAddress(t *testing.T) {
	addressStr := "12gpXQVcCL2qhTNQgyLVdCFG2Qs2px98nV"
	pubKeyHash, err := GetPubKeyHash(addressStr)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("Public Key Hash:", hex.EncodeToString(pubKeyHash))
}
