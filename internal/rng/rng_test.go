package rng

import "testing"

func TestDeterminismAndStreams(t *testing.T) {
	t.Log("用例1: 同种子序列逐数一致；用例2: 不同子流相互独立；用例3: 不同种子不同")
	a1 := New(123)
	a2 := New(123)
	b := New(124)
	for i := 0; i < 1000; i++ {
		x, y, z := a1.Float64(), a2.Float64(), b.Float64()
		if x != y {
			t.Fatalf("同种子第 %d 个随机数不一致", i)
		}
		if x == z {
			t.Fatalf("不同种子随机数意外相同（位置 %d）", i)
		}
	}

	s1 := New(7).Stream("source")
	s1again := New(7).Stream("source")
	s2 := New(7).Stream("channel")
	sameCount := 0
	for i := 0; i < 200; i++ {
		v1, v2, v3 := s1.Bit(), s1again.Bit(), s2.Bit()
		if v1 != v2 {
			t.Fatalf("同名子流不可复现（位置 %d）", i)
		}
		if v1 == v3 {
			sameCount++
		}
	}
	if sameCount > 140 { // 独立随机 200 次全/大多相同几乎不可能
		t.Fatalf("不同命名子流疑似相关，相同比例 %d/200", sameCount)
	}
	t.Log("同名子流可复现、不同名子流独立，验证通过")
}

func TestBitBalance(t *testing.T) {
	r := New(99)
	ones := 0
	const n = 20000
	for i := 0; i < n; i++ {
		ones += r.Bit()
	}
	frac := float64(ones) / n
	if frac < 0.48 || frac > 0.52 {
		t.Fatalf("Bit 不平衡: ones 比例 %.3f", frac)
	}
	t.Logf("Bit 0/1 比例 %.3f，接近等概率", frac)
}
