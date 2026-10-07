package device

import "math"

// Basis 是 BB84 的两组测量基。
type Basis int

const (
	BasisRectilinear Basis = 0 // Z 基：|H>, |V>
	BasisDiagonal    Basis = 1 // X 基：|+>, |->
)

// Bit 是经典比特 0/1。
type Bit = uint8

// Qubit 用偏振态的两个复振幅表示一个光子量子比特：
// 计算基（Z）下 |ψ> = A0|H> + A1|V>。
type Qubit struct {
	A0 complex128
	A1 complex128
}

var (
	ketH = Qubit{A0: 1, A1: 0} // Z0
	ketV = Qubit{A0: 0, A1: 1} // Z1
	// X 基：|+>=(|H>+|V>)/√2， |->=(|H>-|V>)/√2
	ketP = Qubit{A0: 1 / math.Sqrt2, A1: 1 / math.Sqrt2}
	ketM = Qubit{A0: 1 / math.Sqrt2, A1: -1 / math.Sqrt2}
)

// prepareState 返回某基下编码某比特的偏振态。
func prepareState(b Basis, bit Bit) Qubit {
	if b == BasisRectilinear {
		if bit == 0 {
			return ketH
		}
		return ketV
	}
	if bit == 0 {
		return ketP
	}
	return ketM
}

// basisState 返回目标基下 0/1 本征态。
func basisState(b Basis, bit Bit) Qubit {
	return prepareState(b, bit)
}

// basisStates 返回该基的两个本征态（用于测量概率计算）。
func basisStates(b Basis) (Qubit, Qubit) {
	if b == BasisRectilinear {
		return ketH, ketV
	}
	return ketP, ketM
}

// measureProb 计算在 b 基测量 q 时得到结果 bit 的概率 |<φ|ψ>|²。
func measureProb(q Qubit, b Basis, bit Bit) float64 {
	s0, s1 := basisStates(b)
	var phi Qubit
	if bit == 0 {
		phi = s0
	} else {
		phi = s1
	}
	amp := real(phi.A0)*real(q.A0) + imag(phi.A0)*imag(q.A0) +
		real(phi.A1)*real(q.A1) + imag(phi.A1)*imag(q.A1)
	return amp * amp
}
