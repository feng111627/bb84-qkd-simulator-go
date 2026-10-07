# BB84 量子密钥分发模拟器（Go）

一条能在本地跑通、结果**可复现**、结论**说得清楚**的 BB84 模拟链路：
从 Alice 制备偏振光子，经过量子信道（损耗 / 正常噪声 / Eve 窃听），
到 Bob 探测，再走完**筛后 → 误码率估计 → 纠错 → 隐私放大**全部经典后处理，
最终产出共享密钥或给出可区分的中止原因。

## 1. 协议流程

```
                         量子阶段（确定性 PRNG 驱动）
 Alice: 选基(比特) → 制备偏振光子(H/V/+/-)
                       │
        量子信道 Channel:
          ① Eve 截获-重发（按 eavesdrop_fraction 随机选择脉冲）
          ② 正常去极化噪声（按 channel_noise 概率替换为随机 BB84 态）
          ③ 信道损耗（按 channel_loss 概率丢光子）
                       │
 Bob: 随机选基 → 量子测量（Born 概率抽样）；真空脉冲按暗计数概率产生随机读数
                       │
                         经典后处理（公开信道）
  ④ Sifting 筛后：只保留 Bob 探到、且双方基一致的位置
  ⑤ QBER 估计：随机抽样公开比对，计算误码率与“能否仅用噪声解释”的统计量
  ⑥ 安全判定（两道判据，见第 3 节）
  ⑦ 纠错：简化 CASCADE（分块奇偶校验 + 二分定位 + 级联回纠）
  ⑧ 隐私放大：GF(2) Toeplitz 通用哈希，输出最终密钥
```

### 物理建模的关键数字（有单测验证）

- Bob 与光子**同基测量**：无噪声时 100% 正确；
- 去极化噪声强度 `channel_noise = p` 时，同基误码率 ≈ **p/2**
  （1/4 概率换成同基反态、1/4 换成同基正态、1/2 换成异基态而同基测成 50/50）；
- Eve **全比例截获-重发**时，同基筛后位置误码率恰为 **1/4**
  （Eve 选错基的概率 1/2 × 选错基时出错 1/2）；
- 暗计数结果完全随机，对误码率贡献 `dark_fraction × 1/2`。

## 2. 可调参数

全部参数在 [`config/config.go`](config/config.go)，可用 CLI 覆盖：

| 参数 | 含义 | 默认 |
|---|---|---|
| `seed` | **唯一**随机种子，决定全部随机性 | 1 |
| `n` | Alice 制备的脉冲总数 | 20000 |
| `source_pulse_success` | 每个脉冲真正发出光子的概率 | 0.9 |
| `channel_loss` | 信道丢光子概率 | 0.2 |
| `channel_noise` | 正常去极化噪声强度（误码≈noise/2） | 0.03 |
| `eavesdrop_fraction` | Eve 截获-重发的脉冲比例 | 0 |
| `detector_efficiency` | Bob 探测量子效率 | 0.9 |
| `detector_dark_count` | 每脉冲暗计数概率 | 0.0005 |
| `sample_fraction` | QBER 抽样比例 | 0.3 |
| `abort_threshold` | QBER 硬安全阈值 | 0.08 |
| `leakage_margin` | 隐私放大额外安全裕量（比特） | 32 |
| `key_min_length_bits` | 最终密钥最小长度 | 64 |

预设场景：`default`（弱噪声成功）、`clean`（理想无噪）、`noisy`（强噪声成功）、
`eavesdrop-low`（未超阈值但统计判据识别窃听）、`eavesdrop-high`（超阈值中止）。

## 3. 如何区分“正常噪声”与“窃听”

判定分两道，任一不过即**明确中止**并给出不同原因：

1. **硬阈值判据**：抽样 QBER > `abort_threshold`
   → `abort_qber_threshold`（信息泄露不可接受）。
2. **统计显著性判据**：即使 QBER 未超阈值，也检验“误码能否仅由正常信道噪声解释”。
   在原假设 H0（无窃听，单比特错误率为期望噪声 QBER）下，误码数服从二项分布，
   计算标准化偏离 `z = (观测误码数 - 期望误码数) / 标准差`；
   `z > 2.33`（99% 单侧分位）→ `abort_eavesdrop_suspected`。

