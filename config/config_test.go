package config

import "testing"

func TestValidate(t *testing.T) {
	valid := []Config{Default()}
	for _, name := range Presets() {
		c, err := Preset(name)
		if err != nil {
			t.Fatalf("预设 %s 出错: %v", name, err)
		}
		valid = append(valid, c)
	}
	for i, c := range valid {
		if err := c.Validate(); err != nil {
			t.Fatalf("用例 %d 应为合法配置: %v", i, err)
		}
	}
	t.Logf("用例: 默认 + %d 个预设全部合法", len(Presets()))

	bad := []struct {
		name string
		mod  func(*Config)
	}{
		{"N 非正", func(c *Config) { c.N = 0 }},
		{"噪声超界", func(c *Config) { c.ChannelNoise = 1.2 }},
		{"窃听比例为负", func(c *Config) { c.EavesdropFraction = -0.1 }},
		{"抽样比例为0", func(c *Config) { c.SampleFraction = 0 }},
		{"阈值为0", func(c *Config) { c.AbortThreshold = 0 }},
		{"最小密钥长度为负", func(c *Config) { c.KeyMinLength = -1 }},
	}
	for _, bc := range bad {
		t.Run(bc.name, func(t *testing.T) {
			c := Default()
			bc.mod(&c)
			if err := c.Validate(); err == nil {
				t.Fatalf("非法配置未被拒绝: %s", bc.name)
			} else {
				t.Logf("正确拒绝: %v", err)
			}
		})
	}
}

func TestPresetsDiffer(t *testing.T) {
	c, _ := Preset("clean")
	n, _ := Preset("noisy")
	if c.ChannelNoise != 0 || n.ChannelNoise <= c.ChannelNoise {
		t.Fatalf("clean 与 noisy 预设参数没有区分度")
	}
	e, _ := Preset("eavesdrop-high")
	if e.EavesdropFraction == 0 {
		t.Fatalf("eavesdrop-high 必须配置窃听比例")
	}
	if _, err := Preset("不存在"); err == nil {
		t.Fatalf("未知预设名应报错")
	}
	t.Log("预设参数区分度正确，未知名称报错")
}
