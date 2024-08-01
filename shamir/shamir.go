// shamir.go 实现Shamir秘密共享的主要逻辑

package shamir

import (
	"crypto/rand"
	"errors"
	"math/big"
)

// GenerateStandardShares 生成标准的秘密份额
// 参数：
//   - secret: []byte 要分割的秘密
//   - n: int 份额总数
//   - k: int 最小需要的份额数量
//   - primeOptional: ...*big.Int 可选的自定义素数
//
// 返回值：
//   - [][2]*big.Int 生成的秘密份额
//   - error 可能的错误
func GenerateStandardShares(secret []byte, n, k int, primeOptional ...*big.Int) ([][2]*big.Int, error) {
	// 参数验证
	if n < k || k < 2 {
		return nil, errors.New("参数不合法: n 必须大于或等于 k，且 k 必须大于或等于 2")
	}

	// 确定使用的素数
	var prime *big.Int
	if len(primeOptional) > 0 && primeOptional[0] != nil {
		prime = primeOptional[0]
	} else {
		// 如果没有提供素数，使用默认素数
		// 默认素数，这里使用了一个256位的安全素数作为默认值
		//
		// 优点:
		// 可靠性：一个事先挑选好的、经过充分测试的素数可以确保其数学属性符合要求，从而保证了系统的可靠性。
		// 性能：使用固定的素数消除了动态生成素数的需要，减少了计算开销，特别是在不需要每次都生成独特素数的场景中。
		//
		// 缺点:
		// 安全性：如果攻击者知道了使用的固定素数，他们可能会利用这一信息设计特定的攻击。
		// 然而，实际上，由于Shamir秘密共享的安全性并不直接依赖于素数的不可预测性，这个问题不像在其他加密场景中那么严重。
		prime, _ = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)

	}

	// 将秘密转换为大整数
	secretInt := new(big.Int).SetBytes(secret)
	if secretInt.Cmp(prime) >= 0 {
		return nil, errors.New("秘密大于素数模，需要一个更小的秘密或更大的素数模")
	}

	// 生成多项式系数，第一个系数是秘密本身
	coeffs := make([]*big.Int, k)
	coeffs[0] = secretInt
	for i := 1; i < k; i++ {
		coeff, err := rand.Int(rand.Reader, prime)
		if err != nil {
			return nil, errors.New("生成多项式系数时出错")
		}
		coeffs[i] = coeff
	}

	// 生成份额
	shares := make([][2]*big.Int, n)
	for i := 1; i <= n; i++ {
		x := big.NewInt(int64(i))
		y := evalPolynomial(coeffs, x, prime)
		shares[i-1] = [2]*big.Int{x, y}
	}

	return shares, nil
}

// GenerateSharesWithFixedShare 生成秘密份额，包含一个固定的份额
// 参数：
//   - secret: []byte 要分割的秘密
//   - n: int 份额总数
//   - k: int 最小需要的份额数量
//   - fixedX: *big.Int 固定的x值
//   - fixedY: *big.Int 固定的y值
//   - prime: *big.Int 可选的自定义素数
//
// 返回值：
//   - [][2]*big.Int 生成的秘密份额
//   - error 可能的错误
func GenerateSharesWithFixedShare(secret []byte, n, k int, fixedX, fixedY *big.Int, prime *big.Int) ([][2]*big.Int, error) {
	// 参数验证
	if n < k || k < 2 {
		return nil, errors.New("参数不合法: n 必须大于或等于 k，且 k 必须大于或等于 2")
	}

	// 首个系数是秘密本身
	secretInt := new(big.Int).SetBytes(secret)
	coeffs := []*big.Int{secretInt}

	// 为简化处理，我们只考虑k=2的情况，即一个线性多项式：f(x) = a0 + a1*x
	if k != 2 {
		return nil, errors.New("当前实现仅支持k=2的情况")
	}

	// 计算第二个系数a1，以确保多项式通过固定点(fixedX, fixedY)
	// 计算方式为：a1 = (fixedY - a0) / fixedX
	fixedYMinusA0 := new(big.Int).Sub(fixedY, secretInt) // fixedY - a0
	a1 := new(big.Int).Div(fixedYMinusA0, fixedX)        // (fixedY - a0) / fixedX

	coeffs = append(coeffs, a1) // 添加计算出的第二个系数

	// 使用调整后的系数生成秘密份额
	shares := make([][2]*big.Int, n)
	for i := 1; i <= n; i++ {
		x := big.NewInt(int64(i))
		y := evalPolynomial(coeffs, x, prime)
		shares[i-1] = [2]*big.Int{x, y}
	}

	return shares, nil
}

