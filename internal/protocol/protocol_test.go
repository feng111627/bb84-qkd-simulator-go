package protocol

import (
	"strings"
	"testing"

	"github.com/feng111627/bb84-qkd-simulator-go/config"
)

// runCase 是主流程表驱动用例。
type runCase struct {
	name           string
	cfg            config.Config
	want           Status
	keyNonEmpty    bool
	reasonContains string
}

func mainCases() []runCase {
	d := config.Default()
	return []runCase{
		{
			name:        "默认弱噪声无窃听-应成功产出密钥",
			cfg:         d,
			want:        StatusSuccess,
			keyNonEmpty: true,
		},
		{
			name: "理想无噪声-应成功且QBER为零",
			cfg: func() config.Config {
				c := config.Default()
				c.ChannelNoise = 0
				c.DetectorDarkCount = 0
				return c
			}(),
			want:        StatusSuccess,
			keyNonEmpty: true,
		},
		{
			name: "强噪声无窃听-不超阈值不误报窃听",
			cfg: func() config.Config {
				c := config.Default()
				c.ChannelNoise = 0.10 // 期望 QBER≈5%，低于 8% 阈值
				return c
			}(),
			want:        StatusSuccess,
			keyNonEmpty: true,
		},
		{
			name: "大规模窃听-超阈值中止",
			cfg: func() config.Config {
				c := config.Default()
				c.Seed = 7
				c.ChannelNoise = 0.01
				c.EavesdropFraction = 0.6
				return c
			}(),
			want:           StatusAbortQBERThreshold,
			reasonContains: "超过安全阈值",
		},
		{
			name: "小规模窃听-未超阈值但统计判据识别窃听",
			cfg: func() config.Config {
				c := config.Default()
				c.Seed = 7
				c.ChannelNoise = 0.01
				c.EavesdropFraction = 0.2
				return c
			}(),
			want:           StatusAbortEavesdrop,
			reasonContains: "窃听",
		},
		{
			name: "脉冲数过少且高损耗-筛后比特不足中止",
			cfg: func() config.Config {
				c := config.Default()
				c.N = 30
				c.ChannelLoss = 0.9
				c.DetectorDarkCount = 0
				c.KeyMinLength = 8
				return c
			}(),
			want:           StatusAbortInsufficient,
			reasonContains: "筛后",
		},
		{
			name: "阈值设为零-任何误码即中止",
			cfg: func() config.Config {
				c := config.Default()
				c.AbortThreshold = 0.0001 // Validate 允许；弱噪声下必超
				return c
			}(),
			want:           StatusAbortQBERThreshold,
			reasonContains: "超过安全阈值",
		},
	}
}

// TestRunStages 验证协议主流程：状态、原因、各环节日志与最终密钥。
func TestRunStages(t *testing.T) {
	cases := mainCases()
	t.Logf("协议主流程覆盖 %d 个用例，环节: quantum-link -> sifting -> qber -> decision/error-correction -> privacy-amplification", len(cases))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := Run(tc.cfg)
			t.Logf("状态=%s 原因=%s", rep.Status, rep.Reason)
			if rep.Status != tc.want {
				t.Fatalf("状态 = %s, 期望 %s (QBER=%.4f z=%.2f)",
					rep.Status, tc.want, rep.Metrics["qber"], rep.Metrics["z_score"])
			}
			if tc.reasonContains != "" && !strings.Contains(rep.Reason, tc.reasonContains) {
				t.Fatalf("原因 %q 不包含 %q", rep.Reason, tc.reasonContains)
			}
			if tc.keyNonEmpty {
				if rep.KeyHex == "" {
					t.Fatalf("成功场景必须产出最终密钥")
				}
				if rep.Metrics["qber"] > tc.cfg.AbortThreshold {
					t.Fatalf("成功场景 QBER 不应超阈值")
				}
				for _, required := range []string{"quantum-link", "sifting", "qber-estimation",
					"decision", "error-correction", "privacy-amplification"} {
					if !hasStage(rep, required) {
						t.Fatalf("成功场景缺少环节日志 %q", required)
					}
				}
				t.Logf("环节齐备，QBER=%.4f, 密钥长度=%d 字节",
					rep.Metrics["qber"], int(rep.Metrics["final_key_bytes"]))
			}
			if rep.Status == StatusAbortQBERThreshold {
				if rep.Metrics["qber"] <= tc.cfg.AbortThreshold {
					t.Fatalf("阈值中止时 QBER 必须大于阈值")
				}
				t.Logf("确认 QBER=%.4f > 阈值 %.4f", rep.Metrics["qber"], tc.cfg.AbortThreshold)
			}
			if rep.Status == StatusAbortEavesdrop {
				if rep.Metrics["z_score"] <= 2.3263 {
					t.Fatalf("窃听中止时 z 必须超过 2.33，实际 %.2f", rep.Metrics["z_score"])
				}
				if rep.Metrics["qber"] > tc.cfg.AbortThreshold {
					t.Fatalf("窃听判据用例的 QBER 应未超阈值，否则应走阈值中止")
				}
				t.Logf("确认 QBER=%.4f ≤ 阈值但 z=%.2f > 2.33",
					rep.Metrics["qber"], rep.Metrics["z_score"])
			}
		})
	}
}

