package config

import (
	"errors"
	"fmt"
)

// Config 是一次 BB84 模拟链路的全部可调参数。
type Config struct {
	// Seed 是整条链路唯一的随机性来源。同一 Seed 重跑，
	// 原始比特、筛后密钥与最终密钥逐比特一致。
	Seed int64 `json:"seed"`
	// N 是 Alice 制备的脉冲总数。
	N int `json:"n"`

	// SourcePulseSuccess 是光源每次脉冲真正发出光子（而非真空）的概率。
	SourcePulseSuccess float64 `json:"source_pulse_success"`

	// ChannelLoss 是光子在信道中丢失的概率。
	ChannelLoss float64 `json:"channel_loss"`
	// ChannelNoise 是“正常信道噪声”的去极化强度（0~1）：
	// 以该概率对偏振态做去极化，对应误码贡献约为 ChannelNoise/2。
	ChannelNoise float64 `json:"channel_noise"`
	// EavesdropFraction 是 Eve 实施截获-重发攻击的脉冲比例（0~1）。
	// 被攻击脉冲会以 1/4 概率产生误码，并引入额外的基选择痕迹。
	EavesdropFraction float64 `json:"eavesdrop_fraction"`

	// DetectorEfficiency 是 Bob 单光子探测器的探测量子效率。
	DetectorEfficiency float64 `json:"detector_efficiency"`
	// DetectorDarkCount 是每个脉冲产生一次暗计数的概率。
	DetectorDarkCount float64 `json:"detector_dark_count"`

	// SampleFraction 是筛后密钥中用于 QBER 估计的抽样比例。
	SampleFraction float64 `json:"sample_fraction"`
	// AbortThreshold 是 QBER 的安全阈值，超过即中止协议。
	AbortThreshold float64 `json:"abort_threshold"`

	// LeakageMargin 是隐私放大时在纠错泄露之外额外扣除的安全裕量（比特）。
	LeakageMargin int `json:"leakage_margin"`
	// KeyMinLength 是允许产出最终密钥的最小长度（比特）。
	KeyMinLength int `json:"key_min_length_bits"`
}

// Default 返回一组默认配置：无窃听、弱正常噪声的可用链路。
func Default() Config {
	return Config{
		Seed:               1,
		N:                  20000,
		SourcePulseSuccess: 0.9,
		ChannelLoss:        0.2,
		ChannelNoise:       0.03,
		EavesdropFraction:  0,
		DetectorEfficiency: 0.9,
		DetectorDarkCount:  0.0005,
		SampleFraction:     0.3,
		AbortThreshold:     0.08,
		LeakageMargin:      32,
		KeyMinLength:       64,
	}
}

// Presets 返回所有内置预设的名称。
func Presets() []string {
	return []string{"default", "clean", "noisy", "eavesdrop-low", "eavesdrop-high"}
}

// Preset 按名称返回一组预设配置：
//
//	default        正常弱噪声、无窃听，协议成功；
//	clean         理想无噪声、无窃听，QBER≈0；
//	noisy         强正常噪声但无窃听，QBER 高但统计上仍属噪声；
//	eavesdrop-low 少量截获-重发，QBER 未超阈值，但统计判据识别出窃听；
//	eavesdrop-high 大规模截获-重发，QBER 直接超过安全阈值。
func Preset(name string) (Config, error) {
	c := Default()
	switch name {
	case "default":
	case "clean":
		c.ChannelNoise = 0
		c.DetectorDarkCount = 0
	case "noisy":
		c.ChannelNoise = 0.10 // 期望 QBER ≈ 5%，低于 8% 阈值
	case "eavesdrop-low":
		c.ChannelNoise = 0.01
		c.EavesdropFraction = 0.2 // 额外 QBER ≈ 5%，总量在阈值附近/以下
		c.Seed = 7
	case "eavesdrop-high":
		c.ChannelNoise = 0.01
		c.EavesdropFraction = 0.6 // 额外 QBER ≈ 15%，远超阈值
		c.Seed = 7
	default:
		return Config{}, fmt.Errorf("未知预设 %q，可选: %v", name, Presets())
	}
	return c, nil
}

// Validate 检查参数取值是否合法。
func (c Config) Validate() error {
	prob := func(name string, v float64) error {
		if v < 0 || v > 1 {
			return fmt.Errorf("%s 必须在 [0,1] 区间，当前 %v", name, v)
		}
		return nil
	}
	if c.N <= 0 {
		return errors.New("n 必须为正整数")
	}
	if err := prob("source_pulse_success", c.SourcePulseSuccess); err != nil {
		return err
	}
	if err := prob("channel_loss", c.ChannelLoss); err != nil {
		return err
	}
	if err := prob("channel_noise", c.ChannelNoise); err != nil {
		return err
	}
	if err := prob("eavesdrop_fraction", c.EavesdropFraction); err != nil {
		return err
	}
	if err := prob("detector_efficiency", c.DetectorEfficiency); err != nil {
		return err
	}
	if err := prob("detector_dark_count", c.DetectorDarkCount); err != nil {
		return err
	}
	if c.SampleFraction <= 0 || c.SampleFraction >= 1 {
		return errors.New("sample_fraction 必须在 (0,1) 区间")
	}
	if c.AbortThreshold <= 0 || c.AbortThreshold > 0.5 {
		return errors.New("abort_threshold 必须在 (0,0.5] 区间")
	}
	if c.LeakageMargin < 0 {
		return errors.New("leakage_margin 不能为负")
	}
	if c.KeyMinLength <= 0 {
		return errors.New("key_min_length_bits 必须为正")
	}
	return nil
}
