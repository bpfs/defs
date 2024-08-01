package base58

import (
	"bytes"
	"testing"
)

var (
	raw5k       = bytes.Repeat([]byte{0xff}, 5000)
	raw100k     = bytes.Repeat([]byte{0xff}, 100*1000)
	encoded5k   = Encode(raw5k)
	encoded100k = Encode(raw100k)
)

func BenchmarkBase58Encode_5K(b *testing.B) {
	b.SetBytes(int64(len(raw5k)))
	for i := 0; i < b.N; i++ {
		Encode(raw5k)
	}
}

func BenchmarkBase58Encode_100K(b *testing.B) {
	b.SetBytes(int64(len(raw100k)))
	for i := 0; i < b.N; i++ {
		Encode(raw100k)
	}
}

func BenchmarkBase58Decode_5K(b *testing.B) {
	b.SetBytes(int64(len(encoded5k)))
	for i := 0; i < b.N; i++ {
		Decode(encoded5k)
	}
}

func BenchmarkBase58Decode_100K(b *testing.B) {
	b.SetBytes(int64(len(encoded100k)))
	for i := 0; i < b.N; i++ {
		Decode(encoded100k)
	}
}
