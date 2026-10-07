package device

import (
	"testing"

	"github.com/feng111627/bb84-qkd-simulator-go/internal/rng"
)

// TestPrepareAndMeasure 验证量子态制备与 Born 概率测量的解析正确性：
// 同基无噪声应 100% 测得编码比特；异基应 50/50；Eve 截获重发的 1/4 误码率。
func TestPrepareAndMeasure(t *testing.T) {
	t.Run("同基测量确定性正确", func(t *testing.T) {
		for _, b := range []Basis{BasisRectilinear, BasisDiagonal} {
			for bit := Bit(0); bit <= 1; bit++ {
				q := prepareState(b, bit)
				p0 := measureProb(q, b, bit)
				if p0 < 0.999 {
					t.Fatalf("基=%v bit=%d 同基测得自身概率应≈1，实际 %.3f", b, bit, p0)
				}
				wrong := bit ^ 1
				if p := measureProb(q, b, wrong); p > 0.001 {
					t.Fatalf("同基测得正交态概率应≈0，实际 %.3f", p)
				}
			}
		}
		t.Log("Z/X 基下 4 个 BB84 态自洽：同基 100% 正确")
	})

	t.Run("异基测量各50%", func(t *testing.T) {
		q := ketH
		p := measureProb(q, BasisDiagonal, 0)
		if p < 0.49 || p > 0.51 {
			t.Fatalf("Z 态在 X 基应 50%%，实际 %.3f", p)
		}
		t.Log("|H> 在 X 基测得 |+>/|-> 概率各 0.5")
	})
}

// TestEndToEndPhysics 用大样本统计验证物理层关键数字。
func TestEndToEndPhysics(t *testing.T) {
	t.Run("理想信道同基QBER为零", func(t *testing.T) {
		r := rng.New(1)
		src := NewSource(1, r.Stream("source"))
		ch := NewChannel(0, 0, 0, r.Stream("channel"))
		det := NewDetector(1, 0, r.Stream("detector"))
		err, same := 0, 0
		for i := 0; i < 20000; i++ {
			p := src.Emit()
			origBasis, origBit := p.Basis, p.Bit
			p = ch.Transmit(p, i)
			d := det.Measure(p, i)
			if d.Detected && d.Basis == origBasis {
				same++
				if d.Bit != origBit {
					err++
				}
			}
		}
		if err != 0 {
			t.Fatalf("理想链路同基误码应为 0，实际 %d/%d", err, same)
		}
		t.Logf("理想链路 %d 个同基探测全部正确", same)
	})

	t.Run("全截获重发同基误码约25%", func(t *testing.T) {
		r := rng.New(3)
		src := NewSource(1, r.Stream("source"))
		ch := NewChannel(0, 0, 1.0, r.Stream("channel"))
		det := NewDetector(1, 0, r.Stream("detector"))
		err, same := 0, 0
		for i := 0; i < 40000; i++ {
			p := src.Emit()
			origBasis, origBit := p.Basis, p.Bit
			p = ch.Transmit(p, i)
			d := det.Measure(p, i)
			if d.Detected && d.Basis == origBasis {
				same++
				if d.Bit != origBit {
					err++
				}
			}
		}
		qber := float64(err) / float64(same)
		if qber < 0.23 || qber > 0.27 {
			t.Fatalf("全截获-重发同基 QBER 应≈0.25，实际 %.3f", qber)
		}
		t.Logf("全截获重发同基 QBER=%.3f（理论 0.25）", qber)
	})

	t.Run("去极化噪声误码约noise/2", func(t *testing.T) {
		r := rng.New(5)
		src := NewSource(1, r.Stream("source"))
		ch := NewChannel(0, 0.10, 0, r.Stream("channel"))
		det := NewDetector(1, 0, r.Stream("detector"))
		err, same := 0, 0
		for i := 0; i < 40000; i++ {
			p := src.Emit()
			ob, kb := p.Basis, p.Bit
			p = ch.Transmit(p, i)
			d := det.Measure(p, i)
			if d.Detected && d.Basis == ob {
				same++
				if d.Bit != kb {
					err++
				}
			}
		}
		qber := float64(err) / float64(same)
		if qber < 0.04 || qber > 0.06 {
			t.Fatalf("noise=0.1 同基 QBER 应≈0.05，实际 %.3f", qber)
		}
		t.Logf("去极化 noise=0.10 同基 QBER=%.3f（理论 0.05）", qber)
	})
}
