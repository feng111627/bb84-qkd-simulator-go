package protocol

import (
	"math"

	"github.com/feng111627/bb84-qkd-simulator-go/internal/rng"
)

// PAResult 是隐私放大结果。
type PAResult struct {
	Key       []byte
	InLength  int
	OutLength int
}

// SecretKeyLength 按保守的单向量子信息界估计可保留的安全密钥长度：
//
//	L = n_remain - h(qber)*n_sifted - leakEC - margin
//
// 其中 h 是二元熵：被抽走的样本比例使 Eve 可能知道约 h(qber) 比例的
// 原始筛后比特；leakEC 是纠错协商公开的奇偶校验数。
func SecretKeyLength(nRemain int, qber float64, nSifted, leakEC, margin int) int {
	lost := binaryEntropy(qber)*float64(nSifted) + float64(leakEC) + float64(margin)
	return int(float64(nRemain) - lost + 0.5)
}

func binaryEntropy(p float64) float64 {
	if p <= 0 || p >= 1 {
		return 0
	}
	return -p*log2(p) - (1-p)*log2(1-p)
}

func log2(x float64) float64 {
	return math.Log2(x)
}

// PrivacyAmplify 用 GF(2) 上的 Toeplitz 矩阵做通用哈希：
// out 个输出比特中的每一个都是输入比特串的一个随机线性组合（异或）。
// 矩阵由确定性随机流公开生成（通用哈希族只要求矩阵选择公开且独立于密钥）。
func PrivacyAmplify(input []uint8, outBits int, r rng.Rand) PAResult {
	inBits := len(input)
	res := PAResult{InLength: inBits, OutLength: outBits}
	if outBits <= 0 {
		return res
	}
	sr := r.Stream("privacy-amplification")

	// Toeplitz 矩阵由第一行 (outBits-1 个系数) 与第一列 (inBits 个系数)
	// 共 inBits+outBits-2 个随机比特确定，T[i][j]=diag[i-j]。
	diag := make([]byte, inBits+outBits-1)
	for i := range diag {
		diag[i] = byte(sr.Bit())
	}

	out := make([]byte, (outBits+7)/8)
	for i := 0; i < outBits; i++ {
		var bit byte
		// T[i][j] = diag[j + (outBits-1-i)]：索引只依赖 j-i，
		// 故矩阵沿对角线恒定，即 Toeplitz 矩阵。
		base := outBits - 1 - i
		for j := 0; j < inBits; j++ {
			if diag[base+j] == 1 {
				bit ^= input[j]
			}
		}
		if bit != 0 {
			out[i/8] |= 1 << uint(i%8)
		}
	}
	res.Key = out
	return res
}
