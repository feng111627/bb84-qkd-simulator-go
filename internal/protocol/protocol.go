// Package protocol 实现 BB84 链路编排与全部经典后处理：
// 筛后(sifting)、误码率估计(QBER)、纠错(CASCADE)、隐私放大。
package protocol

import (
	"encoding/hex"
	"fmt"

	"github.com/feng111627/bb84-qkd-simulator-go/config"
	"github.com/feng111627/bb84-qkd-simulator-go/internal/device"
	"github.com/feng111627/bb84-qkd-simulator-go/internal/rng"
)

// aliceRecord 是 Alice 对自己每个脉冲的记录。
type aliceRecord struct {
	Basis device.Basis
	Bit   device.Bit
	Sent  bool
}

// Status 是一次链路运行的最终状态。
type Status string

const (
	StatusSuccess            Status = "success"
	StatusAbortQBERThreshold Status = "abort_qber_threshold"
	StatusAbortEavesdrop     Status = "abort_eavesdrop_suspected"
	StatusAbortKeyTooShort   Status = "abort_key_too_short"
	StatusAbortInsufficient  Status = "abort_insufficient_bits"
)

// StageLog 记录一个环节的结果，用于日志与报告。
type StageLog struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
	Values []KV   `json:"values,omitempty"`
}

// KV 是报告中的一个命名指标。
type KV struct {
	K string  `json:"k"`
	V float64 `json:"v"`
}

// Report 是一次完整运行的全部输出，JSON 序列化后即历史记录。
type Report struct {
	Status    Status             `json:"status"`
	Reason    string             `json:"reason,omitempty"`
	Config    config.Config      `json:"config"`
	Stages    []StageLog         `json:"stages"`
	RawHex    string             `json:"raw_bits_hex,omitempty"`
	SiftedHex string             `json:"sifted_key_hex,omitempty"`
	KeyHex    string             `json:"final_key_hex,omitempty"`
	Metrics   map[string]float64 `json:"metrics"`
}

