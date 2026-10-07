// Package rng 封装整条链路使用的确定性随机源。
// 所有随机性（基选择、比特、噪声、窃听、暗计数、抽样、纠错协商、隐私放大……）
// 都来自同一颗种子，从而保证同一 Config.Seed 下多次重跑结果逐比特一致。
//
// 每个环节通过 Stream(name) 取一条命名子流：子流由「父流种子 + 名字」共同
// 派生，既随根种子变化，又彼此独立；即使日后在某环节新增随机调用，
// 也不会改变其它环节历史上拿到的随机序列（历史结论可复现）。
package rng

import (
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
)

// Rand 是链路使用的随机源接口。
type Rand interface {
	// Bit 返回等概率的 0/1。
	Bit() int
	// Intn 返回 [0,n) 的均匀整数。
	Intn(n int) int
	// Float64 返回 [0,1) 的均匀浮点。
	Float64() float64
	// Stream 派生一条独立的命名子随机流。
	Stream(name string) Rand
}

type prng struct {
	path string
	s1   int64
	s2   int64
	r    *rand.Rand
}

// New 以种子创建根随机流。
func New(seed int64) Rand {
	return newAt("root", seed, 0x6b6579) // 固定第二分量，仅用于打散 PCG 状态
}

func newAt(path string, s1, s2 int64) *prng {
	return &prng{
		path: path,
		s1:   s1,
		s2:   s2,
		r:    rand.New(rand.NewPCG(uint64(s1), uint64(s2))),
	}
}

// Stream 用 SHA-256(s1 || s2 || path/name) 派生子流的两个 PCG 种子，
// 保证：随父（最终是根）种子变化；不同名字的子流彼此独立；同名子流恒定。
func (p *prng) Stream(name string) Rand {
	childPath := p.path + "/" + name
	h := sha256.New()
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(p.s1))
	h.Write(buf[:])
	binary.BigEndian.PutUint64(buf[:], uint64(p.s2))
	h.Write(buf[:])
	h.Write([]byte(childPath))
	sum := h.Sum(nil)
	ns1 := int64(binary.BigEndian.Uint64(sum[0:8]))
	ns2 := int64(binary.BigEndian.Uint64(sum[8:16]))
	return newAt(childPath, ns1, ns2)
}

func (p *prng) Bit() int {
	return int(p.r.Uint64() & 1)
}

func (p *prng) Intn(n int) int {
	if n <= 0 {
		panic("rng.Intn: n 必须为正")
	}
	return p.r.IntN(n)
}

func (p *prng) Float64() float64 {
	return p.r.Float64()
}
