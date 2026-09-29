package main

import (
	"sort"
	"strings"
)

// Snapshot of the channel order at https://awbz.cn/tv/channels.json.
// Channels absent from the M3U are omitted; others follow in M3U order.
var preferredChannels = []string{
	"CCTV1 综合", "CCTV2 财经", "CCTV3 综艺", "CCTV4 中文国际", "CCTV5 体育", "CCTV5+ 体育赛事", "CCTV6 电影", "CCTV7 国防军事", "CCTV8 电视剧", "CCTV9 纪录", "CCTV10 科教", "CCTV11 戏曲", "CCTV12 社会与法", "CCTV13 新闻", "CCTV14 少儿", "CCTV15 音乐", "CCTV16 奥林匹克", "CCTV17 农业农村",
	"北京卫视", "河北卫视", "湖南卫视", "浙江卫视", "江苏卫视", "东方卫视", "广东卫视", "深圳卫视", "山东卫视", "安徽卫视", "河南卫视", "湖北卫视", "四川卫视", "重庆卫视", "辽宁卫视", "天津卫视", "黑龙江卫视", "江西卫视", "吉林卫视", "陕西卫视", "山西卫视", "云南卫视", "贵州卫视", "广西卫视", "海南卫视", "甘肃卫视", "青海卫视", "宁夏卫视", "新疆卫视", "内蒙古卫视", "西藏卫视", "东南卫视",
}

func normalizedName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), ""))
}

func orderChannels(channels []*channel) []*channel {
	positions := make(map[string]int, len(preferredChannels))
	for i, name := range preferredChannels {
		positions[normalizedName(name)] = i
	}
	selected := make([]*channel, 0, len(preferredChannels))
	for _, ch := range channels {
		if i, ok := positions[normalizedName(ch.Name)]; ok {
			ch.Name = preferredChannels[i]
			if i < 18 {
				ch.Group = "央视"
			} else {
				ch.Group = "地方卫视"
			}
			selected = append(selected, ch)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		return positions[normalizedName(selected[i].Name)] < positions[normalizedName(selected[j].Name)]
	})
	return selected
}
