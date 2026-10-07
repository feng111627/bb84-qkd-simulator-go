package device

import "github.com/feng111627/bb84-qkd-simulator-go/internal/rng"

// Channel 是量子信道：
//   - 损耗：光子以 loss 概率在信道中丢失；
//   - 噪声：以 noise 概率对偏振做去极化（替换为 4 个 BB84 态之一），
//     对同基测量贡献约 noise/2 的误码；
//   - 窃听：Eve 对 eavesdropFraction 比例的在途光子实施截获-重发，
//     她用随机基测量并按测量结果重发，同基筛后位置上以 1/4 概率出错。
type Channel struct {
	loss              float64
	noise             float64
	eavesdropFraction float64
	rLoss             rng.Rand
	rNoise            rng.Rand
	rEve              rng.Rand
}

// NewChannel 创建信道，各环节使用独立命名随机流。
func NewChannel(loss, noise, eavesdropFraction float64, r rng.Rand) *Channel {
	return &Channel{
		loss:              loss,
		noise:             noise,
		eavesdropFraction: eavesdropFraction,
		rLoss:             r.Stream("channel-loss"),
		rNoise:            r.Stream("channel-noise"),
		rEve:              r.Stream("eavesdrop"),
	}
}

// Transmit 让一个脉冲依次经过 Eve、噪声与损耗。
func (c *Channel) Transmit(p EmittedPulse, index int) EmittedPulse {
	// Eve 的截获-重发只对真正在途的光子有意义。
	if p.Present && c.eavesdropFraction > 0 && c.rEve.Float64() < c.eavesdropFraction {
		eveBasis := Basis(c.rEve.Bit())
		// Eve 在 eveBasis 下测量真实光子，得到 0/1。
		p0 := measureProb(p.Photon, eveBasis, 0)
		eveBit := Bit(0)
		if c.rEve.Float64() >= p0 {
			eveBit = 1
		}
		// Eve 按自己的测量结果制备并重新发出光子。
		p.Photon = prepareState(eveBasis, eveBit)
		p.EveTouched = true
	}

	// 正常信道噪声：去极化，替换为随机一个 BB84 偏振态。
	if p.Present && c.noise > 0 && c.rNoise.Float64() < c.noise {
		nb := Basis(c.rNoise.Bit())
		nk := Bit(c.rNoise.Bit())
		p.Photon = prepareState(nb, nk)
	}

	// 信道损耗。
	if p.Present && c.rLoss.Float64() < c.loss {
		p.Present = false
	}
	return p
}