// Run 用给定配置执行一次端到端 BB84 模拟。
func Run(cfg config.Config) Report {
	rep := &Report{Config: cfg, Metrics: map[string]float64{}}

	fail := func(st Status, format string, args ...any) Report {
		rep.Status = st
		rep.Reason = fmt.Sprintf(format, args...)
		fillMetrics(rep)
		return *rep
	}
	log := func(name, detail string, kvs ...KV) {
		rep.Stages = append(rep.Stages, StageLog{Name: name, Detail: detail, Values: kvs})
	}

	root := rng.New(cfg.Seed)

	// ---------- 1. 量子链路：制备 -> 信道 -> 探测 ----------
	src := device.NewSource(cfg.SourcePulseSuccess, root.Stream("source"))
	ch := device.NewChannel(cfg.ChannelLoss, cfg.ChannelNoise, cfg.EavesdropFraction, root.Stream("channel"))
	det := device.NewDetector(cfg.DetectorEfficiency, cfg.DetectorDarkCount, root.Stream("detector"))

	aliceRecs := make([]aliceRecord, cfg.N)
	detections := make([]device.Detection, 0, cfg.N)
	rawBits := make([]byte, 0, cfg.N)
	emitted, darkDet := 0, 0
	for i := 0; i < cfg.N; i++ {
		p := src.Emit()
		aliceRecs[i] = aliceRecord{Basis: p.Basis, Bit: p.Bit, Sent: p.Present}
		if p.Present {
			emitted++
			rawBits = append(rawBits, p.Bit)
		}
		p = ch.Transmit(p, i)
		d := det.Measure(p, i)
		if d.Detected {
			detections = append(detections, d)
			if d.DarkCount {
				darkDet++
			}
		}
	}
	rep.RawHex = hex.EncodeToString(packBits(rawBits))
	log("quantum-link", fmt.Sprintf(
		"制备 %d 个脉冲，其中 %d 个发出光子；Bob 成功探测 %d 次（暗计数 %d 次）",
		cfg.N, emitted, len(detections), darkDet),
		KV{"pulses", float64(cfg.N)}, KV{"emitted", float64(emitted)},
		KV{"detections", float64(len(detections))}, KV{"dark_detections", float64(darkDet)})

	// ---------- 2. 筛后：基比对 ----------
	sk := Sift(aliceRecs, detections)
	if len(sk.Alice) < 10 {
		return fail(StatusAbortInsufficient,
			"筛后比特仅 %d 个，不足以完成后处理（检查损耗/效率/脉冲数配置）", len(sk.Alice))
	}
	rep.SiftedHex = hex.EncodeToString(packBits(sk.Alice))
	eveInSift := 0
	darkInSift := 0
	for i := range sk.Alice {
		if sk.EveTouched[i] {
			eveInSift++
		}
		if sk.DarkCount[i] {
			darkInSift++
		}
	}
	log("sifting", fmt.Sprintf(
		"基比对后保留 %d 个位置（其中暗计数 %d，被 Eve 触碰 %d）",
		len(sk.Alice), darkInSift, eveInSift),
		KV{"sifted_bits", float64(len(sk.Alice))},
		KV{"sift_rate", float64(len(sk.Alice)) / float64(len(detections))},
		KV{"dark_fraction_in_sifted", float64(darkInSift) / float64(len(sk.Alice))},
		KV{"eve_fraction_in_sifted", float64(eveInSift) / float64(len(sk.Alice))})

	// ---------- 3. QBER 估计 + 统计判据 ----------
	darkFraction := float64(darkInSift) / float64(len(sk.Alice))
	noiseExpect := ExpectedNoiseQBER(cfg.ChannelNoise, darkFraction)
	q, restAlice, restBob := EstimateQBER(sk, cfg.SampleFraction, noiseExpect, root.Stream("protocol"))
	log("qber-estimation", fmt.Sprintf(
		"抽样 %d 比特，发现 %d 个误码，QBER=%.4f；正常噪声期望 %.4f（99%%上界 %.4f），z=%.2f",
		q.SampleSize, q.Errors, q.QBER, noiseExpect, q.NoiseUpper, q.ZScore),
		KV{"sample_size", float64(q.SampleSize)}, KV{"errors", float64(q.Errors)},
		KV{"qber", q.QBER}, KV{"noise_expected", noiseExpect},
		KV{"noise_upper99", q.NoiseUpper}, KV{"z_score", q.ZScore},
		KV{"sample_eve_fraction", q.EveFractionObserved})

	// 3a. 硬安全阈值：超过即中止。
	if q.QBER > cfg.AbortThreshold {
		return fail(StatusAbortQBERThreshold,
			"估计 QBER=%.2f%% 超过安全阈值 %.2f%%，窃听导致的信息泄露不可接受，协议中止",
			100*q.QBER, 100*cfg.AbortThreshold)
	}
	// 3b. 统计判据：QBER 虽未超阈值，但显著偏离“仅正常噪声”假设 -> 窃听迹象。
	if q.EavesdropSuspected() {
		return fail(StatusAbortEavesdrop,
			"QBER=%.2f%% 虽低于阈值 %.2f%%，但误码数显著高于正常噪声期望 %.2f%%（z=%.2f>2.33），判定存在窃听迹象，协议中止",
			100*q.QBER, 100*cfg.AbortThreshold, 100*noiseExpect, q.ZScore)
	}
	log("decision", fmt.Sprintf(
		"QBER %.4f ≤ 阈值 %.4f，且 z=%.2f ≤ 2.33：误码可由正常信道噪声解释，继续后处理",
		q.QBER, cfg.AbortThreshold, q.ZScore),
		KV{"threshold", cfg.AbortThreshold})

	// ---------- 4. 纠错 ----------
	if len(restAlice) < 10 {
		return fail(StatusAbortInsufficient, "抽样后剩余比特仅 %d 个，不足以纠错", len(restAlice))
	}
	cr := Correct(restAlice, restBob, root.Stream("protocol"), q.QBER)
	log("error-correction", fmt.Sprintf(
		"CASCADE 共 %d 轮，翻转 %d 个比特，公开 %d 个奇偶校验位，残余不一致 %d",
		cr.Iterations, cr.Flips, cr.ParityLeak, cr.ResidualDiff),
		KV{"ec_rounds", float64(cr.Iterations)}, KV{"ec_flips", float64(cr.Flips)},
		KV{"ec_leak_bits", float64(cr.ParityLeak)},
		KV{"ec_residual_diff", float64(cr.ResidualDiff)})
	if cr.ResidualDiff > 0 {
		return fail(StatusAbortKeyTooShort,
			"纠错后仍有 %d 个比特不一致，无法得到一致密钥", cr.ResidualDiff)
	}

	// ---------- 5. 隐私放大 ----------
	secretBits := SecretKeyLength(len(restAlice), q.QBER, len(sk.Alice), cr.ParityLeak, cfg.LeakageMargin)
	log("privacy-amplification-setup", fmt.Sprintf(
		"按单向量子信息界估计安全长度：%d - h(%.4f)*%d - %d(泄露) - %d(裕量) = %d 比特",
		len(restAlice), q.QBER, len(sk.Alice), cr.ParityLeak, cfg.LeakageMargin, secretBits),
		KV{"secret_bits", float64(secretBits)})
	if secretBits < cfg.KeyMinLength {
		return fail(StatusAbortKeyTooShort,
			"安全密钥长度仅 %d 比特，低于最小要求 %d 比特，协议中止",
			secretBits, cfg.KeyMinLength)
	}
	pa := PrivacyAmplify(cr.Corrected, secretBits, root.Stream("protocol"))
	rep.KeyHex = hex.EncodeToString(pa.Key)
	log("privacy-amplification", fmt.Sprintf(
		"Toeplitz 通用哈希将 %d 比特压缩为 %d 比特最终密钥", pa.InLength, pa.OutLength),
		KV{"final_key_bytes", float64(len(pa.Key))})

	rep.Status = StatusSuccess
	rep.Reason = fmt.Sprintf("协议成功：QBER=%.2f%%（正常噪声范围内），产出 %d 比特安全密钥",
		100*q.QBER, pa.OutLength)
	fillMetrics(rep)
	return *rep
}

func fillMetrics(rep *Report) {
	for _, st := range rep.Stages {
		for _, kv := range st.Values {
			rep.Metrics[kv.K] = kv.V
		}
	}
}

// packBits 把 0/1 比特切片紧凑打包成字节（每字节 8 比特，小端位序），
// 末尾不足 8 比特的部分补零；长度信息由上下文已知。
func packBits(bits []byte) []byte {
	out := make([]byte, (len(bits)+7)/8)
	for i, b := range bits {
		if b&1 != 0 {
			out[i/8] |= 1 << uint(i%8)
		}
	}
	return out
}
