package protocol

import (
	"testing"

	"github.com/feng111627/bb84-qkd-simulator-go/internal/device"
	"github.com/feng111627/bb84-qkd-simulator-go/internal/rng"
)

func mkDetection(i int, detected bool, b device.Basis, bit device.Bit) device.Detection {
	return device.Detection{Index: i, Detected: detected, Basis: b, Bit: bit}
}

// TestSift 覆盖筛后环节：基不一致丢弃、未探测丢弃、基一致保留且顺序正确。
func TestSift(t *testing.T) {
	alice := []aliceRecord{
		{Basis: device.BasisRectilinear, Bit: 0},
		{Basis: device.BasisDiagonal, Bit: 1},
		{Basis: device.BasisRectilinear, Bit: 1},
		{Basis: device.BasisDiagonal, Bit: 0},
	}
	dets := []device.Detection{
		mkDetection(0, true, device.BasisRectilinear, 0),  // 同基同值
		mkDetection(1, true, device.BasisRectilinear, 0),  // 异基丢弃
		mkDetection(2, false, device.BasisRectilinear, 1), // 未探测丢弃
		mkDetection(3, true, device.BasisDiagonal, 1),     // 同基但误码，保留
	}
	sk := Sift(alice, dets)
	if len(sk.Alice) != 2 || sk.Indices[0] != 0 || sk.Indices[1] != 3 {
		t.Fatalf("筛后位置错误: %v", sk.Indices)
	}
	if sk.Alice[1] != 0 || sk.Bob[1] != 1 {
		t.Fatalf("筛后比特取值错误")
	}
	t.Logf("用例: 同基保留/异基丢弃/未探测丢弃 -> 保留位置 %v，符合预期", sk.Indices)
}

// TestEstimateQBER 覆盖 QBER 环节：抽样确定、误码计数、噪声判据方向。
func TestEstimateQBER(t *testing.T) {
	mk := func(n, errs int, eve bool) SiftedKey {
		sk := SiftedKey{
			Alice:      make([]device.Bit, n),
			Bob:        make([]device.Bit, n),
			EveTouched: make([]bool, n),
			DarkCount:  make([]bool, n),
		}
		for i := 0; i < n; i++ {
			if i < errs {
				sk.Bob[i] = 1
			}
			sk.EveTouched[i] = eve
		}
		return sk
	}
	t.Run("零误码", func(t *testing.T) {
		q, a, b := EstimateQBER(mk(1000, 0, false), 0.5, 0.05, rng.New(1))
		if q.Errors != 0 || q.QBER != 0 || q.EavesdropSuspected() {
			t.Fatalf("零误码不应判窃听: %+v", q)
		}
		if len(a)+len(b) != 2*(1000-q.SampleSize) {
			t.Fatalf("抽样后剩余长度不闭合")
		}
		t.Logf("零误码: QBER=0, 样本=%d, 剩余=%d, 不判窃听", q.SampleSize, len(a))
	})
	t.Run("误码符合噪声期望-不判窃听", func(t *testing.T) {
		// 3000 样本、期望 5% 时出现 160 个误码（5.3%），属正常波动。
		sk := mk(6000, 320, false) // 抽样约一半
		q, _, _ := EstimateQBER(sk, 0.5, 0.05, rng.New(2))
		if q.EavesdropSuspected() {
			t.Fatalf("符合噪声期望的波动不应判窃听, z=%.2f", q.ZScore)
		}
		t.Logf("噪声波动: errors=%d/%d QBER=%.4f z=%.2f 不显著", q.Errors, q.SampleSize, q.QBER, q.ZScore)
	})
	t.Run("误码远超噪声期望-判窃听", func(t *testing.T) {
		sk := mk(6000, 1200, true)
		q, _, _ := EstimateQBER(sk, 0.5, 0.01, rng.New(3))
		if !q.EavesdropSuspected() {
			t.Fatalf("20%% 误码而噪声期望 1%%，必须判窃听, z=%.2f", q.ZScore)
		}
		if q.EveFractionObserved < 0.99 {
			t.Fatalf("样本中 Eve 触碰比例应≈1")
		}
		t.Logf("窃听: QBER=%.4f z=%.2f 显著，样本被触碰比例 %.2f", q.QBER, q.ZScore, q.EveFractionObserved)
	})
}

