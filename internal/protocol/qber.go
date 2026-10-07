package protocol

import (
	"math"

	"github.com/feng111627/bb84-qkd-simulator-go/internal/device"
	"github.com/feng111627/bb84-qkd-simulator-go/internal/rng"
)

// 单侧假设检验的显著性水平：正常噪声解释不了、且 QBER 落在
// “仅有噪声”分布 99% 分位之外（z=2.326），即判定统计上存在窃听迹象。
const zOneSided99 = 2.3263

// QBERResult 是误码率估计与统计判据的结果。
type QBERResult struct {
	SampleSize          int
	Errors              int
	DarkErrors          int // 误码中来自暗计数的个数（模拟中可直接归因）
	EveErrors           int // 误码中来自被 Eve 截获重发脉冲的个数
	QBER                float64
	NoiseExpect         float64 // 仅由正常信道噪声 + 暗计数可解释的期望 QBER
	NoiseUpper          float64 // 期望 QBER 的 99% 单侧置信上界
	ZScore              float64 // 观测误码数相对正常噪声假设的标准化偏离
	EveFractionObserved float64 // 样本中实际被 Eve 触碰的比例（报告用）
}

// ExpectedNoiseQBER 给定暗计数在探测中的占比，计算“仅正常噪声”下的期望 QBER：
//
//	亮脉冲误码率 = noise/2；暗计数结果完全随机，误码率 = 1/2；
//	q = (1-fdark)*noise/2 + fdark/2。
func ExpectedNoiseQBER(channelNoise, darkFraction float64) float64 {
	return (1-darkFraction)*channelNoise/2 + darkFraction*0.5
}

// EstimateQBER 从筛后密钥中随机抽取样本估计误码率，
// 并对“这些误码能否仅由正常信道噪声解释”做单侧假设检验。
// 返回估计结果与留在手中（未公开）的 Alice/Bob 剩余串。
func EstimateQBER(sk SiftedKey, sampleFraction float64, noiseExpect float64, r rng.Rand) (QBERResult, []device.Bit, []device.Bit) {
	n := len(sk.Alice)
	order := r.Stream("qber-sample")

	// Fisher-Yates 洗牌后取前 k 个作为公开比对样本。
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	k := int(math.Round(float64(n) * sampleFraction))
	if k < 1 {
		k = 1
	}
	if k > n {
		k = n
	}
	for i := 0; i < k; i++ {
		j := i + order.Intn(n-i)
		idx[i], idx[j] = idx[j], idx[i]
	}
	sample := idx[:k]
	rest := idx[k:]

	res := QBERResult{SampleSize: k, NoiseExpect: noiseExpect}
	for _, i := range sample {
		res.EveFractionObserved += b2f(sk.EveTouched[i])
		if sk.Alice[i] != sk.Bob[i] {
			res.Errors++
			if sk.DarkCount[i] {
				res.DarkErrors++
			}
			if sk.EveTouched[i] {
				res.EveErrors++
			}
		}
	}
	res.EveFractionObserved /= float64(k)
	res.QBER = float64(res.Errors) / float64(k)

	// 二项分布单侧检验：H0 = 误码仅来自正常噪声，单样本错误率 noiseExpect。
	mean := noiseExpect * float64(k)
	sd := math.Sqrt(noiseExpect * (1 - noiseExpect) * float64(k))
	if sd > 0 {
		res.ZScore = (float64(res.Errors) - mean) / sd
	}
	// “仅有噪声”时 QBER 的 99% 单侧置信上界（Wilson）。
	res.NoiseUpper = wilsonUpperBound(res.Errors, k, 0.99)

	restAlice := make([]device.Bit, 0, len(rest))
	restBob := make([]device.Bit, 0, len(rest))
	for _, i := range rest {
		restAlice = append(restAlice, sk.Alice[i])
		restBob = append(restBob, sk.Bob[i])
	}
	return res, restAlice, restBob
}

// EavesdropSuspected 判断统计上是否有窃听迹象：
// 观测误码显著偏离正常噪声假设（z 超过 99% 单侧临界值）。
func (q QBERResult) EavesdropSuspected() bool {
	return q.ZScore > zOneSided99
}

func wilsonUpperBound(errors, n int, conf float64) float64 {
	if n == 0 {
		return 0
	}
	z := zOneSided99
	p := float64(errors) / float64(n)
	denom := 1 + z*z/float64(n)
	center := p + z*z/(2*float64(n))
	margin := z * math.Sqrt(p*(1-p)/float64(n)+z*z/(4*float64(n)*float64(n)))
	return (center + margin) / denom
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
