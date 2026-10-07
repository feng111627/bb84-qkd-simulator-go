package protocol

import (
	"math"

	"github.com/feng111627/bb84-qkd-simulator-go/internal/rng"
)

// CorrectResult 是纠错（简化 CASCADE）结果。
type CorrectResult struct {
	Corrected    []uint8
	Iterations   int
	ParityLeak   int // 协商中公开的奇偶校验比特数（窃听者可获得的信息量上界）
	ResidualDiff int // 纠正后双方仍不一致的比特数
	Flips        int // Bob 串被翻转的比特总数
}

type cascade struct {
	alice  []uint8
	bob    []uint8
	perms  [][]int // 每一轮的随机置换（置换位置 -> 原始下标）
	blockK []int   // 每一轮的块大小
	leak   int
	flips  int
}

// Correct 以 Alice 串为参照，用简化 CASCADE 协议纠正 Bob 串：
// 每一轮按随机置换把密钥分块、公开块奇偶校验、二分查找错误比特并翻转，
// 新发现的翻转会级联回到此前各轮重新触发纠错。
// qber 用于估计初始块大小（k1≈0.73/q）。
func Correct(alice, bob []uint8, r rng.Rand, qber float64) CorrectResult {
	n := len(alice)
	out := make([]uint8, n)
	copy(out, bob)
	if n == 0 {
		return CorrectResult{Corrected: out}
	}

	c := &cascade{alice: alice, bob: out}
	sr := r.Stream("error-correction")

	k1 := int(math.Round(0.73 / math.Max(qber, 1e-4)))
	k1 = clamp(k1, 4, n)

	passes := 0
	for k := k1; ; k *= 2 {
		perm := srPerm(sr, n)
		c.perms = append(c.perms, perm)
		c.blockK = append(c.blockK, k)
		passes++

		found := 0
		for start := 0; start < n; start += k {
			block := perm[start:min(start+k, n)]
			if c.parityDiff(block) { // parityDiff 内部计一次泄露
				idx := c.binaryFix(block)
				found++
				c.cascadeFix(len(c.perms)-1, idx)
			}
		}
		if found == 0 || k >= n {
			break
		}
		if passes >= 8 {
			break
		}
	}

	diff := 0
	for i := range alice {
		if alice[i] != out[i] {
			diff++
		}
	}
	return CorrectResult{
		Corrected:    out,
		Iterations:   passes,
		ParityLeak:   c.leak,
		ResidualDiff: diff,
		Flips:        c.flips,
	}
}

// parityDiff 判断一组位置上 Alice/Bob 的奇偶校验是否不同，同时计一次泄露。
func (c *cascade) parityDiff(pos []int) bool {
	c.leak++ // 公开一次奇偶校验（窃听者信息上界）
	var pa, pb uint8
	for _, i := range pos {
		pa ^= c.alice[i]
		pb ^= c.bob[i]
	}
	return pa != pb
}

// binaryFix 在一个奇偶不一致的块内二分定位并翻转一个错误比特，返回原始下标。
func (c *cascade) binaryFix(block []int) int {
	for len(block) > 1 {
		mid := len(block) / 2
		left := block[:mid]
		if c.parityDiff(left) {
			block = left
		} else {
			block = block[mid:]
		}
	}
	idx := block[0]
	c.bob[idx] ^= 1
	c.flips++
	return idx
}

// cascadeFix: 在 pass 轮发现 origIdx 被翻转，它使此前各轮包含该比特的块
// 奇偶性改变，回到这些块继续二分纠错。
func (c *cascade) cascadeFix(pass int, origIdx int) {
	for q := pass - 1; q >= 0; q-- {
		perm := c.perms[q]
		k := c.blockK[q]
		// 找到 origIdx 在第 q 轮置换中的位置。
		posAt := 0
		for posAt = range perm {
			if perm[posAt] == origIdx {
				break
			}
		}
		start := (posAt / k) * k
		block := perm[start:min(start+k, len(perm))]
		if c.parityDiff(block) {
			idx := c.binaryFix(block)
			c.cascadeFix(q, idx)
		}
	}
}

func srPerm(r rng.Rand, n int) []int {
	perm := make([]int, n)
	for i := range perm {
		perm[i] = i
	}
	for i := n - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		perm[i], perm[j] = perm[j], perm[i]
	}
	return perm
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
