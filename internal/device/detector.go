package device

import "github.com/feng111627/bb84-qkd-simulator-go/internal/rng"

// Detection 是 Bob 的探测器对一个脉冲的读数。
type Detection struct {
	Index      int
	Detected   bool
	Basis      Basis
	Bit        Bit
	DarkCount  bool
	EveTouched bool
}

// Detector 是 Bob 的基选择与单光子探测端。
type Detector struct {
	efficiency float64
	darkCount  float64
	rBasis     rng.Rand
	rDetect    rng.Rand
	rDark      rng.Rand
}

// NewDetector 创建探测器。
func NewDetector(efficiency, darkCount float64, r rng.Rand) *Detector {
	return &Detector{
		efficiency: efficiency,
		darkCount:  darkCount,
		rBasis:     r.Stream("bob-basis"),
		rDetect:    r.Stream("detection"),
		rDark:      r.Stream("dark-count"),
	}
}

// Measure 让 Bob 随机选择基并对到达的脉冲做偏振测量：
//   - 光子存在：以 efficiency 概率被探到，结果按 Born 概率抽样，物理正确；
//   - 光子缺失（真空）：以 darkCount 概率产生暗计数，结果 0/1 等概率。
func (d *Detector) Measure(p EmittedPulse, index int) Detection {
	out := Detection{Index: index, Basis: Basis(d.rBasis.Bit()), EveTouched: p.EveTouched}

	if p.Present {
		if d.rDetect.Float64() < d.efficiency {
			out.Detected = true
			p0 := measureProb(p.Photon, out.Basis, 0)
			if d.rDetect.Float64() >= p0 {
				out.Bit = 1
			}
		}
	} else if d.rDark.Float64() < d.darkCount {
		out.Detected = true
		out.DarkCount = true
		out.Bit = Bit(d.rDark.Bit())
	}
	return out
}
