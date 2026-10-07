package device

import "github.com/feng111627/bb84-qkd-simulator-go/internal/rng"

// EmittedPulse 表示光源一次发光：真空（Present=false）或一个偏振光子。
type EmittedPulse struct {
	Present bool
	Photon  Qubit
	Basis   Basis
	Bit     Bit
	// EveTouched 标记该脉冲是否被 Eve 截获-重发过（信道填写）。
	EveTouched bool
}

// Source 是 Alice 的弱相干光源：以 pulseSuccess 概率真正发出单光子，
// 否则输出真空脉冲；基与比特均匀随机选取。
type Source struct {
	pulseSuccess float64
	r            rng.Rand
}

// NewSource 创建光源。
func NewSource(pulseSuccess float64, r rng.Rand) *Source {
	return &Source{pulseSuccess: pulseSuccess, r: r}
}

// Emit 制备一个脉冲，返回光子以及 Alice 侧记录的基与比特。
// 真空脉冲也带记录（Bob 可能因暗计数误探测）。
func (s *Source) Emit() EmittedPulse {
	b := Basis(s.r.Bit())
	bit := Bit(s.r.Bit())
	p := EmittedPulse{Basis: b, Bit: bit}
	if s.r.Float64() < s.pulseSuccess {
		p.Present = true
		p.Photon = prepareState(b, bit)
	}
	return p
}