这样：

| 场景 | QBER | z | 结论 |
|---|---|---|---|
| `noisy`（noise=0.10） | ≈5.2% | ≈0.3 | 噪声可解释，**继续**产出密钥 |
| `eavesdrop-low`（20% 截获） | ≈5.7% | ≈32 | 未超 8% 阈值，但**判为窃听中止** |
| `eavesdrop-high`（60% 截获） | ≈14% | ≈87 | **超阈值中止** |

此外还有 `abort_insufficient_bits`（筛后/剩余比特不足）和
`abort_key_too_short`（安全长度不足）两类中止原因。

隐私放大的安全长度按保守的单向量子信息界估计：
`L = n_remain − h₂(QBER)·n_sifted − leakEC − margin`，
其中 `leakEC` 是 CASCADE 实际公开的奇偶校验比特数。

## 4. 可复现性是怎么保证的

- 链路里**所有**随机事件（基、比特、噪声触发、Eve 决策、暗计数、
  抽样洗牌、纠错协商、隐私放大矩阵）都来自 `internal/rng` 的同一颗种子；
- 每个环节取**命名子流** `Stream("source") / Stream("channel-loss") / …`，
  子流由“父种子 + 名字”经 SHA-256 派生：不同环节互相独立，
  以后在某环节新增随机调用也不会改变其它环节的历史序列；
- 同 `seed` 重跑：原始光子比特、筛后密钥、最终密钥、结论逐项一致
  （有 `TestSeedReproducibility` 与 `-reproduce` 双重保证）。

## 5. 使用方法

```bash
go build ./...

# 跑全部预设，打印每个环节与结论对照表（退出码非 0 表示有场景不符合预期）
go run ./cmd/bb84sim -all

# 单场景 / 覆盖参数 / JSON 报告
go run ./cmd/bb84sim -preset clean -seed 42
go run ./cmd/bb84sim -preset noisy -noise 0.12 -threshold 0.15
go run ./cmd/bb84sim -preset default -json

# 写历史记录并用记录复现（换参数后历史结论照样可核对）
go run ./cmd/bb84sim -all -write-records
go run ./cmd/bb84sim -reproduce records/eavesdrop-low.json
```

记录文件含完整配置、分阶段日志、QBER/z 等指标以及原始比特、
筛后密钥与最终密钥（hex）。`-reproduce` 会用记录中的配置重跑并逐项比对，
被篡改的记录会被识别为不一致。

## 6. 验证方法

```bash
go test ./... -v           # 全部单测（日志含每个环节、每个用例与结果）
go test ./... -cover       # 覆盖率（internal/protocol ≈95%）
go vet ./...
```

测试覆盖：

- **协议主流程与边界**：成功产出密钥、超阈值中止、未超阈值但窃听中止、
  比特不足中止、低阈值即中止；
- **种子复现**：多个种子两次运行逐项一致、不同种子密钥不同；
- **噪声 vs 窃听对比**：5% 纯噪声放行（z 不显著）、同等 QBER 的窃听拦截（z 显著）；
- **物理层统计**：理想链路零误码、截获-重发 1/4 误码、去极化 noise/2 误码；
- **后处理环节**：筛后保留/丢弃规则、QBER 抽样与假设检验、
  CASCADE 完全纠错且可复现、Toeplitz 长度/雪崩性/边界、安全长度单调性；
- **配置层**：合法配置、非法参数拒绝、预设区分度。

## 7. 目录结构

```
config/                 参数、默认值、预设、校验
internal/rng/           确定性随机源（PCG + 命名子流派生）
internal/device/        Qubit/光源/量子信道/探测器（物理建模）
internal/protocol/      链路编排 + 筛后/QBER/CASCADE/隐私放大
cmd/bb84sim/            命令行：预设、覆盖、记录、复现校验
records/                运行记录（-write-records 生成，可用于复现）
```

> 说明：这是教学/验证用途的经典模拟，不包含多光子脉冲、PNS 攻击、
> 有限长效应修正与设备无关安全性证明；安全长度公式为教学化的保守估计。