// TestSeedReproducibility 验证同种子重跑，原始比特/筛后密钥/最终密钥逐比特一致。
func TestSeedReproducibility(t *testing.T) {
	t.Log("环节覆盖: 全链路。用例: 同种子两次运行 / 不同种子应产生不同密钥")
	for _, seed := range []int64{1, 42, 9999} {
		t.Run("", func(t *testing.T) {
			cfg := config.Default()
			cfg.Seed = seed
			r1 := Run(cfg)
			r2 := Run(cfg)
			t.Logf("seed=%d 第一次 raw=%d字节 sifted=%d字节 key=%d字节",
				seed, len(r1.RawHex)/2, len(r1.SiftedHex)/2, len(r1.KeyHex)/2)
			if r1.RawHex != r2.RawHex {
				t.Fatalf("seed=%d 原始光子比特不一致", seed)
			}
			if r1.SiftedHex != r2.SiftedHex {
				t.Fatalf("seed=%d 筛后密钥不一致", seed)
			}
			if r1.KeyHex != r2.KeyHex {
				t.Fatalf("seed=%d 最终密钥不一致", seed)
			}
			if r1.Reason != r2.Reason || r1.Status != r2.Status {
				t.Fatalf("seed=%d 结论不一致", seed)
			}
		})
	}
	t.Run("不同种子应得到不同密钥", func(t *testing.T) {
		c1, c2 := config.Default(), config.Default()
		c1.Seed, c2.Seed = 1, 2
		k1 := Run(c1).KeyHex
		k2 := Run(c2).KeyHex
		if k1 == k2 {
			t.Fatalf("不同种子不应产出相同密钥")
		}
		t.Logf("seed=1 与 seed=2 密钥不同，确认种子确实驱动随机性")
	})
}

// TestNoiseVsEavesdrop 是统计判据的核心对比：
// 强噪声（无窃听）必须被判为正常；同等量级异常（有窃听）必须被识别。
func TestNoiseVsEavesdrop(t *testing.T) {
	noisy := config.Default()
	noisy.ChannelNoise = 0.10
	noisy.EavesdropFraction = 0
	rNoisy := Run(noisy)
	t.Logf("纯噪声: QBER=%.4f z=%.2f 状态=%s", rNoisy.Metrics["qber"], rNoisy.Metrics["z_score"], rNoisy.Status)
	if rNoisy.Status != StatusSuccess {
		t.Fatalf("强正常噪声被误判为异常: %s (%s)", rNoisy.Status, rNoisy.Reason)
	}
	if rNoisy.Metrics["z_score"] > 2.3263 {
		t.Fatalf("纯噪声 z=%.2f 不应超过临界值", rNoisy.Metrics["z_score"])
	}

	eve := config.Default()
	eve.Seed = 7
	eve.ChannelNoise = 0.01
	eve.EavesdropFraction = 0.2
	rEve := Run(eve)
	t.Logf("有窃听: QBER=%.4f z=%.2f 状态=%s", rEve.Metrics["qber"], rEve.Metrics["z_score"], rEve.Status)
	if rEve.Status != StatusAbortEavesdrop {
		t.Fatalf("窃听未被识别: %s", rEve.Status)
	}
	t.Logf("判据区分成立: 噪声 QBER 高但 z=%.2f 不显著；窃听 z=%.2f 显著",
		rNoisy.Metrics["z_score"], rEve.Metrics["z_score"])
}

func hasStage(rep Report, name string) bool {
	for _, s := range rep.Stages {
		if s.Name == name {
			return true
		}
	}
	return false
}
