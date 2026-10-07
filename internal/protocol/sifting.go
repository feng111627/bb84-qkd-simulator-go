package protocol

import "github.com/feng111627/bb84-qkd-simulator-go/internal/device"

// SiftedKey 保存基比对之后的筛后密钥。
type SiftedKey struct {
	Indices    []int // 在原始脉冲序列中的下标
	Alice      []device.Bit
	Bob        []device.Bit
	EveTouched []bool // 对应脉冲是否被 Eve 截获重发（仅用于报告/校验）
	DarkCount  []bool // 对应探测是否来自暗计数（仅用于报告/校验）
}

// Sift 执行基比对：保留 Bob 成功探测且双方所选基一致的位置。
func Sift(alice []aliceRecord, detections []device.Detection) SiftedKey {
	sk := SiftedKey{}
	for _, det := range detections {
		if !det.Detected {
			continue
		}
		a := alice[det.Index]
		if a.Basis != det.Basis {
			continue
		}
		sk.Indices = append(sk.Indices, det.Index)
		sk.Alice = append(sk.Alice, a.Bit)
		sk.Bob = append(sk.Bob, det.Bit)
		sk.EveTouched = append(sk.EveTouched, det.EveTouched)
		sk.DarkCount = append(sk.DarkCount, det.DarkCount)
	}
	return sk
}
