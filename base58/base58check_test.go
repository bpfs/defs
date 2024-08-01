package base58

import (
	"testing"
)

var checkEncodingStringTests = []struct {
	version byte
	in      string
	out     string
}{
	{20, "", "3MNQE1X"},
	{20, " ", "B2Kr6dBE"},
	{20, "-", "B3jv1Aft"},
	{20, "0", "B482yuaX"},
	{20, "1", "B4CmeGAC"},
	{20, "-1", "mM7eUf6kB"},
	{20, "11", "mP7BMTDVH"},
	{20, "abc", "4QiVtDjUdeq"},
	{20, "1234598760", "ZmNb8uQn5zvnUohNCEPP"},
	{20, "abcdefghijklmnopqrstuvwxyz", "K2RYDcKfupxwXdWhSAxQPCeiULntKm63UXyx5MvEH2"},
	{20, "00000000000000000000000000000000000000000000000000000000000000", "bi1EWXwJay2udZVxLJozuTb8Meg4W9c6xnmJaRDjg6pri5MBAxb9XwrpQXbtnqEoRV5U2pixnFfwyXC8tRAVC8XxnjK"},
}

func TestBase58Check(t *testing.T) {
	for x, test := range checkEncodingStringTests {
		// 测试编码
		if res := CheckEncode([]byte(test.in), test.version); res != test.out {
			t.Errorf("CheckEncode test #%d failed: got %s, want: %s", x, res, test.out)
		}

		// 测试解码
		res, version, err := CheckDecode(test.out)
		switch {
		case err != nil:
			t.Errorf("CheckDecode test #%d failed with err: %v", x, err)

		case version != test.version:
			t.Errorf("CheckDecode test #%d failed: got version: %d want: %d", x, version, test.version)

		case string(res) != test.in:
			t.Errorf("CheckDecode test #%d failed: got: %s want: %s", x, res, test.in)
		}
	}

	// 测试两种解码失败的情况
	// case 1: 校验和错误
	_, _, err := CheckDecode("3MNQE1Y")
	if err != ErrChecksum {
		t.Error("Checkdecode test failed, expected ErrChecksum")
	}
	// case 2: 格式无效（字符串长度低于 5 意味着版本字节和/或校验和字节丢失）。
	testString := ""
	for len := 0; len < 4; len++ {
		testString += "x"
		_, _, err = CheckDecode(testString)
		if err != ErrInvalidFormat {
			t.Error("Checkdecode test failed, expected ErrInvalidFormat")
		}
	}

}