// TestCorrect 覆盖纠错环节：纠正全部错误、确定性、泄露非负、双方一致。
func TestCorrect(t *testing.T) {
	r := rng.New(11)
	n := 2000
	alice := make([]uint8, n)
	bob := make([]uint8, n)
	for i := range alice {
		alice[i] = uint8(r.Bit())
		bob[i] = alice[i]
	}
	// 人为引入约 4% 错误。
	wantFlips := 0
	for i := 0; i < n; i += 25 {
		bob[i] ^= 1
		wantFlips++
	}
	cr := Correct(alice, bob, rng.New(11), 0.04)
	if cr.ResidualDiff != 0 {
		t.Fatalf("纠错后残余不一致 %d", cr.ResidualDiff)
	}
	if cr.Flips != wantFlips {
		t.Fatalf("翻转数 = %d, 期望 %d", cr.Flips, wantFlips)
	}
	if cr.ParityLeak <= 0 {
		t.Fatalf("纠错必然产生公开奇偶校验位")
	}
	// 确定性：同种子同输入。
	cr2 := Correct(alice, bob, rng.New(11), 0.04)
	if hexB(cr.Corrected) != hexB(cr2.Corrected) {
		t.Fatalf("纠错结果不可复现")
	}
	t.Logf("CASCADE: %d 个错误全部纠正, 翻转=%d, 泄露=%d 比特, 轮数=%d, 可复现",
		wantFlips, cr.Flips, cr.ParityLeak, cr.Iterations)
}

// TestPrivacyAmplify 覆盖隐私放大：长度正确、确定性、雪崩式改变、长度不足给 0。
func TestPrivacyAmplify(t *testing.T) {
	r := rng.New(5)
	in := make([]uint8, 512)
	for i := range in {
		in[i] = uint8(r.Bit())
	}
	pa1 := PrivacyAmplify(in, 256, rng.New(5))
	pa2 := PrivacyAmplify(in, 256, rng.New(5))
	if pa1.OutLength != 256 || len(pa1.Key) != 32 {
		t.Fatalf("输出长度错误: %d 比特 / %d 字节", pa1.OutLength, len(pa1.Key))
	}
	if hexB(pa1.Key) != hexB(pa2.Key) {
		t.Fatalf("同种子隐私放大不可复现")
	}
	// 翻转输入 1 比特，输出应大幅改变（线性通用哈希的雪崩性）。
	in2 := make([]uint8, 512)
	copy(in2, in)
	in2[0] ^= 1
	pa3 := PrivacyAmplify(in2, 256, rng.New(5))
	diff := 0
	for i := range pa1.Key {
		x := pa1.Key[i] ^ pa3.Key[i]
		for x != 0 {
			diff += int(x & 1)
			x >>= 1
		}
	}
	if diff < 80 || diff > 176 { // 期望约 128/256
		t.Fatalf("输入 1 比特改变后输出差异比特 %d，不符合雪崩预期", diff)
	}
	zero := PrivacyAmplify(in, 0, rng.New(5))
	if len(zero.Key) != 0 {
		t.Fatalf("outBits=0 应输出空密钥")
	}
	t.Logf("Toeplitz: 512->256 比特可复现；输入翻 1 位输出改变 %d/256 位；零长度边界正确", diff)
}

// TestSecretKeyLength 覆盖安全长度公式的单调性与边界。
func TestSecretKeyLength(t *testing.T) {
	base := SecretKeyLength(4000, 0.02, 6000, 300, 32)
	highQ := SecretKeyLength(4000, 0.10, 6000, 300, 32)
	moreLeak := SecretKeyLength(4000, 0.02, 6000, 1000, 32)
	if !(highQ < base) || !(moreLeak < base) {
		t.Fatalf("安全长度应随 QBER/泄露增大而减小: base=%d highQ=%d moreLeak=%d", base, highQ, moreLeak)
	}
	t.Logf("安全长度: 基准=%d, 高QBER=%d, 大泄露=%d（均符合单调递减）", base, highQ, moreLeak)
}

func hexB(b []byte) string {
	const hexd = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, x := range b {
		out[2*i] = hexd[x>>4]
		out[2*i+1] = hexd[x&0xf]
	}
	return string(out)
}
