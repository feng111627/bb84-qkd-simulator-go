// Command bb84sim 在本地跑通一条可复现的 BB84 量子密钥分发模拟链路：
// 从制备光子、过信道、探测，到筛后、QBER 估计、纠错与隐私放大产出最终密钥。
//
// 用法示例：
//
//	bb84sim -preset default           # 跑单个预设，打印分环节报告
//	bb84sim -all                      # 跑全部预设，给出结论对照表
//	bb84sim -preset clean -seed 42    # 覆盖种子等参数
//	bb84sim -all -write-records       # 把每次运行的完整报告写入 records/
//	bb84sim -reproduce records/default.json   # 读历史配置重跑并逐项比对
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/feng111627/bb84-qkd-simulator-go/config"
	"github.com/feng111627/bb84-qkd-simulator-go/internal/protocol"
)

func main() {
	var (
		preset       = flag.String("preset", "default", "预设场景: "+fmt.Sprint(config.Presets()))
		all          = flag.Bool("all", false, "依次运行全部预设并打印结论对照表")
		seed         = flag.Int64("seed", 0, "覆盖随机种子（0 表示用预设种子）")
		n            = flag.Int("n", 0, "覆盖脉冲数")
		noise        = flag.Float64("noise", -1, "覆盖信道去极化噪声强度 [0,1]")
		eavesdrop    = flag.Float64("eavesdrop", -1, "覆盖 Eve 截获-重发比例 [0,1]")
		loss         = flag.Float64("loss", -1, "覆盖信道损耗 [0,1]")
		threshold    = flag.Float64("threshold", 0, "覆盖 QBER 安全阈值")
		asJSON       = flag.Bool("json", false, "以 JSON 输出单次运行报告")
		writeRecords = flag.Bool("write-records", false, "把报告写入 records/<名称>.json")
		reproduce    = flag.String("reproduce", "", "读取历史报告 JSON，用其中配置重跑并比对结果")
		recordsDir   = flag.String("records-dir", "records", "记录目录")
	)
	flag.Parse()

	switch {
	case *reproduce != "":
		if err := reproduceRun(*reproduce); err != nil {
			fmt.Fprintf(os.Stderr, "复现失败: %v\n", err)
			os.Exit(1)
		}
	case *all:
		names := config.Presets()
		fail := false
		fmt.Println("BB84 模拟链路：全部预设场景")
		fmt.Println("==========================================================")
		for _, name := range names {
			cfg, err := config.Preset(name)
			must(err)
			rep := protocol.Run(cfg)
			printReportSummary(name, rep)
			if *writeRecords {
				must(writeRecord(filepath.Join(*recordsDir, name+".json"), rep))
			}
			if rep.Status != expectedStatus(name) {
				fail = true
			}
		}
		fmt.Println("==========================================================")
		fmt.Println("结论区分：clean/default/noisy 为正常信道噪声（成功）；")
		fmt.Println("eavesdrop-low 未超阈值但统计判据识别为窃听；eavesdrop-high 超过安全阈值。")
		if fail {
			fmt.Fprintln(os.Stderr, "存在场景结论与预期不符！")
			os.Exit(2)
		}
	default:
		cfg, err := config.Preset(*preset)
		must(err)
		applyOverrides(&cfg, *seed, *n, *noise, *eavesdrop, *loss, *threshold)
		must(cfg.Validate())
		rep := protocol.Run(cfg)
		if *asJSON {
			fmt.Println(mustJSON(rep))
			return
		}
		printReportSummary(*preset, rep)
		if *writeRecords {
			must(writeRecord(filepath.Join(*recordsDir, *preset+".json"), rep))
		}
		if rep.Status != protocol.StatusSuccess {
			os.Exit(3)
		}
	}
}

func applyOverrides(cfg *config.Config, seed int64, n int, noise, eaves, loss, thr float64) {
	if seed != 0 {
		cfg.Seed = seed
	}
	if n > 0 {
		cfg.N = n
	}
	if noise >= 0 {
		cfg.ChannelNoise = noise
	}
	if eaves >= 0 {
		cfg.EavesdropFraction = eaves
	}
	if loss >= 0 {
		cfg.ChannelLoss = loss
	}
	if thr > 0 {
		cfg.AbortThreshold = thr
	}
}

// expectedStatus 是每个预设在协议判据下应当得到的结论，
// 用于 -all 时自动核对“噪声不被当成窃听、窃听不会被放行”。
func expectedStatus(name string) protocol.Status {
	switch name {
	case "eavesdrop-low":
		return protocol.StatusAbortEavesdrop
	case "eavesdrop-high":
		return protocol.StatusAbortQBERThreshold
	default:
		return protocol.StatusSuccess
	}
}

func printReportSummary(name string, rep protocol.Report) {
	cfg := rep.Config
	fmt.Printf("\n【场景 %s】 seed=%d n=%d noise=%.3f eavesdrop=%.3f loss=%.2f\n",
		name, cfg.Seed, cfg.N, cfg.ChannelNoise, cfg.EavesdropFraction, cfg.ChannelLoss)
	for _, st := range rep.Stages {
		fmt.Printf("  [%s] %s\n", st.Name, st.Detail)
	}
	mark := "✅"
	if rep.Status != protocol.StatusSuccess {
		mark = "🛑"
	}
	fmt.Printf("  %s 结论(%s): %s\n", mark, rep.Status, rep.Reason)
	if rep.Status == protocol.StatusSuccess {
		shown := rep.KeyHex
		if len(shown) > 64 {
			shown = shown[:64] + "…"
		}
		fmt.Printf("     最终密钥(%d 字节): %s\n", len(rep.KeyHex)/2, shown)
	}
}

func writeRecord(path string, rep protocol.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(mustJSONIndent(rep)+"\n"), 0o644)
}

// reproduceRun 读取历史报告，用其中的配置重跑，并逐项比对关键产物。
func reproduceRun(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var saved protocol.Report
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	if err := saved.Config.Validate(); err != nil {
		return err
	}
	fresh := protocol.Run(saved.Config)

	type check struct {
		name string
		got  any
		want any
	}
	short := func(s string) string {
		if len(s) > 32 {
			return s[:32] + "…"
		}
		if s == "" {
			return "<空>"
		}
		return s
	}
	f4 := func(x float64) string { return fmt.Sprintf("%.4f", x) }
	checks := []check{
		{"运行状态", fresh.Status, saved.Status},
		{"原始光子比特", short(fresh.RawHex), short(saved.RawHex)},
		{"筛后密钥", short(fresh.SiftedHex), short(saved.SiftedHex)},
		{"最终密钥", short(fresh.KeyHex), short(saved.KeyHex)},
		{"QBER", f4(fresh.Metrics["qber"]), f4(saved.Metrics["qber"])},
		{"z 统计量", f4(fresh.Metrics["z_score"]), f4(saved.Metrics["z_score"])},
	}
	ok := true
	fmt.Printf("复现校验（种子=%d）\n", saved.Config.Seed)
	for _, c := range checks {
		pass := c.got == c.want
		if !pass {
			ok = false
		}
		fmt.Printf("  [%s] %-14s 本次=%v 历史=%v\n", mark(pass), c.name, c.got, c.want)
	}
	if !ok {
		return fmt.Errorf("重跑结果与历史记录不一致")
	}
	fmt.Println("全部一致，历史结论可复现。")
	return nil
}

func mark(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	must(err)
	return string(b)
}

func mustJSONIndent(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	must(err)
	return string(b)
}