// evalPolynomial 在给定的x处评估多项式的值
// 参数：
//   - coeffs: []*big.Int 多项式的系数
//   - x: *big.Int 给定的点
//   - prime: *big.Int 素数模
//
// 返回值：
//   - *big.Int 多项式在点x的值
func evalPolynomial(coeffs []*big.Int, x, prime *big.Int) *big.Int {
	result := big.NewInt(0)
	xPow := big.NewInt(1) // x的当前幂次，初始化为x^0

	for _, coeff := range coeffs {
		term := new(big.Int).Mul(coeff, xPow)
		term.Mod(term, prime) // 取模确保结果在有限字段内
		result.Add(result, term)
		xPow.Mul(xPow, x) // 更新x的幂次
	}

	return result.Mod(result, prime)
}

// RecoverSecret 通过份额恢复秘密
// 参数：
//   - shares: [][2]*big.Int 用于恢复秘密的份额集合
//   - prime: *big.Int 进行计算时使用的素数模
//
// 返回值：
//   - []byte 恢复的秘密
//   - error 可能的错误
func RecoverSecret(shares [][2]*big.Int, prime *big.Int) ([]byte, error) {
	// 检查份额数量是否满足最小要求
	if len(shares) < 2 {
		return nil, errors.New("至少需要两个份额才能恢复秘密")
	}

	// 初始化拉格朗日插值结果为0
	secret := big.NewInt(0)

	// 遍历所有份额，使用拉格朗日插值法计算秘密值
	for i, shareI := range shares {
		// 拉格朗日基本多项式初始化为1
		lagrangePolynomial := big.NewInt(1)

		// 对于当前份额，计算与其他所有份额的拉格朗日基本多项式
		for j, shareJ := range shares {
			if i == j {
				continue // 跳过与自身相同的份额
			}

			// 计算拉格朗日基础多项式
			xi, xj := shareI[0], shareJ[0]

			// 计算拉格朗日基本多项式的分子和分母
			numerator := new(big.Int).Set(xj)       // 分子为xj
			denominator := new(big.Int).Sub(xj, xi) // 分母为(xj - xi)

			// 确保分母为正，求模逆
			denominator.Add(denominator, prime)                               // 避免负数
			denominator.Mod(denominator, prime)                               // 模素数
			denominatorInverse := new(big.Int).ModInverse(denominator, prime) // 分母的逆元

			// 如果无法计算分母的逆元，说明输入的份额无效
			if denominatorInverse == nil {
				return nil, errors.New("无法计算分母的逆元，输入的份额可能无效")
			}

			// 计算当前份额的拉格朗日基本多项式值
			lagrangePolynomial.Mul(lagrangePolynomial, numerator)          // 累乘分子
			lagrangePolynomial.Mod(lagrangePolynomial, prime)              // 取模
			lagrangePolynomial.Mul(lagrangePolynomial, denominatorInverse) // 累乘分母的
			lagrangePolynomial.Mod(lagrangePolynomial, prime)              // 取模
		}

		// 使用当前份额的拉格朗日基本多项式值计算其对秘密的贡献，并累加到秘密值中
		yi := shareI[1]                                          // 当前份额的y值
		contribution := new(big.Int).Mul(yi, lagrangePolynomial) // 计算每项
		contribution.Mod(contribution, prime)                    // 取模
		secret.Add(secret, contribution)                         // 累加到秘密
		secret.Mod(secret, prime)                                // 取模
	}

	// 返回秘密值的字节序列和nil表示没有错误
	return secret.Bytes(), nil
}
