package base58

import (
	"math/big"
)

var bigRadix = [...]*big.Int{
	big.NewInt(0),
	big.NewInt(58),
	big.NewInt(58 * 58),
	big.NewInt(58 * 58 * 58),
	big.NewInt(58 * 58 * 58 * 58),
	big.NewInt(58 * 58 * 58 * 58 * 58),
	big.NewInt(58 * 58 * 58 * 58 * 58 * 58),
	big.NewInt(58 * 58 * 58 * 58 * 58 * 58 * 58),
	big.NewInt(58 * 58 * 58 * 58 * 58 * 58 * 58 * 58),
	big.NewInt(58 * 58 * 58 * 58 * 58 * 58 * 58 * 58 * 58),
	bigRadix10,
}

var bigRadix10 = big.NewInt(58 * 58 * 58 * 58 * 58 * 58 * 58 * 58 * 58 * 58) // 58^10

// Decode 将修改后的 base58 字符串解码为字节切片。
// 参数：
//   - b: base58 编码的字符串
//
// 返回：
//   - 解码后的字节切片
func Decode(b string) []byte {
	answer := big.NewInt(0)
	scratch := new(big.Int)

	// 每次迭代时使用 big.Int 进行计算都很慢。
	//    x += b58[b[i]] * j
	//    j *= 58
	//
	// 相反，我们可以尝试对 int64 进行尽可能多的计算。
	// 我们可以使用 int64 表示 10 位 base58 数字。
	//
	// 因此我们将尝试一次转换 10 个 base58 数字。
	// 粗略的想法是计算 `t`，这样：
	//
	//   t := b58[b[i+9]] * 58^9 ... + b58[b[i+1]] * 58^1 + b58[b[i]] * 58^0
	//   x *= 58^10
	//   x += t
	//
	// 当然，此外，当 b 不是 58^10 的倍数时，我们还需要处理边界条件。
	// 在这种情况下，我们将使用 bigRadix[n] 查找适当的功率。
	for t := b; len(t) > 0; {
		n := len(t)
		if n > 10 {
			n = 10
		}

		total := uint64(0)
		for _, v := range t[:n] {
			if v > 255 {
				return []byte("")
			}

			tmp := b58[v]
			if tmp == 255 {
				return []byte("")
			}
			total = total*58 + uint64(tmp)
		}

		answer.Mul(answer, bigRadix[n])
		scratch.SetUint64(total)
		answer.Add(answer, scratch)

		t = t[n:]
	}

	tmpval := answer.Bytes()

	var numZeros int
	for numZeros = 0; numZeros < len(b); numZeros++ {
		if b[numZeros] != alphabetIdx0 {
			break
		}
	}
	flen := numZeros + len(tmpval)
	val := make([]byte, flen)
	copy(val[numZeros:], tmpval)

	return val
}

// Encode 将字节切片编码为修改后的 base58 字符串。
// 参数：
//   - b: 待编码的字节切片
//
// 返回：
//   - base58 编码的字符串
func Encode(b []byte) string {
	x := new(big.Int)
	x.SetBytes(b)

	// 输出的最大长度为 log58(2^(8*len(b))) == len(b) * 8 / log(58)
	maxlen := int(float64(len(b))*1.365658237309761) + 1
	answer := make([]byte, 0, maxlen)
	mod := new(big.Int)
	for x.Sign() > 0 {
		// 每次迭代时使用 big.Int 进行计算都很慢。
		//    x, mod = x / 58, x % 58
		//
		// 相反，我们可以尝试对 int64 进行尽可能多的计算。
		//    x, mod = x / 58^10, x % 58^10
		//
		// 这将为我们提供 mod，它是 10 位 base58 数字。
		// 我们将循环 10 次以转换为答案。

		x.DivMod(x, bigRadix10, mod)
		if x.Sign() == 0 {
			// 当 x = 0 时，我们需要确保不添加任何额外的零。
			m := mod.Int64()
			for m > 0 {
				answer = append(answer, alphabet[m%58])
				m /= 58
			}
		} else {
			m := mod.Int64()
			for i := 0; i < 10; i++ {
				answer = append(answer, alphabet[m%58])
				m /= 58
			}
		}
	}

	// 前导零字节
	for _, i := range b {
		if i != 0 {
			break
		}
		answer = append(answer, alphabetIdx0)
	}

	// 撤销
	alen := len(answer)
	for i := 0; i < alen/2; i++ {
		answer[i], answer[alen-1-i] = answer[alen-1-i], answer[i]
	}

	return string(answer)
}
